package clips

import (
	"os"
	"path/filepath"
)

// Library loads pre-recorded PCM clips, preferring on-disk overrides in
// homeDir/audio/ over the embedded defaults.
type Library struct {
	homeDir string
}

func NewLibrary(homeDir string) *Library {
	return &Library{homeDir: homeDir}
}

// Load returns the PCM bytes for the named clip. It checks homeDir/audio/<name>.pcm
// first; if that file is absent or empty it falls back to the embedded asset.
// Returns nil if the clip is not found in either location.
func (l *Library) Load(name string) []byte {
	if l != nil && l.homeDir != "" {
		p := filepath.Join(l.homeDir, "audio", name+".pcm")
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return data
		}
	}

	data, err := embeddedAssets.ReadFile("assets/audio/" + name + ".pcm")
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}
