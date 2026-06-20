// Command wakeword-eval scores a candidate wake-word classifier (.onnx)
// against folders of 16 kHz mono WAVs using the on-device detector and the
// bundled openWakeWord base models. It reports true-accept rate, false
// accepts (and an hourly estimate), and a suggested threshold, and exits
// non-zero if the model violates the [1,16,96]->[1,1] contract or evaluation
// finds no clean separating threshold.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var o Options
	flag.StringVar(&o.LibraryPath, "lib", "third_party/wakeword/libonnxruntime.so", "path to libonnxruntime.so")
	flag.StringVar(&o.MelModel, "mel", "third_party/wakeword/models/melspectrogram.onnx", "path to melspectrogram.onnx")
	flag.StringVar(&o.EmbModel, "emb", "third_party/wakeword/models/embedding_model.onnx", "path to embedding_model.onnx")
	flag.StringVar(&o.Model, "model", "", "path to the candidate classifier .onnx (required)")
	flag.StringVar(&o.Positives, "positives", "", "dir of WAVs that SHOULD wake (required)")
	flag.StringVar(&o.Negatives, "negatives", "", "dir of WAVs that should NOT wake (required)")
	flag.Float64Var(&o.Threshold, "threshold", 0.5, "decision threshold for accept/false-accept counts")
	flag.IntVar(&o.Threads, "threads", 2, "ONNX Runtime intra-op threads")
	flag.Parse()

	if o.Model == "" || o.Positives == "" || o.Negatives == "" {
		fmt.Fprintln(os.Stderr, "wakeword-eval: -model, -positives and -negatives are required")
		flag.Usage()
		os.Exit(2)
	}

	rep, err := Run(o)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wakeword-eval: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("model:            %s\n", o.Model)
	fmt.Printf("threshold:        %.3f\n", o.Threshold)
	fmt.Printf("positives:        %d  accepted %d  (%.1f%% true-accept)\n", rep.Positives, rep.PositiveAccepts, rep.TrueAcceptRate*100)
	fmt.Printf("negatives:        %d clips, %.1f s\n", rep.Negatives, rep.NegativeSeconds)
	fmt.Printf("false accepts:    %d  (~%.2f / hour)\n", rep.FalseAccepts, rep.FalseAcceptsHour)
	if rep.Separable {
		fmt.Printf("suggested thresh: %.3f\n", rep.SuggestedThresh)
	} else {
		fmt.Printf("suggested thresh: (none — positive/negative scores overlap)\n")
	}

	// Non-zero exit when the classes don't separate, so CI / scripts can gate.
	if !rep.Separable {
		os.Exit(3)
	}
}
