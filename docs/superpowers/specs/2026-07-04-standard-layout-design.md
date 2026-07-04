# mdv — Standard Go Project Layout refactor

Restructure the single-package app into the Standard Go Project Layout
with real package boundaries, and eliminate the shared `cfg` global by
passing configuration explicitly.

## Target layout

```
cmd/mdv/main.go              package main: flags, config load, wiring, lifecycle
internal/config/             package config: Config, Default(), Load(), path resolution
internal/render/             package render: Renderer + markdown/HTML assembly
internal/render/assets/      base.css + mermaid.min.js (go:embed lives with its package)
internal/server/             package server: Server, Hub, Reloader (watch)
pkg/mermaid/                 the diagram engine, moved verbatim (git mv)
```

## Goals

- Every package has one responsibility and an explicit interface; no
  package-level mutable config anywhere.
- `internal/config`: `type Config` (same fields), `Default() Config`,
  `Load() Config` (XDG path + warnings to stderr), parser unexported.
- `internal/render`: `type Renderer struct { Cfg config.Config }` with
  methods `Body(src []byte) ([]byte, bool, error)`, `Page(body []byte,
  title string, includeMermaidJS bool) []byte`, `StaticPage(...)` (same
  signature shape), plus `Assets() fs.FS` (sub-FS rooted at the embedded
  `assets/` dir) for the server's `/_assets/` route. Wikilink rewrite,
  chroma highlight, and the mermaid dispatch stay unexported internals.
- `internal/server`: `NewHub() *Hub`, `New(baseDir string, hub *Hub,
  r *render.Renderer) *Server` (Handler/Current/SetOnNav as today),
  `NewReloader(...)` unchanged semantics.
- `cmd/mdv`: thin main — parse flags, `cfg := config.Load()`, apply
  `-mermaid-renderer` override, `-html` branch, else wire
  server/watcher/webview. Behavior byte-identical to today (usage text,
  exit codes, error prefixes).
- `pkg/mermaid`: `git mv mermaid pkg/mermaid`; only the import path
  changes (`github.com/dgunther/mdv/pkg/mermaid`). No API or golden
  changes.
- Taskfile: `go build -o mdv ./cmd/mdv`, `go install ./cmd/mdv`.
  README build/import references updated.

## Non-goals

- No behavior changes: rendering, config semantics, CLI surface, themes,
  goldens all byte-identical.
- No new abstractions beyond the Renderer struct (no interfaces with one
  implementation).
- Historical docs under docs/superpowers/ untouched.

## Test strategy

Tests move with their packages. Render/server tests construct
configurations directly (`render.Renderer{Cfg: config.Default()}` or a
modified copy) — the `defer func(old Config){cfg=old}(cfg)` global-restore
pattern is deleted everywhere. Golden files and the mermaid package tests
are untouched. Suite must be green after EVERY task, not just at the end.

## Sequencing (each step independently green)

1. `git mv mermaid pkg/mermaid` + fix the single import.
2. Extract `internal/config` (move config.go/config_test.go, export
   Default/Load, temporary alias wiring in root package until step 3-4
   consume it).
3. Extract `internal/render` (Renderer struct, assets move, all render
   tests rewritten off the global).
4. Extract `internal/server`, create `cmd/mdv`, delete root-package
   remnants, update Taskfile/README, full verification: suite + race,
   `go vet`, viewer smoke, `-html` smoke, `grep` for stale import paths.

## Verification

- `go build ./... && go vet ./... && go test ./... -race` green after
  each task and at the end.
- `gofmt -l .` clean; no package-level mutable state anywhere
  (`grep -n "^var cfg" ` returns nothing).
- Behavior parity: usage text, `-html` output for the comparison fixture
  byte-identical to pre-refactor output (capture before, diff after).
- `task build`/`task install` produce a working `mdv`.
