# Emotional Talking Mouth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every animatable BMO emotion keep its eyes/brows and natural mouth at silence, and open the shared `speaking`-style teeth/tongue mouth while audio plays; also fix the hello clip's late mouth start.

**Architecture:** Each emotion SVG is a Go-template driven by amplitude param `$m`, sampled at 6 discrete steps (0, 0.2, 0.4, 0.6, 0.8, 1.0). Replace each emotion's `$m`-scaled mouth with a conditional ladder: at `$m == 0` draw the emotion's natural resting mouth; for higher buckets draw the exact `speaking_1..5` open-mouth markup (known-good on device, no arc-templating). No engine or routing changes. Issue #1 is a measure-then-fix on the amplitude path.

**Tech Stack:** Go, `text/template` SVG, oksvg rasterizer, SDL2 (CGO) for build/test.

---

## Background facts (verified against source — do not re-derive)

- `internal/face/anim_frames.go`: `renderAnimTemplate(data, param string, val float64)` always passes `$m` as **float64**; `buildFrames` computes `val = From + (To-From)*i/(n-1)` for `i` in `[0, n)`. With `From=0, To=1, Steps=6` the sampled values are `0, 0.2, 0.4, 0.6, 0.8, 1.0`. `renderRestSVG` executes with empty data so `{{$m := or .m 0.0}}` yields `0.0`.
- Go `text/template` has built-in `eq`, `lt`, `le`, `gt`, `ge`. Numeric literals with a decimal point (e.g. `0.3`) are `float64`, so `lt $m 0.3` compares float64 to float64. `eq $m 0.0` is exact (0.0 is representable).
- Driver (`internal/face/anim_driver.go`): `DriverAmplitude` with **no Idle** returns frame 0 at `signal <= 0` (silence → natural mouth) and maps `signal` through `sqrt` to a higher step as voice rises (talking → open mouth). The core-set emotion defs in `DefaultAnimations()` already use `Template{Param:"m", From:0, To:1, Steps:6}` + `DriverAmplitude{Curve:curveSqrt}` with no idle. **Unchanged by this plan.**
- The emotion's natural resting mouth is exactly what the current SVG renders at `$m = 0` today.
- The shared open mouth = the mouth markup from `speaking_1.svg` … `speaking_5.svg` (the dark rounded-rect interior + white teeth arc + green tongue interior, plus a green tongue-tip on frames 3–5). `speaking_0` (nearly closed) is intentionally NOT used — the first talking bucket jumps to `speaking_1` so the mouth visibly opens the instant audio starts.
- Tests run with `CGO_ENABLED=1 go test ./...`. Lint `golangci-lint run ./...`.
- `internal/face/anim_templates_test.go` already has `TestCoreTemplatesRenderRestAndOpen` (rest ≠ open per emotion) — keep it; it stays valid.
- Rest-fidelity: `TestEmbeddedFacesGeometry` samples `neutral`, `concerned`, `smile` at rest; the fidelity manifest covers the still-static Figma faces. Both are preserved because `$m = 0` art is unchanged.

## File Structure

| File | Change | Responsibility |
|------|--------|----------------|
| `internal/face/anim_templates_test.go` | Modify | Add `TestEmotionTalkingMouthOpensWithTeeth` (table-driven, per-emotion subtests). |
| `internal/face/assets/{neutral,happy,sad,content,concerned,angry}.svg` | Modify | Mouth → conditional ladder: natural line mouth at `$m=0`, shared open mouth otherwise. |
| `internal/face/assets/{smile,excited}.svg` | Modify | Mouth → conditional ladder: natural closed-lip teeth/tongue at `$m=0`, shared open mouth otherwise. |
| `cmd/bmo-pak/main.go` (+ `internal/clips/player.go` or `internal/assistant/voice.go`) | Modify | Issue #1: temporary amplitude instrumentation, then minimal responsiveness fix. |

---

## SHARED OPEN-MOUTH LADDER (verbatim — identical in every emotion file)

This is the `{{else …}}` portion of every emotion's mouth block. It is reproduced in full in each task below; it must be byte-identical across all 8 files.

```
  {{else if lt $m 0.3}}
  <rect x="106" y="106" width="68" height="12" rx="6" ry="6" fill="#1a1a1a"/>
  <path d="M 106.61 109.36 A 6.00 6.00 0 0 1 112.00 106.00 L 168.00 106.00 A 6.00 6.00 0 0 1 173.39 109.36 Z" fill="#e4e4e4"/>
  <path d="M 106.61 109.36 L 173.39 109.36 A 6.00 6.00 0 0 1 174.00 112.00 L 174.00 112.00 A 6.00 6.00 0 0 1 168.00 118.00 L 112.00 118.00 A 6.00 6.00 0 0 1 106.00 112.00 L 106.00 112.00 A 6.00 6.00 0 0 1 106.61 109.36 Z" fill="#1a7848"/>
  {{else if lt $m 0.5}}
  <rect x="106" y="106" width="68" height="18" rx="9" ry="9" fill="#1a1a1a"/>
  <path d="M 106.92 111.04 A 9.00 9.00 0 0 1 115.00 106.00 L 165.00 106.00 A 9.00 9.00 0 0 1 173.08 111.04 Z" fill="#e4e4e4"/>
  <path d="M 106.92 111.04 L 173.08 111.04 A 9.00 9.00 0 0 1 174.00 115.00 L 174.00 115.00 A 9.00 9.00 0 0 1 165.00 124.00 L 115.00 124.00 A 9.00 9.00 0 0 1 106.00 115.00 L 106.00 115.00 A 9.00 9.00 0 0 1 106.92 111.04 Z" fill="#1a7848"/>
  {{else if lt $m 0.7}}
  <rect x="106" y="106" width="68" height="24" rx="12" ry="12" fill="#1a1a1a"/>
  <path d="M 107.22 112.72 A 12.00 12.00 0 0 1 118.00 106.00 L 162.00 106.00 A 12.00 12.00 0 0 1 172.78 112.72 Z" fill="#e4e4e4"/>
  <path d="M 107.22 112.72 L 172.78 112.72 A 12.00 12.00 0 0 1 174.00 118.00 L 174.00 118.00 A 12.00 12.00 0 0 1 162.00 130.00 L 118.00 130.00 A 12.00 12.00 0 0 1 106.00 118.00 L 106.00 118.00 A 12.00 12.00 0 0 1 107.22 112.72 Z" fill="#1a7848"/>
  <path d="M 127.33 130.00 Q 140.00 121.36 152.67 130.00 Z" fill="#16ae81"/>
  {{else if lt $m 0.9}}
  <rect x="106" y="106" width="68" height="30" rx="15" ry="15" fill="#1a1a1a"/>
  <path d="M 107.53 114.40 A 15.00 15.00 0 0 1 121.00 106.00 L 159.00 106.00 A 15.00 15.00 0 0 1 172.47 114.40 Z" fill="#e4e4e4"/>
  <path d="M 107.53 114.40 L 172.47 114.40 A 15.00 15.00 0 0 1 174.00 121.00 L 174.00 121.00 A 15.00 15.00 0 0 1 159.00 136.00 L 121.00 136.00 A 15.00 15.00 0 0 1 106.00 121.00 L 106.00 121.00 A 15.00 15.00 0 0 1 107.53 114.40 Z" fill="#1a7848"/>
  <path d="M 124.17 136.00 Q 140.00 125.20 155.83 136.00 Z" fill="#16ae81"/>
  {{else}}
  <rect x="106" y="106" width="68" height="36" rx="16" ry="16" fill="#1a1a1a"/>
  <path d="M 107.14 116.08 A 16.00 16.00 0 0 1 122.00 106.00 L 158.00 106.00 A 16.00 16.00 0 0 1 172.86 116.08 Z" fill="#e4e4e4"/>
  <path d="M 107.14 116.08 L 172.86 116.08 A 16.00 16.00 0 0 1 174.00 122.00 L 174.00 126.00 A 16.00 16.00 0 0 1 158.00 142.00 L 122.00 142.00 A 16.00 16.00 0 0 1 106.00 126.00 L 106.00 122.00 A 16.00 16.00 0 0 1 107.14 116.08 Z" fill="#1a7848"/>
  <path d="M 121.00 142.00 Q 140.00 129.04 159.00 142.00 Z" fill="#16ae81"/>
  {{end}}
```

---

## Task 1: Failing per-emotion test

**Files:**
- Modify: `internal/face/anim_templates_test.go`

- [ ] **Step 1: Add the test**

Append to `internal/face/anim_templates_test.go`:

```go
func TestEmotionTalkingMouthOpensWithTeeth(t *testing.T) {
	lib := NewLibrary(t.TempDir()) // embedded assets only

	const (
		teeth  = 0xe4e4e4 // white teeth band
		tongue = 0x1a7848 // dark green mouth interior
	)
	has := func(buf []uint32, rgb uint32) bool {
		for _, px := range buf {
			if px&0x00ffffff == rgb {
				return true
			}
		}
		return false
	}

	// Emotions whose natural resting mouth is a thin line (no teeth at rest).
	lineMouth := map[string]bool{
		ExprNeutral: true, ExprHappy: true, ExprSad: true,
		ExprContent: true, ExprConcerned: true, ExprAngry: true,
	}

	for _, name := range []string{
		ExprNeutral, ExprHappy, ExprSmile, ExprExcited,
		ExprContent, ExprConcerned, ExprSad, ExprAngry,
	} {
		t.Run(name, func(t *testing.T) {
			data, ok := lib.rawBytes(name)
			if !ok {
				t.Fatalf("%s: no embedded bytes", name)
			}
			rest, err := Rasterize(renderRestSVG(data), 280, 210)
			if err != nil {
				t.Fatalf("%s: rest rasterize: %v", name, err)
			}
			openSVG, err := renderAnimTemplate(data, "m", 1)
			if err != nil {
				t.Fatalf("%s: render m=1: %v", name, err)
			}
			open, err := Rasterize(openSVG, 280, 210)
			if err != nil {
				t.Fatalf("%s: open rasterize: %v", name, err)
			}
			// Talking (m=1) must show the shared open teeth/tongue mouth.
			if !has(open, teeth) {
				t.Errorf("%s: open frame missing teeth (#e4e4e4)", name)
			}
			if !has(open, tongue) {
				t.Errorf("%s: open frame missing tongue interior (#1a7848)", name)
			}
			// Line-mouth emotions must show their natural mouth at rest, not teeth.
			if lineMouth[name] && has(rest, teeth) {
				t.Errorf("%s: teeth present at rest — natural mouth should show at silence", name)
			}
		})
	}
}
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestEmotionTalkingMouthOpensWithTeeth -v`
Expected: FAIL — current `happy/sad/content/concerned/angry` open frames have no teeth (line mouth just opens); `excited` already has teeth but the subtests for line emotions fail. Several subtests fail.

- [ ] **Step 3: Commit the failing test**

```bash
git add internal/face/anim_templates_test.go
git commit -m "test(face): assert emotional talking opens the shared teeth/tongue mouth"
```

---

## Task 2: Rewrite the six line-mouth emotions

**Files:**
- Modify: `internal/face/assets/neutral.svg`, `happy.svg`, `sad.svg`, `content.svg`, `concerned.svg`, `angry.svg`

For each file, replace the single `$m`-driven mouth `<path>` with a conditional block: `{{if eq $m 0.0}}` + the natural mouth (the exact line below) + the SHARED OPEN-MOUTH LADDER. Eyes/brows/background are unchanged.

- [ ] **Step 1: Write `internal/face/assets/neutral.svg`**

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80"  cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  {{if eq $m 0.0}}
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
  {{else if lt $m 0.3}}
  <rect x="106" y="106" width="68" height="12" rx="6" ry="6" fill="#1a1a1a"/>
  <path d="M 106.61 109.36 A 6.00 6.00 0 0 1 112.00 106.00 L 168.00 106.00 A 6.00 6.00 0 0 1 173.39 109.36 Z" fill="#e4e4e4"/>
  <path d="M 106.61 109.36 L 173.39 109.36 A 6.00 6.00 0 0 1 174.00 112.00 L 174.00 112.00 A 6.00 6.00 0 0 1 168.00 118.00 L 112.00 118.00 A 6.00 6.00 0 0 1 106.00 112.00 L 106.00 112.00 A 6.00 6.00 0 0 1 106.61 109.36 Z" fill="#1a7848"/>
  {{else if lt $m 0.5}}
  <rect x="106" y="106" width="68" height="18" rx="9" ry="9" fill="#1a1a1a"/>
  <path d="M 106.92 111.04 A 9.00 9.00 0 0 1 115.00 106.00 L 165.00 106.00 A 9.00 9.00 0 0 1 173.08 111.04 Z" fill="#e4e4e4"/>
  <path d="M 106.92 111.04 L 173.08 111.04 A 9.00 9.00 0 0 1 174.00 115.00 L 174.00 115.00 A 9.00 9.00 0 0 1 165.00 124.00 L 115.00 124.00 A 9.00 9.00 0 0 1 106.00 115.00 L 106.00 115.00 A 9.00 9.00 0 0 1 106.92 111.04 Z" fill="#1a7848"/>
  {{else if lt $m 0.7}}
  <rect x="106" y="106" width="68" height="24" rx="12" ry="12" fill="#1a1a1a"/>
  <path d="M 107.22 112.72 A 12.00 12.00 0 0 1 118.00 106.00 L 162.00 106.00 A 12.00 12.00 0 0 1 172.78 112.72 Z" fill="#e4e4e4"/>
  <path d="M 107.22 112.72 L 172.78 112.72 A 12.00 12.00 0 0 1 174.00 118.00 L 174.00 118.00 A 12.00 12.00 0 0 1 162.00 130.00 L 118.00 130.00 A 12.00 12.00 0 0 1 106.00 118.00 L 106.00 118.00 A 12.00 12.00 0 0 1 107.22 112.72 Z" fill="#1a7848"/>
  <path d="M 127.33 130.00 Q 140.00 121.36 152.67 130.00 Z" fill="#16ae81"/>
  {{else if lt $m 0.9}}
  <rect x="106" y="106" width="68" height="30" rx="15" ry="15" fill="#1a1a1a"/>
  <path d="M 107.53 114.40 A 15.00 15.00 0 0 1 121.00 106.00 L 159.00 106.00 A 15.00 15.00 0 0 1 172.47 114.40 Z" fill="#e4e4e4"/>
  <path d="M 107.53 114.40 L 172.47 114.40 A 15.00 15.00 0 0 1 174.00 121.00 L 174.00 121.00 A 15.00 15.00 0 0 1 159.00 136.00 L 121.00 136.00 A 15.00 15.00 0 0 1 106.00 121.00 L 106.00 121.00 A 15.00 15.00 0 0 1 107.53 114.40 Z" fill="#1a7848"/>
  <path d="M 124.17 136.00 Q 140.00 125.20 155.83 136.00 Z" fill="#16ae81"/>
  {{else}}
  <rect x="106" y="106" width="68" height="36" rx="16" ry="16" fill="#1a1a1a"/>
  <path d="M 107.14 116.08 A 16.00 16.00 0 0 1 122.00 106.00 L 158.00 106.00 A 16.00 16.00 0 0 1 172.86 116.08 Z" fill="#e4e4e4"/>
  <path d="M 107.14 116.08 L 172.86 116.08 A 16.00 16.00 0 0 1 174.00 122.00 L 174.00 126.00 A 16.00 16.00 0 0 1 158.00 142.00 L 122.00 142.00 A 16.00 16.00 0 0 1 106.00 126.00 L 106.00 122.00 A 16.00 16.00 0 0 1 107.14 116.08 Z" fill="#1a7848"/>
  <path d="M 121.00 142.00 Q 140.00 129.04 159.00 142.00 Z" fill="#16ae81"/>
  {{end}}
</svg>
```

- [ ] **Step 2: Write the other five.** Each is identical in structure — same `{{if eq $m 0.0}} … {{end}}` ladder (the SHARED OPEN-MOUTH LADDER above), differing only in eyes/brows and the natural-mouth line. Use these heads + natural mouths, then paste the SHARED OPEN-MOUTH LADDER (the `{{else if lt $m 0.3}} … {{end}}` block) verbatim before `</svg>`:

`happy.svg` — eyes + natural mouth:
```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  {{if eq $m 0.0}}
  <path d="M 108 111 Q 140 134 172 111" stroke="#1a1a1a" stroke-width="5" fill="none" stroke-linecap="round"/>
  <<<SHARED OPEN-MOUTH LADDER>>>
</svg>
```

`sad.svg` — natural mouth `<path d="M 116 113 Q 140 96 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>`, eyes `<circle cx="80" cy="78" r="6.5"/>` + `<circle cx="199" cy="78" r="6.5"/>` (both `fill="#1a1a1a"`).

`content.svg` — no eye circles; closed-eye arcs + natural mouth:
```
  <path d="M 61 73 Q 80 88 99 73" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 73 Q 199 88 218 73" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  {{if eq $m 0.0}}
  <path d="M 116 111 Q 140 125 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
```

`concerned.svg` — eyes `cy="82"`, worried brows, natural mouth `M 116 111 Q 140 97 160 111`:
```
  <circle cx="80"  cy="82" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="82" r="6.5" fill="#1a1a1a"/>
  <path d="M 65 54 L 96 70"   stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 70 L 214 54" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  {{if eq $m 0.0}}
  <path d="M 116 111 Q 140 97 160 111" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
```

`angry.svg` — angry brows, eyes `cy="78"`, natural frown `M 116 113 Q 140 99 160 113`:
```
  <path d="M 60 60 L 96 74" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <path d="M 183 74 L 219 60" stroke="#1a1a1a" stroke-width="5" stroke-linecap="round"/>
  <circle cx="80" cy="78" r="6.5" fill="#1a1a1a"/>
  <circle cx="199" cy="78" r="6.5" fill="#1a1a1a"/>
  {{if eq $m 0.0}}
  <path d="M 116 113 Q 140 99 160 113" stroke="#1a1a1a" stroke-width="4" fill="none" stroke-linecap="round"/>
```

> Replace `<<<SHARED OPEN-MOUTH LADDER>>>` with the full `{{else if lt $m 0.3}} … {{end}}` block from the "SHARED OPEN-MOUTH LADDER" section. All six files end with that identical block then `</svg>`.

- [ ] **Step 3: Run the test — expect PASS for these six subtests**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run 'TestEmotionTalkingMouthOpensWithTeeth/(neutral|happy|sad|content|concerned|angry)' -v`
Expected: PASS.

- [ ] **Step 4: Confirm rest-fidelity holds**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run 'TestEmbeddedFacesGeometry|TestCoreTemplatesRenderRestAndOpen|TestNewExpressionFidelity'`
Expected: PASS (m=0 art for neutral/concerned unchanged).

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./...
git add internal/face/assets/neutral.svg internal/face/assets/happy.svg internal/face/assets/sad.svg internal/face/assets/content.svg internal/face/assets/concerned.svg internal/face/assets/angry.svg
git commit -m "face: line-mouth emotions open the shared talking mouth when speaking"
```

---

## Task 3: Rewrite smile and excited (teeth/tongue at rest)

**Files:**
- Modify: `internal/face/assets/smile.svg`, `internal/face/assets/excited.svg`

These two already show a closed-lip teeth/tongue mouth at rest. Keep that as the `{{if eq $m 0.0}}` natural branch (drop the old `$m`-driven gap path), then append the SHARED OPEN-MOUTH LADDER.

- [ ] **Step 1: Write `internal/face/assets/smile.svg`**

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 61 85 Q 80 70 99 85"    stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  <path d="M 180 85 Q 199 70 218 85" stroke="#1a1a1a" stroke-width="7" fill="none" stroke-linecap="round"/>
  {{if eq $m 0.0}}
  <rect x="98" y="101" width="84" height="43" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 113 A 20 20 0 0 1 118 101 L 162 101 A 20 20 0 0 1 180.33 113 Z" fill="#e4e4e4"/>
  <path d="M 99.67 113 L 180.33 113 A 20 20 0 0 1 182 121 L 182 124 A 20 20 0 0 1 162 144 L 118 144 A 20 20 0 0 1 98 124 L 98 121 A 20 20 0 0 1 99.67 113 Z" fill="#1a7848"/>
  <path d="M 116 144 A 24 8 0 0 1 164 144 Z" fill="#16ae81"/>
  <<<SHARED OPEN-MOUTH LADDER>>>
</svg>
```

- [ ] **Step 2: Write `internal/face/assets/excited.svg`** — identical body but star eyes instead of the smile arcs:

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 280 210">{{$m := or .m 0.0}}
  <rect x="0" y="0" width="280" height="210" fill="#4ECBA8"/>
  <rect x="6" y="5" width="268" height="200" rx="26" ry="26" fill="#4ECBA8"/>
  <rect x="12" y="10" width="256" height="188" rx="12" ry="12" fill="#90e5c8"/>
  <path d="M 80.0 65.0 L 83.1 73.8 L 92.4 74.0 L 84.9 79.6 L 87.6 88.5 L 80.0 83.2 L 72.4 88.5 L 75.1 79.6 L 67.6 74.0 L 76.9 73.8 Z" fill="#f4c531"/>
  <path d="M 199.0 65.0 L 202.1 73.8 L 211.4 74.0 L 203.9 79.6 L 206.6 88.5 L 199.0 83.2 L 191.4 88.5 L 194.1 79.6 L 186.6 74.0 L 195.9 73.8 Z" fill="#f4c531"/>
  {{if eq $m 0.0}}
  <rect x="98" y="101" width="84" height="43" rx="20" ry="20" fill="#1a1a1a"/>
  <path d="M 99.67 113 A 20 20 0 0 1 118 101 L 162 101 A 20 20 0 0 1 180.33 113 Z" fill="#e4e4e4"/>
  <path d="M 99.67 113 L 180.33 113 A 20 20 0 0 1 182 121 L 182 124 A 20 20 0 0 1 162 144 L 118 144 A 20 20 0 0 1 98 124 L 98 121 A 20 20 0 0 1 99.67 113 Z" fill="#1a7848"/>
  <path d="M 116 144 A 24 8 0 0 1 164 144 Z" fill="#16ae81"/>
  <<<SHARED OPEN-MOUTH LADDER>>>
</svg>
```

> Replace `<<<SHARED OPEN-MOUTH LADDER>>>` with the full `{{else if lt $m 0.3}} … {{end}}` block verbatim, then `</svg>`.

- [ ] **Step 3: Run the test — all 8 subtests pass**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestEmotionTalkingMouthOpensWithTeeth -v`
Expected: PASS for all 8 (smile/excited keep teeth at rest, which is allowed — they are not in the `lineMouth` set).

- [ ] **Step 4: Confirm smile rest geometry unchanged**

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestEmbeddedFacesGeometry -v`
Expected: PASS — smile's sampled rest points (teeth/interior/tongue) are unchanged; only the removed flat gap path (which drew nothing at the sampled points) is gone.

- [ ] **Step 5: Lint + commit**

```bash
golangci-lint run ./...
git add internal/face/assets/smile.svg internal/face/assets/excited.svg
git commit -m "face: smile/excited open the shared talking mouth; drop the gap overlay"
```

---

## Task 4: Full verification of the core model

**Files:** none (verification only)

- [ ] **Step 1: Full suite**

Run: `CGO_ENABLED=1 go test ./...`
Expected: ALL PASS.

- [ ] **Step 2: Race + lint**

Run: `CGO_ENABLED=1 go test -race ./internal/face/` then `golangci-lint run ./...`
Expected: ok, 0 issues.

- [ ] **Step 3: Build**

Run: `CGO_ENABLED=1 go build ./...`
Expected: exit 0.

No commit (verification only).

---

## Task 5: Fix the late mouth start on the hello clip (issue #1)

**Files:**
- Modify: `cmd/bmo-pak/main.go` (instrumentation, then fix)
- Possibly modify: `internal/clips/player.go` or `internal/face/anim_driver.go` (depending on measured cause)

This is a measure-then-fix task: the cause (low-amplitude mapping vs. amplitude latency) must be confirmed before choosing the fix.

- [ ] **Step 1: Add temporary instrumentation**

In `cmd/bmo-pak/main.go`, inside the render loop right after `speakAmp` is computed (the `var speakAmp float32 … } ` block), add:

```go
		if clipPlaying {
			logger.Debugf("clipamp: amp=%.3f speakingReady=%t", speakAmp, animEngine.Ready(face.ExprSpeaking))
		}
```

- [ ] **Step 2: Build, deploy, capture**

```bash
./scripts/release.sh && ./scripts/deploy.sh
```
Ensure device `log_level` is `debug`. Launch BMO, let the hello clip play, then pull the log:
```bash
adb pull /mnt/SDCARD/.userdata/tg5040/logs/BMO.txt /tmp/bmo-hello.txt
grep clipamp /tmp/bmo-hello.txt | head -40
```

- [ ] **Step 3: Decide the fix from the evidence**

- If the first ~0.3–0.5s of `clipamp` lines show `amp≈0.00x` (tiny but rising) while `speakingReady=true` → **cause (a): low amplitude maps to a near-closed frame.** Apply a gain/floor (Step 4a).
- If `clipamp` shows `amp` already healthy (>0.1) from the first line but the mouth still looked late, or `speakingReady=false` for the first lines → **cause (b): readiness/latency.** Apply the matching fix (Step 4b).

- [ ] **Step 4a: Low-amplitude gain (most likely)**

In `internal/face/anim_driver.go`, in `Step`'s `DriverAmplitude` branch, boost the signal before the curve so speech onset opens the mouth. Replace:

```go
		v := float64(signal)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		if d.Curve == curveSqrt {
			v = math.Sqrt(v)
		}
		return clampStep(int(v*float64(steps-1)+0.5), steps)
```

with a gained version (1.8× gain, clamped) so quiet speech reaches at least the first open step:

```go
		v := float64(signal)
		if v < 0 {
			v = 0
		}
		v *= amplitudeGain
		if v > 1 {
			v = 1
		}
		if d.Curve == curveSqrt {
			v = math.Sqrt(v)
		}
		return clampStep(int(v*float64(steps-1)+0.5), steps)
```

and add near the top of `internal/face/anim_driver.go`:

```go
// amplitudeGain scales the raw voice amplitude before the response curve so
// quiet speech still opens the mouth promptly instead of sitting near-closed.
const amplitudeGain = 1.8
```

Add a test in `internal/face/anim_driver_test.go`:

```go
func TestAmplitudeGainOpensOnQuietSpeech(t *testing.T) {
	d := Driver{Kind: DriverAmplitude, Curve: curveSqrt}
	// A quiet-but-present signal must reach at least the first open step.
	if got := d.Step(0, 0, 0.06, 6); got < 1 {
		t.Fatalf("quiet speech step = %d, want >= 1", got)
	}
	// Silence still rests at frame 0.
	if got := d.Step(0, 0, 0, 6); got != 0 {
		t.Fatalf("silence step = %d, want 0", got)
	}
}
```

Run: `CGO_ENABLED=1 go test ./internal/face/ -run TestAmplitudeGain -v` → PASS.

- [ ] **Step 4b: Readiness/latency (only if Step 3 indicates cause b)**

If `speakingReady=false` during the late window, prewarm earlier / widen the readiness wait already gating the startup clip (`animEngine.Ready(face.ExprSpeaking)` at the startup-clip gate). If amplitude latency, align `clips.Player`'s amplitude store to the played (not written) chunk. Pick the minimal change indicated and add a focused regression test mirroring the measured gap.

- [ ] **Step 5: Remove instrumentation**

Delete the `clipamp` `logger.Debugf` line added in Step 1.

- [ ] **Step 6: Verify + commit**

```bash
CGO_ENABLED=1 go test ./...
golangci-lint run ./...
git add -A
git commit -m "fix(face): open the talking mouth promptly on quiet speech onset"
```

- [ ] **Step 7: Build + deploy + on-device confirmation**

```bash
./scripts/release.sh && ./scripts/deploy.sh
```
Confirm on device: the hello clip's mouth moves from the first audible word; excited/angry/other emotions talk with the open teeth/tongue mouth and rest with their natural mouth.

---

## Self-Review

- [ ] **Spec coverage:** Core model (all 8 emotions, natural at rest / shared open mouth talking) → Tasks 2–3. Shared open mouth from speaking geometry → SHARED OPEN-MOUTH LADDER. Responsiveness fix (#1) → Task 5. Testing (teeth/tongue at open, no teeth at rest for line emotions, rest-fidelity, regression guard) → Task 1 + Task 4. ✔
- [ ] **Placeholder scan:** The `<<<SHARED OPEN-MOUTH LADDER>>>` marker is defined verbatim once and explicitly reproduced into each file; not a TBD. Task 5 enumerates concrete branches (4a/4b) gated by a measurement, with full code for the likely branch. ✔
- [ ] **Type/consistency:** `$m` is float64 throughout; thresholds use float literals (`0.0`, `0.3`, `0.5`, `0.7`, `0.9`). Teeth `#e4e4e4`, tongue `#1a7848`, tongue-tip `#16ae81` consistent across ladder and tests. `amplitudeGain` defined once. ✔
- [ ] No `Co-Authored-By` trailer in any commit.
- [ ] `CGO_ENABLED=1 go test ./...` and `golangci-lint run ./...` clean at every commit.
