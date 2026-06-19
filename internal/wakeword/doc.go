// Package wakeword runs the openWakeWord ONNX pipeline (melspectrogram ->
// Google speech-embedding -> wake-word classifier) on a streaming 16 kHz mono
// S16LE audio source and reports detections when the classifier score crosses
// a threshold.
//
// Feasibility, on-device latency, CPU and RAM budget are documented in
// docs/superpowers/2026-06-19-p2.0-wakeword-feasibility-findings.md. Key
// findings applied here: use 2 intra-op threads (never 4), feed melspectrogram
// a continuous stream (8 mel frames per 80 ms hop), and apply openWakeWord's
// mel/10+2 normalization before the embedding model.
//
// The onnxruntime shared library is loaded at runtime via SetSharedLibraryPath
// (the yalue/onnxruntime_go binding dlopen's it); models and the .so ship as
// pak assets, not embedded.
package wakeword
