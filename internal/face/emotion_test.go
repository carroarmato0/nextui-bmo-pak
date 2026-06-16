package face

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmotionNamesExcludeFunctional(t *testing.T) {
	names := EmotionNames()
	// want = canonical names that are not functional faces. speaking is a
	// functional face that is intentionally absent from CanonicalNames (its
	// standalone asset is retired and it folds to neutral), so count the
	// functionals actually present in CanonicalNames rather than assuming every
	// functional name is canonical.
	want := 0
	for _, n := range CanonicalNames {
		if !isFunctional(n) {
			want++
		}
	}
	if len(names) != want {
		t.Fatalf("EmotionNames len = %d, want %d", len(names), want)
	}
	in := map[string]bool{}
	for _, n := range names {
		in[n] = true
	}
	for _, f := range FunctionalNames {
		if in[f] {
			t.Errorf("EmotionNames must not contain functional face %q", f)
		}
	}
	// Every emotion name must resolve to itself, or the model would be told
	// about a face BMO cannot show.
	for _, n := range names {
		if got := Canonical(n); got != n {
			t.Errorf("Canonical(%q) = %q, want self-resolving", n, got)
		}
	}
}

func TestEmotionFaceNamesInDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<svg/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("happy.svg")
	write("grumpy.svg")
	write("neutral.svg")
	write("speaking.svg") // functional — excluded
	write("notes.txt")    // not an svg — excluded

	got := EmotionFaceNamesInDir(dir)
	want := []string{"grumpy", "happy", "neutral"} // sorted, functional/non-svg dropped
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if EmotionFaceNamesInDir(filepath.Join(dir, "missing")) != nil {
		t.Error("missing dir should yield nil")
	}
}
