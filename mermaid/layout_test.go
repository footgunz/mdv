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
