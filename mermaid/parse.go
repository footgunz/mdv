package mermaid

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	headerRe = regexp.MustCompile(`^(graph|flowchart)\s+(TD|TB|LR|RL|BT)\s*$`)
	// -- label --> rewritten to -->|label| before splitting
	inlineLabelRe = regexp.MustCompile(`(--|==|-\.)\s+(.+?)\s+(-->|---|==>|-\.->)`)
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
			cur = &Subgraph{ID: "sg_" + m[1], Title: strings.Trim(strings.TrimSpace(title), `"`)}
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
	for _, part := range splitTopLevel(s, '&') {
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

// splitTopLevel splits s on sep, but only where bracket depth is zero and
// not inside a double-quoted span, so `a[Fish & Chips]` stays one part.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth, inQuotes, start := 0, false, 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			inQuotes = !inQuotes
		case inQuotes:
			// inside quotes, ignore brackets and separators
		case c == '[' || c == '(' || c == '{':
			depth++
		case c == ']' || c == ')' || c == '}':
			depth--
		case c == sep && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
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
