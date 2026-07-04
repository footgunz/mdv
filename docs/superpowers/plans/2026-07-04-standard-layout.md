# Standard Go Project Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restructure mdv into cmd/mdv + internal/{config,render,server} + pkg/mermaid with explicit config plumbing and zero behavior change.

**Architecture:** Four tasks, each ending green: move the mermaid package; extract config; extract render behind a `Renderer` struct (killing the cfg global for rendering, with a small root-package bridge); extract server + thin cmd/mdv and delete the bridge. A pre-captured `-html` export is the byte-parity oracle.

**Tech Stack:** Go stdlib, `git mv`.

## Global Constraints

- ZERO behavior change: usage text, error prefixes, exit codes, config semantics, rendered bytes. Parity oracle: `-html` output of the comparison fixture captured before Task 1 must be byte-identical after Task 4.
- Suite green (`go test ./... -race`) at the END OF EVERY TASK.
- No package-level mutable state in the final result (`var cfg` bridge exists only between Tasks 3 and 4).
- Goldens and `pkg/mermaid` internals byte-untouched (only the directory moves).
- Branch: `standard-layout` (already created). Tests govern.

---

### Task 1: Parity capture + move mermaid to pkg/

**Files:**
- Move: `mermaid/` → `pkg/mermaid/` (git mv)
- Modify: `render.go` (import)

**Interfaces:**
- Produces: import path `github.com/dgunther/mdv/pkg/mermaid` (Tasks 3+ use it).

- [ ] **Step 1: Capture the parity oracle**

```bash
S=/private/tmp/claude-501/-Users-dgunther-Projects-mdthing/bd66f461-771b-4ddc-a8a4-40bc758a705e/scratchpad
go build -o mdv .
XDG_CONFIG_HOME="$S/xdg-light" ./mdv -html "$S/compare.md" > "$S/parity-before.html"
wc -c "$S/parity-before.html"
```

- [ ] **Step 2: Move + fix import**

```bash
git mv mermaid pkg/mermaid
perl -pi -e 's{github.com/dgunther/mdv/mermaid}{github.com/dgunther/mdv/pkg/mermaid}' render.go
go build ./... && go test ./... -race
```

Expected: green; `git status` shows renames only.

- [ ] **Step 3: Commit**

```bash
git add -A && git commit -m "Move mermaid engine to pkg/mermaid"
```

---

### Task 2: Extract internal/config

**Files:**
- Move: `config.go` → `internal/config/config.go`, `config_test.go` → `internal/config/config_test.go`
- Modify: root `render.go`, `server.go`, `main.go`, `server_test.go`, `render_test.go` (type/constructor references)

**Interfaces:**
- Produces: package `config` — `type Config struct{...}` (fields unchanged), `func Default() Config`, `func Load() Config`. `parseConfig`/`path` unexported (tests are in-package).

- [ ] **Step 1: Move and re-package**

```bash
mkdir -p internal/config
git mv config.go internal/config/config.go
git mv config_test.go internal/config/config_test.go
```

In both moved files: `package main` → `package config`. Rename `defaultConfig` → `Default`, `LoadConfig` → `Load`, `configPath` → `path` (all callers inside the two files). DELETE the `var cfg = defaultConfig()` line from config.go — the global moves to a root bridge next step.

- [ ] **Step 2: Bridge the root package**

Create `bridge.go` at the repo root:

```go
package main

import "github.com/dgunther/mdv/internal/config"

// cfg bridges the old package-global to the extracted config package.
// Deleted in the cmd/mdv extraction — do not add new readers.
var cfg = config.Default()
```

Root-wide mechanical rename: every root-package reference `Config` → `config.Config` (the `defer func(old Config)` test patterns and `main.go`'s `cfg = LoadConfig()` → `cfg = config.Load()`). Run:

```bash
grep -n 'Config\b\|LoadConfig\|defaultConfig' main.go render.go server.go render_test.go server_test.go
```

and fix each (main.go: `config.Load()`; tests: `defer func(old config.Config) { cfg = old }(cfg)`; render_test's `cfg = defaultConfig()` → `cfg = config.Default()`).

- [ ] **Step 3: Green + commit**

```bash
go build ./... && go vet ./... && go test ./... -race
git add -A && git commit -m "Extract internal/config"
```

---

### Task 3: Extract internal/render with Renderer

**Files:**
- Move: `render.go` → `internal/render/render.go`, `assets.go` → `internal/render/assets.go`, `assets/` → `internal/render/assets/`, `render_test.go` → `internal/render/render_test.go`
- Modify: root `server.go`, `main.go`, `server_test.go`, `bridge.go`

**Interfaces:**
- Produces: package `render` —
  - `type Renderer struct { Cfg config.Config }`
  - `func (r Renderer) Body(src []byte) ([]byte, bool, error)` (was RenderBody)
  - `func (r Renderer) Page(body []byte, title string, includeMermaidJS bool) []byte` (was RenderPage)
  - `func (r Renderer) StaticPage(body []byte, title string, includeMermaidJS bool) []byte`
  - `func Assets() fs.FS` — sub-FS rooted at the embedded assets dir (server's `/_assets/`).

- [ ] **Step 1: Move files**

```bash
mkdir -p internal/render
git mv render.go internal/render/render.go
git mv assets.go internal/render/assets.go
git mv assets internal/render/assets
git mv render_test.go internal/render/render_test.go
```

- [ ] **Step 2: Re-package render.go**

In `internal/render/render.go` (`package render`, import `internal/config` + `pkg/mermaid`):
- Add `type Renderer struct{ Cfg config.Config }`.
- `RenderBody` → `func (r Renderer) Body(...)`; it constructs `cr := &codeRenderer{cfg: r.Cfg}`; `codeRenderer` gains field `cfg config.Config`; inside `renderFenced`/`highlight` every `cfg.` read becomes `r2.cfg.` (the codeRenderer receiver — pick a non-shadowing name).
- `RenderPage` → `func (r Renderer) Page(...)`; `RenderStaticPage` → `func (r Renderer) StaticPage(...)`; every `cfg.` → `r.Cfg.`.
- `rewriteWikilinks`, `codeText`, `highlight` stay unexported functions.

In `internal/render/assets.go` add:

```go
// Assets returns the embedded static files (base.css, mermaid.min.js)
// rooted at their directory, for the viewer's /_assets/ route.
func Assets() fs.FS {
	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err) // embedded dir is always present
	}
	return sub
}
```

(import `"io/fs"`; `package render`).

- [ ] **Step 3: Rewrite render tests off the global**

In `internal/render/render_test.go` (`package render`): add

```go
func testRenderer(mod func(*config.Config)) Renderer {
	c := config.Default()
	if mod != nil {
		mod(&c)
	}
	return Renderer{Cfg: c}
}
```

Mechanical rewrite: `RenderBody(x)` → `testRenderer(nil).Body(x)`; `RenderPage(b, t, f)` → `testRenderer(nil).Page(b, t, f)`; every test that mutated `cfg` (theme/renderer/css) constructs via `testRenderer(func(c *config.Config){ c.Theme = "dark" })` and DELETES its `defer func(old ...)` restore line. Assertions unchanged.

- [ ] **Step 4: Bridge the root server**

`bridge.go` gains (temporarily):

```go
var renderer = render.Renderer{Cfg: cfg}
```

Root `server.go`: `RenderBody(src)` → `renderer.Body(src)`; `RenderPage(...)` → `renderer.Page(...)`; the `/_assets/` route's `fs.Sub(assetsFS, "assets")` block → `http.FileServer(http.FS(render.Assets()))` (keep StripPrefix). `main.go`: after the flag override, add `renderer = render.Renderer{Cfg: cfg}` (so overrides apply) — html branch uses `renderer.Body/StaticPage`. `server_test.go`: tests that mutated `cfg` for render behavior now also reassign `renderer = render.Renderer{Cfg: cfg}` after mutating (bridge-era only; Task 4 rewrites these properly).

- [ ] **Step 5: Green + parity + commit**

```bash
go build ./... && go vet ./... && go test ./... -race
S=/private/tmp/claude-501/-Users-dgunther-Projects-mdthing/bd66f461-771b-4ddc-a8a4-40bc758a705e/scratchpad
go build -o mdv . && XDG_CONFIG_HOME="$S/xdg-light" ./mdv -html "$S/compare.md" | cmp - "$S/parity-before.html" && echo "parity holds"
git add -A && git commit -m "Extract internal/render behind an explicit Renderer"
```

---

### Task 4: Extract internal/server, create cmd/mdv, delete bridge

**Files:**
- Move: `server.go` → `internal/server/server.go`, `watch.go` → `internal/server/watch.go`, `server_test.go` → `internal/server/server_test.go`, `watch_test.go` → `internal/server/watch_test.go`
- Create: `cmd/mdv/main.go`
- Delete: root `main.go`, `bridge.go`
- Modify: `Taskfile.yml`, `README.md`

**Interfaces:**
- Produces: package `server` — `NewHub() *Hub`, `New(baseDir string, hub *Hub, r render.Renderer) *Server` (Handler/Current/SetOnNav unchanged), `NewReloader(current func() string, onChange func()) (*Reloader, error)` unchanged.

- [ ] **Step 1: Move + re-package server**

```bash
mkdir -p internal/server
git mv server.go internal/server/server.go
git mv watch.go internal/server/watch.go
git mv server_test.go internal/server/server_test.go
git mv watch_test.go internal/server/watch_test.go
```

`package server` in all four. `Server` gains field `r render.Renderer`; `NewServer(baseDir, hub)` → `New(baseDir string, hub *Hub, r render.Renderer) *Server`; `serveMarkdown` uses `s.r.Body`/`s.r.Page`; `/_user.css` route reads `s.r.Cfg.CSS`; `/_assets/` uses `render.Assets()`. Tests: `NewServer(dir, NewHub())` → `New(dir, NewHub(), render.Renderer{Cfg: config.Default()})`; the user-CSS test builds `render.Renderer{Cfg: c}` with `c.CSS` set (no globals, no reassignment dance).

- [ ] **Step 2: Create `cmd/mdv/main.go`**

```go
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/dgunther/mdv/internal/config"
	"github.com/dgunther/mdv/internal/render"
	"github.com/dgunther/mdv/internal/server"
	webview "github.com/webview/webview_go"
)

func main() {
	rendererFlag := flag.String("mermaid-renderer", "", "mermaid renderer: native or js (overrides config)")
	htmlFlag := flag.Bool("html", false, "render self-contained HTML to stdout and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdv [-html] [-mermaid-renderer native|js] <file.md>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdv:", err)
		os.Exit(1)
	}
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "mdv: cannot read %s\n", flag.Arg(0))
		os.Exit(1)
	}

	cfg := config.Load()
	switch *rendererFlag {
	case "":
		// defer to config
	case "native", "js":
		cfg.MermaidRenderer = *rendererFlag
	default:
		fmt.Fprintln(os.Stderr, "mdv: -mermaid-renderer must be native or js")
		flag.Usage()
		os.Exit(2)
	}
	rend := render.Renderer{Cfg: cfg}

	if *htmlFlag {
		src, err := os.ReadFile(abs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		body, fallback, err := rend.Body(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		os.Stdout.Write(rend.StaticPage(body, filepath.Base(abs), fallback))
		return
	}

	hub := server.NewHub()
	srv := server.New(filepath.Dir(abs), hub, rend)

	if cfg.Watch {
		reloader, err := server.NewReloader(srv.Current, hub.Broadcast)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdv:", err)
			os.Exit(1)
		}
		defer reloader.Close()
		srv.SetOnNav(func(navAbs string) {
			reloader.Watch(filepath.Dir(navAbs))
		})
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "mdv:", err)
		os.Exit(1)
	}
	// No graceful shutdown: WebKit keeps the SSE connection open past window
	// close, so Server.Shutdown would wait on it forever and the process would
	// linger in the dock. Process exit closes every socket anyway.
	httpSrv := &http.Server{Handler: srv.Handler()}
	go httpSrv.Serve(ln)

	url := fmt.Sprintf("http://%s/%s", ln.Addr().String(), filepath.Base(abs))

	w := webview.New(false)
	defer w.Destroy()
	w.SetTitle(filepath.Base(abs))
	w.SetSize(cfg.WindowWidth, cfg.WindowHeight, webview.HintNone)

	// Ctrl-C in the launching terminal closes the window cleanly. Terminate
	// touches AppKit, so it must run on the UI thread via Dispatch — calling
	// it straight from this goroutine segfaults.
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		w.Dispatch(w.Terminate)
		<-sig
		os.Exit(130) // second Ctrl-C: force quit
	}()

	w.Navigate(url)
	w.Run() // blocks until the window is closed
}
```

Delete root `main.go` and `bridge.go`:

```bash
git rm main.go bridge.go
```

- [ ] **Step 3: Taskfile + README**

Taskfile: build `go build -o mdv ./cmd/mdv`; install `go install ./cmd/mdv`. README Build section: `go build -o mdv ./cmd/mdv`.

- [ ] **Step 4: Full verification**

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./... -race
grep -rn '"github.com/dgunther/mdv/mermaid"' . --exclude-dir=.git --exclude-dir=docs && echo STALE || echo "imports clean"
grep -rn '^var cfg' cmd internal pkg && echo GLOBAL || echo "no cfg global"
S=/private/tmp/claude-501/-Users-dgunther-Projects-mdthing/bd66f461-771b-4ddc-a8a4-40bc758a705e/scratchpad
task clean && task build && XDG_CONFIG_HOME="$S/xdg-light" ./mdv -html "$S/compare.md" | cmp - "$S/parity-before.html" && echo "PARITY HOLDS"
./mdv -x 2>&1 | head -2
task install && ls ~/go/bin/mdv
```

Then a viewer smoke (open/SIGINT/clean exit).

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "Adopt standard Go project layout with cmd/mdv and internal packages"
```

---

## Self-Review

**Spec coverage:** target layout exactly (Tasks 1–4); config API Default/Load with unexported parse/path (Task 2); Renderer struct + methods + Assets() (Task 3); server.New signature + no-global tests (Task 4); thin cmd/mdv with byte-identical behavior (Task 4 code = current main.go modulo package wiring); Taskfile/README (Task 4); parity oracle captured Task 1, checked Tasks 3+4; per-task green; bridge created Task 2/3 and deleted Task 4; goldens/pkg-mermaid untouched. ✓

**Placeholder scan:** mechanical-rewrite steps name exact from→to mappings; no TBDs.

**Type consistency:** `config.Config`/`Default`/`Load`; `render.Renderer{Cfg}`/`Body`/`Page`/`StaticPage`/`Assets`; `server.New(baseDir, hub, rend)`/`NewHub`/`NewReloader` — consistent across tasks; cmd/mdv code uses exactly these.
