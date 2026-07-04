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
