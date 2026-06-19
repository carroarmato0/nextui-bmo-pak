// Package mod discovers mods: subfolders or .zip archives of mods/ that
// customize persona, voice, quotes, faces, and audio. mods/default has overlay
// semantics (per-asset fallback to BMO); other entries are self-contained
// characters.
package mod

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover returns selectable mods under modsRoot. The synthetic "default" is
// always first. Remaining entries are directories and <id>.zip archives, by id.
// When both a directory and a .zip exist for the same id, the directory wins
// and a warning is logged via logf (which may be nil). Non-directory,
// dot-prefixed, and non-.zip files are ignored.
func Discover(modsRoot string, logf func(format string, args ...any)) []Mod {
	def := Mod{
		ID:        DefaultID,
		Root:      filepath.Join(modsRoot, DefaultID),
		IsDefault: true,
	}
	def.Manifest = manifestFor(def.Root, def.ID, def.IsDefault)
	out := []Mod{def}

	entries, err := os.ReadDir(modsRoot)
	if err != nil {
		return out
	}

	// roots maps id -> Root path; directories take precedence over .zip files.
	roots := map[string]string{}
	isDir := map[string]bool{}
	var ids []string

	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			if name == DefaultID {
				continue
			}
			if _, seen := roots[name]; !seen {
				ids = append(ids, name)
			}
			roots[name] = filepath.Join(modsRoot, name)
			isDir[name] = true
			continue
		}
		if !strings.EqualFold(filepath.Ext(name), ".zip") {
			continue
		}
		id := strings.TrimSuffix(name, filepath.Ext(name))
		if id == DefaultID || id == "" {
			continue
		}
		if isDir[id] {
			if logf != nil {
				logf("mod %q: both directory and .zip present; using directory", id)
			}
			continue
		}
		if _, seen := roots[id]; !seen {
			ids = append(ids, id)
		}
		roots[id] = filepath.Join(modsRoot, name)
	}
	// Note: os.ReadDir returns entries sorted by name, and "<id>" always sorts
	// before "<id>.zip", so a directory is always seen before its same-named
	// archive — the isDir check above is sufficient for dir-wins precedence.

	sort.Strings(ids)
	for _, id := range ids {
		root := roots[id]
		out = append(out, Mod{
			ID:       id,
			Root:     root,
			Manifest: manifestFor(root, id, false),
		})
	}
	return out
}

// manifestFor reads a mod's manifest by transiently opening its source (so zip
// file descriptors are not held open across discovery). Returns the zero
// Manifest on any error.
func manifestFor(root, id string, isDefault bool) Manifest {
	tmp := Mod{Root: root, ID: id, IsDefault: isDefault}
	if err := tmp.Open(nil); err != nil {
		return Manifest{}
	}
	defer tmp.Close()
	return LoadManifest(tmp.FS)
}

// Active returns the mod in mods matching id, or the default entry (index 0).
func Active(mods []Mod, id string) Mod {
	id = strings.TrimSpace(id)
	if id != "" {
		for _, m := range mods {
			if m.ID == id {
				return m
			}
		}
	}
	if len(mods) > 0 {
		return mods[0]
	}
	return Mod{ID: DefaultID, IsDefault: true}
}
