package devctx

import (
	"reflect"
	"testing"
	"time"
)

func TestParseQuotes(t *testing.T) {
	content := "Who wants to play video games?\n\n# comment line\n  Check, please!  \n"
	got := ParseQuotes(content)
	want := []string{"Who wants to play video games?", "Check, please!"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseQuotes = %#v, want %#v", got, want)
	}
	if ParseQuotes("") != nil {
		t.Fatal("empty content must yield no quotes")
	}
}

func TestProactiveNudgeQuoteFallback(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetJournal(&Journal{entries: []RemarkEntry{
		{When: now.Add(-30 * time.Minute), Topic: KeySaves, Subject: KeySaves, Reply: "saves!"},
		{When: now.Add(-20 * time.Minute), Topic: TopicQuote, Subject: "Check, please!", Reply: "Check, please!"},
	}})
	b.SetQuotes(func() []string {
		return []string{"Who wants to play video games?", "Check, please!"}
	})
	sawQuote, sawSilence := false, false
	for i := 0; i < 300; i++ {
		n, ok := b.ProactiveNudge()
		if !ok {
			sawSilence = true
			continue
		}
		sawQuote = true
		if !n.Verbatim || n.Topic != TopicQuote {
			t.Fatalf("expected a verbatim quote nudge, got %+v", n)
		}
		// "Check, please!" sits in the journal window: never re-picked.
		if n.Text != "Who wants to play video games?" {
			t.Fatalf("journaled quote re-picked: %+v", n)
		}
		if n.Subject != n.Text {
			t.Fatalf("quote subject must equal its text: %+v", n)
		}
	}
	if !sawQuote || !sawSilence {
		t.Fatalf("expected both quotes and silence over 300 runs (quote=%v silence=%v)", sawQuote, sawSilence)
	}
}

func TestProactiveNudgeQuotesExhausted(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetJournal(&Journal{entries: []RemarkEntry{
		{When: now.Add(-30 * time.Minute), Topic: KeySaves, Subject: KeySaves, Reply: "saves!"},
		{When: now.Add(-20 * time.Minute), Topic: TopicQuote, Subject: "Check, please!", Reply: "Check, please!"},
	}})
	b.SetQuotes(func() []string { return []string{"Check, please!"} })
	for i := 0; i < 100; i++ {
		if n, ok := b.ProactiveNudge(); ok {
			t.Fatalf("every quote is journaled: expected silence, got %+v", n)
		}
	}
}

func TestProactiveNudgeRealTopicsBeatQuotes(t *testing.T) {
	now := time.Now().UTC()
	saves := Section{Key: KeySaves, Title: "SAVE FILES", Body: "y", Freshest: now.Add(-1 * time.Hour)}
	b := nudgeBuilder(t, nil, saves)
	b.SetJournal(&Journal{})
	b.SetQuotes(func() []string { return []string{"Check, please!"} })
	for i := 0; i < 50; i++ {
		n, ok := b.ProactiveNudge()
		if !ok || n.Verbatim {
			t.Fatalf("real topic must always beat the quote fallback: ok=%v n=%+v", ok, n)
		}
	}
}
