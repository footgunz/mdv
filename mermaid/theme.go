package mermaid

// Theme colors track assets/base.css (light) and its body.dark block so
// native diagrams match the page.
type Theme struct {
	NodeFill, NodeStroke, EdgeStroke, Text, SubgraphFill, FontFamily string
	FontSize                                                         float64
}

var Light = Theme{
	NodeFill:     "#f6f8fa",
	NodeStroke:   "#57606a",
	EdgeStroke:   "#57606a",
	Text:         "#24292f",
	SubgraphFill: "#eaecef",
	FontFamily:   `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	FontSize:     14,
}

var Dark = Theme{
	NodeFill:     "#161b22",
	NodeStroke:   "#8b949e",
	EdgeStroke:   "#8b949e",
	Text:         "#c9d1d9",
	SubgraphFill: "#21262d",
	FontFamily:   `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`,
	FontSize:     14,
}
