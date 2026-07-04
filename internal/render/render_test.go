package render

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dgunther/mdv/internal/config"
)

func testRenderer(mod func(*config.Config)) Renderer {
	c := config.Default()
	if mod != nil {
		mod(&c)
	}
	return Renderer{Cfg: c}
}

func TestRenderStaticPage(t *testing.T) {
	page := string(testRenderer(nil).StaticPage([]byte("<p>hi</p>"), "Doc", false))
	for _, want := range []string{"<title>Doc</title>", "<p>hi</p>", "<style>", ".markdown-body"} {
		if !strings.Contains(page, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, bad := range []string{"/_assets/", "/wails/", "mermaid.initialize"} {
		if strings.Contains(page, bad) {
			t.Fatalf("static page must not contain %q", bad)
		}
	}
}

func TestRenderStaticPageMermaidJS(t *testing.T) {
	page := string(testRenderer(nil).StaticPage([]byte("x"), "t", true))
	if !strings.Contains(page, "mermaid.initialize") {
		t.Fatal("fallback static page must bootstrap mermaid")
	}
	if len(page) < 1_000_000 {
		t.Fatalf("mermaid payload not inlined (len=%d)", len(page))
	}
	if strings.Contains(page, "/_assets/") {
		t.Fatal("static page must not reference served assets")
	}
}

func TestRenderStaticPageUserCSSAndTheme(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "u.css")
	if err := os.WriteFile(css, []byte(".markdown-body{letter-spacing:9px}"), 0o644); err != nil {
		t.Fatal(err)
	}
	rend := testRenderer(func(c *config.Config) {
		c.CSS = css
		c.Theme = "dark"
	})
	page := string(rend.StaticPage([]byte("x"), "t", false))
	if !strings.Contains(page, "letter-spacing:9px") {
		t.Fatal("user css not inlined")
	}
	if !strings.Contains(page, `<body class="dark">`) {
		t.Fatal("dark body class missing")
	}

	rend.Cfg.CSS = filepath.Join(dir, "missing.css")
	page = string(rend.StaticPage([]byte("x"), "t", false))
	if !strings.Contains(page, "<style>") || strings.Contains(page, "letter-spacing") {
		t.Fatal("unreadable user css must be skipped silently")
	}
}

func TestRenderBodyGFMTable(t *testing.T) {
	out, _, err := testRenderer(nil).Body([]byte("| a | b |\n|---|---|\n| 1 | 2 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<table") {
		t.Fatalf("expected a table, got: %s", out)
	}
}

func TestRenderBodyMermaidPassthrough(t *testing.T) {
	src := "```mermaid\ngantt\ntitle X\n```\n"
	out, fallback, err := testRenderer(nil).Body([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `<pre class="mermaid">`) || !fallback {
		t.Fatalf("expected fallback pre, got: %s", s)
	}
	if !strings.Contains(s, "gantt") {
		t.Fatalf("expected diagram source preserved, got: %s", s)
	}
}

func TestRenderBodyNativeMermaid(t *testing.T) {
	out, fallback, err := testRenderer(nil).Body([]byte("```mermaid\ngraph TD\nA[Hi] --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<svg") || !strings.Contains(s, "mermaid-svg") {
		t.Fatalf("expected native svg, got: %s", s)
	}
	if strings.Contains(s, `<pre class="mermaid">`) {
		t.Fatalf("native path must not emit fallback pre: %s", s)
	}
	if fallback {
		t.Fatal("fallback flag must be false for supported diagram")
	}
}

func TestRenderBodyMermaidFallback(t *testing.T) {
	out, fallback, err := testRenderer(nil).Body([]byte("```mermaid\nclassDiagram\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<pre class="mermaid">`) {
		t.Fatalf("unsupported diagram must fall back: %s", out)
	}
	if !fallback {
		t.Fatal("fallback flag must be true")
	}
	// fallback source must stay HTML-escaped (regression guard)
	out2, _, err := testRenderer(nil).Body([]byte("```mermaid\nclassDiagram\na->>b\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out2), "a-&gt;&gt;b") {
		t.Fatalf("fallback source not escaped: %s", out2)
	}
}

func TestRenderBodyNativeSequence(t *testing.T) {
	out, fallback, err := testRenderer(nil).Body([]byte("```mermaid\nsequenceDiagram\nAlice->>Bob: hi\nBob-->>Alice: yo\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<svg") || strings.Contains(s, `<pre class="mermaid">`) {
		t.Fatalf("sequence must render natively: %s", s)
	}
	if fallback {
		t.Fatal("fallback flag must be false")
	}
}

func TestRenderPageMermaidJSConditional(t *testing.T) {
	with := string(testRenderer(nil).Page([]byte("x"), "t", true))
	without := string(testRenderer(nil).Page([]byte("x"), "t", false))
	if !strings.Contains(with, "mermaid.min.js") || !strings.Contains(with, "mermaid.initialize") {
		t.Fatal("fallback page must include mermaid js")
	}
	if strings.Contains(without, "mermaid.min.js") || strings.Contains(without, "mermaid.initialize") {
		t.Fatal("native-only page must not ship mermaid js")
	}
}

func TestRenderBodyNativeMermaidDarkTheme(t *testing.T) {
	rend := testRenderer(func(c *config.Config) { c.Theme = "dark" })
	out, _, err := rend.Body([]byte("```mermaid\ngraph TD\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "#161b22") { // mermaid.Dark.NodeFill
		t.Fatalf("dark theme not applied to svg: %s", out)
	}
}

func TestWikilinkSimple(t *testing.T) {
	out, _, err := testRenderer(nil).Body([]byte("see [[Home]] now"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `href="Home.md"`) {
		t.Fatalf("expected rewritten href, got: %s", s)
	}
	if !strings.Contains(s, ">Home<") {
		t.Fatalf("expected label Home, got: %s", s)
	}
}

func TestWikilinkAliased(t *testing.T) {
	out, _, err := testRenderer(nil).Body([]byte("[[notes/a|Alpha]]"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `href="notes/a.md"`) {
		t.Fatalf("expected href notes/a.md, got: %s", s)
	}
	if !strings.Contains(s, ">Alpha<") {
		t.Fatalf("expected label Alpha, got: %s", s)
	}
}

func TestWikilinkKeepsExtension(t *testing.T) {
	out, _, err := testRenderer(nil).Body([]byte("[[diagram.png]]"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `href="diagram.png"`) {
		t.Fatalf("expected extension preserved, got: %s", out)
	}
}

func TestRenderBodyCodeHighlight(t *testing.T) {
	src := "```go\nfmt.Println(1)\n```\n"
	out, _, err := testRenderer(nil).Body([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "Println") {
		t.Fatalf("expected code content, got: %s", out)
	}
	if !strings.Contains(s, "<pre") {
		t.Fatalf("expected a <pre> block, got: %s", out)
	}
	for _, bad := range []string{"<html", "<style", "<body"} {
		if strings.Contains(s, bad) {
			t.Fatalf("expected fragment, not a full document (found %q): %s", bad, out)
		}
	}
}

func TestRenderBodyCodeHighlightUnknownLanguage(t *testing.T) {
	src := "```zzznotalang\n<b>hi</b>\n```\n"
	out, _, err := testRenderer(nil).Body([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "<pre><code>") {
		t.Fatalf("expected plain <pre><code> fallback, got: %s", out)
	}
	if !strings.Contains(s, "&lt;b&gt;hi&lt;/b&gt;") {
		t.Fatalf("expected code to be HTML-escaped, got: %s", out)
	}
}

func TestRenderPage(t *testing.T) {
	page := string(testRenderer(nil).Page([]byte("<p>hi</p>"), "My Doc", true))
	for _, want := range []string{
		"<title>My Doc</title>",
		"<p>hi</p>",
		"/_assets/base.css",
		"/_assets/mermaid.min.js",
		"/wails/runtime.js",
		"mdv:reload",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("page missing %q:\n%s", want, page)
		}
	}
}

func TestRenderPageTheme(t *testing.T) {
	rend := testRenderer(func(c *config.Config) {
		c.Theme = "dark"
		c.MermaidTheme = "dark"
	})
	page := string(rend.Page([]byte("x"), "t", true))
	if !strings.Contains(page, `<body class="dark">`) {
		t.Fatalf("dark theme missing body class:\n%s", page)
	}
	if !strings.Contains(page, `theme:'dark'`) {
		t.Fatalf("mermaid theme not passed:\n%s", page)
	}

	rend = testRenderer(nil)
	page = string(rend.Page([]byte("x"), "t", true))
	if !strings.Contains(page, `<body>`) || strings.Contains(page, `class="dark"`) {
		t.Fatalf("light theme should have plain body:\n%s", page)
	}
	if !strings.Contains(page, `theme:'default'`) {
		t.Fatalf("mermaid default theme not passed:\n%s", page)
	}
}

func TestRenderPageMermaidThemeEscaping(t *testing.T) {
	rend := testRenderer(func(c *config.Config) { c.MermaidTheme = "x'-alert(1)-'" })
	page := string(rend.Page([]byte("x"), "t", true))
	if strings.Contains(page, `theme:'x'-`) {
		t.Fatalf("mermaid theme value broke out of the JS string literal unescaped:\n%s", page)
	}
	if !strings.Contains(page, template.JSEscapeString(rend.Cfg.MermaidTheme)) {
		t.Fatalf("expected escaped mermaid theme value present:\n%s", page)
	}
}

func TestRenderBodyMermaidJSMode(t *testing.T) {
	rend := testRenderer(func(c *config.Config) { c.MermaidRenderer = "js" })
	out, fallback, err := rend.Body([]byte("```mermaid\ngraph TD\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "<svg") {
		t.Fatalf("js mode must not render natively: %s", s)
	}
	if !strings.Contains(s, `<pre class="mermaid">`) || !fallback {
		t.Fatalf("js mode must use the fallback path (fallback=%v): %s", fallback, s)
	}
}

func TestRenderBodyNativeState(t *testing.T) {
	out, fallback, err := testRenderer(nil).Body([]byte("```mermaid\nstateDiagram-v2\n[*] --> A\nA --> [*]\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<svg") || fallback {
		t.Fatalf("state must render natively (fallback=%v): %s", fallback, out)
	}
}

func TestRenderBodyNativePie(t *testing.T) {
	out, fallback, err := testRenderer(nil).Body([]byte("```mermaid\npie\ntitle T\n\"a\" : 1\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<svg") || fallback {
		t.Fatalf("pie must render natively (fallback=%v): %s", fallback, out)
	}
}
