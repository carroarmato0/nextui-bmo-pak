package devctx

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// SystemCollector reports device health: model, uptime, memory, SD card
// space, battery. Every sub-reading is independently best-effort; the
// collector only fails when nothing at all is readable. Evergreen — it
// never claims to be news.
type SystemCollector struct {
	Model       string // human device name from the device tree, may be ""
	UptimePath  string // /proc/uptime
	MeminfoPath string // /proc/meminfo
	DiskPath    string // mount point to statfs, e.g. /mnt/SDCARD
	PowerDir    string // /sys/class/power_supply
}

func (SystemCollector) Key() string { return KeySystem }

func (c SystemCollector) Collect(now time.Time) (Section, error) {
	var parts []string
	if m := strings.TrimSpace(c.Model); m != "" {
		parts = append(parts, fmt.Sprintf("You live inside a %s handheld.", m))
	}
	if up, ok := readUptime(c.UptimePath); ok {
		parts = append(parts, fmt.Sprintf("You have been awake for %s.", up))
	}
	if mem, ok := readMemUsedPercent(c.MeminfoPath); ok {
		parts = append(parts, fmt.Sprintf("Memory is %d%% used.", mem))
	}
	if used, total, ok := diskUsage(c.DiskPath); ok {
		parts = append(parts, fmt.Sprintf("SD card: %.1fG used of %.1fG.", used, total))
	}
	if bat, ok := readBattery(c.PowerDir); ok {
		parts = append(parts, fmt.Sprintf("Battery is at %d%%.", bat))
	}
	if len(parts) == 0 {
		return Section{}, fmt.Errorf("no system facts available")
	}
	return Section{Key: KeySystem, Title: "YOUR BODY (THE DEVICE)", Body: strings.Join(parts, " ")}, nil
}

func readUptime(path string) (string, bool) {
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "", false
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || secs <= 0 {
		return "", false
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%d days and %d hours", days, hours), true
	case hours > 0:
		return fmt.Sprintf("%d hours and %d minutes", hours, mins), true
	default:
		return fmt.Sprintf("%d minutes", mins), true
	}
}

func readMemUsedPercent(path string) (int, bool) {
	if path == "" {
		return 0, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var totalKB, availKB float64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB = v
		case "MemAvailable:":
			availKB = v
		}
	}
	if totalKB <= 0 || availKB < 0 || availKB > totalKB {
		return 0, false
	}
	return int(math.Round((1 - availKB/totalKB) * 100)), true
}

func diskUsage(path string) (usedG, totalG float64, ok bool) {
	if path == "" {
		return 0, 0, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, 0, false
	}
	const g = 1 << 30
	total := float64(st.Blocks) * float64(st.Bsize) / g
	free := float64(st.Bavail) * float64(st.Bsize) / g
	if total <= 0 {
		return 0, 0, false
	}
	return total - free, total, true
}

func readBattery(dir string) (int, bool) {
	if dir == "" {
		return 0, false
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*", "capacity"))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		if v, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && v >= 0 && v <= 100 {
			return v, true
		}
	}
	return 0, false
}
