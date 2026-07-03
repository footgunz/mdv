package main

import (
	"strings"
	"testing"
)

func TestRenderBodyGFMTable(t *testing.T) {
	out, err := RenderBody([]byte("| a | b |\n|---|---|\n| 1 | 2 |\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "<table") {
		t.Fatalf("expected a table, got: %s", out)
	}
}

func TestRenderBodyMermaidPassthrough(t *testing.T) {
	src := "```mermaid\ngraph TD\nA-->B\n```\n"
	out, err := RenderBody([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `<pre class="mermaid">`) {
		t.Fatalf("expected mermaid pre, got: %s", s)
	}
	if !strings.Contains(s, "graph TD") {
		t.Fatalf("expected diagram source preserved, got: %s", s)
	}
	// arrows must be HTML-escaped so the DOM textContent is the raw source
	if !strings.Contains(s, "A--&gt;B") {
		t.Fatalf("expected escaped arrows, got: %s", s)
	}
}

func TestWikilinkSimple(t *testing.T) {
	out, err := RenderBody([]byte("see [[Home]] now"))
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
	out, err := RenderBody([]byte("[[notes/a|Alpha]]"))
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
	out, err := RenderBody([]byte("[[diagram.png]]"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `href="diagram.png"`) {
		t.Fatalf("expected extension preserved, got: %s", out)
	}
}

func TestRenderBodyCodeHighlight(t *testing.T) {
	src := "```go\nfmt.Println(1)\n```\n"
	out, err := RenderBody([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Println") {
		t.Fatalf("expected code content, got: %s", out)
	}
}

func TestRenderPage(t *testing.T) {
	page := string(RenderPage([]byte("<p>hi</p>"), "My Doc"))
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
	page := string(RenderPage([]byte("x"), "t"))
	if !strings.Contains(page, `<body class="dark">`) {
		t.Fatalf("dark theme missing body class:\n%s", page)
	}
	if !strings.Contains(page, `theme:'dark'`) {
		t.Fatalf("mermaid theme not passed:\n%s", page)
	}

	cfg = defaultConfig()
	page = string(RenderPage([]byte("x"), "t"))
	if !strings.Contains(page, `<body>`) || strings.Contains(page, `class="dark"`) {
		t.Fatalf("light theme should have plain body:\n%s", page)
	}
	if !strings.Contains(page, `theme:'default'`) {
		t.Fatalf("mermaid default theme not passed:\n%s", page)
	}
}
