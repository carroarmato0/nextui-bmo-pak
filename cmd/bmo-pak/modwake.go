package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/carroarmato0/nextui-bmo/internal/mod"
	"github.com/carroarmato0/nextui-bmo/internal/wakeword"
)

// modWakeModelName is the conventional path, within a mod's FS, of a custom
// wake-word classifier that replaces the stock "Hey BMO" model.
const modWakeModelName = "wakeword/wake.onnx"

// buildWakeAssets locates the ONNX runtime library and base models in the pak,
// and resolves the wake classifier from the active mod (validated against the
// detector contract). On any problem — extraction failure, ORT init failure, or
// a model that fails the [_,16,96]->[_,1] contract — it logs and falls back to
// the pak's stock hey_bmo.onnx. The returned cleanup removes any extracted temp
// model; it is a no-op when the default is used. cleanup must be called after
// the detector built from these assets is closed.
func buildWakeAssets(activeMod mod.Mod, pakDir, platform, tmpDir string, logger pttLogger) (wakeAssets, func()) {
	assets := wakeAssets{
		ORTLib:   filepath.Join(pakDir, "lib", platform, "libonnxruntime.so"),
		MelModel: filepath.Join(pakDir, "assets", "wakeword", "melspectrogram.onnx"),
		EmbModel: filepath.Join(pakDir, "assets", "wakeword", "embedding_model.onnx"),
	}
	defaultWake := filepath.Join(pakDir, "assets", "wakeword", "hey_bmo.onnx")

	path, custom, err := resolveWakeModel(activeMod.FS, activeMod.ID, defaultWake, tmpDir)
	if err != nil {
		logger.Warnf("wake model: mod %q extraction failed: %v; model=default", activeMod.ID, err)
		assets.WakeModel = defaultWake
		return assets, func() {}
	}
	if !custom {
		logger.Infof("wake model: model=default")
		assets.WakeModel = defaultWake
		return assets, func() {}
	}

	cleanup := func() { _ = os.Remove(path) }
	if initErr := wakeword.InitEnv(assets.ORTLib); initErr != nil {
		logger.Warnf("wake model: ORT init failed (%v); mod %q ignored; model=default", initErr, activeMod.ID)
		cleanup()
		assets.WakeModel = defaultWake
		return assets, func() {}
	}
	if vErr := wakeword.ValidateClassifier(path); vErr != nil {
		logger.Warnf("wake model: mod %q model invalid (%v); model=default", activeMod.ID, vErr)
		cleanup()
		assets.WakeModel = defaultWake
		return assets, func() {}
	}
	logger.Infof("wake model: model=mod(%s)", activeMod.ID)
	assets.WakeModel = path
	return assets, cleanup
}

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
