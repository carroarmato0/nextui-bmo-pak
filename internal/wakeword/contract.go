package wakeword

import (
	"fmt"

	ort "github.com/yalue/onnxruntime_go"
)

// Wake-word classifier I/O contract: consume 16 openWakeWord embeddings of
// width 96, emit one sigmoid score. A -1 in the want shape is a wildcard so a
// model may declare a fixed (1) or dynamic (-1) batch dimension.
var (
	classifierInputShape  = []int64{-1, classWindow, embDim} // [_,16,96]
	classifierOutputShape = []int64{-1, 1}                   // [_,1]
)

// InitEnv initializes the process-global ONNX Runtime environment from the
// shared library at libPath. Safe to call repeatedly; only the first call has
// effect — subsequent calls with a different libPath are no-ops (the path from
// the first call wins). New also calls this, so code that only uses New need
// not call it.
func InitEnv(libPath string) error {
	return initEnv(libPath)
}

// ValidateClassifier reports whether the ONNX model at path matches the
// wake-word classifier contract: a single float32 input [_,16,96] and a single
// float32 output [_,1]. InitEnv (or New) must have run first.
func ValidateClassifier(path string) error {
	ins, outs, err := ort.GetInputOutputInfo(path)
	if err != nil {
		return fmt.Errorf("model info %s: %w", path, err)
	}
	if len(ins) != 1 {
		return fmt.Errorf("classifier %s: want 1 input, got %d", path, len(ins))
	}
	if len(outs) != 1 {
		return fmt.Errorf("classifier %s: want 1 output, got %d", path, len(outs))
	}
	if !matchShape(ins[0].Dimensions, classifierInputShape) {
		return fmt.Errorf("classifier %s: input shape %v, want [_,%d,%d]", path, ins[0].Dimensions, classWindow, embDim)
	}
	if !matchShape(outs[0].Dimensions, classifierOutputShape) {
		return fmt.Errorf("classifier %s: output shape %v, want [_,1]", path, outs[0].Dimensions)
	}
	if ins[0].DataType != ort.TensorElementDataTypeFloat {
		return fmt.Errorf("classifier %s: input dtype %v, want float32", path, ins[0].DataType)
	}
	if outs[0].DataType != ort.TensorElementDataTypeFloat {
		return fmt.Errorf("classifier %s: output dtype %v, want float32", path, outs[0].DataType)
	}
	return nil
}

// matchShape reports whether got matches want, where want entries of -1 are
// wildcards.
func matchShape(got ort.Shape, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if want[i] != -1 && got[i] != want[i] {
			return false
		}
	}
	return true
}
