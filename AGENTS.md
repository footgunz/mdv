# mdv — agent notes

CLI Markdown viewer: opens a native webview window, renders GFM + Mermaid,
live-reloads on save. See README.md for user-facing behavior and config keys.

## Build & test

    go build -o mdv ./cmd/mdv    # or: task build
    go test ./...                # headless, no window needed
    ./mdv README.md              # manual check; Ctrl-C or close window to quit

cgo is required for `cmd/mdv` (webview). Linux needs `libwebkit2gtk-4.1-dev`;
an LSP configured for linux/amd64 will report a broken `webview_go` import —
ignore it. The library packages (`internal/...`, `pkg/mermaid`) build and test
without cgo or a display.

## Layout

- `cmd/mdv` — flag parsing, window + HTTP server wiring, dock icon.
- `internal/config` — `~/.config/mdv/config` parser. New key = field on
  `Config` + case in `parseConfig` + README + `examples/config`.
- `internal/render` — goldmark + chroma + mermaid → HTML. `Renderer.Body`
  for fragments, `StaticPage` for `-html` export. `assets/` (base.css,
  mermaid.min.js) is embedded; served at `/_assets/`.
- `internal/server` — file serving, SSE reload hub, fsnotify watcher.
- `pkg/mermaid` — native mermaid→SVG engine (parse → IR → dagre layout in
  goja → SVG). Unsupported syntax returns `ErrUnsupported` and the caller
  falls back to bundled mermaid.js; never partially render.

## Gotchas

- **AppKit is single-threaded.** Anything touching the window from a
  goroutine must go through `w.Dispatch(...)` — calling webview methods
  directly off-thread segfaults (see the Ctrl-C handler in main.go).
- **No graceful HTTP shutdown, on purpose.** WebKit holds the SSE connection
  open after the window closes, so `Server.Shutdown` would hang; process
  exit closes the sockets (comment in main.go).
- **Dock icon:** `cmd/mdv/icon.png` is embedded and set at runtime
  (`icon_darwin.go`). Regenerate from `icon.svg` with the qlmanage command
  in its header comment. qlmanage flattens transparency onto white, so
  `icon_darwin.go` clips to the tile's rounded rect (48,48 928×928 r=220) —
  if you change the tile geometry in the SVG, update the clip to match.
  The generic exec icon flashing during quit is expected (no .app bundle).
- Binaries at repo root (`mdv`) are gitignored build artifacts.
