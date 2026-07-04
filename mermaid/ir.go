package mermaid

// Intermediate representation of a parsed diagram. parse fills identity and
// labels; text measurement fills W/H; layout fills X/Y and edge geometry.

type Shape int

const (
	ShapeRect Shape = iota
	ShapeRound
	ShapeStadium
	ShapeDiamond
	ShapeCircle
)

type EdgeStyle int

const (
	EdgeSolid EdgeStyle = iota
	EdgeDotted
	EdgeThick
)

type Point struct{ X, Y float64 }

type Node struct {
	ID, Label string
	Shape     Shape
	W, H      float64 // set by measurement
	X, Y      float64 // center, set by layout
}

type Edge struct {
	From, To, Label string
	Style           EdgeStyle
	Directed        bool
	Points          []Point // polyline, set by layout
	LabelX, LabelY  float64
	LabelW, LabelH  float64
}

type Subgraph struct {
	ID, Title  string
	Children   []string // node IDs
	X, Y, W, H float64  // top-left + size, set by layout
}

type Graph struct {
	Direction     string // TD, TB, LR, RL, BT
	Nodes         []*Node
	Edges         []*Edge
	Subgraphs     []*Subgraph
	Width, Height float64 // set by layout
}

func (g *Graph) node(id string) *Node {
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}
