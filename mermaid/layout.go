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
	if len(out.Edges) != len(g.Edges) {
		return fmt.Errorf("dagre returned %d edges, want %d", len(out.Edges), len(g.Edges))
	}
	// out.Edges is index-aligned with g.Edges (same order through the JSON
	// round-trip); do not reorder either side.
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
