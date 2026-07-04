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
