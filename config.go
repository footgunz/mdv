package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is mdv's runtime configuration. Extending the schema = add a
// field here and a case in parseConfig.
type Config struct {
	WindowWidth     int
	WindowHeight    int
	Theme           string // "light" or "dark"
	CSS             string // path to an extra user stylesheet, "" = none
	MermaidTheme    string // passed to mermaid.initialize
	MermaidRenderer string // "native" (built-in SVG engine) or "js" (mermaid.js)
	Watch           bool   // live reload on file change
}

func defaultConfig() Config {
	return Config{
		WindowWidth:     900,
		WindowHeight:    1000,
		Theme:           "light",
		MermaidTheme:    "default",
		MermaidRenderer: "native",
		Watch:           true,
	}
}

// cfg is the active configuration, set by main() before anything renders.
var cfg = defaultConfig()

// parseConfig parses ghostty-style "key = value" lines. Bad input never
// fails: each problem produces a warning and the key keeps its default.
func parseConfig(src []byte) (Config, []string) {
	c := defaultConfig()
	var warns []string
	mermaidSet := false
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			warns = append(warns, fmt.Sprintf("bad line %q", line))
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		switch key {
		case "window-width":
			warns = setDim(&c.WindowWidth, key, val, warns)
		case "window-height":
			warns = setDim(&c.WindowHeight, key, val, warns)
		case "theme":
			if val == "light" || val == "dark" {
				c.Theme = val
			} else {
				warns = append(warns, fmt.Sprintf("theme must be light or dark, got %q", val))
			}
		case "css":
			c.CSS = expandHome(val)
		case "mermaid-theme":
			c.MermaidTheme = val
			mermaidSet = true
		case "mermaid-renderer":
			if val == "native" || val == "js" {
				c.MermaidRenderer = val
			} else {
				warns = append(warns, fmt.Sprintf("mermaid-renderer must be native or js, got %q", val))
			}
		case "watch":
			switch val {
			case "true":
				c.Watch = true
			case "false":
				c.Watch = false
			default:
				warns = append(warns, fmt.Sprintf("watch must be true or false, got %q", val))
			}
		default:
			warns = append(warns, fmt.Sprintf("unknown key %q", key))
		}
	}
	if !mermaidSet && c.Theme == "dark" {
		c.MermaidTheme = "dark"
	}
	return c, warns
}

func setDim(dst *int, key, val string, warns []string) []string {
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return append(warns, fmt.Sprintf("%s must be a positive integer, got %q", key, val))
	}
	*dst = n
	return warns
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// configPath follows XDG explicitly: os.UserConfigDir() would return
// ~/Library/Application Support on macOS, which is not where this belongs.
func configPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "mdv", "config")
}

// LoadConfig reads the config file. A missing file is silent defaults; a
// broken one warns on stderr and keeps going — config never stops a view.
func LoadConfig() Config {
	path := configPath()
	if path == "" {
		return defaultConfig()
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return defaultConfig()
	}
	c, warns := parseConfig(src)
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "mdv: config:", w)
	}
	return c
}
