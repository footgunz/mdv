package main

import (
	"html/template"
	"strings"
	"testing"
)

func TestRenderBodyGFMTable(t *testing.T) {
	out, _, err := RenderBody([]byte("| a | b |\n|---|---|\n| 1 | 2 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<table") {
		t.Fatalf("expected a table, got: %s", out)
	}
}

func TestRenderBodyMermaidPassthrough(t *testing.T) {
	src := "```mermaid\ngantt\ntitle X\n```\n"
	out, fallback, err := RenderBody([]byte(src))
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
	out, fallback, err := RenderBody([]byte("```mermaid\ngraph TD\nA[Hi] --> B\n```\n"))
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
	out, fallback, err := RenderBody([]byte("```mermaid\nclassDiagram\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `<pre class="mermaid">`) {
		t.Fatalf("unsupported diagram must fall back: %s", out)
	}
	if !fallback {
		t.Fatal("fallback flag must be true")
	}
}

func TestRenderPageMermaidJSConditional(t *testing.T) {
	with := string(RenderPage([]byte("x"), "t", true))
	without := string(RenderPage([]byte("x"), "t", false))
	if !strings.Contains(with, "mermaid.min.js") || !strings.Contains(with, "mermaid.initialize") {
		t.Fatal("fallback page must include mermaid js")
	}
	if strings.Contains(without, "mermaid.min.js") || strings.Contains(without, "mermaid.initialize") {
		t.Fatal("native-only page must not ship mermaid js")
	}
}

func TestRenderBodyNativeMermaidDarkTheme(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)
	cfg.Theme = "dark"
	out, _, err := RenderBody([]byte("```mermaid\ngraph TD\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "#161b22") { // mermaid.Dark.NodeFill
		t.Fatalf("dark theme not applied to svg: %s", out)
	}
}

func TestWikilinkSimple(t *testing.T) {
	out, _, err := RenderBody([]byte("see [[Home]] now"))
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
	out, _, err := RenderBody([]byte("[[notes/a|Alpha]]"))
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
	out, _, err := RenderBody([]byte("[[diagram.png]]"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `href="diagram.png"`) {
		t.Fatalf("expected extension preserved, got: %s", out)
	}
}

func TestRenderBodyCodeHighlight(t *testing.T) {
	src := "```go\nfmt.Println(1)\n```\n"
	out, _, err := RenderBody([]byte(src))
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
	out, _, err := RenderBody([]byte(src))
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
	page := string(RenderPage([]byte("<p>hi</p>"), "My Doc", true))
	for _, want := range []string{
		"<title>My Doc</title>",
		"<p>hi</p>",
		"/_assets/base.css",
		"/_assets/mermaid.min.js",
		"/_events",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("page missing %q:\n%s", want, page)
		}
	}
}

func TestRenderPageTheme(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)

	cfg.Theme = "dark"
	cfg.MermaidTheme = "dark"
	page := string(RenderPage([]byte("x"), "t", true))
	if !strings.Contains(page, `<body class="dark">`) {
		t.Fatalf("dark theme missing body class:\n%s", page)
	}
	if !strings.Contains(page, `theme:'dark'`) {
		t.Fatalf("mermaid theme not passed:\n%s", page)
	}

	cfg = defaultConfig()
	page = string(RenderPage([]byte("x"), "t", true))
	if !strings.Contains(page, `<body>`) || strings.Contains(page, `class="dark"`) {
		t.Fatalf("light theme should have plain body:\n%s", page)
	}
	if !strings.Contains(page, `theme:'default'`) {
		t.Fatalf("mermaid default theme not passed:\n%s", page)
	}
}

func TestRenderPageMermaidThemeEscaping(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)

	cfg.MermaidTheme = "x'-alert(1)-'"
	page := string(RenderPage([]byte("x"), "t", true))
	if strings.Contains(page, `theme:'x'-`) {
		t.Fatalf("mermaid theme value broke out of the JS string literal unescaped:\n%s", page)
	}
	if !strings.Contains(page, template.JSEscapeString(cfg.MermaidTheme)) {
		t.Fatalf("expected escaped mermaid theme value present:\n%s", page)
	}
}
