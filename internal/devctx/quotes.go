package devctx

import "strings"

// ParseQuotes splits quotes.txt content into one quote per line, skipping
// blank lines and #-comments.
func ParseQuotes(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// SetQuotes installs the source of verbatim fallback quotes used when every
// real remark topic is on cooldown. The source is consulted at pick time so
// edits to quotes.txt apply without a restart.
func (b *Builder) SetQuotes(fn func() []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.quotes = fn
}
