package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseConfigEmpty(t *testing.T) {
	c, warns := parseConfig(nil)
	if c != defaultConfig() {
		t.Fatalf("empty input should give defaults, got %+v", c)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	d := defaultConfig()
	if d.WindowWidth != 900 || d.WindowHeight != 1000 || d.Theme != "light" ||
		d.CSS != "" || d.MermaidTheme != "default" || !d.Watch {
		t.Fatalf("bad defaults: %+v", d)
	}
}

func TestParseConfigAllKeys(t *testing.T) {
	src := `
# a comment
window-width = 1200
window-height = 800

theme = dark
mermaid-theme = forest
watch = false
`
	c, warns := parseConfig([]byte(src))
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if c.WindowWidth != 1200 || c.WindowHeight != 800 || c.Theme != "dark" ||
		c.MermaidTheme != "forest" || c.Watch {
		t.Fatalf("got %+v", c)
	}
}

func TestParseConfigCSSTildeExpansion(t *testing.T) {
	c, _ := parseConfig([]byte("css = ~/notes/custom.css"))
	home, _ := os.UserHomeDir()
	if c.CSS != filepath.Join(home, "notes/custom.css") {
		t.Fatalf("got %q", c.CSS)
	}
	c, _ = parseConfig([]byte("css = /abs/path.css"))
	if c.CSS != "/abs/path.css" {
		t.Fatalf("absolute path mangled: %q", c.CSS)
	}
}

func TestParseConfigMermaidFollowsDarkTheme(t *testing.T) {
	c, _ := parseConfig([]byte("theme = dark"))
	if c.MermaidTheme != "dark" {
		t.Fatalf("mermaid should default to dark with dark theme, got %q", c.MermaidTheme)
	}
	// explicit mermaid-theme wins regardless of order
	c, _ = parseConfig([]byte("mermaid-theme = forest\ntheme = dark"))
	if c.MermaidTheme != "forest" {
		t.Fatalf("explicit mermaid-theme overridden: %q", c.MermaidTheme)
	}
}

func TestParseConfigWarnings(t *testing.T) {
	cases := []struct {
		src  string
		want string // substring of the warning
	}{
		{"nonsense-key = 1", "unknown key"},
		{"window-width = abc", "window-width"},
		{"window-width = -5", "window-width"},
		{"watch = maybe", "watch"},
		{"theme = blue", "theme"},
		{"no equals sign here", "bad line"},
	}
	for _, tc := range cases {
		c, warns := parseConfig([]byte(tc.src))
		if len(warns) != 1 || !strings.Contains(warns[0], tc.want) {
			t.Fatalf("%q: want one warning containing %q, got %v", tc.src, tc.want, warns)
		}
		if c != defaultConfig() {
			t.Fatalf("%q: bad value must keep defaults, got %+v", tc.src, c)
		}
	}
}

func TestConfigPathXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if got := configPath(); got != filepath.Join("/xdg", "mdthing", "config") {
		t.Fatalf("got %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	if got := configPath(); got != filepath.Join(home, ".config", "mdthing", "config") {
		t.Fatalf("got %q", got)
	}
}

func TestParseConfigMermaidRenderer(t *testing.T) {
	if d := defaultConfig(); d.MermaidRenderer != "native" {
		t.Fatalf("default renderer %q, want native", d.MermaidRenderer)
	}
	c, warns := parseConfig([]byte("mermaid-renderer = js"))
	if c.MermaidRenderer != "js" || len(warns) != 0 {
		t.Fatalf("js: got %q warns %v", c.MermaidRenderer, warns)
	}
	c, warns = parseConfig([]byte("mermaid-renderer = native"))
	if c.MermaidRenderer != "native" || len(warns) != 0 {
		t.Fatalf("native: got %q warns %v", c.MermaidRenderer, warns)
	}
	c, warns = parseConfig([]byte("mermaid-renderer = webgl"))
	if c.MermaidRenderer != "native" || len(warns) != 1 || !strings.Contains(warns[0], "mermaid-renderer") {
		t.Fatalf("bad value: got %q warns %v", c.MermaidRenderer, warns)
	}
}
