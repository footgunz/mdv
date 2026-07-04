package mermaid

// Sequence-diagram IR. parseSequence fills identity/text and the item tree;
// layoutSequence fills all geometry (X/Y/W/H, activations, autonumbers);
// emitSequence renders it.

type HeadStyle int

const (
	HeadArrow HeadStyle = iota
	HeadCross
	HeadNone
)

type NotePos int

const (
	NoteLeft NotePos = iota
	NoteRight
	NoteOver
)

type Participant struct {
	ID, Label string
	Actor     bool
	X, W, H   float64 // X = lifeline center, set by layout
}

type SeqItem interface{ seqItem() }

type SeqMessage struct {
	From, To, Text             string
	Dashed                     bool
	Head                       HeadStyle
	ActivateTo, DeactivateFrom bool
	Y                          float64
	Num                        int // 1-based when Autonumber, else 0
}

type SeqNote struct {
	Pos        NotePos
	A, B       string // B set only for "over A,B"
	Text       string
	X, Y, W, H float64
}

type SeqSection struct {
	Label string
	Items []SeqItem
}

type SeqFrame struct {
	Kind       string // loop, opt, alt, par
	Sections   []SeqSection
	X, Y, W, H float64
	DividerYs  []float64 // one per section boundary (len = len(Sections)-1)
}

type SeqActivate struct {
	P  string
	On bool // true = activate, false = deactivate
}

type SeqActivation struct {
	P      *Participant
	Level  int // nesting depth, 0-based
	Y0, Y1 float64
}

type SeqDiagram struct {
	Participants  []*Participant
	Items         []SeqItem
	Autonumber    bool
	Activations   []SeqActivation
	Width, Height float64
}

func (*SeqMessage) seqItem()  {}
func (*SeqNote) seqItem()     {}
func (*SeqFrame) seqItem()    {}
func (*SeqActivate) seqItem() {}

func (d *SeqDiagram) participant(id string) *Participant {
	for _, p := range d.Participants {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Stub replaced by Task 4 (seq_svg.go). layoutSequence lives in seq_layout.go.
func emitSequence(d *SeqDiagram, t Theme) []byte { return nil }
