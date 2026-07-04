package mermaid

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
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

func TestSeqLayoutAutoCloseDeterministic(t *testing.T) {
	src := "sequenceDiagram\nactivate a\nactivate b\na->>b: x\nb->>a: y"
	first, err := Render([]byte(src), Light)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		out, err := Render([]byte(src), Light)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(out, first) {
			t.Fatalf("render %d differs from first", i)
		}
	}
}

func TestSeqLayoutNoteClearsFollowingContent(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
participant a
participant b
Note over a,b: heads up
alt yes
  a->>b: one
end`)
	n := d.Items[0].(*SeqNote)
	f := d.Items[1].(*SeqFrame)
	if f.Y < n.Y+n.H {
		t.Fatalf("frame top %.1f overlaps note bottom %.1f", f.Y, n.Y+n.H)
	}
	// same for a message following a note
	d2 := layoutSeqFixture(t, "sequenceDiagram\nparticipant a\nNote over a: hi\na->>a: next")
	n2 := d2.Items[0].(*SeqNote)
	m2 := d2.Items[1].(*SeqMessage)
	if m2.Y < n2.Y+n2.H {
		t.Fatalf("message y %.1f inside note (bottom %.1f)", m2.Y, n2.Y+n2.H)
	}
}

func TestSeqLayoutDividerLabelRoom(t *testing.T) {
	d := layoutSeqFixture(t, `sequenceDiagram
alt cached
  a->>b: one
else fresh
  a->>b: two
end`)
	f := d.Items[0].(*SeqFrame)
	m2 := f.Sections[1].Items[0].(*SeqMessage)
	// the section label is drawn in [divider, divider+seqHeaderH]; the next
	// message's text sits just above its line — it must clear the label band
	if m2.Y-seqRowPad < f.DividerYs[0]+seqHeaderH {
		t.Fatalf("second-section message %.1f crowds divider label (divider %.1f + header %.1f)",
			m2.Y, f.DividerYs[0], seqHeaderH)
	}
}

func TestSeqSVGSelfMessageCurved(t *testing.T) {
	out := renderSeq(t, "sequenceDiagram\na->>a: think", Light)
	i := strings.Index(out, `class="seq-msg"`)
	if i < 0 {
		t.Fatal("no self-message path")
	}
	seg := out[i : i+220]
	if !strings.Contains(seg, " C ") {
		t.Fatalf("self-message should be a cubic curve: %s", seg)
	}
	if strings.Contains(seg, " h ") || strings.Contains(seg, " v ") {
		t.Fatalf("rectangular loop segments still present: %s", seg)
	}
}

func TestSeqSVGSelfMessageLabelClearsCurve(t *testing.T) {
	out := renderSeq(t, "sequenceDiagram\na->>a: think", Light)
	// label must start right of the curve's widest rendered point
	// (bezier midpoint x = from + seqSelfW*1.6*0.75 = from+36.75); with the
	// fixed offset the label text x is from + seqSelfW*1.6 + seqNotePad = from+54
	i := strings.Index(out, `class="seq-msg"`)
	seg := out[i:]
	var fromX float64
	if _, err := fmt.Sscanf(seg[strings.Index(seg, `d="M `)+5:], "%f", &fromX); err != nil {
		t.Fatal(err)
	}
	j := strings.Index(seg, "<text x=\"")
	var textX float64
	if _, err := fmt.Sscanf(seg[j+9:], "%f", &textX); err != nil {
		t.Fatal(err)
	}
	if textX < fromX+seqSelfW*1.6 {
		t.Fatalf("label x %.1f overlaps curve (from %.1f, bulge to %.1f)", textX, fromX, fromX+seqSelfW*1.6)
	}
}

func TestSeqLayoutActorMinimums(t *testing.T) {
	d := layoutSeqFixture(t, "sequenceDiagram\nparticipant a as Hi\nparticipant b as A Rather Long Participant Label Indeed\na->>b: x")
	pa, pb := d.participant("a"), d.participant("b")
	if pa.W != seqActorMinW || pa.H != seqActorMinH {
		t.Fatalf("short label must clamp to minimums, got %.1fx%.1f", pa.W, pa.H)
	}
	if pb.W <= seqActorMinW {
		t.Fatalf("long label must exceed minimum width: %.1f", pb.W)
	}
}

func TestSeqLayoutRowRhythm(t *testing.T) {
	d := layoutSeqFixture(t, "sequenceDiagram\na->>b: one\nb->>a: two")
	m1 := d.Items[0].(*SeqMessage)
	m2 := d.Items[1].(*SeqMessage)
	_, lineH := measureText("Ag", Light.FontSize)
	if delta := m2.Y - m1.Y; delta < lineH+16 {
		t.Fatalf("row delta %.1f, want >= lineH(%.1f)+16", delta, lineH)
	}
}

func TestSeqLayoutSelfMessageLastParticipant(t *testing.T) {
	d := layoutSeqFixture(t, "sequenceDiagram\na->>b: hi\nb->>b: thinking hard about it")
	pb := d.participant("b")
	_ = d.Items[1].(*SeqMessage) // verify it's a message
	labelW, _ := measureText("thinking hard about it", Light.FontSize)
	// Width must accommodate participant b's position + self-message bulge + label + margin
	minWidth := pb.X + seqSelfW*1.6 + seqNotePad + labelW + seqMargin
	if d.Width < minWidth {
		t.Fatalf("diagram width %.1f too narrow for self-message at last participant, want >= %.1f", d.Width, minWidth)
	}
}
