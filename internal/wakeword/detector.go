package wakeword

import (
	"fmt"
	"sync"

	"github.com/carroarmato0/nextui-bmo/internal/audio"
	ort "github.com/yalue/onnxruntime_go"
)

const (
	hopSamples  = 1280 // 80 ms at 16 kHz
	melBins     = 32
	melPerHop   = 8 // mel frames produced per hop in a continuous stream
	embWindow   = 76
	embStride   = 8
	classWindow = 16
	embDim      = 96
	// melContext is extra trailing audio fed to the melspectrogram model each
	// hop so the windowed transform produces at least melPerHop full frames;
	// only the newest melPerHop frames are appended to the rolling buffer.
	melContext = 4800 // 300 ms

	defaultThreshold       = 0.5
	defaultThreads         = 2 // never 4 — see feasibility findings
	defaultRefractorySteps = 12
)

// Config configures a Detector. Paths point at the openWakeWord ONNX models and
// the onnxruntime shared library.
type Config struct {
	LibraryPath string
	MelModel    string
	EmbModel    string
	WakeModel   string
	Threshold   float64
	Threads     int
	// RefractorySteps suppresses re-firing for N hops after a detection.
	RefractorySteps int
}

// Detection is emitted when the wake score crosses the threshold.
type Detection struct {
	Score float64
}

// Detector is a streaming wake-word detector. Write is safe for a single
// producer goroutine; it is not safe for concurrent callers.
type Detector struct {
	mu            sync.Mutex
	mel, emb, cls *ort.DynamicAdvancedSession
	threshold     float64
	refractory    int
	sinceFired    int

	pending        []float32 // samples not yet grouped into a hop
	raw            []float32 // rolling audio window (<= hopSamples+melContext)
	melBuf         []float32 // flattened [frames][melBins]
	melFrames      int
	framesSinceEmb int
	embBuf         []float32 // flattened [n][embDim]
	embCount       int
}

var (
	envOnce sync.Once
	envErr  error
)

func initEnv(libPath string) error {
	envOnce.Do(func() {
		if libPath != "" {
			ort.SetSharedLibraryPath(libPath)
		}
		envErr = ort.InitializeEnvironment()
	})
	return envErr
}

// New loads the three models and initializes the process-global ORT
// environment (once). LibraryPath must be consistent across detectors.
func New(c Config) (*Detector, error) {
	if c.Threshold <= 0 {
		c.Threshold = defaultThreshold
	}
	if c.Threads <= 0 {
		c.Threads = defaultThreads
	}
	if c.RefractorySteps <= 0 {
		c.RefractorySteps = defaultRefractorySteps
	}
	if err := initEnv(c.LibraryPath); err != nil {
		return nil, fmt.Errorf("init onnxruntime: %w", err)
	}
	mel, err := newSession(c.MelModel, c.Threads)
	if err != nil {
		return nil, err
	}
	emb, err := newSession(c.EmbModel, c.Threads)
	if err != nil {
		mel.Destroy()
		return nil, err
	}
	cls, err := newSession(c.WakeModel, c.Threads)
	if err != nil {
		mel.Destroy()
		emb.Destroy()
		return nil, err
	}
	d := &Detector{
		mel:        mel,
		emb:        emb,
		cls:        cls,
		threshold:  c.Threshold,
		refractory: c.RefractorySteps,
		sinceFired: c.RefractorySteps,
	}
	// Pre-seed the embedding buffer with zero embeddings so the classifier
	// produces output from the first real embedding (matches openWakeWord's
	// zero-initialized feature buffer; lets short utterances score).
	d.embBuf = make([]float32, classWindow*embDim)
	d.embCount = classWindow
	return d, nil
}

func newSession(path string, threads int) (*ort.DynamicAdvancedSession, error) {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return nil, fmt.Errorf("model info %s: %w", path, err)
	}
	opts, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer opts.Destroy()
	if err := opts.SetIntraOpNumThreads(threads); err != nil {
		return nil, err
	}
	if err := opts.SetInterOpNumThreads(threads); err != nil {
		return nil, err
	}
	s, err := ort.NewDynamicAdvancedSession(path, names(ins), names(outs), opts)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", path, err)
	}
	return s, nil
}

func names(io []ort.InputOutputInfo) []string {
	out := make([]string, len(io))
	for i, x := range io {
		out[i] = x.Name
	}
	return out
}

// Write appends S16LE mono 16 kHz PCM and returns detections produced by the
// newly completed 80 ms hops.
func (d *Detector) Write(pcm []byte) []Detection {
	samples := audio.S16LEToFloat32(pcm)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = append(d.pending, samples...)
	var out []Detection
	for len(d.pending) >= hopSamples {
		hop := d.pending[:hopSamples]
		d.pending = d.pending[hopSamples:]
		if det, ok := d.step(hop); ok {
			out = append(out, det)
		}
	}
	// Reclaim the consumed prefix so pending does not grow unbounded.
	if len(d.pending) == 0 {
		d.pending = d.pending[:0]
	}
	return out
}

// Reset clears the rolling buffers (e.g. when the detector is paused and
// resumed) so stale context cannot produce a spurious detection.
func (d *Detector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending = d.pending[:0]
	d.raw = d.raw[:0]
	d.melBuf = d.melBuf[:0]
	d.melFrames = 0
	d.framesSinceEmb = 0
	for i := range d.embBuf {
		d.embBuf[i] = 0
	}
	d.embBuf = d.embBuf[:classWindow*embDim]
	d.embCount = classWindow
	d.sinceFired = d.refractory
}

// step runs one 80 ms hop through the pipeline.
func (d *Detector) step(hop []float32) (Detection, bool) {
	d.raw = append(d.raw, hop...)
	if max := hopSamples + melContext; len(d.raw) > max {
		d.raw = d.raw[len(d.raw)-max:]
	}
	frames := d.runMel(d.raw)
	if len(frames) > melPerHop {
		frames = frames[len(frames)-melPerHop:]
	}
	for _, f := range frames {
		d.melBuf = append(d.melBuf, f...)
	}
	d.melFrames += len(frames)
	d.framesSinceEmb += len(frames)

	for d.melFrames >= embWindow && d.framesSinceEmb >= embStride {
		start := (d.melFrames - embWindow) * melBins
		d.embBuf = append(d.embBuf, d.runEmb(d.melBuf[start:start+embWindow*melBins])...)
		d.embCount++
		d.framesSinceEmb -= embStride
		d.trimMel()
	}

	if d.sinceFired < d.refractory {
		d.sinceFired++
	}

	start := (d.embCount - classWindow) * embDim
	score := d.runCls(d.embBuf[start : start+classWindow*embDim])
	d.trimEmb()
	if score >= d.threshold && d.sinceFired >= d.refractory {
		d.sinceFired = 0
		return Detection{Score: score}, true
	}
	return Detection{}, false
}

// runMel runs the melspectrogram model and returns normalized mel frames as
// [][melBins] (openWakeWord normalization: value/10 + 2).
func (d *Detector) runMel(samples []float32) [][]float32 {
	in, err := ort.NewTensor(ort.NewShape(1, int64(len(samples))), append([]float32(nil), samples...))
	if err != nil {
		return nil
	}
	defer in.Destroy()
	outs := []ort.Value{nil}
	if err := d.mel.Run([]ort.Value{in}, outs); err != nil {
		return nil
	}
	out := outs[0].(*ort.Tensor[float32])
	defer out.Destroy()
	shp := out.GetShape()
	nFrames := int(shp[len(shp)-2])
	data := out.GetData()
	frames := make([][]float32, nFrames)
	for f := 0; f < nFrames; f++ {
		row := make([]float32, melBins)
		for b := 0; b < melBins; b++ {
			row[b] = data[f*melBins+b]/10 + 2
		}
		frames[f] = row
	}
	return frames
}

func (d *Detector) runEmb(window []float32) []float32 {
	in, err := ort.NewTensor(ort.NewShape(1, embWindow, melBins, 1), append([]float32(nil), window...))
	if err != nil {
		return make([]float32, embDim)
	}
	defer in.Destroy()
	outs := []ort.Value{nil}
	if err := d.emb.Run([]ort.Value{in}, outs); err != nil {
		return make([]float32, embDim)
	}
	out := outs[0].(*ort.Tensor[float32])
	defer out.Destroy()
	return append([]float32(nil), out.GetData()...)
}

func (d *Detector) runCls(feats []float32) float64 {
	in, err := ort.NewTensor(ort.NewShape(1, classWindow, embDim), append([]float32(nil), feats...))
	if err != nil {
		return 0
	}
	defer in.Destroy()
	outs := []ort.Value{nil}
	if err := d.cls.Run([]ort.Value{in}, outs); err != nil {
		return 0
	}
	out := outs[0].(*ort.Tensor[float32])
	defer out.Destroy()
	data := out.GetData()
	if len(data) == 0 {
		return 0
	}
	return float64(data[0])
}

func (d *Detector) trimMel() {
	keep := embWindow + embStride
	if d.melFrames > keep {
		drop := d.melFrames - keep
		d.melBuf = d.melBuf[drop*melBins:]
		d.melFrames = keep
	}
}

func (d *Detector) trimEmb() {
	keep := classWindow + 4
	if d.embCount > keep {
		drop := d.embCount - keep
		d.embBuf = d.embBuf[drop*embDim:]
		d.embCount = keep
	}
}

// Close releases the model sessions. It does not destroy the process-global ORT
// environment (other detectors may still be using it).
func (d *Detector) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var firstErr error
	for _, s := range []*ort.DynamicAdvancedSession{d.mel, d.emb, d.cls} {
		if s == nil {
			continue
		}
		if err := s.Destroy(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	d.mel, d.emb, d.cls = nil, nil, nil
	return firstErr
}
