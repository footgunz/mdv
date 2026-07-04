package mermaid

import (
	"errors"
	"strings"
	"testing"
)

func TestPieParse(t *testing.T) {
	p, err := parsePie("pie showData\ntitle Languages\n%% comment\n\"Go\" : 60\n\"JS\" : 25.5\n\"Other\" : 14.5")
	if err != nil {
		t.Fatal(err)
	}
	if !p.ShowData || p.Title != "Languages" || len(p.Slices) != 3 {
		t.Fatalf("got %+v", p)
	}
	// sorted by value descending
	if p.Slices[0].Label != "Go" || p.Slices[1].Label != "JS" || p.Slices[2].Label != "Other" {
		t.Fatalf("order: %+v", p.Slices)
	}
	p2, err := parsePie("pie\n\"b\" : 1\n\"a\" : 2")
	if err != nil || p2.ShowData || p2.Title != "" {
		t.Fatalf("plain pie: %+v %v", p2, err)
	}
	if p2.Slices[0].Label != "a" {
		t.Fatalf("descending sort: %+v", p2.Slices)
	}
}

func TestPieParseErrors(t *testing.T) {
	for _, src := range []string{
		"pie",                       // no slices
		"pie\n\"a\" : 0",            // zero total
		"pie\n\"a\" : -3",           // negative
		"pie\n\"a\" : x",            // bad number
		"pie\nnonsense here",        // unknown line
		"pie extra words\n\"a\": 1", // bad header
	} {
		if _, err := parsePie(src); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: want ErrUnsupported, got %v", src, err)
		}
	}
}

func TestPieSVG(t *testing.T) {
	out := string(mustRenderPie(t, "pie showData\ntitle Split\n\"Alpha\" : 70\n\"Beta\" : 30", Light))
	if c := strings.Count(out, `class="pie-slice"`); c != 2 {
		t.Fatalf("slices %d, want 2\n%s", c, out)
	}
	for _, want := range []string{">Split<", "70%", "30%", ">Alpha [70]<", ">Beta [30]<", Light.Palette[0], Light.Palette[1]} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q\n%s", want, out)
		}
	}
	if c := strings.Count(out, `class="pie-legend"`); c != 2 {
		t.Fatalf("legend rows %d, want 2", c)
	}
}

func TestPieSVGSingleSliceAndSmallSlices(t *testing.T) {
	one := string(mustRenderPie(t, "pie\n\"all\" : 5", Light))
	if !strings.Contains(one, "<circle") {
		t.Fatalf("single slice must render a full circle:\n%s", one)
	}
	small := string(mustRenderPie(t, "pie\n\"big\" : 97\n\"tiny\" : 3", Light))
	if strings.Contains(small, ">3%<") {
		t.Fatal("slices under 5% must not get a percent label")
	}
	if !strings.Contains(small, ">97%<") {
		t.Fatal("big slice keeps its percent label")
	}
}

func TestPiePaletteCyclesAndThemes(t *testing.T) {
	var b strings.Builder
	b.WriteString("pie\n")
	for i := 0; i < 10; i++ {
		b.WriteString(`"s` + string(rune('a'+i)) + `" : 1` + "\n")
	}
	out := string(mustRenderPie(t, b.String(), Light))
	if !strings.Contains(out, Light.Palette[0]) || strings.Count(out, Light.Palette[0]) < 2 {
		t.Fatal("palette must cycle past 8 slices (color 0 reused)")
	}
	dark := string(mustRenderPie(t, "pie\n\"a\" : 1", Dark))
	if !strings.Contains(dark, Dark.Palette[0]) {
		t.Fatal("dark palette not applied")
	}
	if len(Light.Palette) != 8 || len(Dark.Palette) != 8 {
		t.Fatalf("palettes must have 8 entries: %d/%d", len(Light.Palette), len(Dark.Palette))
	}
}

func TestPieEscaping(t *testing.T) {
	out := string(mustRenderPie(t, "pie\ntitle <b>&title\n\"<script>\" : 1", Light))
	if strings.Contains(out, "<script>") || strings.Contains(out, "<b>&title") {
		t.Fatalf("unescaped user text:\n%s", out)
	}
}

func mustRenderPie(t *testing.T, src string, theme Theme) []byte {
	t.Helper()
	out, err := Render([]byte(src), theme)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}
