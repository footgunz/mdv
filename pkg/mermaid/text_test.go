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

func TestMeasureGraphNodePadding(t *testing.T) {
	g := mustParse(t, "graph TD\na[label]")
	measureGraph(g, Light)
	w, h := measureText("label", Light.FontSize)
	n := g.node("a")
	if n.W != w+40 || n.H != h+30 {
		t.Fatalf("rect padding: got %.1fx%.1f, want %.1fx%.1f (text+40, text+30)", n.W, n.H, w+40, h+30)
	}
}
