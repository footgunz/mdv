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
	// Note: font-family can contain literal double quotes (e.g. "Segoe UI"),
	// so it must be XML-attribute-escaped, not Go-%q-escaped (which produces
	// backslash escapes that are invalid inside an XML attribute value).
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" class="mermaid-svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="%s" font-size="%.0f">`,
		g.Width, g.Height, g.Width, g.Height, html.EscapeString(t.FontFamily), t.FontSize)
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
