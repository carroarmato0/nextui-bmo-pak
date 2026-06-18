package qr

import "testing"

func TestMatrixRejectsEmpty(t *testing.T) {
	if _, err := Matrix(""); err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestMatrixIsSquareWithQuietZone(t *testing.T) {
	m, err := Matrix("https://github.com/carroarmato0/nextui-bmo-pak")
	if err != nil {
		t.Fatalf("Matrix: %v", err)
	}
	if len(m) == 0 {
		t.Fatal("empty matrix")
	}
	for i, row := range m {
		if len(row) != len(m) {
			t.Fatalf("row %d length %d, want square %d", i, len(row), len(m))
		}
	}
	// The quiet zone means the outermost ring is all light modules.
	for i := range m {
		if m[0][i] || m[len(m)-1][i] || m[i][0] || m[i][len(m)-1] {
			t.Fatalf("expected light quiet-zone border, found a dark module at edge %d", i)
		}
	}
	// A non-trivial code must contain at least some dark modules.
	dark := false
	for _, row := range m {
		for _, c := range row {
			if c {
				dark = true
			}
		}
	}
	if !dark {
		t.Fatal("matrix has no dark modules")
	}
}
