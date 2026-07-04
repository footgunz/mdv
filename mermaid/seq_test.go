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
