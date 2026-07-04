# Pie and State Diagrams Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Native rendering for mermaid pie charts (arcs + legend, themed palette) and state diagrams (parser targeting the existing flowchart IR + two pseudo-state shapes).

**Architecture:** Pie is one self-contained file (`pie.go`: parse → geometry → SVG, no dagre). State is a parser (`state.go`) producing the flowchart `Graph`; `measureGraph`/`layout`/`emit` are reused with two new `Shape` cases. `Render` gains the dispatch cases. Viewer untouched.

**Tech Stack:** Go stdlib only (`math`, `sort`, `strconv` join the usual imports).

## Global Constraints

- Fallback contract: any out-of-subset construct → `ErrUnsupported`-wrapped error; no panics on user input; pie with non-positive total errors.
- All user text through `html.EscapeString`; deterministic `%.1f` floats; goldens via `-update`, eyeballed.
- No new dependencies; no new package-level mutable state; themes stay page-matched.
- Branch: `pie-state-diagrams` (already created).
- **If plan code conflicts with plan tests, the tests govern** — fix the implementation, note the deviation.

---

### Task 1: Pie charts end-to-end

**Files:**
- Create: `mermaid/pie.go`
- Modify: `mermaid/theme.go` (Palette field + values), `mermaid/mermaid.go` (dispatch case)
- Modify: `render_test.go` (integration test)
- Create: `mermaid/pie_test.go`, `mermaid/testdata/pie-basic.mmd` (+ generated golden)

**Interfaces:**
- Consumes: `unsup`, `measureText`, `Theme`, `ErrUnsupported`.
- Produces: `type pieSlice struct { Label string; Value float64 }`, `type pieChart struct { Title string; ShowData bool; Slices []pieSlice }`, `func parsePie(src string) (*pieChart, error)`, `func emitPie(p *pieChart, t Theme) []byte`, `Theme.Palette []string`.

- [ ] **Step 1: Write the failing tests**

Create `mermaid/pie_test.go`:

```go
package mermaid

import (
	"errors"
	"strings"
	"testing"
)

func TestPieParse(t *testing.T) {
	p, err := parsePie("pie showData\ntitle Languages\n%% comment\n\"Go\" : 60\n\"JS\" : 25.5\n\"Other\" : 14.5")
	if err != nil {
		t.Fatal(err)
	}
	if !p.ShowData || p.Title != "Languages" || len(p.Slices) != 3 {
		t.Fatalf("got %+v", p)
	}
	// sorted by value descending
	if p.Slices[0].Label != "Go" || p.Slices[1].Label != "JS" || p.Slices[2].Label != "Other" {
		t.Fatalf("order: %+v", p.Slices)
	}
	p2, err := parsePie("pie\n\"b\" : 1\n\"a\" : 2")
	if err != nil || p2.ShowData || p2.Title != "" {
		t.Fatalf("plain pie: %+v %v", p2, err)
	}
	if p2.Slices[0].Label != "a" {
		t.Fatalf("descending sort: %+v", p2.Slices)
	}
}

func TestPieParseErrors(t *testing.T) {
	for _, src := range []string{
		"pie",                       // no slices
		"pie\n\"a\" : 0",            // zero total
		"pie\n\"a\" : -3",           // negative
		"pie\n\"a\" : x",            // bad number
		"pie\nnonsense here",        // unknown line
		"pie extra words\n\"a\": 1", // bad header
	} {
		if _, err := parsePie(src); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: want ErrUnsupported, got %v", src, err)
		}
	}
}

func TestPieSVG(t *testing.T) {
	out := string(mustRenderPie(t, "pie showData\ntitle Split\n\"Alpha\" : 70\n\"Beta\" : 30", Light))
	if c := strings.Count(out, `class="pie-slice"`); c != 2 {
		t.Fatalf("slices %d, want 2\n%s", c, out)
	}
	for _, want := range []string{">Split<", "70%", "30%", ">Alpha [70]<", ">Beta [30]<", Light.Palette[0], Light.Palette[1]} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q\n%s", want, out)
		}
	}
	if c := strings.Count(out, `class="pie-legend"`); c != 2 {
		t.Fatalf("legend rows %d, want 2", c)
	}
}

func TestPieSVGSingleSliceAndSmallSlices(t *testing.T) {
	one := string(mustRenderPie(t, "pie\n\"all\" : 5", Light))
	if !strings.Contains(one, "<circle") {
		t.Fatalf("single slice must render a full circle:\n%s", one)
	}
	small := string(mustRenderPie(t, "pie\n\"big\" : 97\n\"tiny\" : 3", Light))
	if strings.Contains(small, ">3%<") {
		t.Fatal("slices under 5% must not get a percent label")
	}
	if !strings.Contains(small, ">97%<") {
		t.Fatal("big slice keeps its percent label")
	}
}

func TestPiePaletteCyclesAndThemes(t *testing.T) {
	var b strings.Builder
	b.WriteString("pie\n")
	for i := 0; i < 10; i++ {
		b.WriteString(`"s` + string(rune('a'+i)) + `" : 1` + "\n")
	}
	out := string(mustRenderPie(t, b.String(), Light))
	if !strings.Contains(out, Light.Palette[0]) || strings.Count(out, Light.Palette[0]) < 2 {
		t.Fatal("palette must cycle past 8 slices (color 0 reused)")
	}
	dark := string(mustRenderPie(t, "pie\n\"a\" : 1", Dark))
	if !strings.Contains(dark, Dark.Palette[0]) {
		t.Fatal("dark palette not applied")
	}
	if len(Light.Palette) != 8 || len(Dark.Palette) != 8 {
		t.Fatalf("palettes must have 8 entries: %d/%d", len(Light.Palette), len(Dark.Palette))
	}
}

func TestPieEscaping(t *testing.T) {
	out := string(mustRenderPie(t, "pie\ntitle <b>&title\n\"<script>\" : 1", Light))
	if strings.Contains(out, "<script>") || strings.Contains(out, "<b>&title") {
		t.Fatalf("unescaped user text:\n%s", out)
	}
}

func mustRenderPie(t *testing.T, src string, theme Theme) []byte {
	t.Helper()
	out, err := Render([]byte(src), theme)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./mermaid/ -run 'TestPie'`
Expected: FAIL — `undefined: parsePie` etc.

- [ ] **Step 3: Implement**

`mermaid/theme.go` — add to the `Theme` struct:

```go
	Palette []string // categorical slice colors (pie), cycled when exhausted
```

Add to `Light`:

```go
	Palette: []string{"#4e79a7", "#f28e2b", "#59a14f", "#e15759", "#b07aa1", "#edc949", "#76b7b2", "#ff9da7"},
```

Add to `Dark`:

```go
	Palette: []string{"#58a6ff", "#f0883e", "#3fb950", "#f85149", "#bc8cff", "#d29922", "#39c5cf", "#ff7b72"},
```

Create `mermaid/pie.go`:

```go
package mermaid

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type pieSlice struct {
	Label string
	Value float64
}

type pieChart struct {
	Title    string
	ShowData bool
	Slices   []pieSlice
}

var pieSliceRe = regexp.MustCompile(`^"([^"]*)"\s*:\s*([0-9]+(?:\.[0-9]+)?)$`)

func parsePie(src string) (*pieChart, error) {
	p := &pieChart{}
	seenHeader := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !seenHeader {
			switch line {
			case "pie":
			case "pie showData":
				p.ShowData = true
			default:
				return nil, unsup("bad pie header %q", line)
			}
			seenHeader = true
			continue
		}
		if title, ok := strings.CutPrefix(line, "title "); ok {
			p.Title = strings.TrimSpace(title)
			continue
		}
		m := pieSliceRe.FindStringSubmatch(line)
		if m == nil {
			return nil, unsup("pie statement %q", line)
		}
		val, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return nil, unsup("pie value %q", m[2])
		}
		p.Slices = append(p.Slices, pieSlice{Label: m[1], Value: val})
	}
	if !seenHeader {
		return nil, unsup("empty diagram")
	}
	total := 0.0
	for _, s := range p.Slices {
		total += s.Value
	}
	if total <= 0 {
		return nil, unsup("pie needs a positive total")
	}
	// mermaid renders slices largest-first
	sort.SliceStable(p.Slices, func(i, j int) bool { return p.Slices[i].Value > p.Slices[j].Value })
	return p, nil
}

const (
	pieR      = 90.0
	pieMargin = 12.0
	pieSwatch = 12.0
	pieRowH   = 20.0
)

func emitPie(p *pieChart, t Theme) []byte {
	total := 0.0
	for _, s := range p.Slices {
		total += s.Value
	}

	titleH := 0.0
	if p.Title != "" {
		_, th := measureText(p.Title, t.FontSize)
		titleH = th + pieMargin
	}
	cx := pieMargin + pieR
	cy := titleH + pieMargin + pieR

	legendX := cx + pieR + 2*pieMargin
	maxLegend := 0.0
	for _, s := range p.Slices {
		w, _ := measureText(legendText(s, p.ShowData), t.FontSize)
		if w > maxLegend {
			maxLegend = w
		}
	}
	width := legendX + pieSwatch + 6 + maxLegend + pieMargin
	height := cy + pieR + pieMargin
	if lh := titleH + pieMargin + float64(len(p.Slices))*pieRowH + pieMargin; lh > height {
		height = lh
	}

	var b bytes.Buffer
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" class="mermaid-svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="%s" font-size="%.0f">`,
		width, height, width, height, html.EscapeString(t.FontFamily), t.FontSize)
	if p.Title != "" {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-weight="bold" fill="%s">%s</text>`,
			cx, pieMargin+t.FontSize, t.Text, html.EscapeString(p.Title))
	}

	angle := 0.0 // radians from 12 o'clock, clockwise
	for i, s := range p.Slices {
		frac := s.Value / total
		color := t.Palette[i%len(t.Palette)]
		if frac >= 1-1e-9 { // single-slice pie: full circle, arc would degenerate
			fmt.Fprintf(&b, `<circle class="pie-slice" cx="%.1f" cy="%.1f" r="%.1f" fill="%s" stroke="%s" stroke-width="2"/>`,
				cx, cy, pieR, color, t.NodeFill)
		} else if frac > 0 {
			a0, a1 := angle, angle+frac*2*math.Pi
			fmt.Fprintf(&b, `<path class="pie-slice" d="%s" fill="%s" stroke="%s" stroke-width="2"/>`,
				arcPath(cx, cy, pieR, a0, a1), color, t.NodeFill)
		}
		if frac >= 0.05 {
			mid := angle + frac*math.Pi
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%.0f%%</text>`,
				cx+0.6*pieR*math.Sin(mid), cy-0.6*pieR*math.Cos(mid), t.Text, math.Round(frac*100))
		}
		angle += frac * 2 * math.Pi
	}

	ly := titleH + pieMargin
	for i, s := range p.Slices {
		fmt.Fprintf(&b, `<rect class="pie-legend" x="%.1f" y="%.1f" width="%.0f" height="%.0f" fill="%s"/>`,
			legendX, ly, pieSwatch, pieSwatch, t.Palette[i%len(t.Palette)])
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" dominant-baseline="central" fill="%s">%s</text>`,
			legendX+pieSwatch+6, ly+pieSwatch/2, t.Text, html.EscapeString(legendText(s, p.ShowData)))
		ly += pieRowH
	}

	b.WriteString(`</svg>`)
	return b.Bytes()
}

func legendText(s pieSlice, showData bool) string {
	if showData {
		return fmt.Sprintf("%s [%s]", s.Label, strconv.FormatFloat(s.Value, 'f', -1, 64))
	}
	return s.Label
}

// arcPath draws a filled wedge; angles are radians from 12 o'clock, clockwise.
func arcPath(cx, cy, r, a0, a1 float64) string {
	x0, y0 := cx+r*math.Sin(a0), cy-r*math.Cos(a0)
	x1, y1 := cx+r*math.Sin(a1), cy-r*math.Cos(a1)
	large := 0
	if a1-a0 > math.Pi {
		large = 1
	}
	return fmt.Sprintf("M %.1f %.1f L %.1f %.1f A %.1f %.1f 0 %d 1 %.1f %.1f Z", cx, cy, x0, y0, r, r, large, x1, y1)
}
```

`mermaid/mermaid.go` — add to `Render`'s switch:

```go
	case "pie":
		p, err := parsePie(rest)
		if err != nil {
			return nil, err
		}
		return emitPie(p, theme), nil
```

- [ ] **Step 4: Golden + integration test**

```bash
cat > mermaid/testdata/pie-basic.mmd <<'EOF'
pie showData
title Languages
"Go" : 60
"JS" : 25
"Other" : 15
EOF
```

Add to `render_test.go` (root package):

```go
func TestRenderBodyNativePie(t *testing.T) {
	out, fallback, err := RenderBody([]byte("```mermaid\npie\ntitle T\n\"a\" : 1\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<svg") || fallback {
		t.Fatalf("pie must render natively (fallback=%v): %s", fallback, out)
	}
}
```

- [ ] **Step 5: Run everything, regenerate goldens**

Run: `go test ./mermaid/ -update && go test ./... -race`
Expected: PASS. Textually sanity-check `pie-basic.svg`: 3 `pie-slice` paths, legend rows with `[60]`-style values, title.

- [ ] **Step 6: Commit**

```bash
gofmt -w . && go vet ./...
git add mermaid/pie.go mermaid/pie_test.go mermaid/theme.go mermaid/mermaid.go mermaid/testdata render_test.go
git commit -m "Render pie charts natively with themed palette and legend"
```

---

### Task 2: State diagrams end-to-end

**Files:**
- Create: `mermaid/state.go`
- Modify: `mermaid/ir.go` (two Shape consts), `mermaid/text.go` (pseudo-state sizing), `mermaid/svg.go` (two shape cases + skip empty labels), `mermaid/mermaid.go` (dispatch)
- Modify: `render_test.go` (integration test)
- Create: `mermaid/state_test.go`, `mermaid/testdata/state-basic.mmd` (+ golden)

**Interfaces:**
- Consumes: flowchart `Graph`/`Node`/`Edge`/`Subgraph` IR, `measureGraph`, `layout`, `emit`, `unsup`.
- Produces: `func parseState(src string) (*Graph, error)`; `ShapeStateStart`, `ShapeStateEnd` appended to the `Shape` const block.

- [ ] **Step 1: Write the failing tests**

Create `mermaid/state_test.go`:

```go
package mermaid

import (
	"errors"
	"strings"
	"testing"
)

func mustParseState(t *testing.T, src string) *Graph {
	t.Helper()
	g, err := parseState(src)
	if err != nil {
		t.Fatalf("parseState: %v", err)
	}
	return g
}

func TestStateParseTransitionsAndPseudo(t *testing.T) {
	g := mustParseState(t, `stateDiagram-v2
[*] --> Idle
Idle --> Running : start
Running --> Idle : stop
Running --> [*]
[*] --> Off`)
	// one shared start node, one shared end node at top level
	var starts, ends int
	for _, n := range g.Nodes {
		switch n.Shape {
		case ShapeStateStart:
			starts++
			if n.Label != "" {
				t.Fatalf("start pseudo-state must be unlabeled: %+v", n)
			}
		case ShapeStateEnd:
			ends++
		}
	}
	if starts != 1 || ends != 1 {
		t.Fatalf("starts=%d ends=%d, want 1/1 (shared per scope)", starts, ends)
	}
	if len(g.Edges) != 5 {
		t.Fatalf("edges %d, want 5", len(g.Edges))
	}
	var labeled *Edge
	for _, e := range g.Edges {
		if e.Label == "start" {
			labeled = e
		}
		if !e.Directed {
			t.Fatalf("state transitions are directed: %+v", e)
		}
	}
	if labeled == nil || labeled.From != "Idle" || labeled.To != "Running" {
		t.Fatalf("labeled transition wrong: %+v", labeled)
	}
	if g.node("Idle").Shape != ShapeRound {
		t.Fatalf("states render rounded: %+v", g.node("Idle"))
	}
	if g.Direction != "TB" {
		t.Fatalf("default direction %q, want TB", g.Direction)
	}
}

func TestStateParseDeclsAndDescriptions(t *testing.T) {
	g := mustParseState(t, `stateDiagram-v2
state "Long Name" as s1
s2 : A description
s1 --> s2`)
	if g.node("s1").Label != "Long Name" {
		t.Fatalf("decl label: %+v", g.node("s1"))
	}
	if g.node("s2").Label != "A description" {
		t.Fatalf("description label: %+v", g.node("s2"))
	}
}

func TestStateParseComposite(t *testing.T) {
	g := mustParseState(t, `stateDiagram-v2
state Recovery {
  Detect --> Repair
  [*] --> Detect
}
Running --> Detect : fault`)
	if len(g.Subgraphs) != 1 || g.Subgraphs[0].Title != "Recovery" {
		t.Fatalf("subgraph: %+v", g.Subgraphs)
	}
	sg := g.Subgraphs[0]
	if len(sg.Children) != 3 { // Detect, Repair, scoped start
		t.Fatalf("children %v, want 3", sg.Children)
	}
	// composite scope gets its own start node, distinct from any top-level one
	var comp *Node
	for _, n := range g.Nodes {
		if n.Shape == ShapeStateStart {
			comp = n
		}
	}
	if comp == nil || !contains(sg.Children, comp.ID) {
		t.Fatalf("composite-scoped start missing: %+v", comp)
	}
}

func TestStateParseDirection(t *testing.T) {
	g := mustParseState(t, "stateDiagram-v2\ndirection LR\na --> b")
	if g.Direction != "LR" {
		t.Fatalf("direction %q", g.Direction)
	}
}

func TestStateParseUnsupported(t *testing.T) {
	for _, src := range []string{
		"stateDiagram-v2\nstate fork1 <<fork>>",
		"stateDiagram-v2\nnote right of A: hi",
		"stateDiagram-v2\nstate A {\nstate B {\nc --> d\n}\n}",
		"stateDiagram-v2\nstate A {\ndirection LR\na --> b\n}",
		"stateDiagram-v2\nstate A {\na --> b",
		"stateDiagram-v2\nA --> B --> C",
		"stateDiagram-v2\nwhat is this",
	} {
		if _, err := parseState(src); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: want ErrUnsupported, got %v", src, err)
		}
	}
}

func TestStateSVG(t *testing.T) {
	out, err := Render([]byte("stateDiagram-v2\n[*] --> A\nA --> [*]"), Light)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<svg") {
		t.Fatalf("no svg: %s", s)
	}
	// start dot: one filled circle with NodeStroke fill; end: double circle
	if c := strings.Count(s, `class="state-start"`); c != 1 {
		t.Fatalf("start dots %d, want 1\n%s", c, s)
	}
	if c := strings.Count(s, `class="state-end"`); c != 1 {
		t.Fatalf("end doubles %d, want 1\n%s", c, s)
	}
	// pseudo-states must not emit empty <text> labels
	if strings.Contains(s, `></text>`) {
		t.Fatalf("empty label text emitted:\n%s", s)
	}
	if _, err := Render([]byte("stateDiagram\na --> b"), Light); err != nil {
		t.Fatalf("v1 keyword must dispatch too: %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./mermaid/ -run 'TestState'`
Expected: FAIL — `undefined: parseState`, `ShapeStateStart`.

- [ ] **Step 3: Implement**

`mermaid/ir.go` — extend the Shape const block:

```go
	ShapeStateStart // filled dot ([*] as source)
	ShapeStateEnd   // double circle ([*] as target)
```

`mermaid/text.go` — in `measureGraph`'s shape switch add:

```go
		case ShapeStateStart, ShapeStateEnd:
			n.W, n.H = 18, 18
```

(placed so it overrides the text-based sizing — restructure the loop as: measure text first, then the shape switch adjusts, with the pseudo-state case REPLACING W/H entirely.)

`mermaid/svg.go` — in `emit`'s node switch add:

```go
		case ShapeStateStart:
			fmt.Fprintf(&b, `<circle class="state-start" cx="%.1f" cy="%.1f" r="7" fill="%s"/>`, n.X, n.Y, t.NodeStroke)
		case ShapeStateEnd:
			fmt.Fprintf(&b, `<circle class="state-end" cx="%.1f" cy="%.1f" r="8" fill="none" stroke="%s" stroke-width="1.5"/><circle cx="%.1f" cy="%.1f" r="4.5" fill="%s"/>`,
				n.X, n.Y, t.NodeStroke, n.X, n.Y, t.NodeStroke)
```

and wrap the node-label `<text>` emission in `if n.Label != "" { ... }`.

Create `mermaid/state.go`:

```go
package mermaid

import (
	"regexp"
	"strings"
)

var (
	stateTransRe = regexp.MustCompile(`^(\[\*\]|[A-Za-z0-9_.-]+)\s*-->\s*(\[\*\]|[A-Za-z0-9_.-]+)(?:\s*:\s*(.*))?$`)
	stateDeclRe  = regexp.MustCompile(`^state\s+"([^"]*)"\s+as\s+([A-Za-z0-9_.-]+)$`)
	stateCompRe  = regexp.MustCompile(`^state\s+([A-Za-z0-9_.-]+)\s*\{$`)
	stateDescRe  = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*:\s*(.*)$`)
	stateDirRe   = regexp.MustCompile(`^direction\s+(TB|LR)$`)
)

// parseState builds a flowchart Graph from stateDiagram/-v2 source: states are
// rounded nodes, [*] pseudo-states are shared per scope, composites are
// subgraphs. Layout and SVG come from the flowchart pipeline.
func parseState(src string) (*Graph, error) {
	g := &Graph{Direction: "TB"}
	var cur *Subgraph
	scope := "" // suffix for pseudo-state ids, "" at top level

	ensure := func(id, label string, shape Shape) *Node {
		n := g.node(id)
		if n == nil {
			n = &Node{ID: id, Label: label, Shape: shape}
			g.Nodes = append(g.Nodes, n)
		}
		if cur != nil && !contains(cur.Children, id) {
			cur.Children = append(cur.Children, id)
		}
		return n
	}
	endpoint := func(tok string, source bool) string {
		if tok != "[*]" {
			ensure(tok, tok, ShapeRound)
			return tok
		}
		if source {
			id := "__start" + scope
			ensure(id, "", ShapeStateStart)
			return id
		}
		id := "__end" + scope
		ensure(id, "", ShapeStateEnd)
		return id
	}

	seenHeader := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !seenHeader {
			if line != "stateDiagram-v2" && line != "stateDiagram" {
				return nil, unsup("bad header %q", line)
			}
			seenHeader = true
			continue
		}

		switch {
		case stateDirRe.MatchString(line):
			if cur != nil {
				return nil, unsup("direction inside composite")
			}
			g.Direction = stateDirRe.FindStringSubmatch(line)[1]

		case stateCompRe.MatchString(line):
			if cur != nil {
				return nil, unsup("nested composite state")
			}
			name := stateCompRe.FindStringSubmatch(line)[1]
			cur = &Subgraph{ID: "sg_" + name, Title: name}
			scope = "_" + name
			g.Subgraphs = append(g.Subgraphs, cur)
			// the composite is also a referencable state? v1: container only.

		case line == "}":
			if cur == nil {
				return nil, unsup("} without composite")
			}
			cur, scope = nil, ""

		case stateDeclRe.MatchString(line):
			m := stateDeclRe.FindStringSubmatch(line)
			ensure(m[2], m[1], ShapeRound).Label = m[1]

		case stateTransRe.MatchString(line):
			m := stateTransRe.FindStringSubmatch(line)
			from := endpoint(m[1], true)
			to := endpoint(m[2], false)
			g.Edges = append(g.Edges, &Edge{
				From: from, To: to, Label: strings.TrimSpace(m[3]),
				Style: EdgeSolid, Directed: true,
			})

		case stateDescRe.MatchString(line):
			m := stateDescRe.FindStringSubmatch(line)
			ensure(m[1], m[1], ShapeRound).Label = strings.TrimSpace(m[2])

		default:
			return nil, unsup("state statement %q", line)
		}
	}
	if !seenHeader {
		return nil, unsup("empty diagram")
	}
	if cur != nil {
		return nil, unsup("unclosed composite state")
	}
	return g, nil
}
```

ORDERING NOTE: `stateTransRe` must be tried BEFORE `stateDescRe` in the switch (as written) — `A --> B : label` would otherwise never reach the transition arm... actually `stateDescRe`'s id class can't contain spaces so it cannot match a transition; the order above is still the safe one. Also note `state X {` must be tried before `stateDescRe` (`state Foo : ...` is not in the subset and should fall to default → unsup). The switch order in the code block is the required order.

`mermaid/mermaid.go` — add:

```go
	case "stateDiagram", "stateDiagram-v2":
		g, err := parseState(rest)
		if err != nil {
			return nil, err
		}
		measureGraph(g, theme)
		if err := layout(g); err != nil {
			return nil, err
		}
		return emit(g, theme), nil
```

- [ ] **Step 4: Golden + integration test**

```bash
cat > mermaid/testdata/state-basic.mmd <<'EOF'
stateDiagram-v2
[*] --> Idle
Idle --> Running : start
Running --> Idle : stop
Running --> [*]
state Recovery {
  Detect --> Repair
}
Running --> Detect : fault
EOF
```

Add to `render_test.go`:

```go
func TestRenderBodyNativeState(t *testing.T) {
	out, fallback, err := RenderBody([]byte("```mermaid\nstateDiagram-v2\n[*] --> A\nA --> [*]\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<svg") || fallback {
		t.Fatalf("state must render natively (fallback=%v): %s", fallback, out)
	}
}
```

- [ ] **Step 5: Run everything, regenerate goldens**

Run: `go test ./mermaid/ -update && go test ./... -race`
Expected: PASS. Sanity-check `state-basic.svg`: start dot, double-circle end, `Recovery` subgraph box, labeled curved edges.

- [ ] **Step 6: Commit**

```bash
gofmt -w . && go vet ./...
git add mermaid/state.go mermaid/state_test.go mermaid/ir.go mermaid/text.go mermaid/svg.go mermaid/mermaid.go mermaid/testdata render_test.go
git commit -m "Render state diagrams natively via the flowchart pipeline"
```

---

### Task 3 (controller): README + screenshot acceptance

- Update README's mermaid bullet: "common flowcharts, sequence, pie, and state diagrams render natively…".
- Extend the comparison fixture with a pie and a state diagram; re-run the four-panel headless-Chrome comparison (native/js × light/dark); eyeball slice order/percentages against mermaid.js and the state pseudo-states/composite; update the comparison page. Commit README.

---

## Self-Review

**Spec coverage:** pie subset/order/percent-threshold/single-slice/legend/showData/palette-with-8-per-theme/escaping (Task 1, each tested); state subset incl. shared-per-scope `[*]`, decls/descriptions, one-level composites, direction, unsupported table (Task 2, tested); pseudo-state dressing + empty-label suppression (Task 2 svg test); dispatch both keywords (Task 2 test); goldens + integration both types (Tasks 1–2); acceptance + README (Task 3). Fallback fixtures unaffected (gantt/classDiagram remain unsupported). ✓

**Placeholder scan:** one intentional inline design note ("container only" comment) — a statement of v1 scope, not a TBD. Clean otherwise.

**Type consistency:** `parsePie(string) (*pieChart, error)`, `emitPie(*pieChart, Theme) []byte`, `Theme.Palette`, `parseState(string) (*Graph, error)`, `ShapeStateStart/End` — names consistent across tasks; `contains` helper already exists in parse.go.
