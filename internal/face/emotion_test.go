package face

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEmotionNamesExcludeFunctional(t *testing.T) {
	names := EmotionNames()
	// want = canonical names that are neither functional nor static: a face is
	// advertised only if it has an amplitude-driven (lip-syncing) animation.
	// speaking is a functional face intentionally absent from CanonicalNames (its
	// standalone asset is retired and it folds to neutral).
	anims := DefaultAnimations()
	want := 0
	for _, n := range CanonicalNames {
		if isFunctional(n) {
			continue
		}
		if def, ok := anims[n]; !ok || def.Driver.Kind != DriverAmplitude {
			continue
		}
		want++
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
	// Static faces (no talk-mouth) must never reach the model: they would freeze
	// mid-sentence. They stay available as idle poses only.
	for _, s := range []string{ExprCrying, ExprTeary, ExprDizzy, ExprKiss, ExprGrimace, ExprShout, ExprDead, ExprGlitch} {
		if in[s] {
			t.Errorf("EmotionNames must not advertise static face %q to the model", s)
		}
	}
	// Lip-syncing emotions must still be offered, including the ones with a
	// prominent closed rest mouth.
	for _, a := range []string{ExprHappy, ExprAngry, ExprExcited, ExprSmile, ExprLove} {
		if !in[a] {
			t.Errorf("EmotionNames must advertise lip-syncing face %q", a)
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

func TestWhistleLookAroundAreFunctionalIdleFaces(t *testing.T) {
	canon := map[string]bool{}
	for _, n := range CanonicalNames {
		canon[n] = true
	}
	for _, n := range []string{ExprLookAround, ExprWhistle} {
		if !canon[n] {
			t.Errorf("%q must be in CanonicalNames (warms static fallback)", n)
		}
		if !isFunctional(n) {
			t.Errorf("%q must be functional (excluded from the LLM vocab)", n)
		}
	}
	for _, n := range EmotionNames() {
		if n == ExprLookAround || n == ExprWhistle {
			t.Errorf("EmotionNames must not advertise idle face %q to the model", n)
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

func TestFaceNamesInDir(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("<svg/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("happy.svg")
	write("look_around.svg") // functional — INCLUDED here (unlike EmotionFaceNamesInDir)
	write("neutral.svg")
	write("speaking.svg") // functional — included
	write("notes.txt")    // not an svg — excluded

	got := FaceNamesInDir(dir)
	want := []string{"happy", "look_around", "neutral", "speaking"} // sorted, functional kept
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if FaceNamesInDir(filepath.Join(dir, "missing")) != nil {
		t.Error("missing dir should yield nil")
	}
}
