// Command wakeword-spike is the P2.0 feasibility probe for on-device wake-word
// detection (see docs/superpowers/specs/2026-06-19-self-hosted-stt-tts-and-wake-word-design.md).
//
// It loads the openWakeWord ONNX pipeline (melspectrogram -> Google speech
// embedding -> wake-word classifier) through the yalue/onnxruntime_go binding,
// then runs it as a streaming detector on 80 ms / 16 kHz frames to measure:
//
//   - that onnxruntime loads and runs on the target (arm64 Brick) at all,
//   - per-frame and per-model latency in the steady state,
//   - the realtime factor (how much faster than realtime one core is), and
//   - process RSS (the always-on memory cost, the known OOM risk).
//
// This binary is throwaway: it is not wired into bmo-pak and exists only to
// gate Phase 2. Run with -info to dump model I/O, or -wav to score a real clip.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	ort "github.com/yalue/onnxruntime_go"
)

const (
	sampleRate   = 16000
	chunkSamples = 1280 // 80 ms hop, openWakeWord's streaming step
	melBins      = 32
	embWindow    = 76 // mel frames per embedding (~760 ms context)
	embStride    = 8  // mel frames between embeddings (~80 ms)
	classWindow  = 16 // embeddings per classifier call (~1.28 s context)
	embDim       = 96
)

func main() {
	libPath := flag.String("lib", "", "path to libonnxruntime.so")
	modelsDir := flag.String("models", ".", "directory with melspectrogram.onnx, embedding_model.onnx, <wakeword>.onnx")
	wakeword := flag.String("wakeword", "hey_jarvis_v0.1.onnx", "wake-word classifier model filename")
	info := flag.Bool("info", false, "print model I/O info and exit")
	seconds := flag.Float64("seconds", 30, "seconds of audio to stream for the benchmark")
	warmup := flag.Int("warmup", 50, "warmup steps excluded from timing")
	threads := flag.Int("threads", 1, "intra-op threads per session (background detector should be 1)")
	wavPath := flag.String("wav", "", "optional 16kHz mono S16LE WAV to score instead of synthetic audio")
	flag.Parse()

	if *libPath == "" {
		fatalf("missing -lib (path to libonnxruntime.so)")
	}
	ort.SetSharedLibraryPath(*libPath)
	if err := ort.InitializeEnvironment(); err != nil {
		fatalf("InitializeEnvironment: %v", err)
	}
	defer ort.DestroyEnvironment()

	melP := filepath.Join(*modelsDir, "melspectrogram.onnx")
	embP := filepath.Join(*modelsDir, "embedding_model.onnx")
	clsP := filepath.Join(*modelsDir, *wakeword)

	if *info {
		for _, p := range []string{melP, embP, clsP} {
			printIO(p)
		}
		return
	}

	mel := mustSession(melP, *threads)
	defer mel.Destroy()
	emb := mustSession(embP, *threads)
	defer emb.Destroy()
	cls := mustSession(clsP, *threads)
	defer cls.Destroy()

	fmt.Printf("loaded pipeline: %s + embedding + %s (intra-op threads=%d)\n",
		filepath.Base(melP), *wakeword, *threads)

	audio := loadAudio(*wavPath, *seconds)
	totalSteps := len(audio) / chunkSamples
	fmt.Printf("streaming %d steps (%.1fs of audio, %d warmup excluded)\n",
		totalSteps, float64(totalSteps)*chunkSamples/float64(sampleRate), *warmup)

	d := newDetector(mel, emb, cls)

	var melLat, embLat, clsLat, stepLat []float64
	maxScore := 0.0
	for i := 0; i < totalSteps; i++ {
		chunk := audio[i*chunkSamples : (i+1)*chunkSamples]
		t0 := time.Now()
		score, tm, te, tc := d.step(chunk)
		st := time.Since(t0).Seconds() * 1000
		if i >= *warmup {
			stepLat = append(stepLat, st)
			if tm > 0 {
				melLat = append(melLat, tm)
			}
			if te > 0 {
				embLat = append(embLat, te)
			}
			if tc > 0 {
				clsLat = append(clsLat, tc)
			}
		}
		if score > maxScore {
			maxScore = score
		}
	}

	fmt.Println("\n=== latency (ms, steady state) ===")
	report("melspectrogram", melLat)
	report("embedding     ", embLat)
	report("classifier    ", clsLat)
	report("FULL STEP     ", stepLat)

	avgStep := mean(stepLat)
	fmt.Printf("\nframe budget: %.1f ms/step (80ms audio per step)\n", 80.0)
	if avgStep > 0 {
		fmt.Printf("realtime factor: %.1fx (1 core processes %.0fx faster than realtime)\n",
			80.0/avgStep, 80.0/avgStep)
		fmt.Printf("steady-state CPU of one core: %.1f%%\n", avgStep/80.0*100)
	}
	if *wavPath != "" {
		fmt.Printf("max wake score over clip: %.3f\n", maxScore)
	}
	fmt.Printf("process RSS: %s\n", selfRSS())
}

// detector mirrors openWakeWord's streaming structure: melspec frames feed a
// rolling buffer; every embStride frames an embedding is computed; the
// classifier runs on the last classWindow embeddings.
type detector struct {
	mel, emb, cls  *ort.DynamicAdvancedSession
	melBuf         []float32 // flattened [frames][melBins]
	melFrames      int
	framesSinceEmb int
	embBuf         []float32 // flattened [n][embDim]
	embCount       int
}

func newDetector(mel, emb, cls *ort.DynamicAdvancedSession) *detector {
	return &detector{mel: mel, emb: emb, cls: cls}
}

// step processes one 80 ms chunk and returns the latest wake score plus
// per-model latencies in ms (0 if that model did not run this step).
func (d *detector) step(chunk []float32) (score, melMs, embMs, clsMs float64) {
	// --- melspectrogram ---
	in, err := ort.NewTensor(ort.NewShape(1, int64(len(chunk))), chunk)
	if err != nil {
		fatalf("mel input tensor: %v", err)
	}
	outs := []ort.Value{nil}
	t0 := time.Now()
	if err := d.mel.Run([]ort.Value{in}, outs); err != nil {
		fatalf("mel run: %v", err)
	}
	melMs = time.Since(t0).Seconds() * 1000
	in.Destroy()
	melOut := outs[0].(*ort.Tensor[float32])
	shp := melOut.GetShape()
	frames := int(shp[len(shp)-2])
	raw := melOut.GetData()
	// openWakeWord normalization: mel/10 + 2
	for i := range raw {
		raw[i] = raw[i]/10 + 2
	}
	d.melBuf = append(d.melBuf, raw...)
	d.melFrames += frames
	d.framesSinceEmb += frames
	melOut.Destroy()

	// --- embedding: one per embStride new frames once we have a full window ---
	for d.melFrames >= embWindow && d.framesSinceEmb >= embStride {
		start := (d.melFrames - embWindow) * melBins
		window := d.melBuf[start : start+embWindow*melBins]
		// embedding input shape [1, 76, 32, 1]
		et, err := ort.NewTensor(ort.NewShape(1, embWindow, melBins, 1), append([]float32(nil), window...))
		if err != nil {
			fatalf("emb input tensor: %v", err)
		}
		eo := []ort.Value{nil}
		t1 := time.Now()
		if err := d.emb.Run([]ort.Value{et}, eo); err != nil {
			fatalf("emb run: %v", err)
		}
		embMs += time.Since(t1).Seconds() * 1000
		et.Destroy()
		ev := eo[0].(*ort.Tensor[float32])
		d.embBuf = append(d.embBuf, append([]float32(nil), ev.GetData()...)...)
		d.embCount++
		ev.Destroy()
		d.framesSinceEmb -= embStride
		d.trimMel()
	}

	// --- classifier on the last classWindow embeddings ---
	if d.embCount >= classWindow {
		start := (d.embCount - classWindow) * embDim
		feats := d.embBuf[start : start+classWindow*embDim]
		ct, err := ort.NewTensor(ort.NewShape(1, classWindow, embDim), append([]float32(nil), feats...))
		if err != nil {
			fatalf("cls input tensor: %v", err)
		}
		co := []ort.Value{nil}
		t2 := time.Now()
		if err := d.cls.Run([]ort.Value{ct}, co); err != nil {
			fatalf("cls run: %v", err)
		}
		clsMs = time.Since(t2).Seconds() * 1000
		ct.Destroy()
		cv := co[0].(*ort.Tensor[float32])
		score = float64(cv.GetData()[0])
		cv.Destroy()
		d.trimEmb()
	}
	return score, melMs, embMs, clsMs
}

// trimMel/trimEmb keep the rolling buffers bounded so a long run does not grow
// memory unboundedly (the real detector would use ring buffers).
func (d *detector) trimMel() {
	keep := embWindow + embStride
	if d.melFrames > keep {
		drop := d.melFrames - keep
		d.melBuf = d.melBuf[drop*melBins:]
		d.melFrames = keep
	}
}

func (d *detector) trimEmb() {
	keep := classWindow + 4
	if d.embCount > keep {
		drop := d.embCount - keep
		d.embBuf = d.embBuf[drop*embDim:]
		d.embCount = keep
	}
}

func mustSession(path string, threads int) *ort.DynamicAdvancedSession {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		fatalf("GetInputOutputInfo %s: %v", path, err)
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		fatalf("NewSessionOptions: %v", err)
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(threads); err != nil {
		fatalf("SetIntraOpNumThreads: %v", err)
	}
	if err := opts.SetInterOpNumThreads(threads); err != nil {
		fatalf("SetInterOpNumThreads: %v", err)
	}
	inNames := names(ins)
	outNames := names(outs)
	s, err := ort.NewDynamicAdvancedSession(path, inNames, outNames, opts)
	if err != nil {
		fatalf("NewDynamicAdvancedSession %s: %v", path, err)
	}
	return s
}

func names(io []ort.InputOutputInfo) []string {
	out := make([]string, len(io))
	for i, x := range io {
		out[i] = x.Name
	}
	return out
}

func printIO(path string) {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		fatalf("GetInputOutputInfo %s: %v", path, err)
	}
	fmt.Printf("== %s\n", filepath.Base(path))
	for _, i := range ins {
		fmt.Printf("  in  %-20s %v %v\n", i.Name, i.Dimensions, i.DataType)
	}
	for _, o := range outs {
		fmt.Printf("  out %-20s %v %v\n", o.Name, o.Dimensions, o.DataType)
	}
}

// loadAudio returns float32 samples in [-1,1]. With a WAV path it decodes a
// 16kHz mono S16LE file; otherwise it synthesizes low-level noise of the
// requested duration (content does not affect latency, only the score).
func loadAudio(wavPath string, seconds float64) []float32 {
	if wavPath == "" {
		n := int(seconds * sampleRate)
		out := make([]float32, n)
		// deterministic low-amplitude pseudo-noise
		var s uint32 = 0x12345678
		for i := range out {
			s = s*1664525 + 1013904223
			out[i] = (float32(s>>8)/float32(1<<24) - 0.5) * 0.05
		}
		return out
	}
	raw, err := os.ReadFile(wavPath)
	if err != nil {
		fatalf("read wav: %v", err)
	}
	if len(raw) < 44 {
		fatalf("wav too short")
	}
	pcm := raw[44:] // assume canonical 44-byte header
	n := len(pcm) / 2
	out := make([]float32, n)
	for i := 0; i < n; i++ {
		v := int16(uint16(pcm[2*i]) | uint16(pcm[2*i+1])<<8)
		out[i] = float32(v) / 32768
	}
	return out
}

func report(name string, xs []float64) {
	if len(xs) == 0 {
		fmt.Printf("  %s  (did not run)\n", name)
		return
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	fmt.Printf("  %s  avg=%.3f  p50=%.3f  p95=%.3f  max=%.3f  (n=%d)\n",
		name, mean(s), s[len(s)/2], s[int(float64(len(s))*0.95)], s[len(s)-1], len(s))
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func selfRSS() string {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return "unknown"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				kb, _ := strconv.Atoi(fields[1])
				return fmt.Sprintf("%d kB (%.1f MiB)", kb, float64(kb)/1024)
			}
		}
	}
	return "unknown"
}

func fatalf(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "wakeword-spike: "+format+"\n", a...)
	os.Exit(1)
}

var _ = math.Sqrt // reserved for future stddev reporting
