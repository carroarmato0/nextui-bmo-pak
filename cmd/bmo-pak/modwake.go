package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// modWakeModelName is the conventional path, within a mod's FS, of a custom
// wake-word classifier that replaces the stock "Hey BMO" model.
const modWakeModelName = "wakeword/wake.onnx"

// resolveWakeModel returns a filesystem path to the wake classifier to use: the
// active mod's wakeword/wake.onnx (extracted to tmpDir so onnxruntime can load
// it regardless of whether the mod is a directory or a zip) if present, else
// defaultPath. custom reports whether a mod-supplied model was extracted. A
// missing/unreadable mod file is not an error — it falls back to the default.
func resolveWakeModel(modFS fs.FS, modID, defaultPath, tmpDir string) (path string, custom bool, err error) {
	if modFS == nil {
		return defaultPath, false, nil
	}
	data, readErr := fs.ReadFile(modFS, modWakeModelName)
	if readErr != nil {
		return defaultPath, false, nil // absent/unreadable -> default
	}
	out := filepath.Join(tmpDir, "wake-"+modID+".onnx")
	if writeErr := os.WriteFile(out, data, 0o644); writeErr != nil {
		return defaultPath, false, fmt.Errorf("extract wake model for mod %q: %w", modID, writeErr)
	}
	return out, true, nil
}
