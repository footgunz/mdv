package mermaid

import "fmt"

const (
	seqMargin    = 8.0
	seqMinGap    = 60.0
	seqBoxPadX   = 12.0
	seqBoxPadY   = 8.0
	seqRowPad    = 16.0 // vertical padding above each message line
	seqNotePad   = 6.0
	seqFramePad  = 10.0
	seqHeaderH   = 22.0  // frame kind-tab height
	seqSelfW     = 30.0  // self-message loop width
	seqSelfH     = 20.0  // self-message loop drop
	seqActW      = 10.0  // activation rect width
	seqActNest   = 4.0   // x offset per activation nesting level
	seqActorMinW = 150.0 // mermaid's default actor box width
	seqActorMinH = 50.0
)

func layoutSequence(d *SeqDiagram, t Theme) error {
	if len(d.Participants) == 0 {
		return unsup("no participants")
	}
	idx := map[string]int{}
	for i, p := range d.Participants {
		idx[p.ID] = i
	}

	// Participant boxes: uniform height so lifelines start level.
	boxH := 0.0
	for _, p := range d.Participants {
		w, h := measureText(p.Label, t.FontSize)
		p.W, p.H = w+2*seqBoxPadX, h+2*seqBoxPadY
		if p.W < seqActorMinW {
			p.W = seqActorMinW
		}
		if p.H < seqActorMinH {
			p.H = seqActorMinH
		}
		if p.H > boxH {
			boxH = p.H
		}
	}
	for _, p := range d.Participants {
		p.H = boxH
	}

	// Horizontal: per-adjacent-gap minimums, widened by labels crossing them.
	gaps := make([]float64, len(d.Participants)-1)
	for i := range gaps {
		gaps[i] = seqMinGap
	}
	leftPad, rightPad := 0.0, 0.0
	needGap := func(i, j int, w float64) { // between participant idx i and j (i<j), adjacent only
		if j-i == 1 && w > gaps[i] {
			gaps[i] = w
		}
	}
	var scanH func(items []SeqItem)
	scanH = func(items []SeqItem) {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqMessage:
				lw, _ := measureText(numbered(v, d.Autonumber), t.FontSize)
				i, j := idx[v.From], idx[v.To]
				if i == j { // self-message needs room to the right
					if ext := seqSelfW*1.6 + lw + 2*seqNotePad; i == len(d.Participants)-1 && ext > rightPad {
						rightPad = ext
					} else if i < len(d.Participants)-1 {
						needGap(i, i+1, ext+seqMinGap/2)
					}
					continue
				}
				if i > j {
					i, j = j, i
				}
				needGap(i, j, lw+2*seqNotePad+seqMinGap/2)
			case *SeqNote:
				lw, _ := measureText(v.Text, t.FontSize)
				w := lw + 2*seqNotePad
				a := idx[v.A]
				switch v.Pos {
				case NoteLeft:
					if a == 0 && w > leftPad {
						leftPad = w
					} else if a > 0 {
						needGap(a-1, a, w+seqMinGap/2)
					}
				case NoteRight:
					if a == len(d.Participants)-1 && w > rightPad {
						rightPad = w
					} else if a < len(d.Participants)-1 {
						needGap(a, a+1, w+seqMinGap/2)
					}
				case NoteOver:
					if v.B != "" {
						b := idx[v.B]
						if b < a {
							a, b = b, a
						}
						needGap(a, b, w-((d.Participants[a].W+d.Participants[b].W)/2))
					}
				}
			case *SeqFrame:
				for _, s := range v.Sections {
					scanH(s.Items)
				}
			}
		}
	}
	scanH(d.Items)

	x := seqMargin + leftPad + d.Participants[0].W/2
	for i, p := range d.Participants {
		p.X = x
		if i < len(gaps) {
			x += p.W/2 + gaps[i] + d.Participants[i+1].W/2
		}
	}
	d.Width = d.Participants[len(d.Participants)-1].X +
		d.Participants[len(d.Participants)-1].W/2 + rightPad + seqMargin

	// Vertical walk + activations + autonumber.
	_, lineH := measureText("Ag", t.FontSize)
	rowH := lineH + seqRowPad
	y := seqMargin + boxH + rowH
	num := 0
	type actOpen struct {
		y0    float64
		level int
	}
	open := map[string][]actOpen{} // participant id -> stack
	depth := map[string]int{}

	openAct := func(id string, at float64) {
		open[id] = append(open[id], actOpen{y0: at, level: depth[id]})
		depth[id]++
	}
	closeAct := func(id string, at float64) error {
		s := open[id]
		if len(s) == 0 {
			return unsup("deactivate %q without matching activate", id)
		}
		top := s[len(s)-1]
		open[id] = s[:len(s)-1]
		depth[id]--
		d.Activations = append(d.Activations, SeqActivation{
			P: d.participant(id), Level: top.level, Y0: top.y0, Y1: at,
		})
		return nil
	}

	var walkV func(items []SeqItem) error
	walkV = func(items []SeqItem) error {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqMessage:
				if d.Autonumber {
					num++
					v.Num = num
				}
				y += rowH
				v.Y = y
				if v.From == v.To {
					y += seqSelfH // self-loop consumes extra height
				}
				if v.ActivateTo {
					openAct(v.To, v.Y)
				}
				if v.DeactivateFrom {
					if err := closeAct(v.From, v.Y); err != nil {
						return err
					}
				}
			case *SeqNote:
				lw, lh := measureText(v.Text, t.FontSize)
				v.W, v.H = lw+2*seqNotePad, lh+2*seqNotePad
				y += rowH
				v.Y = y
				y = v.Y + v.H + seqRowPad // cursor to note bottom + padding
				a := d.participant(v.A)
				switch v.Pos {
				case NoteLeft:
					v.X = a.X - seqActW - v.W
				case NoteRight:
					v.X = a.X + seqActW
				case NoteOver:
					right := a
					if v.B != "" {
						right = d.participant(v.B)
					}
					lo, hi := a.X, right.X
					if lo > hi {
						lo, hi = hi, lo
					}
					v.X = (lo+hi)/2 - v.W/2
				}
			case *SeqActivate:
				if v.On {
					openAct(v.P, y+rowH/2)
				} else if err := closeAct(v.P, y+rowH/2); err != nil {
					return err
				}
			case *SeqFrame:
				v.Y = y + seqRowPad
				y += seqRowPad + seqHeaderH
				for si, s := range v.Sections {
					if si > 0 {
						y += seqRowPad
						v.DividerYs = append(v.DividerYs, y)
						y += seqHeaderH // room for the [section label] band
					}
					if err := walkV(s.Items); err != nil {
						return err
					}
				}
				y += seqFramePad
				v.H = y - v.Y
				lo, hi := frameSpan(v, d, idx)
				v.X = lo - seqFramePad
				v.W = hi - lo + 2*seqFramePad
			}
		}
		return nil
	}
	if err := walkV(d.Items); err != nil {
		return err
	}

	// Auto-close activations still open.
	bottom := y + rowH
	for _, p := range d.Participants {
		for range open[p.ID] {
			if err := closeAct(p.ID, bottom); err != nil {
				return fmt.Errorf("internal: %w", err)
			}
		}
	}
	d.Height = bottom + seqMargin
	return nil
}

// numbered returns the display text of a message (autonumber prefix applied).
// Shared with seq_svg.go.
func numbered(m *SeqMessage, auto bool) string {
	if auto && m.Num > 0 {
		return fmt.Sprintf("%d. %s", m.Num, m.Text)
	}
	if auto {
		return m.Text // horizontal pass runs before Num assignment; close enough for width
	}
	return m.Text
}

// frameSpan returns the min/max lifeline x among participants referenced
// anywhere inside the frame; empty frames span all participants.
func frameSpan(f *SeqFrame, d *SeqDiagram, idx map[string]int) (float64, float64) {
	lo, hi := -1.0, -1.0
	touch := func(id string) {
		x := d.participant(id).X
		if lo < 0 || x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	var scan func(items []SeqItem)
	scan = func(items []SeqItem) {
		for _, it := range items {
			switch v := it.(type) {
			case *SeqMessage:
				touch(v.From)
				touch(v.To)
			case *SeqNote:
				touch(v.A)
				if v.B != "" {
					touch(v.B)
				}
			case *SeqFrame:
				for _, s := range v.Sections {
					scan(s.Items)
				}
			}
		}
	}
	for _, s := range f.Sections {
		scan(s.Items)
	}
	if lo < 0 {
		lo, hi = d.Participants[0].X, d.Participants[len(d.Participants)-1].X
	}
	return lo, hi
}
