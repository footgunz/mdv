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
