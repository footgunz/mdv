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
