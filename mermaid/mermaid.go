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
	case "sequenceDiagram":
		d, err := parseSequence(rest)
		if err != nil {
			return nil, err
		}
		if err := layoutSequence(d, theme); err != nil {
			return nil, err
		}
		return emitSequence(d, theme), nil
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
