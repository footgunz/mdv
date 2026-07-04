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
	endpoint := func(tok string, source bool) (string, error) {
		if tok != "[*]" {
			if strings.HasPrefix(tok, "__") {
				return "", unsup("reserved state id %q", tok)
			}
			ensure(tok, tok, ShapeRound)
			return tok, nil
		}
		if source {
			id := "__start" + scope
			ensure(id, "", ShapeStateStart)
			return id, nil
		}
		id := "__end" + scope
		ensure(id, "", ShapeStateEnd)
		return id, nil
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
			sgID := "sg_" + name
			for _, sg := range g.Subgraphs {
				if sg.ID == sgID {
					return nil, unsup("composite %q redeclared", name)
				}
			}
			cur = &Subgraph{ID: sgID, Title: name}
			scope = "_" + name
			g.Subgraphs = append(g.Subgraphs, cur)

		case line == "}":
			if cur == nil {
				return nil, unsup("} without composite")
			}
			cur, scope = nil, ""

		case stateDeclRe.MatchString(line):
			m := stateDeclRe.FindStringSubmatch(line)
			if strings.HasPrefix(m[2], "__") {
				return nil, unsup("reserved state id %q", m[2])
			}
			ensure(m[2], m[1], ShapeRound).Label = m[1]

		case stateTransRe.MatchString(line):
			m := stateTransRe.FindStringSubmatch(line)
			from, err := endpoint(m[1], true)
			if err != nil {
				return nil, err
			}
			to, err := endpoint(m[2], false)
			if err != nil {
				return nil, err
			}
			g.Edges = append(g.Edges, &Edge{
				From: from, To: to, Label: strings.TrimSpace(m[3]),
				Style: EdgeSolid, Directed: true,
			})

		case stateDescRe.MatchString(line):
			m := stateDescRe.FindStringSubmatch(line)
			if strings.HasPrefix(m[1], "__") {
				return nil, unsup("reserved state id %q", m[1])
			}
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
	// A transition/description may reference a composite by name before (or
	// after) its `state N { ... }` block is parsed; dagre can't route edges
	// to cluster nodes, so catch it here regardless of declaration order.
	for _, sg := range g.Subgraphs {
		if g.node(sg.Title) != nil {
			return nil, unsup("transition references composite state %q", sg.Title)
		}
	}
	return g, nil
}
