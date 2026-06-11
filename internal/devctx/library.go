package devctx

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LibraryCollector summarizes the ROM library as per-system game counts.
// Game names are deliberately not listed (a full listing is ~6K tokens);
// specific titles reach BMO through saves, play history, and achievements.
type LibraryCollector struct {
	Root string // e.g. /mnt/SDCARD/Roms
}

func (LibraryCollector) Key() string { return KeyLibrary }

func (c LibraryCollector) Collect(now time.Time) (Section, error) {
	systems, err := os.ReadDir(c.Root)
	if err != nil {
		return Section{}, fmt.Errorf("read roms dir: %w", err)
	}
	type sysCount struct {
		name  string
		count int
	}
	var counts []sysCount
	total := 0
	for _, sys := range systems {
		if !sys.IsDir() || strings.HasPrefix(sys.Name(), ".") {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(c.Root, sys.Name()))
		if err != nil {
			continue
		}
		n := 0
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), ".") {
				continue
			}
			n++
		}
		if n == 0 {
			continue
		}
		counts = append(counts, sysCount{sys.Name(), n})
		total += n
	}
	if total == 0 {
		return Section{}, fmt.Errorf("no games found under %s", c.Root)
	}
	sort.Slice(counts, func(i, j int) bool { return counts[i].count > counts[j].count })
	parts := make([]string, 0, len(counts))
	for _, sc := range counts {
		parts = append(parts, fmt.Sprintf("%s: %d", sc.name, sc.count))
	}
	body := fmt.Sprintf("%d games across %d systems. %s.", total, len(counts), strings.Join(parts, "; "))
	return Section{Key: KeyLibrary, Title: "GAME LIBRARY", Body: body}, nil
}
