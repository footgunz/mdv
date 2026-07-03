# Native Mermaid SVG Engine (Core + Flowchart) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `mermaid/` package that renders the common flowchart subset to SVG server-side (parse → IR → dagre-in-goja layout → SVG), wired into the viewer with silent fallback to mermaid.js.

**Architecture:** Pipeline in one new package: `parse.go` builds an IR from flowchart source, `text.go` sizes labels with embedded font metrics, `layout.go` positions the IR by running vendored `dagre.js` inside goja (pure-Go JS interpreter, no DOM), `svg.go` emits themed SVG. `render.go` tries the engine first and falls back to the existing `<pre class="mermaid">` + mermaid.js path on any error; pages load mermaid.min.js only if at least one block fell back.

**Tech Stack:** Go, `github.com/dop251/goja` (MIT), `golang.org/x/image` (BSD, font metrics + embedded goregular TTF), vendored `dagre.js` 0.8.5 (MIT).

## Global Constraints

- No engine error may ever break a page: every failure path returns an error and the caller falls back to mermaid.js silently.
- Unsupported syntax → `mermaid.ErrUnsupported` (or a parse error), never a wrong-looking render.
- Concurrent `Render` calls must be safe (the HTTP server renders per-request). goja VMs are NOT goroutine-safe.
- No GPL code. Do not use or copy from TyphonHill/go-mermaid. goja/dagre/x-image only.
- Pixel-identical mermaid.js output is a non-goal; tests assert structure and invariants, not exact coordinates (goldens excepted, reviewed by eye).
- Existing behavior for unsupported/malformed diagrams must remain byte-identical (`<pre class="mermaid">` escaped source).
- Module: `github.com/dgunther/mdthing`; engine package `github.com/dgunther/mdthing/mermaid`.
- Branch: work happens on `mermaid-svg-engine` (already created).

**If plan code conflicts with plan tests, the tests govern** — fix the implementation, keep the test, note the deviation in your report.

---

### Task 1: Package skeleton — IR, themes, API, diagram detection

**Files:**
- Create: `mermaid/mermaid.go`
- Create: `mermaid/ir.go`
- Create: `mermaid/theme.go`
- Test: `mermaid/mermaid_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (relied on by every later task):
  - `func Render(src []byte, theme Theme) ([]byte, error)` — public API.
  - `var ErrUnsupported error`
  - `type Theme struct { NodeFill, NodeStroke, EdgeStroke, Text, SubgraphFill, FontFamily string; FontSize float64 }`; `var Light, Dark Theme`.
  - IR: `type Graph struct { Direction string; Nodes []*Node; Edges []*Edge; Subgraphs []*Subgraph; Width, Height float64 }`, `type Node struct { ID, Label string; Shape Shape; W, H, X, Y float64 }` (X,Y = center), `type Shape int` with `ShapeRect, ShapeRound, ShapeStadium, ShapeDiamond, ShapeCircle`, `type Edge struct { From, To, Label string; Style EdgeStyle; Directed bool; Points []Point; LabelX, LabelY, LabelW, LabelH float64 }`, `type EdgeStyle int` with `EdgeSolid, EdgeDotted, EdgeThick`, `type Subgraph struct { ID, Title string; Children []string; X, Y, W, H float64 }`, `type Point struct { X, Y float64 }`.
  - `func detect(src string) (kind string, rest string)` — kind is the first keyword (`graph`, `flowchart`, `pie`, ...); rest is the source from the header line on.

- [ ] **Step 1: Write the failing test**

Create `mermaid/mermaid_test.go`:

```go
package mermaid

import (
	"errors"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		src  string
		kind string
	}{
		{"graph TD\nA-->B", "graph"},
		{"flowchart LR\nA-->B", "flowchart"},
		{"%% comment\n\nflowchart TD\nA", "flowchart"},
		{"pie\n\"a\": 1", "pie"},
		{"sequenceDiagram\nA->>B: hi", "sequenceDiagram"},
		{"", ""},
	}
	for _, tc := range cases {
		kind, _ := detect(tc.src)
		if kind != tc.kind {
			t.Fatalf("detect(%q) = %q, want %q", tc.src, kind, tc.kind)
		}
	}
}

func TestRenderUnsupportedKind(t *testing.T) {
	_, err := Render([]byte("sequenceDiagram\nA->>B: hi"), Light)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	_, err = Render([]byte(""), Light)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("empty: want ErrUnsupported, got %v", err)
	}
}

func TestThemes(t *testing.T) {
	if Light.NodeFill == "" || Dark.NodeFill == "" || Light.NodeFill == Dark.NodeFill {
		t.Fatalf("themes must differ and be populated: %+v %+v", Light, Dark)
	}
	if Light.FontSize <= 0 {
		t.Fatalf("font size unset")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./mermaid/`
Expected: FAIL — package doesn't exist / undefined symbols.

- [ ] **Step 3: Implement**

`mermaid/ir.go`:

```go
package mermaid

// Intermediate representation of a parsed diagram. parse fills identity and
// labels; text measurement fills W/H; layout fills X/Y and edge geometry.

type Shape int

const (
	ShapeRect Shape = iota
	ShapeRound
	ShapeStadium
	ShapeDiamond
	ShapeCircle
)

type EdgeStyle int

const (
	EdgeSolid EdgeStyle = iota
	EdgeDotted
	EdgeThick
)

type Point struct{ X, Y float64 }

type Node struct {
	ID, Label string
	Shape     Shape
	W, H      float64 // set by measurement
	X, Y      float64 // center, set by layout
}

type Edge struct {
	From, To, Label        string
	Style                  EdgeStyle
	Directed               bool
	Points                 []Point // polyline, set by layout
	LabelX, LabelY         float64
	LabelW, LabelH         float64
}

type Subgraph struct {
	ID, Title string
	Children  []string // node IDs
	X, Y, W, H float64 // top-left + size, set by layout
}

type Graph struct {
	Direction     string // TD, TB, LR, RL, BT
	Nodes         []*Node
	Edges         []*Edge
	Subgraphs     []*Subgraph
	Width, Height float64 // set by layout
}

func (g *Graph) node(id string) *Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}
```

`mermaid/theme.go`:

```go
package mermaid

// Theme colors track assets/base.css (light) and its body.dark block so
// native diagrams match the page.
type Theme struct {
	NodeFill, NodeStroke, EdgeStroke, Text, SubgraphFill, FontFamily string
	FontSize                                                         float64
}

var Light = Theme{
	NodeFill:     "#f6f8fa",
	NodeStroke:   "#57606a",
	EdgeStroke:   "#57606a",
	Text:         "#24292f",
	SubgraphFill: "#eaecef",
	FontFamily:   `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	FontSize:     14,
}

var Dark = Theme{
	NodeFill:     "#161b22",
	NodeStroke:   "#8b949e",
	EdgeStroke:   "#8b949e",
	Text:         "#c9d1d9",
	SubgraphFill: "#21262d",
	FontFamily:   `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	FontSize:     14,
}
```

`mermaid/mermaid.go`:

```go
// Package mermaid renders a subset of mermaid diagrams to SVG without a
// browser: parse -> IR -> dagre layout (in goja) -> SVG. Anything outside
// the subset returns ErrUnsupported so callers can fall back to mermaid.js.
package mermaid

import (
	"errors"
	"strings"
)

var ErrUnsupported = errors.New("mermaid: unsupported diagram")

// Render converts mermaid source to a themed SVG document fragment.
func Render(src []byte, theme Theme) ([]byte, error) {
	kind, rest := detect(string(src))
	switch kind {
	case "graph", "flowchart":
		g, err := parseFlowchart(rest)
		if err != nil {
			return nil, err
		}
		measureGraph(g, theme)
		if err := layout(g); err != nil {
			return nil, err
		}
		return emit(g, theme), nil
	default:
		return nil, ErrUnsupported
	}
}

// detect returns the first keyword of the first significant line and the
// source starting at that line.
func detect(src string) (string, string) {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "%%") {
			continue
		}
		return strings.Fields(s)[0], strings.Join(lines[i:], "\n")
	}
	return "", ""
}
```

To compile before Tasks 2–5 exist, add temporary stubs at the bottom of `mermaid/mermaid.go` (each later task deletes its stub when implementing the real file):

```go
// Stubs replaced by Tasks 2 (parse.go), 3 (text.go), 4 (layout.go), 5 (svg.go).
func parseFlowchart(src string) (*Graph, error) { return nil, ErrUnsupported }
func measureGraph(g *Graph, t Theme)            {}
func layout(g *Graph) error                     { return ErrUnsupported }
func emit(g *Graph, t Theme) []byte             { return nil }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./mermaid/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add mermaid/
git commit -m "Add mermaid package skeleton: IR, themes, API, detection"
```

---

### Task 2: Flowchart parser

**Files:**
- Create: `mermaid/parse.go` (delete the `parseFlowchart` stub from `mermaid/mermaid.go`)
- Test: `mermaid/parse_test.go`

**Interfaces:**
- Consumes: IR types from Task 1.
- Produces: `func parseFlowchart(src string) (*Graph, error)` — src starts at the `graph`/`flowchart` header line. Returns `ErrUnsupported` (wrapped ok) for out-of-subset syntax.

Subset (from the spec): directions TD/TB/LR/RL/BT; shapes `[r]` `(r)` `([s])` `{d}` `((c))` + bare id; quoted labels; edges `-->` `---` `-.->` `==>` with `|label|` or `-- label -->`; chains `a-->b-->c`; fan-out `a-->b & c`; one-plus level `subgraph [title] ... end`; `%%` comments; `;` separators; `classDef/class/linkStyle/style` lines parsed-and-ignored. `click`, `direction` inside subgraph, and anything unrecognized → error.

- [ ] **Step 1: Write the failing test**

Create `mermaid/parse_test.go`:

```go
package mermaid

import (
	"errors"
	"testing"
)

func mustParse(t *testing.T, src string) *Graph {
	t.Helper()
	g, err := parseFlowchart(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return g
}

func TestParseHeaderDirections(t *testing.T) {
	for _, d := range []string{"TD", "TB", "LR", "RL", "BT"} {
		g := mustParse(t, "graph "+d+"\nA-->B")
		if g.Direction != d {
			t.Fatalf("direction %q, want %q", g.Direction, d)
		}
	}
	g := mustParse(t, "flowchart TD\nA")
	if g.Direction != "TD" {
		t.Fatal("flowchart keyword must work")
	}
	if _, err := parseFlowchart("graph XX\nA"); err == nil {
		t.Fatal("bad direction must error")
	}
}

func TestParseShapes(t *testing.T) {
	g := mustParse(t, `graph TD
a[Rect Label]
b(Round)
c([Stadium])
d{Diamond?}
e((Circle))
f`)
	want := map[string]struct {
		shape Shape
		label string
	}{
		"a": {ShapeRect, "Rect Label"},
		"b": {ShapeRound, "Round"},
		"c": {ShapeStadium, "Stadium"},
		"d": {ShapeDiamond, "Diamond?"},
		"e": {ShapeCircle, "Circle"},
		"f": {ShapeRect, "f"},
	}
	if len(g.Nodes) != len(want) {
		t.Fatalf("node count %d, want %d", len(g.Nodes), len(want))
	}
	for id, w := range want {
		n := g.node(id)
		if n == nil || n.Shape != w.shape || n.Label != w.label {
			t.Fatalf("node %s = %+v, want %+v", id, n, w)
		}
	}
}

func TestParseQuotedLabel(t *testing.T) {
	g := mustParse(t, "graph TD\na[\"has [brackets] inside\"]")
	if g.node("a").Label != "has [brackets] inside" {
		t.Fatalf("got %q", g.node("a").Label)
	}
}

func TestParseEdges(t *testing.T) {
	g := mustParse(t, `graph LR
a --> b
b --- c
c -.-> d
d ==> e
e -->|yes| f
f -- no --> a`)
	type ew struct {
		from, to, label string
		style           EdgeStyle
		directed        bool
	}
	want := []ew{
		{"a", "b", "", EdgeSolid, true},
		{"b", "c", "", EdgeSolid, false},
		{"c", "d", "", EdgeDotted, true},
		{"d", "e", "", EdgeThick, true},
		{"e", "f", "yes", EdgeSolid, true},
		{"f", "a", "no", EdgeSolid, true},
	}
	if len(g.Edges) != len(want) {
		t.Fatalf("edge count %d, want %d", len(g.Edges), len(want))
	}
	for i, w := range want {
		e := g.Edges[i]
		if e.From != w.from || e.To != w.to || e.Label != w.label ||
			e.Style != w.style || e.Directed != w.directed {
			t.Fatalf("edge %d = %+v, want %+v", i, e, w)
		}
	}
}

func TestParseChainAndFanout(t *testing.T) {
	g := mustParse(t, "graph TD\na --> b --> c\nx --> y & z")
	if len(g.Edges) != 4 {
		t.Fatalf("edges %d, want 4 (a->b, b->c, x->y, x->z)", len(g.Edges))
	}
	if g.Edges[1].From != "b" || g.Edges[1].To != "c" {
		t.Fatalf("chain second hop wrong: %+v", g.Edges[1])
	}
	if g.Edges[3].From != "x" || g.Edges[3].To != "z" {
		t.Fatalf("fanout wrong: %+v", g.Edges[3])
	}
}

func TestParseEdgeDefinesNodeWithShape(t *testing.T) {
	g := mustParse(t, "graph TD\na[Start] --> b{Choice}")
	if g.node("a").Shape != ShapeRect || g.node("a").Label != "Start" {
		t.Fatalf("a = %+v", g.node("a"))
	}
	if g.node("b").Shape != ShapeDiamond || g.node("b").Label != "Choice" {
		t.Fatalf("b = %+v", g.node("b"))
	}
	// re-mention without shape must not reset the label
	g = mustParse(t, "graph TD\na[Start] --> b\na --> c")
	if g.node("a").Label != "Start" {
		t.Fatalf("label reset on re-mention: %+v", g.node("a"))
	}
}

func TestParseSubgraph(t *testing.T) {
	g := mustParse(t, `graph TD
subgraph one [Group One]
  a --> b
end
subgraph two
  c
end
a --> c`)
	if len(g.Subgraphs) != 2 {
		t.Fatalf("subgraphs %d, want 2", len(g.Subgraphs))
	}
	s := g.Subgraphs[0]
	if s.Title != "Group One" || len(s.Children) != 2 {
		t.Fatalf("subgraph one = %+v", s)
	}
	if g.Subgraphs[1].Title != "two" {
		t.Fatalf("untitled subgraph should use id as title: %+v", g.Subgraphs[1])
	}
}

func TestParseIgnoredAndComments(t *testing.T) {
	g := mustParse(t, `graph TD
%% a comment
a --> b;
classDef red fill:#f00
class a red
style a fill:#f9f
linkStyle 0 stroke:#f00`)
	if len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("ignored statements leaked: %d nodes %d edges", len(g.Nodes), len(g.Edges))
	}
}

func TestParseUnsupported(t *testing.T) {
	for _, src := range []string{
		"graph TD\nclick a href \"x\"",
		"graph TD\nsubgraph s\ndirection LR\nend",
		"graph TD\na ==>>> b",
		"graph TD\na --> b\nwhatisthis ???",
	} {
		if _, err := parseFlowchart(src); err == nil {
			t.Fatalf("want error for %q", src)
		} else if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: want ErrUnsupported, got %v", src, err)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./mermaid/ -run TestParse`
Expected: FAIL — stub returns `ErrUnsupported` for everything.

- [ ] **Step 3: Implement `mermaid/parse.go`** (and delete the stub in `mermaid.go`)

```go
package mermaid

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	headerRe = regexp.MustCompile(`^(graph|flowchart)\s+(TD|TB|LR|RL|BT)\s*$`)
	// -- label --> rewritten to -->|label| before splitting
	inlineLabelRe = regexp.MustCompile(`(--|==|-\.)\s+([^-=>|]+?)\s+(-->|---|==>|-\.->)`)
	edgeOpRe      = regexp.MustCompile(`\s*(-->|---|-\.->|==>)\s*`)
	labelPipeRe   = regexp.MustCompile(`^\|([^|]*)\|\s*`)
	nodeRe        = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*(\(\(|\(\[|\[|\(|\{)?`)
	subgraphRe    = regexp.MustCompile(`^subgraph\s+([A-Za-z0-9_.-]+)(?:\s*\[(.+)\])?\s*$`)
	ignoreRe      = regexp.MustCompile(`^(classDef|class|linkStyle|style)\b`)
	unsupportedRe = regexp.MustCompile(`^(click|direction)\b`)
)

func unsup(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnsupported}, a...)...)
}

func parseFlowchart(src string) (*Graph, error) {
	lines := strings.Split(src, "\n")
	g := &Graph{}
	var cur *Subgraph // current subgraph, nil at top level

	seenHeader := false
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if i := strings.Index(line, "%%"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		line = strings.TrimSuffix(line, ";")
		if line == "" {
			continue
		}
		if !seenHeader {
			m := headerRe.FindStringSubmatch(line)
			if m == nil {
				return nil, unsup("bad header %q", line)
			}
			g.Direction = m[2]
			seenHeader = true
			continue
		}
		if unsupportedRe.MatchString(line) {
			return nil, unsup("directive %q", line)
		}
		if ignoreRe.MatchString(line) {
			continue // styling statements: parsed-and-ignored in v1
		}
		if m := subgraphRe.FindStringSubmatch(line); m != nil {
			if cur != nil {
				return nil, unsup("nested subgraph") // v1: one level
			}
			title := m[2]
			if title == "" {
				title = m[1]
			}
			cur = &Subgraph{ID: "sg_" + m[1], Title: strings.TrimSpace(title)}
			g.Subgraphs = append(g.Subgraphs, cur)
			continue
		}
		if line == "end" {
			if cur == nil {
				return nil, unsup("end without subgraph")
			}
			cur = nil
			continue
		}
		if err := parseStatement(g, cur, line); err != nil {
			return nil, err
		}
	}
	if !seenHeader {
		return nil, unsup("empty diagram")
	}
	return g, nil
}

// parseStatement handles one node/edge line: a[X] --> b & c --> d ...
func parseStatement(g *Graph, cur *Subgraph, line string) error {
	// normalize `-- label -->` to `-->|label|`
	line = inlineLabelRe.ReplaceAllString(line, "$3|$2|")

	ops := edgeOpRe.FindAllStringSubmatch(line, -1)
	segs := edgeOpRe.Split(line, -1)
	if len(segs) != len(ops)+1 {
		return unsup("cannot parse %q", line)
	}

	prev, err := parseNodeList(g, cur, segs[0])
	if err != nil {
		return err
	}
	for i, op := range ops {
		seg := segs[i+1]
		label := ""
		if m := labelPipeRe.FindStringSubmatch(seg); m != nil {
			label = strings.TrimSpace(m[1])
			seg = seg[len(m[0]):]
		}
		next, err := parseNodeList(g, cur, seg)
		if err != nil {
			return err
		}
		style, directed := EdgeSolid, true
		switch op[1] {
		case "---":
			directed = false
		case "-.->":
			style = EdgeDotted
		case "==>":
			style = EdgeThick
		}
		for _, f := range prev {
			for _, t := range next {
				g.Edges = append(g.Edges, &Edge{
					From: f, To: t, Label: label, Style: style, Directed: directed,
				})
			}
		}
		prev = next
	}
	return nil
}

// parseNodeList parses `a[X]` or `a & b & c`, registering nodes; returns ids.
func parseNodeList(g *Graph, cur *Subgraph, s string) ([]string, error) {
	var ids []string
	for _, part := range strings.Split(s, "&") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, unsup("empty node in %q", s)
		}
		id, err := parseNode(g, cur, part)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

var shapeClose = map[string]struct {
	close string
	shape Shape
}{
	"((": {"))", ShapeCircle},
	"([": {"])", ShapeStadium},
	"[":  {"]", ShapeRect},
	"(":  {")", ShapeRound},
	"{":  {"}", ShapeDiamond},
}

func parseNode(g *Graph, cur *Subgraph, s string) (string, error) {
	m := nodeRe.FindStringSubmatch(s)
	if m == nil {
		return "", unsup("bad node %q", s)
	}
	id, open := m[1], m[2]
	rest := s[len(m[0]):]

	label, shape, hasShape := id, ShapeRect, false
	if open != "" {
		sc, ok := shapeClose[open]
		if !ok || !strings.HasSuffix(rest, sc.close) {
			return "", unsup("bad node syntax %q", s)
		}
		label = strings.TrimSpace(strings.TrimSuffix(rest, sc.close))
		label = strings.Trim(label, `"`)
		shape, hasShape = sc.shape, true
	} else if rest != "" {
		return "", unsup("trailing junk in node %q", s)
	}

	n := g.node(id)
	if n == nil {
		n = &Node{ID: id, Label: label, Shape: shape}
		g.Nodes = append(g.Nodes, n)
	} else if hasShape {
		n.Label, n.Shape = label, shape
	}
	if cur != nil && !contains(cur.Children, id) {
		cur.Children = append(cur.Children, id)
	}
	return id, nil
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./mermaid/ -run TestParse`
Expected: PASS. If a test fails, the tests govern — fix the implementation, note the deviation in your report.

- [ ] **Step 5: Run the whole package + commit**

Run: `go test ./mermaid/`
Expected: PASS (Task 1 tests still green).

```bash
git add mermaid/
git commit -m "Add flowchart subset parser"
```

---

### Task 3: Text measurement

**Files:**
- Create: `mermaid/text.go` (delete the `measureGraph` stub from `mermaid.go`)
- Test: `mermaid/text_test.go`

**Interfaces:**
- Consumes: IR + Theme (Task 1).
- Produces:
  - `func measureText(s string, size float64) (w, h float64)` — string metrics at font size.
  - `func measureGraph(g *Graph, t Theme)` — sets every `Node.W/H` (label size + padding, shape-adjusted) and `Edge.LabelW/LabelH`.

- [ ] **Step 1: Add the dependency**

```bash
go get golang.org/x/image@latest
```

- [ ] **Step 2: Write the failing test**

Create `mermaid/text_test.go`:

```go
package mermaid

import "testing"

func TestMeasureText(t *testing.T) {
	w1, h := measureText("hi", 14)
	w2, _ := measureText("hello there, a longer string", 14)
	if w1 <= 0 || h <= 0 {
		t.Fatalf("non-positive metrics: %f %f", w1, h)
	}
	if w2 <= w1*2 {
		t.Fatalf("longer string must be much wider: %f vs %f", w2, w1)
	}
	wBig, hBig := measureText("hi", 28)
	if wBig <= w1 || hBig <= h {
		t.Fatalf("larger font must be larger: %f %f", wBig, hBig)
	}
}

func TestMeasureGraph(t *testing.T) {
	g := mustParse(t, "graph TD\na[some label] --> b{choice} & c((ok))\na -->|edge label| c")
	measureGraph(g, Light)
	for _, n := range g.Nodes {
		if n.W <= 0 || n.H <= 0 {
			t.Fatalf("node %s unsized: %+v", n.ID, n)
		}
	}
	// diamond needs more room than a rect with the same label
	ga := mustParse(t, "graph TD\nx[choice]\ny{choice}")
	measureGraph(ga, Light)
	if ga.node("y").W <= ga.node("x").W {
		t.Fatalf("diamond not padded wider: %f <= %f", ga.node("y").W, ga.node("x").W)
	}
	// circle is as tall as wide
	if c := g.node("c"); c.W != c.H {
		t.Fatalf("circle not square: %+v", c)
	}
	// labeled edge got label size
	var labeled *Edge
	for _, e := range g.Edges {
		if e.Label != "" {
			labeled = e
		}
	}
	if labeled == nil || labeled.LabelW <= 0 || labeled.LabelH <= 0 {
		t.Fatalf("edge label unsized: %+v", labeled)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./mermaid/ -run TestMeasure`
Expected: FAIL — `measureText` undefined / stub no-op leaves W=0.

- [ ] **Step 4: Implement `mermaid/text.go`** (delete the stub)

```go
package mermaid

import (
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Font metrics come from the embedded Go Regular face. Emitted SVG uses the
// theme's font stack; padding absorbs the small cross-font drift.
// ponytail: one cached face per font size, no eviction — sizes in practice
// are one or two values from themes.
var (
	faceMu sync.Mutex
	faces  = map[float64]font.Face{}
	sfnt   = func() *opentype.Font {
		f, err := opentype.Parse(goregular.TTF)
		if err != nil {
			panic(err) // embedded font cannot fail to parse
		}
		return f
	}()
)

func face(size float64) font.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faces[size]; ok {
		return f
	}
	f, err := opentype.NewFace(sfnt, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingNone,
	})
	if err != nil {
		panic(err)
	}
	faces[size] = f
	return f
}

func measureText(s string, size float64) (w, h float64) {
	f := face(size)
	faceMu.Lock()
	adv := font.MeasureString(f, s)
	m := f.Metrics()
	faceMu.Unlock()
	return fixedToF(adv), fixedToF(m.Ascent + m.Descent)
}

func fixedToF(v fixed.Int26_6) float64 { return float64(v) / 64 }

const (
	padX = 16.0 // horizontal label padding inside a node
	padY = 10.0
)

func measureGraph(g *Graph, t Theme) {
	for _, n := range g.Nodes {
		w, h := measureText(n.Label, t.FontSize)
		n.W, n.H = w+2*padX, h+2*padY
		switch n.Shape {
		case ShapeDiamond:
			// a diamond's inscribed box is half its area: pad up
			n.W, n.H = n.W*1.6, n.H*1.8
		case ShapeCircle:
			d := n.W
			if n.H > d {
				d = n.H
			}
			n.W, n.H = d, d
		}
	}
	for _, e := range g.Edges {
		if e.Label != "" {
			e.LabelW, e.LabelH = measureText(e.Label, t.FontSize)
		}
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./mermaid/`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add mermaid/text.go mermaid/text_test.go go.mod go.sum
git commit -m "Add font-metric text measurement for node sizing"
```

---

### Task 4: dagre layout via goja

**Files:**
- Create: `mermaid/layout.go` (delete the `layout` stub from `mermaid.go`)
- Create: `mermaid/dagre.js` (vendored)
- Test: `mermaid/layout_test.go`

**Interfaces:**
- Consumes: IR with W/H set (Tasks 1+3).
- Produces: `func layout(g *Graph) error` — fills `Node.X/Y` (centers), `Edge.Points/LabelX/LabelY`, `Subgraph.X/Y/W/H`, `Graph.Width/Height`.

- [ ] **Step 1: Vendor dagre and add goja**

```bash
curl -L -o mermaid/dagre.js https://unpkg.com/dagre@0.8.5/dist/dagre.min.js
test -s mermaid/dagre.js && head -c 100 mermaid/dagre.js && echo
go get github.com/dop251/goja@latest
```

Expected: file starts with a minified JS preamble (~100KB total). dagre 0.8.5 is MIT; it is the layout engine mermaid itself uses.

- [ ] **Step 2: Write the failing test**

Create `mermaid/layout_test.go`:

```go
package mermaid

import (
	"math"
	"sync"
	"testing"
)

func layoutFixture(t *testing.T, src string) *Graph {
	t.Helper()
	g := mustParse(t, src)
	measureGraph(g, Light)
	if err := layout(g); err != nil {
		t.Fatalf("layout: %v", err)
	}
	return g
}

func TestLayoutPositionsEveryNode(t *testing.T) {
	g := layoutFixture(t, "graph TD\na[Start] --> b{Q} -->|yes| c((C))\nb -->|no| d\nsubgraph s [Grp]\n e --> f\nend\na --> e")
	for _, n := range g.Nodes {
		if math.IsNaN(n.X) || math.IsNaN(n.Y) || math.IsInf(n.X, 0) || math.IsInf(n.Y, 0) {
			t.Fatalf("node %s bad position %+v", n.ID, n)
		}
	}
	if g.Width <= 0 || g.Height <= 0 {
		t.Fatalf("bad graph size %f x %f", g.Width, g.Height)
	}
	for _, n := range g.Nodes {
		if n.X-n.W/2 < -1 || n.Y-n.H/2 < -1 || n.X+n.W/2 > g.Width+1 || n.Y+n.H/2 > g.Height+1 {
			t.Fatalf("node %s outside bounds: %+v graph %fx%f", n.ID, n, g.Width, g.Height)
		}
	}
}

func TestLayoutRespectsDirection(t *testing.T) {
	td := layoutFixture(t, "graph TD\na --> b --> c")
	if !(td.node("a").Y < td.node("b").Y && td.node("b").Y < td.node("c").Y) {
		t.Fatalf("TD rank order broken: a.Y=%f b.Y=%f c.Y=%f",
			td.node("a").Y, td.node("b").Y, td.node("c").Y)
	}
	lr := layoutFixture(t, "graph LR\na --> b --> c")
	if !(lr.node("a").X < lr.node("b").X && lr.node("b").X < lr.node("c").X) {
		t.Fatalf("LR rank order broken")
	}
}

func TestLayoutEdgesHavePoints(t *testing.T) {
	g := layoutFixture(t, "graph TD\na -->|lbl| b")
	e := g.Edges[0]
	if len(e.Points) < 2 {
		t.Fatalf("edge needs >=2 points: %+v", e)
	}
	if e.LabelX == 0 && e.LabelY == 0 {
		t.Fatalf("labeled edge needs label position")
	}
}

func TestLayoutSubgraphBounds(t *testing.T) {
	g := layoutFixture(t, "graph TD\nsubgraph s [G]\na --> b\nend")
	sg := g.Subgraphs[0]
	if sg.W <= 0 || sg.H <= 0 {
		t.Fatalf("subgraph unsized: %+v", sg)
	}
	for _, id := range sg.Children {
		n := g.node(id)
		if n.X < sg.X || n.X > sg.X+sg.W || n.Y < sg.Y || n.Y > sg.Y+sg.H {
			t.Fatalf("node %s outside its subgraph: n=%+v sg=%+v", id, n, sg)
		}
	}
}

func TestLayoutConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g := mustParseNoT("graph TD\na --> b --> c\na --> c")
			measureGraph(g, Light)
			if err := layout(g); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
}

func mustParseNoT(src string) *Graph {
	g, err := parseFlowchart(src)
	if err != nil {
		panic(err)
	}
	return g
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./mermaid/ -run TestLayout`
Expected: FAIL — stub returns `ErrUnsupported`.

- [ ] **Step 4: Implement `mermaid/layout.go`** (delete the stub)

```go
package mermaid

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

//go:embed dagre.js
var dagreSrc string

const layoutJS = `
function __layout(inputJSON) {
	var input = JSON.parse(inputJSON);
	var g = new dagre.graphlib.Graph({ compound: true, multigraph: true });
	g.setGraph({ rankdir: input.rankdir, nodesep: 50, ranksep: 50, marginx: 8, marginy: 8 });
	g.setDefaultEdgeLabel(function () { return {}; });
	input.nodes.forEach(function (n) { g.setNode(n.id, { width: n.w, height: n.h }); });
	input.subgraphs.forEach(function (s) {
		g.setNode(s.id, {});
		s.children.forEach(function (c) { g.setParent(c, s.id); });
	});
	input.edges.forEach(function (e, i) {
		g.setEdge(e.from, e.to, { width: e.lw, height: e.lh, labelpos: "c" }, "e" + i);
	});
	dagre.layout(g);
	var out = { nodes: {}, edges: [], subgraphs: {}, width: g.graph().width, height: g.graph().height };
	input.nodes.forEach(function (n) {
		var p = g.node(n.id);
		out.nodes[n.id] = { x: p.x, y: p.y };
	});
	input.edges.forEach(function (e, i) {
		var p = g.edge(e.from, e.to, "e" + i);
		out.edges.push({ points: p.points, x: p.x === undefined ? 0 : p.x, y: p.y === undefined ? 0 : p.y });
	});
	input.subgraphs.forEach(function (s) {
		var p = g.node(s.id);
		out.subgraphs[s.id] = { x: p.x - p.width / 2, y: p.y - p.height / 2, w: p.width, h: p.height };
	});
	return JSON.stringify(out);
}
`

// One VM, one lock. goja VMs are not goroutine-safe; doc-sized graphs lay
// out in well under a millisecond, so contention is irrelevant.
// ponytail: single global VM behind a mutex; pool VMs only if profiling
// ever shows contention.
var (
	vmMu   sync.Mutex
	vmInst *goja.Runtime
	vmFn   goja.Callable
	vmErr  error
	vmOnce sync.Once
)

func vm() (goja.Callable, error) {
	vmOnce.Do(func() {
		r := goja.New()
		if _, err := r.RunString(dagreSrc); err != nil {
			vmErr = fmt.Errorf("dagre load: %w", err)
			return
		}
		if _, err := r.RunString(layoutJS); err != nil {
			vmErr = fmt.Errorf("layout glue: %w", err)
			return
		}
		fn, ok := goja.AssertFunction(r.Get("__layout"))
		if !ok {
			vmErr = fmt.Errorf("__layout not a function")
			return
		}
		vmInst, vmFn = r, fn
	})
	return vmFn, vmErr
}

type layoutIn struct {
	Rankdir   string       `json:"rankdir"`
	Nodes     []jsNode     `json:"nodes"`
	Edges     []jsEdge     `json:"edges"`
	Subgraphs []jsSubgraph `json:"subgraphs"`
}
type jsNode struct {
	ID string  `json:"id"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
}
type jsEdge struct {
	From string  `json:"from"`
	To   string  `json:"to"`
	LW   float64 `json:"lw"`
	LH   float64 `json:"lh"`
}
type jsSubgraph struct {
	ID       string   `json:"id"`
	Children []string `json:"children"`
}
type layoutOut struct {
	Nodes map[string]struct{ X, Y float64 } `json:"nodes"`
	Edges []struct {
		Points []Point `json:"points"`
		X, Y   float64
	} `json:"edges"`
	Subgraphs map[string]struct{ X, Y, W, H float64 } `json:"subgraphs"`
	Width     float64                                 `json:"width"`
	Height    float64                                 `json:"height"`
}

func layout(g *Graph) error {
	fn, err := vm()
	if err != nil {
		return err
	}
	in := layoutIn{Rankdir: g.Direction, Subgraphs: []jsSubgraph{}, Nodes: []jsNode{}, Edges: []jsEdge{}}
	if in.Rankdir == "TD" {
		in.Rankdir = "TB" // dagre spells top-down TB
	}
	for _, n := range g.Nodes {
		in.Nodes = append(in.Nodes, jsNode{ID: n.ID, W: n.W, H: n.H})
	}
	for _, e := range g.Edges {
		in.Edges = append(in.Edges, jsEdge{From: e.From, To: e.To, LW: e.LabelW, LH: e.LabelH})
	}
	for _, s := range g.Subgraphs {
		in.Subgraphs = append(in.Subgraphs, jsSubgraph{ID: s.ID, Children: s.Children})
	}
	inJSON, err := json.Marshal(in)
	if err != nil {
		return err
	}

	vmMu.Lock()
	res, err := fn(goja.Undefined(), vmInst.ToValue(string(inJSON)))
	vmMu.Unlock()
	if err != nil {
		return fmt.Errorf("dagre: %w", err)
	}

	var out layoutOut
	if err := json.Unmarshal([]byte(res.String()), &out); err != nil {
		return fmt.Errorf("dagre output: %w", err)
	}
	for _, n := range g.Nodes {
		p, ok := out.Nodes[n.ID]
		if !ok {
			return fmt.Errorf("dagre lost node %s", n.ID)
		}
		n.X, n.Y = p.X, p.Y
	}
	for i, e := range g.Edges {
		e.Points = out.Edges[i].Points
		e.LabelX, e.LabelY = out.Edges[i].X, out.Edges[i].Y
	}
	for _, s := range g.Subgraphs {
		p, ok := out.Subgraphs[s.ID]
		if !ok {
			return fmt.Errorf("dagre lost subgraph %s", s.ID)
		}
		s.X, s.Y, s.W, s.H = p.X, p.Y, p.W, p.H
	}
	g.Width, g.Height = out.Width, out.Height
	return nil
}
```

Note: `layoutOut`'s anonymous structs use default JSON field matching for
`X`/`Y`/`W`/`H` (case-insensitive), which matches the lowercase keys the glue
emits. If the unmarshal proves finicky, add explicit `json:"x"` etc. tags —
the tests govern.

- [ ] **Step 5: Run tests, including race**

Run: `go test ./mermaid/ -race`
Expected: PASS, no race reports. If dagre.min.js fails to load in goja (ES incompatibility), STOP and report BLOCKED with the goja error — do not swap layout strategies unilaterally.

- [ ] **Step 6: Commit**

```bash
git add mermaid/layout.go mermaid/layout_test.go mermaid/dagre.js go.mod go.sum
git commit -m "Add dagre-in-goja layout"
```

---

### Task 5: SVG emitter + goldens

**Files:**
- Create: `mermaid/svg.go` (delete the `emit` stub from `mermaid.go`)
- Create: `mermaid/testdata/*.mmd` + `.svg` goldens
- Test: `mermaid/svg_test.go`

**Interfaces:**
- Consumes: positioned IR + Theme.
- Produces: `func emit(g *Graph, t Theme) []byte` — complete `<svg>` element. With this, `Render` is fully live.

- [ ] **Step 1: Write the failing test**

Create `mermaid/svg_test.go`:

```go
package mermaid

import (
	"bytes"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func render(t *testing.T, src string, theme Theme) []byte {
	t.Helper()
	out, err := Render([]byte(src), theme)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

func TestSVGWellFormed(t *testing.T) {
	out := render(t, "graph TD\na[Start] --> b{Q} -->|yes| c((C))\nb -->|no| d([End])\nsubgraph s [G]\n e --- f\nend", Light)
	d := xml.NewDecoder(bytes.NewReader(out))
	for {
		_, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid XML: %v\n%s", err, out)
		}
	}
	s := string(out)
	if !strings.HasPrefix(s, "<svg") || !strings.Contains(s, "viewBox=") {
		t.Fatalf("not an svg root: %.80s", s)
	}
	if !strings.Contains(s, `class="mermaid-svg"`) {
		t.Fatal("missing mermaid-svg class")
	}
}

func TestSVGShapeAndEdgeCounts(t *testing.T) {
	out := string(render(t, "graph TD\na[R] --> b(Rd) --> c([St]) --> d{Di} --> e((Ci))", Light))
	if c := strings.Count(out, "<rect"); c != 3 { // rect + round + stadium
		t.Fatalf("rects = %d, want 3\n%s", c, out)
	}
	if c := strings.Count(out, "<polygon"); c != 1 {
		t.Fatalf("polygons = %d, want 1", c)
	}
	if c := strings.Count(out, "<circle"); c != 1 {
		t.Fatalf("circles = %d, want 1", c)
	}
	if c := strings.Count(out, `class="edge"`); c != 4 {
		t.Fatalf("edges = %d, want 4", c)
	}
	for _, lbl := range []string{">R<", ">Rd<", ">St<", ">Di<", ">Ci<"} {
		if !strings.Contains(out, lbl) {
			t.Fatalf("missing label %s", lbl)
		}
	}
}

func TestSVGEdgeStyles(t *testing.T) {
	out := string(render(t, "graph TD\na --> b\nc -.-> d\ne ==> f\ng --- h", Light))
	if !strings.Contains(out, "stroke-dasharray") {
		t.Fatal("dotted edge missing dasharray")
	}
	if !strings.Contains(out, `marker-end="url(#arrow)"`) {
		t.Fatal("directed edge missing arrowhead")
	}
	// undirected g --- h: exactly 3 of 4 edges carry the marker
	if c := strings.Count(out, `marker-end`); c != 3 {
		t.Fatalf("marker count %d, want 3", c)
	}
}

func TestSVGLabelEscaping(t *testing.T) {
	out := string(render(t, "graph TD\na[\"<script> & fun\"]", Light))
	if strings.Contains(out, "<script>") {
		t.Fatal("label not escaped")
	}
	if !strings.Contains(out, "&lt;script&gt; &amp; fun") {
		t.Fatalf("expected escaped label, got:\n%s", out)
	}
}

func TestSVGThemeApplied(t *testing.T) {
	light := string(render(t, "graph TD\na --> b", Light))
	dark := string(render(t, "graph TD\na --> b", Dark))
	if !strings.Contains(light, Light.NodeFill) || !strings.Contains(dark, Dark.NodeFill) {
		t.Fatal("theme fills not applied")
	}
	if light == dark {
		t.Fatal("themes produced identical output")
	}
}

func TestGoldens(t *testing.T) {
	mmds, err := filepath.Glob("testdata/*.mmd")
	if err != nil || len(mmds) == 0 {
		t.Fatalf("no goldens found: %v", err)
	}
	for _, mmd := range mmds {
		src, err := os.ReadFile(mmd)
		if err != nil {
			t.Fatal(err)
		}
		got := render(t, string(src), Light)
		golden := strings.TrimSuffix(mmd, ".mmd") + ".svg"
		if *update {
			if err := os.WriteFile(golden, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("missing golden %s (run: go test ./mermaid/ -update)", golden)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from golden (run -update and eyeball the diff)", mmd)
		}
	}
}
```

- [ ] **Step 2: Create golden sources**

```bash
mkdir -p mermaid/testdata
cat > mermaid/testdata/simple.mmd <<'EOF'
graph TD
A[Start] --> B{Works?}
B -->|yes| C([Ship it])
B -->|no| D((Fix))
D -.-> A
EOF
cat > mermaid/testdata/subgraph.mmd <<'EOF'
graph LR
subgraph backend [Backend]
  api --> db[(ignored shape)]
end
EOF
```

Note: `db[(...)]` (cylinder) is NOT in the subset — replace the second golden with a supported body:

```bash
cat > mermaid/testdata/subgraph.mmd <<'EOF'
graph LR
subgraph backend [Backend]
  api --> db[Database]
end
web ==> api
EOF
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./mermaid/ -run TestSVG`
Expected: FAIL — `emit` stub returns nil.

- [ ] **Step 4: Implement `mermaid/svg.go`** (delete the stub)

```go
package mermaid

import (
	"bytes"
	"fmt"
	"html"
	"strings"
)

// emit renders a positioned graph as a standalone <svg> element.
func emit(g *Graph, t Theme) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" class="mermaid-svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family=%q font-size="%.0f">`,
		g.Width, g.Height, g.Width, g.Height, t.FontFamily, t.FontSize)
	fmt.Fprintf(&b,
		`<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="%s"/></marker></defs>`,
		t.EdgeStroke)

	for _, s := range g.Subgraphs {
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="4" fill="%s" opacity="0.4"/>`,
			s.X, s.Y, s.W, s.H, t.SubgraphFill)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s" font-weight="bold">%s</text>`,
			s.X+8, s.Y+t.FontSize+4, t.Text, html.EscapeString(s.Title))
	}

	for _, e := range g.Edges {
		if len(e.Points) < 2 {
			continue
		}
		var d strings.Builder
		fmt.Fprintf(&d, "M %.1f %.1f", e.Points[0].X, e.Points[0].Y)
		for _, p := range e.Points[1:] {
			fmt.Fprintf(&d, " L %.1f %.1f", p.X, p.Y)
		}
		attrs := fmt.Sprintf(`fill="none" stroke="%s" stroke-width="1.5"`, t.EdgeStroke)
		if e.Style == EdgeDotted {
			attrs += ` stroke-dasharray="4,4"`
		}
		if e.Style == EdgeThick {
			attrs = strings.Replace(attrs, `stroke-width="1.5"`, `stroke-width="3"`, 1)
		}
		if e.Directed {
			attrs += ` marker-end="url(#arrow)"`
		}
		fmt.Fprintf(&b, `<path class="edge" d="%s" %s/>`, d.String(), attrs)
		if e.Label != "" {
			fmt.Fprintf(&b,
				`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" opacity="0.85"/>`,
				e.LabelX-e.LabelW/2-2, e.LabelY-e.LabelH/2-1, e.LabelW+4, e.LabelH+2, t.NodeFill)
			fmt.Fprintf(&b,
				`<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%s</text>`,
				e.LabelX, e.LabelY, t.Text, html.EscapeString(e.Label))
		}
	}

	for _, n := range g.Nodes {
		x0, y0 := n.X-n.W/2, n.Y-n.H/2
		style := fmt.Sprintf(`fill="%s" stroke="%s" stroke-width="1.5"`, t.NodeFill, t.NodeStroke)
		switch n.Shape {
		case ShapeRect:
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" %s/>`, x0, y0, n.W, n.H, style)
		case ShapeRound:
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="6" %s/>`, x0, y0, n.W, n.H, style)
		case ShapeStadium:
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" %s/>`, x0, y0, n.W, n.H, n.H/2, style)
		case ShapeDiamond:
			fmt.Fprintf(&b, `<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" %s/>`,
				n.X, y0, x0+n.W, n.Y, n.X, y0+n.H, x0, n.Y, style)
		case ShapeCircle:
			fmt.Fprintf(&b, `<circle cx="%.1f" cy="%.1f" r="%.1f" %s/>`, n.X, n.Y, n.W/2, style)
		}
		fmt.Fprintf(&b,
			`<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%s</text>`,
			n.X, n.Y, t.Text, html.EscapeString(n.Label))
	}

	b.WriteString(`</svg>`)
	return b.Bytes()
}
```

- [ ] **Step 5: Generate goldens, eyeball, run everything**

```bash
go test ./mermaid/ -update
go test ./mermaid/ -race
open mermaid/testdata/simple.svg   # visual sanity check in a browser (macOS)
```

Expected: full package PASS with race detector. The opened SVG shows a sensible top-down flowchart (boxes, diamond, labeled edges, dotted return edge).

- [ ] **Step 6: Commit**

```bash
git add mermaid/svg.go mermaid/svg_test.go mermaid/testdata mermaid/mermaid.go
git commit -m "Add SVG emitter with themes and golden tests"
```

---

### Task 6: Viewer integration with JS fallback

**Files:**
- Modify: `render.go` (mermaid branch in `renderFenced`; `RenderBody`/`RenderPage` signatures; conditional mermaid.min.js)
- Modify: `server.go` (adapt to new signatures)
- Modify: `render_test.go`, `server_test.go` (adapt existing calls; new tests)
- Modify: `README.md`

**Interfaces:**
- Consumes: `mermaid.Render`, `mermaid.Light/Dark` (Tasks 1–5); `cfg.Theme` (existing).
- Produces (new signatures — update ALL callers):
  - `func RenderBody(src []byte) (out []byte, usedFallback bool, err error)`
  - `func RenderPage(body []byte, title string, includeMermaidJS bool) []byte`

- [ ] **Step 1: Update the failing tests first**

In `render_test.go`, mechanically adapt every existing `RenderBody(x)` call to `out, _, err := RenderBody(x)` and every `RenderPage(b, title)` to `RenderPage(b, title, true)`. In `server_test.go` no direct calls exist (server uses them internally). Then add:

```go
func TestRenderBodyNativeMermaid(t *testing.T) {
	out, fallback, err := RenderBody([]byte("```mermaid\ngraph TD\nA[Hi] --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<svg") || !strings.Contains(s, "mermaid-svg") {
		t.Fatalf("expected native svg, got: %s", s)
	}
	if strings.Contains(s, `<pre class="mermaid">`) {
		t.Fatalf("native path must not emit fallback pre: %s", s)
	}
	if fallback {
		t.Fatal("fallback flag must be false for supported diagram")
	}
}

func TestRenderBodyMermaidFallback(t *testing.T) {
	out, fallback, err := RenderBody([]byte("```mermaid\nsequenceDiagram\nA->>B: hi\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<pre class="mermaid">`) {
		t.Fatalf("unsupported diagram must fall back: %s", out)
	}
	if !fallback {
		t.Fatal("fallback flag must be true")
	}
}

func TestRenderPageMermaidJSConditional(t *testing.T) {
	with := string(RenderPage([]byte("x"), "t", true))
	without := string(RenderPage([]byte("x"), "t", false))
	if !strings.Contains(with, "mermaid.min.js") || !strings.Contains(with, "mermaid.initialize") {
		t.Fatal("fallback page must include mermaid js")
	}
	if strings.Contains(without, "mermaid.min.js") || strings.Contains(without, "mermaid.initialize") {
		t.Fatal("native-only page must not ship mermaid js")
	}
}

func TestRenderBodyNativeMermaidDarkTheme(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)
	cfg.Theme = "dark"
	out, _, err := RenderBody([]byte("```mermaid\ngraph TD\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "#161b22") { // mermaid.Dark.NodeFill
		t.Fatalf("dark theme not applied to svg: %s", out)
	}
}
```

Also update the existing `TestRenderBodyMermaidPassthrough`: its diagram (`graph TD\nA-->B`) is now natively supported, so REPLACE its body to use an unsupported diagram for the passthrough assertion:

```go
func TestRenderBodyMermaidPassthrough(t *testing.T) {
	src := "```mermaid\ngantt\ntitle X\n```\n"
	out, fallback, err := RenderBody([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `<pre class="mermaid">`) || !fallback {
		t.Fatalf("expected fallback pre, got: %s", s)
	}
	if !strings.Contains(s, "gantt") {
		t.Fatalf("expected diagram source preserved, got: %s", s)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./...`
Expected: FAIL — compile errors from signature changes, then behavior.

- [ ] **Step 3: Implement in `render.go`**

Add import: `"github.com/dgunther/mdthing/mermaid"`.

`codeRenderer` gains a flag; the goldmark render pass is stateful per call, so track fallback on the renderer instance created per `RenderBody` call — simplest: make `RenderBody` construct its converter per call:

```go
// RenderBody converts Markdown to an HTML fragment. usedFallback reports
// whether any mermaid block needed the JS renderer.
func RenderBody(src []byte) ([]byte, bool, error) {
	cr := &codeRenderer{}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(
			renderer.WithNodeRenderers(util.Prioritized(cr, 100)),
		),
	)
	var buf bytes.Buffer
	if err := md.Convert(rewriteWikilinks(src), &buf); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), cr.mermaidFallback, nil
}
```

(Delete the package-level `var md = goldmark.New(...)`; per-call construction is cheap relative to layout and makes the fallback flag race-free.)

In `codeRenderer`:

```go
type codeRenderer struct {
	mermaidFallback bool
}
```

The mermaid branch of `renderFenced` becomes:

```go
	if lang == "mermaid" {
		theme := mermaid.Light
		if cfg.Theme == "dark" {
			theme = mermaid.Dark
		}
		if svg, err := mermaid.Render(code, theme); err == nil {
			w.Write(svg)
			w.WriteString("\n")
			return ast.WalkSkipChildren, nil
		}
		r.mermaidFallback = true
		w.WriteString(`<pre class="mermaid">`)
		template.HTMLEscape(w, code)
		w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}
```

`RenderPage` gains the flag and guards the two script lines:

```go
func RenderPage(body []byte, title string, includeMermaidJS bool) []byte {
```

and:

```go
	if includeMermaidJS {
		b.WriteString(`<script src="/_assets/mermaid.min.js"></script>`)
		fmt.Fprintf(&b, `<script>mermaid.initialize({startOnLoad:true,theme:'%s'});</script>`, template.JSEscapeString(cfg.MermaidTheme))
	}
```

Add centering for native SVGs to `assets/base.css`:

```css
.markdown-body svg.mermaid-svg { display: block; margin: 1em auto; max-width: 100%; height: auto; }
```

- [ ] **Step 4: Adapt `server.go`**

In `serveMarkdown`:

```go
	body, fallback, err := RenderBody(src)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(RenderPage(body, filepath.Base(abs), fallback))
```

And the not-found path: `RenderPage([]byte("<h1>not found</h1>"), "not found", false)`.

- [ ] **Step 5: Run everything**

Run: `go build ./... && go vet ./... && go test ./... -race`
Expected: all PASS.

- [ ] **Step 6: Update `README.md`**

In Features, replace the mermaid bullet:

```markdown
- Inline Mermaid diagrams — common flowcharts render natively to SVG (no
  JS); other diagram types fall back to the bundled mermaid.js
```

- [ ] **Step 7: Manual verification (controller)**

Launch against a file containing a supported flowchart, an unsupported diagram (e.g. `sequenceDiagram`), and a table. Verify: flowchart appears as crisp SVG (inspect: `<svg class="mermaid-svg">` in page source, no mermaid.min.js request for a native-only page), sequence diagram still renders via JS fallback, dark config themes the SVG.

- [ ] **Step 8: Commit**

```bash
git add render.go server.go render_test.go server_test.go assets/base.css README.md
git commit -m "Render supported mermaid flowcharts natively as SVG with JS fallback"
```

---

## Self-Review

**Spec coverage:**
- Package layout mermaid.go/ir.go(+parse/layout/svg/text/theme)/dagre.js — Tasks 1–5. (Spec folds IR into mermaid.go's description; a separate `ir.go` is a trivial file split, same package.) ✓
- API `Render(src, theme)`, `ErrUnsupported` — Task 1. ✓
- Flowchart subset incl. directions, 5 shapes + bare id, quoted labels, 4 edge ops, `|label|` + `-- label -->`, chains, fan-out, subgraphs, comments, `;`, ignored style statements, unsupported → error — Task 2 (tests enumerate each). ✓
- dagre-in-goja, vendored MIT dagre, no DOM — Task 4. ✓
- Concurrency safety — Task 4 (mutex + `-race` test). ✓
- Text measurement via x/image + goregular, padding, non-goal pixel fidelity — Task 3. ✓
- SVG: shapes, edge styles + arrowheads, labels, subgraph rects, viewBox + class — Task 5. ✓
- Themes light/dark keyed to base.css — Task 1 (values) + Task 5 (applied test) + Task 6 (cfg.Theme selection). ✓
- Fallback contract + conditional mermaid.min.js — Task 6. ✓
- Golden files with -update — Task 5. ✓
- No GPL / no TyphonHill code — Global Constraints + original IR in Task 1. ✓

**Placeholder scan:** clean — every code step has complete code; the Task 1 stubs are explicitly deleted by named later tasks; the layoutOut JSON-tag note gives the concrete alternative.

**Type consistency:** `Render([]byte, Theme) ([]byte, error)`, `detect(string) (string, string)`, `parseFlowchart(string) (*Graph, error)`, `measureText(string, float64) (float64, float64)`, `measureGraph(*Graph, Theme)`, `layout(*Graph) error`, `emit(*Graph, Theme) []byte`, IR field names (`Node.X/Y` centers; `Subgraph.X/Y` top-left), `RenderBody` → `([]byte, bool, error)`, `RenderPage(body, title, includeMermaidJS)` — consistent across Tasks 1–6; Task 6 updates both callers and all existing tests.
