# mdthing Config System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A ghostty-style `key = value` config file at `$XDG_CONFIG_HOME/mdthing/config` controlling window size, theme, user CSS, mermaid theme, and live reload.

**Architecture:** One new file `config.go` holds a `Config` struct, a pure `parseConfig` (all logic + all tests), and a thin `LoadConfig` that resolves the XDG path. The app is a single `package main`, so the loaded config lives in a package-level `var cfg Config` set at startup; `main.go`, `render.go`, and `server.go` read it directly.

**Tech Stack:** Go stdlib only — no new dependencies.

## Global Constraints

- Config file path: `$XDG_CONFIG_HOME/mdthing/config`, defaulting to `~/.config/mdthing/config`. Do NOT use `os.UserConfigDir()` (returns `~/Library/Application Support` on macOS).
- Format: `key = value` lines, `#` comments, blank lines ignored; split at the first `=`; trim whitespace; keys are case-sensitive kebab-case.
- Config errors are never fatal: missing file → silent defaults; unknown key or bad value → one warning line to stderr (`mdthing: config: ...`), keep the default, continue.
- No new dependencies. View-only app; no CLI flags mirroring config keys; no config live-reload.

> **Note vs. spec:** the spec assigns `~`-expansion and the mermaid-follows-theme default to `LoadConfig`. This plan puts both inside `parseConfig` so they're covered by unit tests; `LoadConfig` stays pure glue (path + read + warn-print). Same behavior, better tested.

---

### Task 1: Config struct and parser

**Files:**
- Create: `config.go`
- Test: `config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (used by Tasks 2–4):
  - `type Config struct { WindowWidth, WindowHeight int; Theme, CSS, MermaidTheme string; Watch bool }`
  - `func defaultConfig() Config` — `{900, 1000, "light", "", "default", true}`.
  - `func parseConfig(src []byte) (Config, []string)` — pure; returns config + warning strings.
  - `var cfg = defaultConfig()` — package-level current config.

- [ ] **Step 1: Write the failing test**

Create `config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestParseConfig`
Expected: FAIL — `undefined: parseConfig`, `undefined: defaultConfig`.

- [ ] **Step 3: Implement `config.go`**

```go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is mdthing's runtime configuration. Extending the schema = add a
// field here and a case in parseConfig.
type Config struct {
	WindowWidth  int
	WindowHeight int
	Theme        string // "light" or "dark"
	CSS          string // path to an extra user stylesheet, "" = none
	MermaidTheme string // passed to mermaid.initialize
	Watch        bool   // live reload on file change
}

func defaultConfig() Config {
	return Config{
		WindowWidth:  900,
		WindowHeight: 1000,
		Theme:        "light",
		MermaidTheme: "default",
		Watch:        true,
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestParseConfig`
Expected: PASS (all five).

- [ ] **Step 5: Commit**

```bash
git add config.go config_test.go
git commit -m "Add config struct and ghostty-style parser"
```

---

### Task 2: XDG path resolution and main wiring

**Files:**
- Modify: `config.go` (add `configPath`, `LoadConfig`)
- Modify: `main.go` (load config; window size; watch toggle)
- Test: `config_test.go`

**Interfaces:**
- Consumes: `parseConfig`, `cfg` (Task 1).
- Produces:
  - `func configPath() string` — `$XDG_CONFIG_HOME/mdthing/config` or `~/.config/mdthing/config`.
  - `func LoadConfig() Config` — reads `configPath()`; missing file → defaults, silent; warnings → stderr prefixed `mdthing: config:`.

- [ ] **Step 1: Write the failing test**

Add to `config_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestConfigPath`
Expected: FAIL — `undefined: configPath`.

- [ ] **Step 3: Add `configPath` and `LoadConfig` to `config.go`**

```go
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
	return filepath.Join(dir, "mdthing", "config")
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
		fmt.Fprintln(os.Stderr, "mdthing: config:", w)
	}
	return c
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./... -run TestConfigPath`
Expected: PASS.

- [ ] **Step 5: Wire into `main.go`**

At the top of `main()`, right after the argument check and before anything uses configuration:

```go
	cfg = LoadConfig()
```

Replace the fixed window size line:

```go
	w.SetSize(900, 1000, webview.HintNone)
```

with:

```go
	w.SetSize(cfg.WindowWidth, cfg.WindowHeight, webview.HintNone)
```

Wrap the reloader block (creation, `defer reloader.Close()`, and `srv.SetOnNav(...)`) so `watch = false` skips it entirely:

```go
	if cfg.Watch {
		reloader, err := NewReloader(srv.Current, hub.Broadcast)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdthing:", err)
			os.Exit(1)
		}
		defer reloader.Close()
		srv.SetOnNav(func(navAbs string) {
			reloader.Watch(filepath.Dir(navAbs))
		})
	}
```

- [ ] **Step 6: Verify build and full suite**

Run: `go build ./... && go test ./...`
Expected: builds, all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add config.go config_test.go main.go
git commit -m "Load config from XDG path and wire window size and watch"
```

---

### Task 3: Dark theme and mermaid theme rendering

**Files:**
- Modify: `render.go` (`RenderPage` body class + mermaid theme; chroma style)
- Modify: `assets/base.css` (dark palette block)
- Test: `render_test.go`

**Interfaces:**
- Consumes: `cfg` (Task 1).
- Produces: `RenderPage` output varies with `cfg.Theme`, `cfg.MermaidTheme` — same signature `func RenderPage(body []byte, title string) []byte`.

- [ ] **Step 1: Write the failing test**

Add to `render_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestRenderPageTheme`
Expected: FAIL — no `class="dark"`, no `theme:'...'` in output.

- [ ] **Step 3: Update `RenderPage` in `render.go`**

Replace the body-open and mermaid-init lines:

```go
	b.WriteString(`</title><link rel="stylesheet" href="/_assets/base.css"></head><body>`)
```

becomes:

```go
	b.WriteString(`</title><link rel="stylesheet" href="/_assets/base.css">`)
	if cfg.CSS != "" {
		b.WriteString(`<link rel="stylesheet" href="/_user.css">`)
	}
	if cfg.Theme == "dark" {
		b.WriteString(`</head><body class="dark">`)
	} else {
		b.WriteString(`</head><body>`)
	}
```

and:

```go
	b.WriteString(`<script>mermaid.initialize({startOnLoad:true});</script>`)
```

becomes:

```go
	fmt.Fprintf(&b, `<script>mermaid.initialize({startOnLoad:true,theme:'%s'});</script>`, cfg.MermaidTheme)
```

Add `"fmt"` to the `render.go` import block.

(The `/_user.css` link is emitted here; the route serving it is Task 4. A dangling link before Task 4 is harmless — 404s are ignored by browsers.)

- [ ] **Step 4: Switch chroma style with the theme**

In `render.go` `renderFenced`, replace:

```go
	if err := quick.Highlight(&buf, string(code), lang, "html", "github"); err != nil {
```

with:

```go
	style := "github"
	if cfg.Theme == "dark" {
		style = "github-dark"
	}
	if err := quick.Highlight(&buf, string(code), lang, "html", style); err != nil {
```

- [ ] **Step 5: Add the dark palette to `assets/base.css`**

Append:

```css

body.dark { background: #0d1117; color: #c9d1d9; }
body.dark .markdown-body h1, body.dark .markdown-body h2 { border-bottom-color: #21262d; }
body.dark .markdown-body code { background: #161b22; }
body.dark .markdown-body pre { background: #161b22; }
body.dark .markdown-body pre.mermaid { background: none; }
body.dark .markdown-body th, body.dark .markdown-body td { border-color: #30363d; }
body.dark .markdown-body a { color: #58a6ff; }
```

- [ ] **Step 6: Run the tests**

Run: `go test ./...`
Expected: PASS (including all pre-existing render tests).

- [ ] **Step 7: Commit**

```bash
git add render.go assets/base.css render_test.go
git commit -m "Add dark theme and configurable mermaid theme"
```

---

### Task 4: User stylesheet route, README, verification

**Files:**
- Modify: `server.go` (serve `/_user.css`)
- Modify: `README.md` (Configuration section)
- Test: `server_test.go`

**Interfaces:**
- Consumes: `cfg.CSS` (Task 1), `Server.Handler()` (existing).
- Produces: `GET /_user.css` serves the configured file when `cfg.CSS` is set.

- [ ] **Step 1: Write the failing test**

Add to `server_test.go`:

```go
func TestServerUserCSS(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)
	dir := t.TempDir()
	css := filepath.Join(dir, "u.css")
	if err := os.WriteFile(css, []byte("body{color:red}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.CSS = css

	srv := NewServer(dir, NewHub())
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/_user.css")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(readAll(t, resp), "color:red") {
		t.Fatalf("status %d", resp.StatusCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./... -run TestServerUserCSS`
Expected: FAIL — 404 (route not registered; falls through to the baseDir file server).

- [ ] **Step 3: Register the route in `server.go` `Handler()`**

After the `/_assets/` handler:

```go
	mux.HandleFunc("/_user.css", func(w http.ResponseWriter, r *http.Request) {
		if cfg.CSS == "" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, cfg.CSS)
	})
```

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Document in `README.md`**

Append after the Features section:

```markdown

## Configuration

Optional config file at `$XDG_CONFIG_HOME/mdthing/config`
(default `~/.config/mdthing/config`), ghostty-style `key = value` lines:

    # ~/.config/mdthing/config
    window-width = 1200
    window-height = 900
    theme = dark              # light (default) or dark
    css = ~/notes/custom.css  # extra stylesheet, loaded after the built-in one
    mermaid-theme = forest    # default, dark, forest, neutral
    watch = false             # disable live reload

Every key is optional. Unknown keys or bad values print a warning and are
ignored; a missing file means all defaults. `mermaid-theme` follows `theme`
unless set explicitly.
```

- [ ] **Step 6: Manual verification**

```bash
go build -o mdthing .
mkdir -p ~/.config/mdthing
cat > ~/.config/mdthing/config <<'EOF'
window-width = 1100
theme = dark
EOF
./mdthing README.md
```

Verify by hand:
- Window opens 1100 wide with the dark palette; code blocks are dark-highlighted.
- Mermaid diagrams render in the dark mermaid theme (dark background boxes).
- Remove `~/.config/mdthing/config` and relaunch: light theme, 900×1000, all defaults.
- `echo 'bogus = 1' >> ~/.config/mdthing/config` and relaunch: a `mdthing: config: unknown key "bogus"` warning on stderr, app works normally.

- [ ] **Step 7: Commit**

```bash
git add server.go server_test.go README.md
git commit -m "Serve user stylesheet and document configuration"
```

---

## Self-Review

**Spec coverage:**
- XDG path, no `os.UserConfigDir()` — Task 2 `configPath`. ✓
- Format rules (first `=`, trim, `#`, blanks) — Task 1 `parseConfig`. ✓
- All six schema keys with defaults — Task 1 (parse) + Task 2 (window/watch wiring) + Task 3 (theme/mermaid) + Task 4 (css route). ✓
- Never-fatal error handling (missing file silent, unknown key/bad value warn to stderr) — Task 1 warnings + Task 2 `LoadConfig`. ✓
- `~` expansion — Task 1 `expandHome`. ✓
- mermaid-follows-theme default — Task 1 (tested both orders). ✓
- `var cfg` package global set before server/window — Task 1 declaration, Task 2 wiring at top of `main()`. ✓
- Dark palette in `base.css`, chroma `github-dark` — Task 3. ✓
- Tests per spec (defaults, each key, comments, warnings, dark render) — Tasks 1–4. ✓

**Placeholder scan:** none — every step has complete code and exact commands.

**Type consistency:** `Config` fields (`WindowWidth`, `WindowHeight`, `Theme`, `CSS`, `MermaidTheme`, `Watch`), `defaultConfig()`, `parseConfig(src []byte) (Config, []string)`, `configPath() string`, `LoadConfig() Config`, `cfg` — used with identical names/signatures across all four tasks. `RenderPage` signature unchanged.
