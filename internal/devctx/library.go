package devctx

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LibraryCollector summarizes the ROM library grouped by platform.
type LibraryCollector struct {
	Root   string // e.g. /mnt/SDCARD/Roms
	Detail string // "full" or "random"; defaults to "full" if empty
}

func (c LibraryCollector) Key() string { return KeyLibrary }

func (c LibraryCollector) Collect(_ time.Time) (Section, error) {
	groups, err := platformGroups(c.Root)
	if err != nil {
		return Section{}, err
	}

	type platformTitles struct {
		name   string
		titles []string
	}

	totalTitles := 0
	platforms := make([]platformTitles, 0, len(groups))

	for _, g := range groups {
		seen := map[string]struct{}{}
		for _, dir := range g.Dirs {
			entries, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
					continue
				}
				title := gameName(filepath.Base(e.Name()))
				seen[title] = struct{}{}
			}
		}
		if len(seen) == 0 {
			continue
		}
		titles := make([]string, 0, len(seen))
		for t := range seen {
			titles = append(titles, t)
		}
		sort.Strings(titles)
		totalTitles += len(titles)
		platforms = append(platforms, platformTitles{name: g.Name, titles: titles})
	}

	if totalTitles == 0 {
		return Section{}, fmt.Errorf("no games found under %s", c.Root)
	}

	fullMode := c.Detail == "" || c.Detail == "full"

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec

	lines := make([]string, 0, len(platforms)+1)
	lines = append(lines, fmt.Sprintf("%d platforms, %d total titles.", len(platforms), totalTitles))

	for _, p := range platforms {
		var line string
		if fullMode {
			line = fmt.Sprintf("%s: %s", p.name, strings.Join(p.titles, ", "))
		} else {
			pick := p.titles[rng.Intn(len(p.titles))]
			line = fmt.Sprintf("%s (%d titles): e.g. %s", p.name, len(p.titles), pick)
		}
		lines = append(lines, line)
	}

	body := strings.Join(lines, "\n")
	return Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: body}, nil
}
