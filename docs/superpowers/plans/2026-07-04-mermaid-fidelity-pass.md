# Mermaid Fidelity Pass Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close three visual gaps vs mermaid.js: curved flowchart edges, curved self-messages, and mermaid-like box sizing.

**Architecture:** All changes land in the existing emitters/layout constants: an `edgePath` midpoint-quadratic helper in `svg.go`, a cubic self-message path in `seq_svg.go`, and sizing constants in `seq_layout.go`/`text.go`. Goldens regenerate per task; the acceptance gate is the four-panel screenshot comparison rerun by the controller.

**Tech Stack:** Go stdlib only. No new dependencies.

## Global Constraints

- Marker anchoring must survive: every curved path still terminates exactly at its final dagre/layout point so `marker-end` arrowheads sit where they did.
- Fonts stay 14px page-matched (deliberate divergence from mermaid). Themes, escaping, fallback contract untouched.
- Deterministic output: fixed-precision floats only; goldens regenerated with `-update` and eyeballed each task.
- Branch: `mermaid-fidelity` (already created).
- **If plan code conflicts with plan tests, the tests govern** — fix the implementation, note the deviation.

---

### Task 1: Curved flowchart edges

**Files:**
- Modify: `mermaid/svg.go` (edge path construction)
- Test: `mermaid/svg_test.go`

**Interfaces:**
- Consumes: `Point`, edge `Points` from layout.
- Produces: `func edgePath(pts []Point) string` — SVG path `d` attribute (used only within svg.go; Task 2 does NOT reuse it).

- [ ] **Step 1: Write the failing test**

Add to `mermaid/svg_test.go`:

```go
func TestSVGEdgesCurved(t *testing.T) {
	out := string(render(t, "graph TD\na --> b\nb --> c\na --> c", Light))
	if !strings.Contains(out, " Q ") {
		t.Fatalf("edges should contain quadratic segments:\n%s", out)
	}
}

func TestEdgePath(t *testing.T) {
	two := edgePath([]Point{{0, 0}, {10, 10}})
	if two != "M 0.0 0.0 L 10.0 10.0" {
		t.Fatalf("two-point edge must stay straight: %q", two)
	}
	three := edgePath([]Point{{0, 0}, {10, 0}, {10, 10}})
	if !strings.Contains(three, "Q 10.0 0.0") {
		t.Fatalf("interior point must become a Q control point: %q", three)
	}
	if !strings.HasSuffix(three, "L 10.0 10.0") {
		t.Fatalf("path must terminate at the exact final point (marker anchor): %q", three)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./mermaid/ -run 'TestSVGEdgesCurved|TestEdgePath'`
Expected: FAIL — `undefined: edgePath`, no `Q` in output.

- [ ] **Step 3: Implement**

In `mermaid/svg.go`, add:

```go
// edgePath renders points as a smooth path: straight for two points,
// midpoint-quadratic smoothing through interior points otherwise (same
// visual family as mermaid's curveBasis). The path always terminates at
// the exact final point so marker-end anchors correctly.
func edgePath(pts []Point) string {
	var d strings.Builder
	fmt.Fprintf(&d, "M %.1f %.1f", pts[0].X, pts[0].Y)
	if len(pts) == 2 {
		fmt.Fprintf(&d, " L %.1f %.1f", pts[1].X, pts[1].Y)
		return d.String()
	}
	mid := func(a, b Point) Point { return Point{(a.X + b.X) / 2, (a.Y + b.Y) / 2} }
	m := mid(pts[0], pts[1])
	fmt.Fprintf(&d, " L %.1f %.1f", m.X, m.Y)
	for i := 1; i < len(pts)-1; i++ {
		m = mid(pts[i], pts[i+1])
		fmt.Fprintf(&d, " Q %.1f %.1f %.1f %.1f", pts[i].X, pts[i].Y, m.X, m.Y)
	}
	fmt.Fprintf(&d, " L %.1f %.1f", pts[len(pts)-1].X, pts[len(pts)-1].Y)
	return d.String()
}
```

In `emit`'s edge loop, replace the manual `strings.Builder` path construction:

```go
		var d strings.Builder
		fmt.Fprintf(&d, "M %.1f %.1f", e.Points[0].X, e.Points[0].Y)
		for _, p := range e.Points[1:] {
			fmt.Fprintf(&d, " L %.1f %.1f", p.X, p.Y)
		}
```

with:

```go
		d := edgePath(e.Points)
```

and change the later `d.String()` reference to `d`.

- [ ] **Step 4: Regenerate goldens, verify, eyeball**

Run: `go test ./mermaid/ -update && go test ./... -race`
Expected: PASS. Open or textually inspect `mermaid/testdata/simple.svg` — edge `d` attributes now contain `Q` segments; arrowheads still at node borders.

- [ ] **Step 5: Commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/svg.go mermaid/svg_test.go mermaid/testdata
git commit -m "Smooth flowchart edges with midpoint-quadratic curves"
```

---

### Task 2: Curved self-messages

**Files:**
- Modify: `mermaid/seq_svg.go` (self-message branch)
- Test: `mermaid/seq_test.go`

**Interfaces:**
- Consumes: `SeqMessage` geometry, `seqSelfW`/`seqSelfH` constants.
- Produces: no new symbols — path shape change only.

- [ ] **Step 1: Write the failing test**

Add to `mermaid/seq_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./mermaid/ -run TestSeqSVGSelfMessageCurved`
Expected: FAIL — path uses `h`/`v` segments.

- [ ] **Step 3: Implement**

In `mermaid/seq_svg.go`, replace the self-message path:

```go
				fmt.Fprintf(&b, `<path class="seq-msg" d="M %.1f %.1f h %.1f v %.1f h %.1f" fill="none" stroke="%s"%s%s/>`,
					from, v.Y, seqSelfW, seqSelfH, -seqSelfW+4, t.EdgeStroke, dash, marker)
```

with:

```go
				fmt.Fprintf(&b, `<path class="seq-msg" d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" fill="none" stroke="%s"%s%s/>`,
					from, v.Y, from+seqSelfW*1.6, v.Y, from+seqSelfW*1.6, v.Y+seqSelfH, from+6, v.Y+seqSelfH, t.EdgeStroke, dash, marker)
```

(Label emission below it is unchanged.)

- [ ] **Step 4: Regenerate goldens, verify**

Run: `go test ./mermaid/ -update && go test ./... -race`
Expected: PASS (seq-basic golden contains the self-message).

- [ ] **Step 5: Commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/seq_svg.go mermaid/seq_test.go mermaid/testdata
git commit -m "Curve sequence self-messages"
```

---

### Task 3: Box sizing calibration

**Files:**
- Modify: `mermaid/seq_layout.go` (constants + participant minimums)
- Modify: `mermaid/text.go` (node padding)
- Test: `mermaid/seq_test.go`, `mermaid/text_test.go`

**Interfaces:**
- Consumes: existing constants and sizing code.
- Produces: `seqActorMinW = 150.0`, `seqActorMinH = 50.0`, `seqRowPad = 16.0` (was 10), flowchart `padX = 20.0` (was 16), `padY = 15.0` (was 10).

- [ ] **Step 1: Write the failing tests**

Add to `mermaid/seq_test.go`:

```go
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
```

Add to `mermaid/text_test.go`:

```go
func TestMeasureGraphNodePadding(t *testing.T) {
	g := mustParse(t, "graph TD\na[label]")
	measureGraph(g, Light)
	w, h := measureText("label", Light.FontSize)
	n := g.node("a")
	if n.W != w+40 || n.H != h+30 {
		t.Fatalf("rect padding: got %.1fx%.1f, want %.1fx%.1f (text+40, text+30)", n.W, n.H, w+40, h+30)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./mermaid/ -run 'TestSeqLayoutActorMinimums|TestSeqLayoutRowRhythm|TestMeasureGraphNodePadding'`
Expected: FAIL — `seqActorMinW` undefined; padding 40/30 not met; row delta ~27 < required.

- [ ] **Step 3: Implement**

`mermaid/seq_layout.go` constants: change `seqRowPad = 10.0` to `seqRowPad = 16.0`; add:

```go
	seqActorMinW = 150.0 // mermaid's default actor box width
	seqActorMinH = 50.0
```

In the participant sizing loop, after `p.W, p.H = w+2*seqBoxPadX, h+2*seqBoxPadY`:

```go
		if p.W < seqActorMinW {
			p.W = seqActorMinW
		}
		if p.H < seqActorMinH {
			p.H = seqActorMinH
		}
```

`mermaid/text.go`: change `padX = 16.0` → `padX = 20.0`, `padY = 10.0` → `padY = 15.0`.

- [ ] **Step 4: Full suite + goldens**

Run: `go test ./mermaid/ -update && go test ./... -race`
Expected: PASS. If any pre-existing layout test asserts old spacing implicitly (e.g. divider-room formulas), they are formula-based and should adapt; investigate any failure rather than loosening assertions.

- [ ] **Step 5: Commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/seq_layout.go mermaid/text.go mermaid/seq_test.go mermaid/text_test.go mermaid/testdata
git commit -m "Calibrate box sizing toward mermaid.js proportions"
```

---

### Task 4 (controller): screenshot acceptance

Re-run the four-panel headless-Chrome comparison (native/js × light/dark) against the same `compare.md`, view all four, and judge: edges curved, self-message arced, box proportions in the same family as mermaid.js, nothing newly overlapping. Update the comparison page with the new captures.

---

## Self-Review

**Spec coverage:** curved edges incl. marker-anchor guarantee + 2-point straight case (Task 1, both tested); cubic self-message (Task 2, tested incl. absence of h/v); sequence minimums + row rhythm + flowchart padding with exact spec values 150/50/16/20/15 (Task 3, tested); goldens per task; screenshot acceptance (Task 4, controller). Non-goals respected — no badge/mirror work, no deps. ✓

**Placeholder scan:** clean.

**Type consistency:** `edgePath([]Point) string` defined and used only in svg.go; constants named identically across Tasks 3's files and tests.
