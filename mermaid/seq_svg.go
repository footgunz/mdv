package mermaid

import (
	"bytes"
	"fmt"
	"html"
)

// emitSequence renders a positioned sequence diagram. Same conventions as
// the flowchart emitter: escaped user text, %.1f floats, themed attributes.
func emitSequence(d *SeqDiagram, t Theme) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" class="mermaid-svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="%s" font-size="%.0f">`,
		d.Width, d.Height, d.Width, d.Height, html.EscapeString(t.FontFamily), t.FontSize)
	fmt.Fprintf(&b,
		`<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="%s"/></marker>`+
			`<marker id="cross" viewBox="0 0 10 10" refX="5" refY="5" markerWidth="8" markerHeight="8" orient="auto"><path d="M 2 2 L 8 8 M 8 2 L 2 8" stroke="%s" stroke-width="1.5"/></marker></defs>`,
		t.EdgeStroke, t.EdgeStroke)

	boxTop := seqMargin
	lifeTop := boxTop + d.Participants[0].H

	// Lifelines first (behind everything).
	for _, p := range d.Participants {
		fmt.Fprintf(&b, `<line class="lifeline" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-dasharray="4,4"/>`,
			p.X, lifeTop, p.X, d.Height-seqMargin, t.EdgeStroke)
	}

	// Frames behind messages.
	var frames func(items []SeqItem)
	frames = func(items []SeqItem) {
		for _, it := range items {
			f, ok := it.(*SeqFrame)
			if !ok {
				continue
			}
			fmt.Fprintf(&b, `<rect class="seq-frame" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="none" stroke="%s"/>`,
				f.X, f.Y, f.W, f.H, t.EdgeStroke)
			fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`,
				f.X, f.Y, 46.0, seqHeaderH, t.SubgraphFill)
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s" font-weight="bold">%s</text>`,
				f.X+6, f.Y+seqHeaderH-6, t.Text, html.EscapeString(f.Kind))
			if lbl := f.Sections[0].Label; lbl != "" {
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s">[%s]</text>`,
					f.X+52, f.Y+seqHeaderH-6, t.Text, html.EscapeString(lbl))
			}
			for si, dy := range f.DividerYs {
				fmt.Fprintf(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-dasharray="4,4"/>`,
					f.X, dy, f.X+f.W, dy, t.EdgeStroke)
				if lbl := f.Sections[si+1].Label; lbl != "" {
					fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s">[%s]</text>`,
						f.X+6, dy+seqHeaderH-6, t.Text, html.EscapeString(lbl))
				}
			}
			for _, s := range f.Sections {
				frames(s.Items)
			}
		}
	}
	frames(d.Items)

	// Activations (over lifelines, under messages).
	for _, a := range d.Activations {
		x := a.P.X - seqActW/2 + float64(a.Level)*seqActNest
		fmt.Fprintf(&b, `<rect class="activation" x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="%s"/>`,
			x, a.Y0, seqActW, a.Y1-a.Y0, t.NodeFill, t.NodeStroke)
	}

	// Messages and notes in stream order.
	var items func(items []SeqItem)
	items = func(list []SeqItem) {
		for _, it := range list {
			switch v := it.(type) {
			case *SeqMessage:
				text := v.Text
				from, to := participantX(d, v.From), participantX(d, v.To)
				dash := ""
				if v.Dashed {
					dash = ` stroke-dasharray="6,3"`
				}
				marker := ""
				switch v.Head {
				case HeadArrow:
					marker = ` marker-end="url(#arrow)"`
				case HeadCross:
					marker = ` marker-end="url(#cross)"`
				}
				if v.From == v.To { // self-message loop
					fmt.Fprintf(&b, `<path class="seq-msg" d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" fill="none" stroke="%s"%s%s/>`,
						from, v.Y, from+seqSelfW*1.6, v.Y, from+seqSelfW*1.6, v.Y+seqSelfH, from+6, v.Y+seqSelfH, t.EdgeStroke, dash, marker)
					fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" fill="%s">%s</text>`,
						from+seqSelfW*1.6+seqNotePad, v.Y+seqSelfH/2+4, t.Text, html.EscapeString(text))
					if d.Autonumber {
						autonumBadge(&b, from, v.Y, v.Num, t)
					}
					continue
				}
				fmt.Fprintf(&b, `<line class="seq-msg" x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s"%s%s/>`,
					from, v.Y, to, v.Y, t.EdgeStroke, dash, marker)
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" fill="%s">%s</text>`,
					(from+to)/2, v.Y-5, t.Text, html.EscapeString(text))
				if d.Autonumber {
					autonumBadge(&b, from, v.Y, v.Num, t)
				}
			case *SeqNote:
				fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="%s"/>`,
					v.X, v.Y, v.W, v.H, t.SubgraphFill, t.NodeStroke)
				fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%s</text>`,
					v.X+v.W/2, v.Y+v.H/2, t.Text, html.EscapeString(v.Text))
			case *SeqFrame:
				for _, s := range v.Sections {
					items(s.Items)
				}
			}
		}
	}
	items(d.Items)

	// Participant boxes last (on top of lifeline starts).
	for _, p := range d.Participants {
		fmt.Fprintf(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s" stroke="%s" stroke-width="1.5"/>`,
			p.X-p.W/2, boxTop, p.W, p.H, t.NodeFill, t.NodeStroke)
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%s</text>`,
			p.X, boxTop+p.H/2, t.Text, html.EscapeString(p.Label))
	}

	b.WriteString(`</svg>`)
	return b.Bytes()
}

func participantX(d *SeqDiagram, id string) float64 { return d.participant(id).X }

// autonumBadge draws a mermaid-style circled message number at the line start.
func autonumBadge(b *bytes.Buffer, x, y float64, n int, t Theme) {
	fmt.Fprintf(b, `<circle class="autonumber" cx="%.1f" cy="%.1f" r="8" fill="%s"/>`, x, y, t.Text)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" font-size="10" fill="%s">%d</text>`, x, y, t.NodeFill, n)
}
