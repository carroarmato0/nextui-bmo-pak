package devctx

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver; works with CGO_ENABLED=0
)

// PlayLogCollector reads NextUI's game tracking database (read-only) and
// summarizes recently played and most-played games. NextUI may hold the DB
// open while a game runs; any error here just omits the section.
type PlayLogCollector struct {
	DBPath string // e.g. /mnt/SDCARD/.userdata/shared/game_logs.sqlite
	Detail string // "full" or "random"; defaults to "full" (20 entries) if empty
}

func (PlayLogCollector) Key() string { return KeyPlayLog }

func (c PlayLogCollector) withDetail(d string) Collector {
	c.Detail = d
	return c
}

type playRow struct {
	name    string
	seconds int64
	last    int64 // unix epoch of most recent session
}

func (c PlayLogCollector) Collect(now time.Time) (Section, error) {
	if _, err := os.Stat(c.DBPath); err != nil {
		return Section{}, fmt.Errorf("play log db: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+c.DBPath+"?mode=ro")
	if err != nil {
		return Section{}, fmt.Errorf("open play log db: %w", err)
	}
	defer db.Close()

	// Determine limit based on Detail mode.
	limit := 20 // full mode (default)
	if strings.EqualFold(c.Detail, "random") {
		limit = 5
	}

	// The same game can exist as several rom rows; group by name.
	recent, err := queryPlayRows(db, fmt.Sprintf(`
		SELECT r.name, SUM(p.play_time), MAX(p.created_at)
		FROM play_activity p JOIN rom r ON r.id = p.rom_id
		GROUP BY r.name ORDER BY MAX(p.created_at) DESC LIMIT %d`, limit))
	if err != nil {
		return Section{}, err
	}
	if len(recent) == 0 {
		return Section{}, fmt.Errorf("play log is empty")
	}
	top, err := queryPlayRows(db, fmt.Sprintf(`
		SELECT r.name, SUM(p.play_time), MAX(p.created_at)
		FROM play_activity p JOIN rom r ON r.id = p.rom_id
		GROUP BY r.name ORDER BY SUM(p.play_time) DESC LIMIT %d`, limit))
	if err != nil {
		return Section{}, err
	}

	freshest := time.Unix(recent[0].last, 0).UTC()
	recentParts := make([]string, 0, len(recent))
	for _, r := range recent {
		recentParts = append(recentParts, fmt.Sprintf("%s (last played %s, %s total)",
			r.name, RelTime(time.Unix(r.last, 0).UTC(), now), playDuration(r.seconds)))
	}
	topParts := make([]string, 0, len(top))
	for _, r := range top {
		topParts = append(topParts, fmt.Sprintf("%s (%s total)", r.name, playDuration(r.seconds)))
	}
	body := fmt.Sprintf("Last play session was %s. Recently played: %s. Most played ever: %s.",
		RelTime(freshest, now), strings.Join(recentParts, "; "), strings.Join(topParts, "; "))
	return Section{Key: KeyPlayLog, Title: "PLAY HISTORY", Body: body, Freshest: freshest}, nil
}

func queryPlayRows(db *sql.DB, query string) ([]playRow, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query play log: %w", err)
	}
	defer rows.Close()
	var out []playRow
	for rows.Next() {
		var r playRow
		if err := rows.Scan(&r.name, &r.seconds, &r.last); err != nil {
			return nil, fmt.Errorf("scan play log: %w", err)
		}
		if strings.TrimSpace(r.name) == "" {
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// playDuration formats seconds of play compactly: "5m", "1h22m".
func playDuration(seconds int64) string {
	if seconds < 60 {
		return "under a minute"
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	if h == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh%02dm", h, m)
}
