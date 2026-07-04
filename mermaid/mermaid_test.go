package mermaid

import (
	"errors"
	"testing"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		src  string
		kind string
	}{
		{"graph TD\nA-->B", "graph"},
		{"flowchart LR\nA-->B", "flowchart"},
		{"%% comment\n\nflowchart TD\nA", "flowchart"},
		{"pie\n\"a\": 1", "pie"},
		{"sequenceDiagram\nA->>B: hi", "sequenceDiagram"},
		{"", ""},
	}
	for _, tc := range cases {
		kind, _ := detect(tc.src)
		if kind != tc.kind {
			t.Fatalf("detect(%q) = %q, want %q", tc.src, kind, tc.kind)
		}
	}
}

func TestRenderUnsupportedKind(t *testing.T) {
	_, err := Render([]byte("sequenceDiagram\nA->>B: hi"), Light)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("want ErrUnsupported, got %v", err)
	}
	_, err = Render([]byte(""), Light)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("empty: want ErrUnsupported, got %v", err)
	}
}

func TestThemes(t *testing.T) {
	if Light.NodeFill == "" || Dark.NodeFill == "" || Light.NodeFill == Dark.NodeFill {
		t.Fatalf("themes must differ and be populated: %+v %+v", Light, Dark)
	}
	if Light.FontSize <= 0 {
		t.Fatalf("font size unset")
	}
}
