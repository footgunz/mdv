package mermaid

import (
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Font metrics come from the embedded Go Regular face. Emitted SVG uses the
// theme's font stack; padding absorbs the small cross-font drift.
// ponytail: one cached face per font size, no eviction — sizes in practice
// are one or two values from themes.
var (
	faceMu sync.Mutex
	faces  = map[float64]font.Face{}
	sfnt   = func() *opentype.Font {
		f, err := opentype.Parse(goregular.TTF)
		if err != nil {
			panic(err) // embedded font cannot fail to parse
		}
		return f
	}()
)

func face(size float64) font.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	if f, ok := faces[size]; ok {
		return f
	}
	f, err := opentype.NewFace(sfnt, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingNone,
	})
	if err != nil {
		panic(err)
	}
	faces[size] = f
	return f
}

func measureText(s string, size float64) (w, h float64) {
	f := face(size)
	faceMu.Lock()
	adv := font.MeasureString(f, s)
	m := f.Metrics()
	faceMu.Unlock()
	return fixedToF(adv), fixedToF(m.Ascent + m.Descent)
}

func fixedToF(v fixed.Int26_6) float64 { return float64(v) / 64 }

const (
	padX = 20.0 // horizontal label padding inside a node
	padY = 15.0
)

func measureGraph(g *Graph, t Theme) {
	for _, n := range g.Nodes {
		w, h := measureText(n.Label, t.FontSize)
		n.W, n.H = w+2*padX, h+2*padY
		switch n.Shape {
		case ShapeDiamond:
			// a diamond's inscribed box is half its area: pad up
			n.W, n.H = n.W*1.6, n.H*1.8
		case ShapeCircle:
			d := n.W
			if n.H > d {
				d = n.H
			}
			n.W, n.H = d, d
		case ShapeStateStart, ShapeStateEnd:
			n.W, n.H = 18, 18
		}
	}
	for _, e := range g.Edges {
		if e.Label != "" {
			e.LabelW, e.LabelH = measureText(e.Label, t.FontSize)
		}
	}
}
