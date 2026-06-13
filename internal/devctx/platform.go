package devctx

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// suffixRe matches a trailing " (EmulatorCode)" suffix on NextUI directory names.
var suffixRe = regexp.MustCompile(`^(.*\S)\s+\([^)]+\)$`)

// baseName strips the trailing " (EmulatorCode)" suffix from a NextUI directory
// name, e.g. "Game Boy Advance (GBA)" → "Game Boy Advance". Names without the
// suffix are returned unchanged.
func baseName(dirName string) string {
	if m := suffixRe.FindStringSubmatch(dirName); m != nil {
		return m[1]
	}
	return dirName
}

// platformGroup represents a canonical platform (e.g. "Game Boy Advance") that
// may be served by one or more emulator-variant subdirectories under the ROM
// root.
type platformGroup struct {
	Name string   // canonical display name, result of baseName
	Dirs []string // full paths of all emulator-variant subdirs
}

// platformGroups scans root for platform subdirectories, groups them by their
// baseName, counts non-hidden files in each group, skips groups with zero
// files, and returns the remainder sorted descending by total file count.
// Groups with equal file counts retain lexicographic directory order.
// The only hard error is when root itself cannot be read.
func platformGroups(root string) ([]platformGroup, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	// Preserve insertion order for determinism within same-count ties.
	type groupEntry struct {
		name  string
		dirs  []string
		count int
	}
	order := []string{}
	groups := map[string]*groupEntry{}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		dirPath := filepath.Join(root, e.Name())
		base := baseName(e.Name())

		if _, exists := groups[base]; !exists {
			order = append(order, base)
			groups[base] = &groupEntry{name: base}
		}
		g := groups[base]
		g.dirs = append(g.dirs, dirPath)

		// Count non-hidden files in this subdir; silently skip unreadable dirs.
		children, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}
		for _, c := range children {
			if !c.IsDir() && !strings.HasPrefix(c.Name(), ".") {
				g.count++
			}
		}
	}

	// Collect non-empty groups preserving order.
	result := make([]platformGroup, 0, len(order))
	for _, name := range order {
		g := groups[name]
		if g.count == 0 {
			continue
		}
		result = append(result, platformGroup{Name: g.name, Dirs: g.dirs})
	}

	// Sort descending by total file count; stable to keep original order on ties.
	counts := map[string]int{}
	for _, name := range order {
		counts[name] = groups[name].count
	}
	sort.SliceStable(result, func(i, j int) bool {
		return counts[result[i].Name] > counts[result[j].Name]
	})

	return result, nil
}

// gameName strips up to two trailing extensions from a save/ROM filename:
// "Pokemon Red (USA).zip.sav" → "Pokemon Red (USA)".
func gameName(file string) string {
	name := file
	for i := 0; i < 2; i++ {
		ext := filepath.Ext(name)
		if ext == "" || len(ext) == len(name) {
			break
		}
		name = strings.TrimSuffix(name, ext)
	}
	return name
}
