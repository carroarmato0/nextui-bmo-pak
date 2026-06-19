package clips

import "io/fs"

// Library loads pre-recorded PCM clips, preferring on-disk overrides in the
// mod's audio/ filesystem over the embedded defaults.
type Library struct {
	fsys fs.FS // rooted at the mod's audio/; nil = embedded only
}

func NewLibrary(fsys fs.FS) *Library {
	return &Library{fsys: fsys}
}

// Load returns the PCM bytes for the named clip. It checks <name>.pcm in fsys
// first; if that file is absent or empty it falls back to the embedded asset.
// Returns nil if the clip is not found in either location.
func (l *Library) Load(name string) []byte {
	if l != nil && l.fsys != nil {
		if data, err := fs.ReadFile(l.fsys, name+".pcm"); err == nil && len(data) > 0 {
			return data
		}
	}
	data, err := embeddedAssets.ReadFile("assets/audio/" + name + ".pcm")
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}
