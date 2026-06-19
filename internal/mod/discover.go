package mod

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover returns the selectable mods under modsRoot. The synthetic "default"
// entry (overlay on embedded BMO) is always first, even when mods/default does
// not exist on disk. Remaining entries are the existing subfolders in
// alphabetical order. Non-directories and dot-prefixed folders are ignored.
func Discover(modsRoot string) []Mod {
	def := Mod{
		ID:        DefaultID,
		Root:      filepath.Join(modsRoot, DefaultID),
		IsDefault: true,
	}
	def.Manifest = LoadManifest(os.DirFS(def.Root))
	out := []Mod{def}

	entries, err := os.ReadDir(modsRoot)
	if err != nil {
		return out
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == DefaultID || strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		root := filepath.Join(modsRoot, name)
		out = append(out, Mod{
			ID:       name,
			Root:     root,
			Manifest: LoadManifest(os.DirFS(root)),
		})
	}
	return out
}

// Active returns the mod in mods matching id, or the default entry (index 0)
// when id is empty or not found. mods must be the slice returned by Discover.
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
		return mods[0] // the default entry
	}
	return Mod{ID: DefaultID, IsDefault: true}
}
