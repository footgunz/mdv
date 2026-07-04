package mermaid

import (
	"bytes"
	"encoding/xml"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

func render(t *testing.T, src string, theme Theme) []byte {
	t.Helper()
	out, err := Render([]byte(src), theme)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

func TestSVGWellFormed(t *testing.T) {
	out := render(t, "graph TD\na[Start] --> b{Q} -->|yes| c((C))\nb -->|no| d([End])\nsubgraph s [G]\n e --- f\nend", Light)
	d := xml.NewDecoder(bytes.NewReader(out))
	for {
		_, err := d.Token()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("invalid XML: %v\n%s", err, out)
		}
	}
	s := string(out)
	if !strings.HasPrefix(s, "<svg") || !strings.Contains(s, "viewBox=") {
		t.Fatalf("not an svg root: %.80s", s)
	}
	if !strings.Contains(s, `class="mermaid-svg"`) {
		t.Fatal("missing mermaid-svg class")
	}
}

func TestSVGShapeAndEdgeCounts(t *testing.T) {
	out := string(render(t, "graph TD\na[R] --> b(Rd) --> c([St]) --> d{Di} --> e((Ci))", Light))
	if c := strings.Count(out, "<rect"); c != 3 { // rect + round + stadium
		t.Fatalf("rects = %d, want 3\n%s", c, out)
	}
	if c := strings.Count(out, "<polygon"); c != 1 {
		t.Fatalf("polygons = %d, want 1", c)
	}
	if c := strings.Count(out, "<circle"); c != 1 {
		t.Fatalf("circles = %d, want 1", c)
	}
	if c := strings.Count(out, `class="edge"`); c != 4 {
		t.Fatalf("edges = %d, want 4", c)
	}
	for _, lbl := range []string{">R<", ">Rd<", ">St<", ">Di<", ">Ci<"} {
		if !strings.Contains(out, lbl) {
			t.Fatalf("missing label %s", lbl)
		}
	}
}

func TestSVGEdgeStyles(t *testing.T) {
	out := string(render(t, "graph TD\na --> b\nc -.-> d\ne ==> f\ng --- h", Light))
	if !strings.Contains(out, "stroke-dasharray") {
		t.Fatal("dotted edge missing dasharray")
	}
	if !strings.Contains(out, `marker-end="url(#arrow)"`) {
		t.Fatal("directed edge missing arrowhead")
	}
	// undirected g --- h: exactly 3 of 4 edges carry the marker
	if c := strings.Count(out, `marker-end`); c != 3 {
		t.Fatalf("marker count %d, want 3", c)
	}
}

func TestSVGLabelEscaping(t *testing.T) {
	out := string(render(t, "graph TD\na[\"<script> & fun\"]", Light))
	if strings.Contains(out, "<script>") {
		t.Fatal("label not escaped")
	}
	if !strings.Contains(out, "&lt;script&gt; &amp; fun") {
		t.Fatalf("expected escaped label, got:\n%s", out)
	}
}

func TestSVGThemeApplied(t *testing.T) {
	light := string(render(t, "graph TD\na --> b", Light))
	dark := string(render(t, "graph TD\na --> b", Dark))
	if !strings.Contains(light, Light.NodeFill) || !strings.Contains(dark, Dark.NodeFill) {
		t.Fatal("theme fills not applied")
	}
	if light == dark {
		t.Fatal("themes produced identical output")
	}
}

func TestGoldens(t *testing.T) {
	mmds, err := filepath.Glob("testdata/*.mmd")
	if err != nil || len(mmds) == 0 {
		t.Fatalf("no goldens found: %v", err)
	}
	for _, mmd := range mmds {
		src, err := os.ReadFile(mmd)
		if err != nil {
			t.Fatal(err)
		}
		got := render(t, string(src), Light)
		golden := strings.TrimSuffix(mmd, ".mmd") + ".svg"
		if *update {
			if err := os.WriteFile(golden, got, 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatalf("missing golden %s (run: go test ./mermaid/ -update)", golden)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s differs from golden (run -update and eyeball the diff)", mmd)
		}
	}
}
