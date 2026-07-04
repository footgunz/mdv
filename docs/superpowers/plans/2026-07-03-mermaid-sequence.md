# Native Mermaid Sequence Diagrams Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Native SVG rendering for the common sequence-diagram subset in the `mermaid/` package — participants, six arrow types, notes, activations, nested loop/alt/opt/par frames, autonumber — with everything else still falling back to mermaid.js.

**Architecture:** Pure-Go deterministic pipeline mirroring the flowchart one: `parseSequence` (line-based, frame stack) → `layoutSequence` (horizontal lifeline spacing pass + vertical recursive item walk, sizes from the existing `measureText`) → `emitSequence` (same SVG conventions, themes, escaping). `Render` gains a `case "sequenceDiagram"`. No dagre/goja involvement; `render.go`/`server.go` untouched except viewer tests flipping expectations.

**Tech Stack:** Go stdlib + existing package infra only. No new dependencies.

## Global Constraints

- No engine error may ever break a page: every failure returns an error; callers fall back to mermaid.js. No path may panic on user input.
- Unsupported/malformed syntax → `ErrUnsupported`-wrapped error, never a silent misrender.
- Reuse existing infra: `measureText`, `Theme`/`Light`/`Dark`, `html.EscapeString` on ALL user-sourced text, fixed-precision floats (`%.1f`) for deterministic goldens, root `<svg>` conventions (`xmlns`, width/height, viewBox, `class="mermaid-svg"`, font attrs).
- Concurrent `Render` must stay safe — sequence code must not add package-level mutable state (`measureText` is already lock-guarded).
- Branch: `mermaid-sequence` (already created). Module `github.com/dgunther/mdthing`, package `mermaid/`.

**If plan code conflicts with plan tests, the tests govern** — fix the implementation, keep the test, note the deviation in your report.

---

### Task 1: Sequence IR and Render dispatch

**Files:**
- Create: `mermaid/seq.go`
- Modify: `mermaid/mermaid.go` (add `case "sequenceDiagram"`)
- Test: `mermaid/seq_test.go`

**Interfaces:**
- Consumes: `Render`, `detect`, `ErrUnsupported`, `Theme` (existing).
- Produces (relied on by Tasks 2–5):
  - `type SeqDiagram struct { Participants []*Participant; Items []SeqItem; Autonumber bool; Activations []SeqActivation; Width, Height float64 }`
  - `type Participant struct { ID, Label string; Actor bool; X, W, H float64 }` — X = lifeline center (layout).
  - `type SeqItem interface{ seqItem() }` implemented by `*SeqMessage`, `*SeqNote`, `*SeqFrame`, `*SeqActivate`.
  - `type HeadStyle int` — `HeadArrow, HeadCross, HeadNone`.
  - `type NotePos int` — `NoteLeft, NoteRight, NoteOver`.
  - `type SeqMessage struct { From, To, Text string; Dashed bool; Head HeadStyle; ActivateTo, DeactivateFrom bool; Y float64; Num int }`
  - `type SeqNote struct { Pos NotePos; A, B, Text string; X, Y, W, H float64 }`
  - `type SeqFrame struct { Kind string; Sections []SeqSection; X, Y, W, H float64; DividerYs []float64 }`; `type SeqSection struct { Label string; Items []SeqItem }`
  - `type SeqActivate struct { P string; On bool }` — the standalone `activate`/`deactivate` statement.
  - `type SeqActivation struct { P *Participant; Level int; Y0, Y1 float64 }` — computed rectangles.
  - Stubs (replaced by Tasks 2–4): `parseSequence(src string) (*SeqDiagram, error)`, `layoutSequence(d *SeqDiagram, t Theme) error`, `emitSequence(d *SeqDiagram, t Theme) []byte`.

- [ ] **Step 1: Write the failing test**

Create `mermaid/seq_test.go`:

```go
package mermaid

import (
	"errors"
	"strings"
	"testing"
)

// mustParseSeq is shared by Tasks 2-4 tests.
func mustParseSeq(t *testing.T, src string) *SeqDiagram {
	t.Helper()
	d, err := parseSequence(src)
	if err != nil {
		t.Fatalf("parseSequence: %v", err)
	}
	return d
}

func TestRenderDispatchesSequence(t *testing.T) {
	// While parseSequence is a stub this must still be ErrUnsupported;
	// after Task 2 lands the error goes away and the second assertion
	// (still-unsupported kind) keeps guarding dispatch.
	_, err := Render([]byte("gantt\ntitle X"), Light)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("gantt must stay unsupported, got %v", err)
	}
	out, err := Render([]byte("sequenceDiagram\nA->>B: hi"), Light)
	if err != nil {
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("sequence dispatch must route to sequence pipeline: %v", err)
		}
		t.Skip("sequence pipeline still stubbed (pre-Task 4)")
	}
	if !strings.Contains(string(out), "<svg") {
		t.Fatalf("expected svg, got: %.80s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./mermaid/ -run TestRenderDispatchesSequence`
Expected: FAIL — `undefined: parseSequence` (compile error is the expected failure at this stage).

- [ ] **Step 3: Implement `mermaid/seq.go`**

```go
package mermaid

// Sequence-diagram IR. parseSequence fills identity/text and the item tree;
// layoutSequence fills all geometry (X/Y/W/H, activations, autonumbers);
// emitSequence renders it.

type HeadStyle int

const (
	HeadArrow HeadStyle = iota
	HeadCross
	HeadNone
)

type NotePos int

const (
	NoteLeft NotePos = iota
	NoteRight
	NoteOver
)

type Participant struct {
	ID, Label string
	Actor     bool
	X, W, H   float64 // X = lifeline center, set by layout
}

type SeqItem interface{ seqItem() }

type SeqMessage struct {
	From, To, Text             string
	Dashed                     bool
	Head                       HeadStyle
	ActivateTo, DeactivateFrom bool
	Y                          float64
	Num                        int // 1-based when Autonumber, else 0
}

type SeqNote struct {
	Pos        NotePos
	A, B       string // B set only for "over A,B"
	Text       string
	X, Y, W, H float64
}

type SeqSection struct {
	Label string
	Items []SeqItem
}

type SeqFrame struct {
	Kind       string // loop, opt, alt, par
	Sections   []SeqSection
	X, Y, W, H float64
	DividerYs  []float64 // one per section boundary (len = len(Sections)-1)
}

type SeqActivate struct {
	P  string
	On bool // true = activate, false = deactivate
}

type SeqActivation struct {
	P      *Participant
	Level  int // nesting depth, 0-based
	Y0, Y1 float64
}

type SeqDiagram struct {
	Participants  []*Participant
	Items         []SeqItem
	Autonumber    bool
	Activations   []SeqActivation
	Width, Height float64
}

func (*SeqMessage) seqItem()  {}
func (*SeqNote) seqItem()     {}
func (*SeqFrame) seqItem()    {}
func (*SeqActivate) seqItem() {}

func (d *SeqDiagram) participant(id string) *Participant {
	for _, p := range d.Participants {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Stubs replaced by Tasks 2 (seq_parse.go), 3 (seq_layout.go), 4 (seq_svg.go).
func parseSequence(src string) (*SeqDiagram, error)     { return nil, ErrUnsupported }
func layoutSequence(d *SeqDiagram, t Theme) error       { return ErrUnsupported }
func emitSequence(d *SeqDiagram, t Theme) []byte        { return nil }
```

In `mermaid/mermaid.go`, add the dispatch case to `Render`:

```go
	case "sequenceDiagram":
		d, err := parseSequence(rest)
		if err != nil {
			return nil, err
		}
		if err := layoutSequence(d, theme); err != nil {
			return nil, err
		}
		return emitSequence(d, theme), nil
```

Note: `Render`'s theme parameter is named `theme` in the existing signature — match whatever the local name is.

- [ ] **Step 4: Run tests**

Run: `go test ./mermaid/`
Expected: PASS — dispatch test hits the stub's `ErrUnsupported` and skips; all existing tests green (`TestRenderUnsupportedKind` still passes because the stub errors).

- [ ] **Step 5: Commit**

```bash
git add mermaid/seq.go mermaid/seq_test.go mermaid/mermaid.go
git commit -m "Add sequence diagram IR and Render dispatch"
```

---

### Task 2: Sequence parser

**Files:**
- Create: `mermaid/seq_parse.go` (delete the `parseSequence` stub from `seq.go`)
- Test: append to `mermaid/seq_test.go`

**Interfaces:**
- Consumes: IR from Task 1, `ErrUnsupported`, `unsup` helper (exists in parse.go).
- Produces: `parseSequence(src string) (*SeqDiagram, error)` — src starts at the `sequenceDiagram` header line.

Parsing rules:
- Only lines STARTING with `%%` are comments (do NOT strip inline `%%` — message text may contain it; this deliberately differs from the flowchart parser).
- Participants: explicit `participant id [as Label]` / `actor id [as Label]`; implicit creation on first use in a MESSAGE (label = id). Notes and activate/deactivate referencing unknown participants ERROR (mermaid does not implicitly create there).
- Re-declaring an existing participant errors (`unsup`).
- Arrow alternation order matters: `-->>` `--x` `-->` `->>` `-x` `->`.
- `autonumber` bare only; `autonumber <anything>` → error.
- Frames: `loop|opt|alt|par [label]` push; `else [label]` only in `alt`, `and [label]` only in `par`; `end` pops; EOF with open frame errors.
- Anything unrecognized → error.

- [ ] **Step 1: Write the failing tests**

Append to `mermaid/seq_test.go`:

```go
func TestSeqParseParticipants(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
participant a
participant b as Bob Service
actor u as User
a->>x: implicit`)
	if len(d.Participants) != 4 {
		t.Fatalf("participants %d, want 4", len(d.Participants))
	}
	order := []string{"a", "b", "u", "x"}
	for i, id := range order {
		if d.Participants[i].ID != id {
			t.Fatalf("order[%d] = %s, want %s", i, d.Participants[i].ID, id)
		}
	}
	if d.participant("b").Label != "Bob Service" {
		t.Fatalf("alias label: %+v", d.participant("b"))
	}
	if !d.participant("u").Actor {
		t.Fatal("actor flag lost")
	}
	if d.participant("x").Label != "x" {
		t.Fatal("implicit participant label must default to id")
	}
}

func TestSeqParseArrows(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
a->>b: solid arrow
a-->>b: dashed arrow
a->b: solid open
a-->b: dashed open
a-xb: solid cross
a--xb: dashed cross`)
	type want struct {
		dashed bool
		head   HeadStyle
	}
	wants := []want{
		{false, HeadArrow}, {true, HeadArrow},
		{false, HeadNone}, {true, HeadNone},
		{false, HeadCross}, {true, HeadCross},
	}
	if len(d.Items) != len(wants) {
		t.Fatalf("items %d, want %d", len(d.Items), len(wants))
	}
	for i, w := range wants {
		m := d.Items[i].(*SeqMessage)
		if m.Dashed != w.dashed || m.Head != w.head || m.From != "a" || m.To != "b" {
			t.Fatalf("msg %d = %+v, want %+v", i, m, w)
		}
	}
	if d.Items[0].(*SeqMessage).Text != "solid arrow" {
		t.Fatalf("text: %q", d.Items[0].(*SeqMessage).Text)
	}
}

func TestSeqParseActivations(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
a->>+b: on
b-->>-a: off
activate a
deactivate a`)
	m0 := d.Items[0].(*SeqMessage)
	m1 := d.Items[1].(*SeqMessage)
	if !m0.ActivateTo || m0.DeactivateFrom {
		t.Fatalf("m0 = %+v", m0)
	}
	if !m1.DeactivateFrom || m1.ActivateTo {
		t.Fatalf("m1 = %+v", m1)
	}
	if a := d.Items[2].(*SeqActivate); a.P != "a" || !a.On {
		t.Fatalf("activate stmt = %+v", a)
	}
	if a := d.Items[3].(*SeqActivate); a.P != "a" || a.On {
		t.Fatalf("deactivate stmt = %+v", a)
	}
}

func TestSeqParseNotes(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
participant a
participant b
Note left of a: L
note right of b: R
Note over a: O1
Note over a,b: O2`)
	wants := []struct {
		pos  NotePos
		a, b string
	}{
		{NoteLeft, "a", ""}, {NoteRight, "b", ""}, {NoteOver, "a", ""}, {NoteOver, "a", "b"},
	}
	for i, w := range wants {
		n := d.Items[i].(*SeqNote)
		if n.Pos != w.pos || n.A != w.a || n.B != w.b {
			t.Fatalf("note %d = %+v, want %+v", i, n, w)
		}
	}
	if d.Items[3].(*SeqNote).Text != "O2" {
		t.Fatalf("text: %+v", d.Items[3])
	}
}

func TestSeqParseFrames(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
loop every minute
  a->>b: ping
end
alt success
  a->>b: yes
else failure
  a->>b: no
end
par first
  a->>b: one
and second
  a->>b: two
end`)
	if len(d.Items) != 3 {
		t.Fatalf("items %d, want 3 frames", len(d.Items))
	}
	loop := d.Items[0].(*SeqFrame)
	if loop.Kind != "loop" || loop.Sections[0].Label != "every minute" || len(loop.Sections) != 1 {
		t.Fatalf("loop = %+v", loop)
	}
	alt := d.Items[1].(*SeqFrame)
	if alt.Kind != "alt" || len(alt.Sections) != 2 || alt.Sections[1].Label != "failure" {
		t.Fatalf("alt = %+v", alt)
	}
	par := d.Items[2].(*SeqFrame)
	if par.Kind != "par" || len(par.Sections) != 2 || par.Sections[1].Label != "second" {
		t.Fatalf("par = %+v", par)
	}
	if len(loop.Sections[0].Items) != 1 {
		t.Fatalf("loop contents: %+v", loop.Sections[0].Items)
	}
}

func TestSeqParseNestedFrames(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
loop outer
  opt inner
    a->>b: deep
  end
  a->>b: shallow
end`)
	outer := d.Items[0].(*SeqFrame)
	inner := outer.Sections[0].Items[0].(*SeqFrame)
	if inner.Kind != "opt" || inner.Sections[0].Label != "inner" {
		t.Fatalf("inner = %+v", inner)
	}
	if _, ok := outer.Sections[0].Items[1].(*SeqMessage); !ok {
		t.Fatalf("sibling after nested frame lost: %+v", outer.Sections[0].Items)
	}
}

func TestSeqParseAutonumberAndComments(t *testing.T) {
	d := mustParseSeq(t, `sequenceDiagram
autonumber
%% a comment line
a->>b: text with %% inside kept`)
	if !d.Autonumber {
		t.Fatal("autonumber not set")
	}
	if m := d.Items[0].(*SeqMessage); m.Text != "text with %% inside kept" {
		t.Fatalf("inline %%%% must be preserved: %q", m.Text)
	}
}

func TestSeqParseUnsupported(t *testing.T) {
	for _, src := range []string{
		"sequenceDiagram\ncritical x\na->>b: y\nend",
		"sequenceDiagram\nbox Group\nparticipant a\nend",
		"sequenceDiagram\ncreate participant a",
		"sequenceDiagram\nautonumber 10",
		"sequenceDiagram\nelse orphan",
		"sequenceDiagram\nend",
		"sequenceDiagram\nloop forever\na->>b: x",
		"sequenceDiagram\nNote over ghost: no such participant",
		"sequenceDiagram\nactivate ghost",
		"sequenceDiagram\nparticipant a\nparticipant a",
		"sequenceDiagram\nalt x\na->>b: y\nand wrong divider\nend",
		"sequenceDiagram\nwhat is this",
	} {
		if _, err := parseSequence(src); err == nil {
			t.Fatalf("want error for %q", src)
		} else if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: want ErrUnsupported, got %v", src, err)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./mermaid/ -run TestSeqParse`
Expected: FAIL — stub returns `ErrUnsupported` for everything.

- [ ] **Step 3: Implement `mermaid/seq_parse.go`** (delete the stub in seq.go)

```go
package mermaid

import (
	"regexp"
	"strings"
)

var (
	seqPartRe  = regexp.MustCompile(`^(participant|actor)\s+([A-Za-z0-9_.-]+)(?:\s+as\s+(.+))?$`)
	seqMsgRe   = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*(-->>|--x|-->|->>|-x|->)\s*([+-]?)\s*([A-Za-z0-9_.-]+)\s*:\s*(.*)$`)
	seqNoteRe  = regexp.MustCompile(`(?i)^note\s+(left of|right of|over)\s+([A-Za-z0-9_.-]+)(?:\s*,\s*([A-Za-z0-9_.-]+))?\s*:\s*(.*)$`)
	seqActRe   = regexp.MustCompile(`^(activate|deactivate)\s+([A-Za-z0-9_.-]+)$`)
	seqFrameRe = regexp.MustCompile(`^(loop|opt|alt|par)(?:\s+(.*))?$`)
	seqDivRe   = regexp.MustCompile(`^(else|and)(?:\s+(.*))?$`)
)

func parseSequence(src string) (*SeqDiagram, error) {
	d := &SeqDiagram{}
	var stack []*SeqFrame

	curItems := func() *[]SeqItem {
		if len(stack) == 0 {
			return &d.Items
		}
		f := stack[len(stack)-1]
		return &f.Sections[len(f.Sections)-1].Items
	}
	ensure := func(id, label string, actor, explicit bool) error {
		if p := d.participant(id); p != nil {
			if explicit {
				return unsup("participant %q redeclared", id)
			}
			return nil
		}
		if label == "" {
			label = id
		}
		d.Participants = append(d.Participants, &Participant{ID: id, Label: label, Actor: actor})
		return nil
	}

	seenHeader := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !seenHeader {
			if line != "sequenceDiagram" {
				return nil, unsup("bad header %q", line)
			}
			seenHeader = true
			continue
		}

		switch {
		case line == "autonumber":
			d.Autonumber = true

		case strings.HasPrefix(line, "autonumber "):
			return nil, unsup("autonumber arguments %q", line)

		case seqPartRe.MatchString(line):
			m := seqPartRe.FindStringSubmatch(line)
			if err := ensure(m[2], strings.TrimSpace(m[3]), m[1] == "actor", true); err != nil {
				return nil, err
			}

		case seqMsgRe.MatchString(line):
			m := seqMsgRe.FindStringSubmatch(line)
			if err := ensure(m[1], "", false, false); err != nil {
				return nil, err
			}
			if err := ensure(m[4], "", false, false); err != nil {
				return nil, err
			}
			msg := &SeqMessage{From: m[1], To: m[4], Text: strings.TrimSpace(m[5])}
			msg.Dashed = strings.HasPrefix(m[2], "--")
			switch strings.TrimPrefix(m[2], "--") {
			case ">>", strings.TrimPrefix("->>", "--"): // ">>"
				msg.Head = HeadArrow
			}
			// Head from the operator, normalized:
			op := m[2]
			switch {
			case strings.HasSuffix(op, ">>"):
				msg.Head = HeadArrow
			case strings.HasSuffix(op, "x"):
				msg.Head = HeadCross
			default:
				msg.Head = HeadNone
			}
			switch m[3] {
			case "+":
				msg.ActivateTo = true
			case "-":
				msg.DeactivateFrom = true
			}
			*curItems() = append(*curItems(), msg)

		case seqNoteRe.MatchString(line):
			m := seqNoteRe.FindStringSubmatch(line)
			n := &SeqNote{A: m[2], B: m[3], Text: strings.TrimSpace(m[4])}
			switch strings.ToLower(m[1]) {
			case "left of":
				n.Pos = NoteLeft
			case "right of":
				n.Pos = NoteRight
			default:
				n.Pos = NoteOver
			}
			if d.participant(n.A) == nil || (n.B != "" && d.participant(n.B) == nil) {
				return nil, unsup("note references unknown participant %q", line)
			}
			if n.Pos != NoteOver && n.B != "" {
				return nil, unsup("two participants only valid with 'over' %q", line)
			}
			*curItems() = append(*curItems(), n)

		case seqActRe.MatchString(line):
			m := seqActRe.FindStringSubmatch(line)
			if d.participant(m[2]) == nil {
				return nil, unsup("%s of unknown participant %q", m[1], m[2])
			}
			*curItems() = append(*curItems(), &SeqActivate{P: m[2], On: m[1] == "activate"})

		case seqFrameRe.MatchString(line):
			m := seqFrameRe.FindStringSubmatch(line)
			f := &SeqFrame{Kind: m[1], Sections: []SeqSection{{Label: strings.TrimSpace(m[2])}}}
			stack = append(stack, f)

		case seqDivRe.MatchString(line):
			m := seqDivRe.FindStringSubmatch(line)
			if len(stack) == 0 {
				return nil, unsup("%q outside a frame", m[1])
			}
			f := stack[len(stack)-1]
			if (m[1] == "else" && f.Kind != "alt") || (m[1] == "and" && f.Kind != "par") {
				return nil, unsup("%q divider in %q frame", m[1], f.Kind)
			}
			f.Sections = append(f.Sections, SeqSection{Label: strings.TrimSpace(m[2])})

		case line == "end":
			if len(stack) == 0 {
				return nil, unsup("end without frame")
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			*curItems() = append(*curItems(), f)

		default:
			return nil, unsup("statement %q", line)
		}
	}
	if !seenHeader {
		return nil, unsup("empty diagram")
	}
	if len(stack) != 0 {
		return nil, unsup("unclosed %q frame", stack[len(stack)-1].Kind)
	}
	return d, nil
}
```

Note: the message-head `switch` above contains one vestigial block (the first
`switch strings.TrimPrefix...`) — REMOVE it; the operative logic is the
`op := m[2]` switch that follows. (Kept here so you notice: the tests define
head mapping — `>>`→Arrow, `x`→Cross, bare `>`→None.)

IMPORTANT ordering: the `case seqFrameRe...` arm must come AFTER `seqPartRe`/`seqMsgRe`/`seqNoteRe`/`seqActRe` arms (as written) — and note `seqFrameRe` could otherwise swallow a message from a participant literally named `loop` (e.g. `loop->>b: x` does NOT match seqFrameRe because of the `(?:\s+...)?$` anchor requiring whitespace or EOL after the keyword — verify with the tests; if it bites, tests govern).

- [ ] **Step 4: Run tests until green**

Run: `go test ./mermaid/ -run TestSeqParse`
Expected: PASS (all 8). Then full package: `go test ./mermaid/` — PASS.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/seq_parse.go mermaid/seq_test.go mermaid/seq.go
git commit -m "Add sequence diagram parser"
```

---

### Task 3: Sequence layout

**Files:**
- Create: `mermaid/seq_layout.go` (delete the `layoutSequence` stub from `seq.go`)
- Test: append to `mermaid/seq_test.go`

**Interfaces:**
- Consumes: IR (Task 1), `measureText` (existing).
- Produces: `layoutSequence(d *SeqDiagram, t Theme) error` — fills Participant.X/W/H, SeqMessage.Y/Num, SeqNote.X/Y/W/H, SeqFrame.X/Y/W/H/DividerYs, d.Activations, d.Width/Height. Errors on unmatched `deactivate`.

Layout constants (package-level in seq_layout.go, shared with Task 4):

```go
const (
	seqMargin   = 8.0
	seqMinGap   = 60.0
	seqBoxPadX  = 12.0
	seqBoxPadY  = 8.0
	seqRowPad   = 10.0 // vertical padding above each message line
	seqNotePad  = 6.0
	seqFramePad = 10.0
	seqHeaderH  = 22.0 // frame kind-tab height
	seqSelfW    = 30.0 // self-message loop width
	seqSelfH    = 20.0 // self-message loop drop
	seqActW     = 10.0 // activation rect width
	seqActNest  = 4.0  // x offset per activation nesting level
)
```

- [ ] **Step 1: Write the failing tests**

Append to `mermaid/seq_test.go`:

```go
func layoutSeqFixture(t *testing.T, src string) *SeqDiagram {
	t.Helper()
	d := mustParseSeq(t, src)
	if err := layoutSequence(d, Light); err != nil {
		t.Fatalf("layoutSequence: %v", err)
	}
	return d
}

func TestSeqLayoutLifelinesOrderedAndSized(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
participant a as Short
participant b as A Much Longer Participant Label
a->>b: hi`)
	pa, pb := d.participant("a"), d.participant("b")
	if pa.W <= 0 || pa.H <= 0 || pb.W <= pa.W {
		t.Fatalf("box sizing wrong: %+v %+v", pa, pb)
	}
	if pa.X >= pb.X {
		t.Fatalf("lifelines out of order: %f >= %f", pa.X, pb.X)
	}
	if d.Width <= pb.X || d.Height <= 0 {
		t.Fatalf("diagram bounds wrong: %f x %f", d.Width, d.Height)
	}
}

func TestSeqLayoutGapFitsMessageLabel(t *testing.T) {
	long := "an extremely long message label that needs lots of horizontal room"
	d := layoutSeqFixture(t, "sequenceDiagram\na->>b: "+long)
	pa, pb := d.participant("a"), d.participant("b")
	lw, _ := measureText(long, Light.FontSize)
	if gap := pb.X - pa.X; gap < lw {
		t.Fatalf("gap %f narrower than label %f", gap, lw)
	}
}

func TestSeqLayoutYMonotonic(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
a->>b: one
b-->>a: two
Note over a,b: pause
a->>a: think
a-xb: three`)
	var prev float64
	var walk func(items []SeqItem)
	walk = func(items []SeqItem) {
		for _, it := range items {
			var y float64
			switch v := it.(type) {
			case *SeqMessage:
				y = v.Y
			case *SeqNote:
				y = v.Y
			case *SeqFrame:
				y = v.Y
				for _, s := range v.Sections {
					walk(s.Items)
				}
				continue
			default:
				continue
			}
			if y <= prev {
				t.Fatalf("y not increasing: %f then %f", prev, y)
			}
			prev = y
		}
	}
	walk(d.Items)
}

func TestSeqLayoutAutonumber(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
autonumber
a->>b: one
loop l
  b->>a: two
end
Note over a: not numbered
a->>b: three`)
	if n := d.Items[0].(*SeqMessage).Num; n != 1 {
		t.Fatalf("first num %d", n)
	}
	if n := d.Items[1].(*SeqFrame).Sections[0].Items[0].(*SeqMessage).Num; n != 2 {
		t.Fatalf("framed num %d", n)
	}
	if n := d.Items[3].(*SeqMessage).Num; n != 3 {
		t.Fatalf("third num %d", n)
	}
}

func TestSeqLayoutFrameContainsChildren(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
alt yes
  a->>b: one
else no
  a->>b: two
end`)
	f := d.Items[0].(*SeqFrame)
	if f.W <= 0 || f.H <= 0 {
		t.Fatalf("frame unsized: %+v", f)
	}
	if len(f.DividerYs) != 1 {
		t.Fatalf("dividers: %v", f.DividerYs)
	}
	m1 := f.Sections[0].Items[0].(*SeqMessage)
	m2 := f.Sections[1].Items[0].(*SeqMessage)
	if m1.Y <= f.Y || m1.Y >= f.DividerYs[0] || m2.Y <= f.DividerYs[0] || m2.Y >= f.Y+f.H {
		t.Fatalf("messages not inside sections: f=%+v m1=%f div=%f m2=%f", f, m1.Y, f.DividerYs[0], m2.Y)
	}
	pa, pb := d.participant("a"), d.participant("b")
	if f.X >= pa.X || f.X+f.W <= pb.X {
		t.Fatalf("frame doesn't span involved lifelines: f=%+v", f)
	}
}

func TestSeqLayoutActivations(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
a->>+b: outer on
a->>+b: inner on
b-->>-a: inner off
b-->>-a: outer off`)
	if len(d.Activations) != 2 {
		t.Fatalf("activations %d, want 2", len(d.Activations))
	}
	var outer, inner SeqActivation
	for _, act := range d.Activations {
		if act.Level == 0 {
			outer = act
		} else {
			inner = act
		}
	}
	if outer.P.ID != "b" || inner.P.ID != "b" || inner.Level != 1 {
		t.Fatalf("activations: %+v %+v", outer, inner)
	}
	if !(outer.Y0 < inner.Y0 && inner.Y1 < outer.Y1) {
		t.Fatalf("inner not nested in outer: outer=%+v inner=%+v", outer, inner)
	}
}

func TestSeqLayoutActivationAutoCloseAndError(t *testing.T) {
	d := layoutSeqFixture(t, "sequenceDiagram\nactivate a\na->>b: x")
	if len(d.Activations) != 1 || d.Activations[0].Y1 <= d.Activations[0].Y0 {
		t.Fatalf("auto-close failed: %+v", d.Activations)
	}
	bad := mustParseSeq(t, "sequenceDiagram\nparticipant a\ndeactivate a")
	if err := layoutSequence(bad, Light); err == nil {
		t.Fatal("unmatched deactivate must error")
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./mermaid/ -run TestSeqLayout`
Expected: FAIL — stub returns `ErrUnsupported`.

- [ ] **Step 3: Implement `mermaid/seq_layout.go`** (delete the stub)

```go
package mermaid

import "fmt"

const (
	seqMargin   = 8.0
	seqMinGap   = 60.0
	seqBoxPadX  = 12.0
	seqBoxPadY  = 8.0
	seqRowPad   = 10.0
	seqNotePad  = 6.0
	seqFramePad = 10.0
	seqHeaderH  = 22.0
	seqSelfW    = 30.0
	seqSelfH    = 20.0
	seqActW     = 10.0
	seqActNest  = 4.0
)

func layoutSequence(d *SeqDiagram, t Theme) error {
	if len(d.Participants) == 0 {
		return unsup("no participants")
	}
	idx := map[string]int{}
	for i, p := range d.Participants {
		idx[p.ID] = i
	}

	// Participant boxes: uniform height so lifelines start level.
	boxH := 0.0
	for _, p := range d.Participants {
		w, h := measureText(p.Label, t.FontSize)
		p.W, p.H = w+2*seqBoxPadX, h+2*seqBoxPadY
		if p.H > boxH {
			boxH = p.H
		}
	}
	for _, p := range d.Participants {
		p.H = boxH
	}

	// Horizontal: per-adjacent-gap minimums, widened by labels crossing them.
	gaps := make([]float64, len(d.Participants)-1)
	for i := range gaps {
		gaps[i] = seqMinGap
	}
	leftPad, rightPad := 0.0, 0.0
	needGap := func(i, j int, w float64) { // between participant idx i and j (i<j), adjacent only
		if j-i == 1 && w > gaps[i] {
			gaps[i] = w
		}
	}
	var scanH func(items []SeqItem)
	scanH = func(items []SeqItem) {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqMessage:
				lw, _ := measureText(numbered(v, d.Autonumber), t.FontSize)
				i, j := idx[v.From], idx[v.To]
				if i == j { // self-message needs room to the right
					if ext := seqSelfW + lw + 2*seqNotePad; i == len(d.Participants)-1 && ext > rightPad {
						rightPad = ext
					} else if i < len(d.Participants)-1 {
						needGap(i, i+1, ext+seqMinGap/2)
					}
					continue
				}
				if i > j {
					i, j = j, i
				}
				needGap(i, j, lw+2*seqNotePad+seqMinGap/2)
			case *SeqNote:
				lw, _ := measureText(v.Text, t.FontSize)
				w := lw + 2*seqNotePad
				a := idx[v.A]
				switch v.Pos {
				case NoteLeft:
					if a == 0 && w > leftPad {
						leftPad = w
					} else if a > 0 {
						needGap(a-1, a, w+seqMinGap/2)
					}
				case NoteRight:
					if a == len(d.Participants)-1 && w > rightPad {
						rightPad = w
					} else if a < len(d.Participants)-1 {
						needGap(a, a+1, w+seqMinGap/2)
					}
				case NoteOver:
					if v.B != "" {
						b := idx[v.B]
						if b < a {
							a, b = b, a
						}
						needGap(a, b, w-((d.Participants[a].W+d.Participants[b].W)/2))
					}
				}
			case *SeqFrame:
				for _, s := range v.Sections {
					scanH(s.Items)
				}
			}
		}
	}
	scanH(d.Items)

	x := seqMargin + leftPad + d.Participants[0].W/2
	for i, p := range d.Participants {
		p.X = x
		if i < len(gaps) {
			x += p.W/2 + gaps[i] + d.Participants[i+1].W/2
		}
	}
	d.Width = d.Participants[len(d.Participants)-1].X +
		d.Participants[len(d.Participants)-1].W/2 + rightPad + seqMargin

	// Vertical walk + activations + autonumber.
	_, lineH := measureText("Ag", t.FontSize)
	rowH := lineH + seqRowPad
	y := seqMargin + boxH + rowH
	num := 0
	type actOpen struct {
		y0    float64
		level int
	}
	open := map[string][]actOpen{} // participant id -> stack
	depth := map[string]int{}

	openAct := func(id string, at float64) {
		open[id] = append(open[id], actOpen{y0: at, level: depth[id]})
		depth[id]++
	}
	closeAct := func(id string, at float64) error {
		s := open[id]
		if len(s) == 0 {
			return unsup("deactivate %q without matching activate", id)
		}
		top := s[len(s)-1]
		open[id] = s[:len(s)-1]
		depth[id]--
		d.Activations = append(d.Activations, SeqActivation{
			P: d.participant(id), Level: top.level, Y0: top.y0, Y1: at,
		})
		return nil
	}

	var walkV func(items []SeqItem) error
	walkV = func(items []SeqItem) error {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqMessage:
				if d.Autonumber {
					num++
					v.Num = num
				}
				y += rowH
				v.Y = y
				if v.From == v.To {
					y += seqSelfH // self-loop consumes extra height
				}
				if v.ActivateTo {
					openAct(v.To, v.Y)
				}
				if v.DeactivateFrom {
					if err := closeAct(v.From, v.Y); err != nil {
						return err
					}
				}
			case *SeqNote:
				lw, lh := measureText(v.Text, t.FontSize)
				v.W, v.H = lw+2*seqNotePad, lh+2*seqNotePad
				y += rowH
				v.Y = y
				y += v.H - rowH + seqRowPad
				a := d.participant(v.A)
				switch v.Pos {
				case NoteLeft:
					v.X = a.X - seqActW - v.W
				case NoteRight:
					v.X = a.X + seqActW
				case NoteOver:
					right := a
					if v.B != "" {
						right = d.participant(v.B)
					}
					lo, hi := a.X, right.X
					if lo > hi {
						lo, hi = hi, lo
					}
					v.X = (lo+hi)/2 - v.W/2
				}
			case *SeqActivate:
				if v.On {
					openAct(v.P, y+rowH/2)
				} else if err := closeAct(v.P, y+rowH/2); err != nil {
					return err
				}
			case *SeqFrame:
				v.Y = y + seqRowPad
				y += seqRowPad + seqHeaderH
				for si, s := range v.Sections {
					if si > 0 {
						y += seqRowPad
						v.DividerYs = append(v.DividerYs, y)
					}
					if err := walkV(s.Items); err != nil {
						return err
					}
				}
				y += seqFramePad
				v.H = y - v.Y
				lo, hi := frameSpan(v, d, idx)
				v.X = lo - seqFramePad
				v.W = hi - lo + 2*seqFramePad
			}
		}
		return nil
	}
	if err := walkV(d.Items); err != nil {
		return err
	}

	// Auto-close activations still open.
	bottom := y + rowH
	for id, s := range open {
		for range s {
			if err := closeAct(id, bottom); err != nil {
				return fmt.Errorf("internal: %w", err)
			}
		}
	}
	d.Height = bottom + seqMargin
	return nil
}

// numbered returns the display text of a message (autonumber prefix applied).
// Shared with seq_svg.go.
func numbered(m *SeqMessage, auto bool) string {
	if auto && m.Num > 0 {
		return fmt.Sprintf("%d. %s", m.Num, m.Text)
	}
	if auto {
		return m.Text // horizontal pass runs before Num assignment; close enough for width
	}
	return m.Text
}

// frameSpan returns the min/max lifeline x among participants referenced
// anywhere inside the frame; empty frames span all participants.
func frameSpan(f *SeqFrame, d *SeqDiagram, idx map[string]int) (float64, float64) {
	lo, hi := -1.0, -1.0
	touch := func(id string) {
		x := d.participant(id).X
		if lo < 0 || x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	var scan func(items []SeqItem)
	scan = func(items []SeqItem) {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqMessage:
				touch(v.From)
				touch(v.To)
			case *SeqNote:
				touch(v.A)
				if v.B != "" {
					touch(v.B)
				}
			case *SeqFrame:
				for _, s := range v.Sections {
					scan(s.Items)
				}
			}
		}
	}
	for _, s := range f.Sections {
		scan(s.Items)
	}
	if lo < 0 {
		lo, hi = d.Participants[0].X, d.Participants[len(d.Participants)-1].X
	}
	return lo, hi
}
```

Note on `numbered`: the horizontal pass runs before autonumbers are assigned, so widths there use the unprefixed text — a few px narrow for numbered labels, absorbed by padding. If a test disagrees, tests govern.

- [ ] **Step 4: Run tests until green**

Run: `go test ./mermaid/ -run TestSeqLayout` then `go test ./mermaid/`
Expected: PASS.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/seq_layout.go mermaid/seq_test.go mermaid/seq.go
git commit -m "Add deterministic sequence diagram layout"
```

---

### Task 4: Sequence SVG emitter + goldens

**Files:**
- Create: `mermaid/seq_svg.go` (delete the `emitSequence` stub from `seq.go`)
- Create: `mermaid/testdata/seq-basic.mmd`, `mermaid/testdata/seq-frames.mmd` (+ generated `.svg` goldens)
- Test: append to `mermaid/seq_test.go`

**Interfaces:**
- Consumes: positioned `SeqDiagram` + Theme; `numbered` helper (Task 3); existing golden harness (`TestGoldens` in svg_test.go globs `testdata/*.mmd` — the new fixtures are picked up automatically).
- Produces: `emitSequence(d *SeqDiagram, t Theme) []byte`. `Render` is now fully live for sequence diagrams (Task 1's dispatch test stops skipping).

- [ ] **Step 1: Write the failing tests**

Append to `mermaid/seq_test.go`:

```go
func renderSeq(t *testing.T, src string, theme Theme) string {
	t.Helper()
	out, err := Render([]byte(src), theme)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return string(out)
}

const seqFixture = `sequenceDiagram
autonumber
participant a as Alice
participant b as Bob
a->>+b: solid request
b-->>-a: dashed reply
a-xb: cross
a->>a: self note to self
Note over a,b: spanning note
loop retry
  a->>b: again
end`

func TestSeqSVGWellFormed(t *testing.T) {
	out := renderSeq(t, seqFixture, Light)
	d := xml.NewDecoder(bytes.NewReader([]byte(out)))
	for {
		_, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid XML: %v\n%s", err, out)
		}
	}
	if !strings.Contains(out, `class="mermaid-svg"`) || !strings.Contains(out, "viewBox=") {
		t.Fatalf("root attrs missing: %.120s", out)
	}
}

func TestSeqSVGElements(t *testing.T) {
	out := renderSeq(t, seqFixture, Light)
	if c := strings.Count(out, `class="lifeline"`); c != 2 {
		t.Fatalf("lifelines %d, want 2", c)
	}
	if c := strings.Count(out, `class="seq-msg"`); c != 5 {
		t.Fatalf("message lines %d, want 5", c)
	}
	if !strings.Contains(out, "url(#cross)") {
		t.Fatal("cross marker not used")
	}
	if !strings.Contains(out, "url(#arrow)") {
		t.Fatal("arrow marker not used")
	}
	if c := strings.Count(out, `class="seq-frame"`); c != 1 {
		t.Fatalf("frames %d, want 1", c)
	}
	if c := strings.Count(out, `class="activation"`); c != 1 {
		t.Fatalf("activations %d, want 1", c)
	}
	for _, want := range []string{">Alice<", ">Bob<", "1. solid request", "5. again", "spanning note", ">loop<"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestSeqSVGEscaping(t *testing.T) {
	out := renderSeq(t, "sequenceDiagram\na->>b: <script>&\"attack\"", Light)
	if strings.Contains(out, "<script>") {
		t.Fatal("unescaped message text")
	}
	if !strings.Contains(out, "&lt;script&gt;") {
		t.Fatalf("expected escaped text:\n%s", out)
	}
}

func TestSeqSVGThemes(t *testing.T) {
	light := renderSeq(t, "sequenceDiagram\na->>b: x", Light)
	dark := renderSeq(t, "sequenceDiagram\na->>b: x", Dark)
	if !strings.Contains(light, Light.NodeFill) || !strings.Contains(dark, Dark.NodeFill) || light == dark {
		t.Fatal("themes not applied")
	}
}

func TestSeqSVGDashedStyles(t *testing.T) {
	out := renderSeq(t, "sequenceDiagram\na-->>b: dashed", Light)
	// the message line (class seq-msg) must carry a dasharray; lifelines are dashed too,
	// so count: 2 lifelines + 1 dashed message = 3 dasharray occurrences
	if c := strings.Count(out, "stroke-dasharray"); c != 3 {
		t.Fatalf("dasharray count %d, want 3\n%s", c, out)
	}
}
```

Add imports to seq_test.go as needed: `"bytes"`, `"encoding/xml"`.

- [ ] **Step 2: Create golden sources**

```bash
cat > mermaid/testdata/seq-basic.mmd <<'EOF'
sequenceDiagram
autonumber
participant c as Client
participant s as Server
c->>+s: GET /doc
s-->>-c: 200 OK
c->>c: render
Note over c,s: happy path
EOF
cat > mermaid/testdata/seq-frames.mmd <<'EOF'
sequenceDiagram
participant a as App
participant d as DB
alt cache hit
  a->>a: serve cached
else miss
  a->>+d: query
  d-->>-a: rows
end
loop poll
  a-xd: heartbeat
end
EOF
```

- [ ] **Step 3: Run tests to verify failure**

Run: `go test ./mermaid/ -run TestSeqSVG`
Expected: FAIL — `emitSequence` stub returns nil (Render returns empty).

- [ ] **Step 4: Implement `mermaid/seq_svg.go`** (delete the stub)

```go
package mermaid

import (
	"bytes"
	"fmt"
	"html"
)

// emitSequence renders a positioned sequence diagram. Same conventions as
// the flowchart emitter: escaped user text, %.1f floats, themed attributes.
func emitSequence(d *SeqDiagram, t Theme) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" class="mermaid-svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="%s" font-size="%.0f">`,
		d.Width, d.Height, d.Width, d.Height, html.EscapeString(t.FontFamily), t.FontSize)
	fmt.Fprintf(&b,
		`<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="%s"/></marker>`+
			`<marker id="cross" viewBox="0 0 10 10" refX="5" refY="5" markerWidth="8" markerHeight="8" orient="auto"><path d="M 2 2 L 8 8 M 8 2 L 2 8" stroke="%s" stroke-width="1.5"/></marker></defs>`,
		t.EdgeStroke, t.EdgeStroke)

	boxTop := seqMargin
	lifeTop := boxTop + d.Participants[0].H

	// Lifelines first (behind everything).
	for _, p := range d.Participants {
		fmt.Fprintf(&b, `<line class="lifeline" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-dasharray="4,4"/>`,
			p.X, lifeTop, p.X, d.Height-seqMargin, t.EdgeStroke)
	}

	// Frames behind messages.
	var frames func(items []SeqItem)
	frames = func(items []SeqItem) {
		for _, it := range items {
			f, ok := it.(*SeqFrame)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, `<rect class="seq-frame" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="%s"/>`,
				f.X, f.Y, f.W, f.H, t.EdgeStroke)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`,
				f.X, f.Y, 46.0, seqHeaderH, t.SubgraphFill)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s" font-weight="bold">%s</text>`,
				f.X+6, f.Y+seqHeaderH-6, t.Text, html.EscapeString(f.Kind))
			if lbl := f.Sections[0].Label; lbl != "" {
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s">[%s]</text>`,
					f.X+52, f.Y+seqHeaderH-6, t.Text, html.EscapeString(lbl))
			}
			for si, dy := range f.DividerYs {
				fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-dasharray="4,4"/>`,
					f.X, dy, f.X+f.W, dy, t.EdgeStroke)
				if lbl := f.Sections[si+1].Label; lbl != "" {
					fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s">[%s]</text>`,
						f.X+6, dy+seqHeaderH-6, t.Text, html.EscapeString(lbl))
				}
			}
			for _, s := range f.Sections {
				frames(s.Items)
			}
		}
	}
	frames(d.Items)

	// Activations (over lifelines, under messages).
	for _, a := range d.Activations {
		x := a.P.X - seqActW/2 + float64(a.Level)*seqActNest
		fmt.Fprintf(&b, `<rect class="activation" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="%s"/>`,
			x, a.Y0, seqActW, a.Y1-a.Y0, t.NodeFill, t.NodeStroke)
	}

	// Messages and notes in stream order.
	var items func(items []SeqItem)
	items = func(list []SeqItem) {
		for _, it := range list {
			switch v := it.(type) {
			case *SeqMessage:
				text := numbered(v, d.Autonumber)
				from, to := participantX(d, v.From), participantX(d, v.To)
				dash := ""
				if v.Dashed {
					dash = ` stroke-dasharray="6,3"`
				}
				marker := ""
				switch v.Head {
				case HeadArrow:
					marker = ` marker-end="url(#arrow)"`
				case HeadCross:
					marker = ` marker-end="url(#cross)"`
				}
				if v.From == v.To { // self-message loop
					fmt.Fprintf(&b, `<path class="seq-msg" d="M %.1f %.1f h %.1f v %.1f h %.1f" fill="none" stroke="%s"%s%s/>`,
						from, v.Y, seqSelfW, seqSelfH, -seqSelfW+4, t.EdgeStroke, dash, marker)
					fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s">%s</text>`,
						from+seqSelfW+seqNotePad, v.Y+seqSelfH/2+4, t.Text, html.EscapeString(text))
					continue
				}
				fmt.Fprintf(&b, `<line class="seq-msg" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s"%s%s/>`,
					from, v.Y, to, v.Y, t.EdgeStroke, dash, marker)
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text>`,
					(from+to)/2, v.Y-5, t.Text, html.EscapeString(text))
			case *SeqNote:
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="%s"/>`,
					v.X, v.Y, v.W, v.H, t.SubgraphFill, t.NodeStroke)
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%s</text>`,
					v.X+v.W/2, v.Y+v.H/2, t.Text, html.EscapeString(v.Text))
			case *SeqFrame:
				for _, s := range v.Sections {
					items(s.Items)
				}
			}
		}
	}
	items(d.Items)

	// Participant boxes last (on top of lifeline starts).
	for _, p := range d.Participants {
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="%s" stroke-width="1.5"/>`,
			p.X-p.W/2, boxTop, p.W, p.H, t.NodeFill, t.NodeStroke)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%s</text>`,
			p.X, boxTop+p.H/2, t.Text, html.EscapeString(p.Label))
	}

	b.WriteString(`</svg>`)
	return b.Bytes()
}

func participantX(d *SeqDiagram, id string) float64 { return d.participant(id).X }
```

Note: the existing flowchart emitter (`svg.go`) also defines an `#arrow` marker — both emitters produce COMPLETE separate SVG documents, so the duplicate marker id is fine across documents; ids only collide within one document (and one document comes from exactly one emitter). If svg.go's marker snippet is identical, do NOT extract a shared helper for two call sites — duplication of one line beats a premature abstraction here.

- [ ] **Step 5: Generate goldens, run everything**

```bash
go test ./mermaid/ -update
go test ./mermaid/ -race
go test ./... -race
open mermaid/testdata/seq-basic.svg 2>/dev/null || true  # eyeball if GUI available
```

Expected: full suite PASS with race detector. Paste the first ~15 lines of `seq-basic.svg` into your report for the controller's eyeball. Sanity: participant boxes at top, dashed lifelines, numbered labels, one activation rect, note rect, loop/alt frames with dividers.

- [ ] **Step 6: Commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/seq_svg.go mermaid/seq_test.go mermaid/seq.go mermaid/testdata
git commit -m "Add sequence diagram SVG emitter with goldens"
```

---

### Task 5: Viewer test flip, README, verification

**Files:**
- Modify: `render_test.go` (sequence fence now native; fallback fixture → gantt)
- Modify: `README.md`
- Test: `render_test.go`

**Interfaces:**
- Consumes: everything above; `RenderBody` (existing signature, unchanged).
- Produces: no new code — updated tests + docs. `render.go`/`server.go` are NOT modified.

- [ ] **Step 1: Update the tests**

In `render_test.go`, `TestRenderBodyMermaidFallback` currently uses a `sequenceDiagram` fixture — that now renders natively, so the test as-is FAILS (run `go test ./... -run TestRenderBodyMermaidFallback` first to see it fail: fallback flag false, `<pre` missing). Replace its fixture with `gantt`:

```go
func TestRenderBodyMermaidFallback(t *testing.T) {
	out, fallback, err := RenderBody([]byte("```mermaid\ngantt\ntitle X\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<pre class="mermaid">`) {
		t.Fatalf("unsupported diagram must fall back: %s", out)
	}
	if !fallback {
		t.Fatal("fallback flag must be true")
	}
	// fallback source must stay HTML-escaped (regression guard)
	out2, _, err := RenderBody([]byte("```mermaid\ngantt\na->>b\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), "a-&gt;&gt;b") {
		t.Fatalf("fallback source not escaped: %s", out2)
	}
}
```

(This also restores the escaped-fallback regression guard the final flowchart review noted was dropped.)

Add the native-sequence integration test:

```go
func TestRenderBodyNativeSequence(t *testing.T) {
	out, fallback, err := RenderBody([]byte("```mermaid\nsequenceDiagram\nAlice->>Bob: hi\nBob-->>Alice: yo\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<svg") || strings.Contains(s, `<pre class="mermaid">`) {
		t.Fatalf("sequence must render natively: %s", s)
	}
	if fallback {
		t.Fatal("fallback flag must be false")
	}
}
```

If `TestRenderBodyMermaidPassthrough` also uses a sequence fixture, flip it to `gantt` the same way (check first — it was already flipped to gantt in the flowchart branch).

- [ ] **Step 2: Run tests**

Run: `go test ./... -race`
Expected: PASS everywhere.

- [ ] **Step 3: Update `README.md`**

Replace the mermaid Features bullet:

```markdown
- Inline Mermaid diagrams — common flowcharts and sequence diagrams render
  natively to SVG (no JS); other diagram types fall back to the bundled
  mermaid.js
```

- [ ] **Step 4: Manual verification (controller)**

Serve a file containing a native flowchart, a native sequence diagram (with autonumber, a note, an alt frame, an activation), and a `gantt` fallback. Verify: both natives are `<svg class="mermaid-svg">`, gantt still ships the `<pre>` + mermaid.min.js, dark config themes the sequence SVG, window opens/closes cleanly.

- [ ] **Step 5: Commit**

```bash
git add render_test.go README.md
git commit -m "Render sequence diagrams natively; restore fallback escaping guard"
```

---

## Self-Review

**Spec coverage:**
- Subset: participants/actor/`as`/implicit-on-message — Task 2 (tested); six arrows + self-messages — Tasks 2 (parse) + 3/4 (self-loop layout/emit); activations stmt + suffixes, nesting, auto-close, unmatched-deactivate error — Tasks 2+3; notes all four forms + unknown-participant error — Tasks 2+3+4; nested loop/opt/alt/else/par/and frames — Tasks 2 (stack) + 3 (recursion, dividers, span) + 4 (tabs/dividers); bare autonumber (+ `autonumber N` error) — Tasks 2+3. ✓
- Everything-else-falls-back (`critical`, `box`, `create`, unknown) — Task 2 unsupported table. ✓
- Pure-Go deterministic, no new deps, no dagre — layout has no imports beyond `fmt`. ✓
- Horizontal adjacent-gap constraint + left/right margin bumps, documented wide-span ceiling — Task 3 (spec non-goal, no test asserts wide-span fit). ✓
- Vertical invariants (monotonic y, frame containment, activation nesting) — Task 3 tests match spec's invariants list. ✓
- SVG: top-only boxes, dashed lifelines, markers (#arrow reused, #cross new), dashed lines, self-loops, note rects, frame tabs + dashed dividers, escaping, root conventions — Task 4 (+escaping/theme/well-formed tests). ✓
- `Render` dispatch case; viewer untouched; integration tests flip; goldens via existing `-update` harness (glob picks up `seq-*.mmd` automatically) — Tasks 1+4+5. ✓
- Concurrency: no new package-level mutable state (all layout state is per-call). ✓

**Placeholder scan:** clean. Two flagged clean-ups inside Task 2/3 code (vestigial switch block to remove; `numbered` width note) are explicit instructions, not TBDs.

**Type consistency:** `parseSequence(string) (*SeqDiagram, error)`, `layoutSequence(*SeqDiagram, Theme) error`, `emitSequence(*SeqDiagram, Theme) []byte`, IR names (`SeqMessage.Y/Num`, `SeqNote.X/Y/W/H`, `SeqFrame.DividerYs`, `SeqActivation.Level/Y0/Y1`), constants (`seqSelfW/H`, `seqActW/Nest`, `seqHeaderH`) — consistent across Tasks 1–4; Task 5 touches only tests/README.
