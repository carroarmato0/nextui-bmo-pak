package wakeword

import (
	"testing"

	ort "github.com/yalue/onnxruntime_go"
)

func TestMatchShape(t *testing.T) {
	cases := []struct {
		name string
		got  ort.Shape
		want []int64
		ok   bool
	}{
		{"exact", ort.Shape{1, 16, 96}, []int64{1, 16, 96}, true},
		{"dynamic batch matches wildcard", ort.Shape{-1, 16, 96}, []int64{-1, 16, 96}, true},
		{"fixed batch matches wildcard", ort.Shape{1, 16, 96}, []int64{-1, 16, 96}, true},
		{"wrong inner dim", ort.Shape{1, 8, 96}, []int64{-1, 16, 96}, false},
		{"wrong rank", ort.Shape{1, 16}, []int64{-1, 16, 96}, false},
		{"output ok", ort.Shape{1, 1}, []int64{-1, 1}, true},
		{"output wrong", ort.Shape{1, 2}, []int64{-1, 1}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchShape(c.got, c.want); got != c.ok {
				t.Fatalf("matchShape(%v,%v)=%v want %v", c.got, c.want, got, c.ok)
			}
		})
	}
}

func TestValidateClassifierAcceptsWakeModel(t *testing.T) {
	cfg := testConfig(t) // skips unless ONNXRUNTIME_LIB/WAKEWORD_* are set
	if err := InitEnv(cfg.LibraryPath); err != nil {
		t.Fatalf("InitEnv: %v", err)
	}
	if err := ValidateClassifier(cfg.WakeModel); err != nil {
		t.Fatalf("wake model should satisfy the contract: %v", err)
	}
}

func TestValidateClassifierRejectsWrongShape(t *testing.T) {
	cfg := testConfig(t)
	if err := InitEnv(cfg.LibraryPath); err != nil {
		t.Fatalf("InitEnv: %v", err)
	}
	// The melspectrogram model has a different I/O shape than a classifier.
	if err := ValidateClassifier(cfg.MelModel); err == nil {
		t.Fatal("mel model should be rejected by the classifier contract")
	}
}
