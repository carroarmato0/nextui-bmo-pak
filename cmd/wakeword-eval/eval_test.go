package main

import (
	"math"
	"testing"
)

func TestFalseAcceptsPerHour(t *testing.T) {
	got := falseAcceptsPerHour(2, 1800) // 2 in half an hour -> 4/hr
	if math.Abs(got-4) > 1e-9 {
		t.Fatalf("got %v want 4", got)
	}
	if falseAcceptsPerHour(5, 0) != 0 {
		t.Fatal("zero duration should yield 0")
	}
}

func TestSuggestThresholdSeparable(t *testing.T) {
	th, ok := suggestThreshold([]float64{0.8, 0.9}, []float64{0.2, 0.3})
	if !ok {
		t.Fatal("expected separable")
	}
	if math.Abs(th-0.55) > 1e-9 { // midpoint of lowestPos(0.8) and highestNeg(0.3)
		t.Fatalf("got %v want 0.55", th)
	}
}

func TestSuggestThresholdOverlap(t *testing.T) {
	if _, ok := suggestThreshold([]float64{0.4}, []float64{0.6}); ok {
		t.Fatal("expected not separable")
	}
	if _, ok := suggestThreshold(nil, []float64{0.1}); ok {
		t.Fatal("no positives -> not separable")
	}
}
