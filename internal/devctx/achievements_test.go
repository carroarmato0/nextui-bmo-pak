package devctx

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeCacheFile encodes v as an rcheevos offline cache file: 4-byte LE
// length prefix + JSON + fake trailing checksum.
func writeCacheFile(t *testing.T, path string, v any) {
	t.Helper()
	payload, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 4, 4+len(payload)+8)
	binary.LittleEndian.PutUint32(data, uint32(len(payload)))
	data = append(data, payload...)
	data = append(data, 0xde, 0xad, 0xbe, 0xef, 0xde, 0xad, 0xbe, 0xef)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReadCachePayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "login.bin")
	writeCacheFile(t, path, map[string]any{"Success": true})
	payload, err := readCachePayload(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(payload) != `{"Success":true}` {
		t.Errorf("payload = %q", payload)
	}
}

func TestReadCachePayloadCorrupt(t *testing.T) {
	dir := t.TempDir()
	short := filepath.Join(dir, "short.bin")
	if err := os.WriteFile(short, []byte{1, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(short); err == nil {
		t.Error("expected error for short file")
	}
	oversize := filepath.Join(dir, "oversize.bin")
	if err := os.WriteFile(oversize, []byte{0xff, 0xff, 0xff, 0x7f, '{', '}'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(oversize); err == nil {
		t.Error("expected error for oversize length prefix")
	}
	zero := filepath.Join(dir, "zero.bin")
	if err := os.WriteFile(zero, []byte{0x00, 0x00, 0x00, 0x00, '{', '}'}, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readCachePayload(zero); err == nil {
		t.Error("expected error for zero length prefix")
	}
	if _, err := readCachePayload(filepath.Join(dir, "missing.bin")); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestDifficultyTag(t *testing.T) {
	cases := []struct {
		points  int
		rarity  float64
		achType string
		want    string
	}{
		{10, 50, "win_condition", "beat the game!"},
		{10, 0, "", ""},
		{25, 3.2, "", "very rare — almost no players have done this"},
		{10, 12.5, "", "impressive — few players have this"},
		{5, 86.5, "", "easy — most players have this"},
		{5, 30, "", "easy — most players have this"},
		{10, 70, "", "easy — most players have this"},
		{10, 40, "progression", "solid"},
	}
	for _, c := range cases {
		if got := difficultyTag(c.points, c.rarity, c.achType); got != c.want {
			t.Errorf("difficultyTag(%d, %v, %q) = %q, want %q", c.points, c.rarity, c.achType, got, c.want)
		}
	}
}
