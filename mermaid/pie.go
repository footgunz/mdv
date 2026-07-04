package mermaid

import (
	"bytes"
	"fmt"
	"html"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type pieSlice struct {
	Label string
	Value float64
}

type pieChart struct {
	Title    string
	ShowData bool
	Slices   []pieSlice
}

var pieSliceRe = regexp.MustCompile(`^"([^"]*)"\s*:\s*([0-9]+(?:\.[0-9]+)?)$`)

func parsePie(src string) (*pieChart, error) {
	p := &pieChart{}
	seenHeader := false
	for _, raw := range strings.Split(src, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if !seenHeader {
			switch line {
			case "pie":
			case "pie showData":
				p.ShowData = true
			default:
				return nil, unsup("bad pie header %q", line)
			}
			seenHeader = true
			continue
		}
		if title, ok := strings.CutPrefix(line, "title "); ok {
			p.Title = strings.TrimSpace(title)
			continue
		}
		m := pieSliceRe.FindStringSubmatch(line)
		if m == nil {
			return nil, unsup("pie statement %q", line)
		}
		val, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return nil, unsup("pie value %q", m[2])
		}
		p.Slices = append(p.Slices, pieSlice{Label: m[1], Value: val})
	}
	if !seenHeader {
		return nil, unsup("empty diagram")
	}
	total := 0.0
	for _, s := range p.Slices {
		total += s.Value
	}
	if total <= 0 {
		return nil, unsup("pie needs a positive total")
	}
	// mermaid renders slices largest-first
	sort.SliceStable(p.Slices, func(i, j int) bool { return p.Slices[i].Value > p.Slices[j].Value })
	return p, nil
}

const (
	pieR      = 90.0
	pieMargin = 12.0
	pieSwatch = 12.0
	pieRowH   = 20.0
)

func emitPie(p *pieChart, t Theme) []byte {
	total := 0.0
	for _, s := range p.Slices {
		total += s.Value
	}

	titleH := 0.0
	if p.Title != "" {
		_, th := measureText(p.Title, t.FontSize)
		titleH = th + pieMargin
	}
	cx := pieMargin + pieR
	cy := titleH + pieMargin + pieR

	legendX := cx + pieR + 2*pieMargin
	maxLegend := 0.0
	for _, s := range p.Slices {
		w, _ := measureText(legendText(s, p.ShowData), t.FontSize)
		if w > maxLegend {
			maxLegend = w
		}
	}
	width := legendX + pieSwatch + 6 + maxLegend + pieMargin
	height := cy + pieR + pieMargin
	if lh := titleH + pieMargin + float64(len(p.Slices))*pieRowH + pieMargin; lh > height {
		height = lh
	}

	var b bytes.Buffer
	fmt.Fprintf(&b,
		`<svg xmlns="http://www.w3.org/2000/svg" class="mermaid-svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f" font-family="%s" font-size="%.0f">`,
		width, height, width, height, html.EscapeString(t.FontFamily), t.FontSize)
	if p.Title != "" {
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" font-weight="bold" fill="%s">%s</text>`,
			cx, pieMargin+t.FontSize, t.Text, html.EscapeString(p.Title))
	}

	angle := 0.0 // radians from 12 o'clock, clockwise
	for i, s := range p.Slices {
		frac := s.Value / total
		color := t.Palette[i%len(t.Palette)]
		if frac >= 1-1e-9 { // single-slice pie: full circle, arc would degenerate
			fmt.Fprintf(&b, `<circle class="pie-slice" cx="%.1f" cy="%.1f" r="%.1f" fill="%s" stroke="%s" stroke-width="2"/>`,
				cx, cy, pieR, color, t.NodeFill)
		} else if frac > 0 {
			a0, a1 := angle, angle+frac*2*math.Pi
			fmt.Fprintf(&b, `<path class="pie-slice" d="%s" fill="%s" stroke="%s" stroke-width="2"/>`,
				arcPath(cx, cy, pieR, a0, a1), color, t.NodeFill)
		}
		if frac >= 0.05 {
			mid := angle + frac*math.Pi
			fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" fill="%s">%.0f%%</text>`,
				cx+0.6*pieR*math.Sin(mid), cy-0.6*pieR*math.Cos(mid), t.Text, math.Round(frac*100))
		}
		angle += frac * 2 * math.Pi
	}

	ly := titleH + pieMargin
	for i, s := range p.Slices {
		fmt.Fprintf(&b, `<rect class="pie-legend" x="%.1f" y="%.1f" width="%.0f" height="%.0f" fill="%s"/>`,
			legendX, ly, pieSwatch, pieSwatch, t.Palette[i%len(t.Palette)])
		fmt.Fprintf(&b, `<text x="%.1f" y="%.1f" dominant-baseline="central" fill="%s">%s</text>`,
			legendX+pieSwatch+6, ly+pieSwatch/2, t.Text, html.EscapeString(legendText(s, p.ShowData)))
		ly += pieRowH
	}

	b.WriteString(`</svg>`)
	return b.Bytes()
}

func legendText(s pieSlice, showData bool) string {
	if showData {
		return fmt.Sprintf("%s [%s]", s.Label, strconv.FormatFloat(s.Value, 'f', -1, 64))
	}
	return s.Label
}

// arcPath draws a filled wedge; angles are radians from 12 o'clock, clockwise.
func arcPath(cx, cy, r, a0, a1 float64) string {
	x0, y0 := cx+r*math.Sin(a0), cy-r*math.Cos(a0)
	x1, y1 := cx+r*math.Sin(a1), cy-r*math.Cos(a1)
	large := 0
	if a1-a0 > math.Pi {
		large = 1
	}
	return fmt.Sprintf("M %.1f %.1f L %.1f %.1f A %.1f %.1f 0 %d 1 %.1f %.1f Z", cx, cy, x0, y0, r, r, large, x1, y1)
}
