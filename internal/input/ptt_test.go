package input

import "testing"

func TestParseButtonCode(t *testing.T) {
	cases := map[string]uint16{
		"BTN_TL":     310,
		"btn_tr":     311,
		"BTN_TL2":    312,
		" BTN_TR2 ":  313,
		"BTN_SELECT": 314,
	}
	for name, want := range cases {
		got, ok := ParseButtonCode(name)
		if !ok {
			t.Fatalf("ParseButtonCode(%q) failed", name)
		}
		if got != want {
			t.Fatalf("ParseButtonCode(%q) = %d, want %d", name, got, want)
		}
	}
	if _, ok := ParseButtonCode("BTN_UNKNOWN"); ok {
		t.Fatal("ParseButtonCode(unknown) = ok, want false")
	}
}

func TestBufferBeginAppendEnd(t *testing.T) {
	buf := NewBuffer(8)
	if buf.Held() {
		t.Fatal("Held() = true, want false")
	}
	buf.Begin()
	if !buf.Held() {
		t.Fatal("Held() = false after Begin")
	}
	buf.Append([]byte{1, 2, 3, 4})
	buf.Append([]byte{5, 6, 7, 8, 9})
	got := buf.End()
	if buf.Held() {
		t.Fatal("Held() = true after End")
	}
	if len(got) != 8 {
		t.Fatalf("End() len = %d, want 8", len(got))
	}
	want := []byte{2, 3, 4, 5, 6, 7, 8, 9}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("End()[%d] = %d, want %d", i, got[i], v)
		}
	}
}

func TestBufferIgnoresWhenNotHeld(t *testing.T) {
	buf := NewBuffer(8)
	buf.Append([]byte{1, 2, 3})
	if got := buf.End(); got != nil {
		t.Fatalf("End() = %v, want nil", got)
	}
}
