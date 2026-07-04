# mdv — agent notes

CLI Markdown viewer: opens a native webview window, renders GFM + Mermaid,
live-reloads on save. See README.md for user-facing behavior and config keys.

## Build & test

    go build -tags desktop,production -o mdv ./cmd/mdv    # or: task build
    go test ./...                # headless, no window needed
    ./mdv README.md              # manual check; Ctrl-C or close window to quit

cgo is required for `cmd/mdv` (Wails v2). Linux needs `libgtk-3-dev
libwebkit2gtk-4.1-dev` and the extra `webkit2_41` build tag. The library
packages (`internal/...`, `pkg/mermaid`) build and test without cgo or a
display.

## Layout

- `cmd/mdv` — flag parsing, Wails app wiring, dock icon.
- `internal/config` — `~/.config/mdv/config` parser. New key = field on
  `Config` + case in `parseConfig` + README + `examples/config`.
- `internal/render` — goldmark + chroma + mermaid → HTML. `Renderer.Body`
  for fragments, `StaticPage` for `-html` export. `assets/` (base.css,
  mermaid.min.js) is embedded; served at `/_assets/`.
- `internal/server` — file serving (mounted as the Wails asset server
  handler; "/" serves the entry file) and the fsnotify watcher.
- `pkg/mermaid` — native mermaid→SVG engine (parse → IR → dagre layout in
  goja → SVG). Unsupported syntax returns `ErrUnsupported` and the caller
  falls back to bundled mermaid.js; never partially render.

## Gotchas

- **The Wails asset server buffers whole responses** (it serves over a
  custom scheme, not TCP) — streaming responses like SSE/websockets can't
  pass through it. Live reload is a Wails event (`mdv:reload` emitted in
  main.go, subscribed in the page template in render.go).
- **Wails v2 needs build tags**: `desktop,production` always (plain
  `go build` produces a dev-mode binary that expects a frontend dev
  server), plus `webkit2_41` on Linux. golangci-lint gets them from
  `.golangci.yml`.
- **`-framework UniformTypeIdentifiers`** in icon_darwin.go's LDFLAGS is
  load-bearing: wails v2.12 references UTType but doesn't link the
  framework itself.
- **Dock icon:** `cmd/mdv/icon.png` is embedded and set at runtime
  (`icon_darwin.go`). Regenerate from `icon.svg` with the qlmanage command
  in its header comment. qlmanage flattens transparency onto white, so
  `icon_darwin.go` clips to the tile's rounded rect (48,48 928×928 r=220) —
  if you change the tile geometry in the SVG, update the clip to match.
  The generic exec icon flashing during quit is expected (no .app bundle).
- Binaries at repo root (`mdv`) are gitignored build artifacts.
