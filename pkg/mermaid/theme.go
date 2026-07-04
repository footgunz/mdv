package mermaid

// Theme colors track assets/base.css (light) and its body.dark block so
// native diagrams match the page.
type Theme struct {
	NodeFill, NodeStroke, EdgeStroke, Text, SubgraphFill, FontFamily string
	FontSize                                                         float64
	Palette                                                          []string // categorical slice colors (pie), cycled when exhausted
}

var Light = Theme{
	NodeFill:     "#f6f8fa",
	NodeStroke:   "#57606a",
	EdgeStroke:   "#57606a",
	Text:         "#24292f",
	SubgraphFill: "#eaecef",
	FontFamily:   `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	FontSize:     14,
	Palette:      []string{"#4e79a7", "#f28e2b", "#59a14f", "#e15759", "#b07aa1", "#edc949", "#76b7b2", "#ff9da7"},
}

var Dark = Theme{
	NodeFill:     "#161b22",
	NodeStroke:   "#8b949e",
	EdgeStroke:   "#8b949e",
	Text:         "#c9d1d9",
	SubgraphFill: "#21262d",
	FontFamily:   `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	FontSize:     14,
	Palette:      []string{"#58a6ff", "#f0883e", "#3fb950", "#f85149", "#bc8cff", "#d29922", "#39c5cf", "#ff7b72"},
}
