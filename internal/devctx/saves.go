package devctx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SavesCollector summarizes save files: counts per system plus the most
// recently touched save names. Save file names carry real game titles, so
// they are BMO's main source of "games with progress".
type SavesCollector struct {
	Root string // e.g. /mnt/SDCARD/Saves
}

func (SavesCollector) Key() string { return KeySaves }

func (c SavesCollector) Collect(now time.Time) (Section, error) {
	systems, err := os.ReadDir(c.Root)
	if err != nil {
		return Section{}, fmt.Errorf("read saves dir: %w", err)
	}
	type saveFile struct {
		game   string
		system string
		mtime  time.Time
	}
	var files []saveFile
	counts := map[string]int{}
	var order []string
	for _, sys := range systems {
		if !sys.IsDir() || strings.HasPrefix(sys.Name(), ".") {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(c.Root, sys.Name()))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if counts[sys.Name()] == 0 {
				order = append(order, sys.Name())
			}
			counts[sys.Name()]++
			files = append(files, saveFile{gameName(e.Name()), sys.Name(), info.ModTime()})
		}
	}
	if len(files) == 0 {
		return Section{}, fmt.Errorf("no save files under %s", c.Root)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	sort.Slice(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })

	countParts := make([]string, 0, len(order))
	for _, sys := range order {
		countParts = append(countParts, fmt.Sprintf("%s: %d", sys, counts[sys]))
	}
	recent := files
	if len(recent) > 5 {
		recent = recent[:5]
	}
	recentParts := make([]string, 0, len(recent))
	for _, f := range recent {
		recentParts = append(recentParts, fmt.Sprintf("%s (%s, %s)", f.game, f.system, RelTime(f.mtime, now)))
	}
	body := fmt.Sprintf("%d save files (%s). Most recently touched: %s.",
		len(files), strings.Join(countParts, ", "), strings.Join(recentParts, "; "))
	return Section{Key: KeySaves, Title: "SAVE FILES", Body: body, Freshest: files[0].mtime}, nil
}

// gameName strips up to two trailing extensions from a save file name:
// "Pokemon Red (USA).zip.sav" → "Pokemon Red (USA)". Parenthesized region
// tags are kept — they are part of how players know their ROMs.
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
