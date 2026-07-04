# mdthing — `-html` static export (sub-project 4)

A `-html` flag that renders one markdown file to self-contained HTML on
stdout: native SVG diagrams inline, mermaid.js inlined only when a diagram
needed the JS fallback, no server/window/watcher.

## Goals

- `mdthing -html notes.md > notes.html` — render once, write stdout, exit 0.
- Self-contained output: `base.css` inlined in `<style>` (light + dark
  rules), user `css` config inlined when set and readable, `body.dark`
  class per `cfg.Theme`.
- `mermaid.min.js` + `mermaid.initialize` (with `cfg.MermaidTheme`,
  JS-escaped) inlined ONLY when `RenderBody` reported a fallback block —
  native-only documents export lean files.
- No live-reload/EventSource script in static output.
- Composes with `-mermaid-renderer js` (every diagram → inlined mermaid.js
  path).
- `-html` never touches webview/server/watcher code — works headless.
- Errors (unreadable file, render failure) → stderr + exit 1. Usage line
  becomes `mdthing [-html] [-mermaid-renderer native|js] <file.md>`.

## Non-goals (YAGNI)

- No `-o` flag, no sibling-file mode — stdout only.
- No asset embedding: relative images and `[[wikilink]]`/relative `.md`
  hrefs are emitted as-is and resolve only while the HTML sits next to its
  sources. Documented in README; data-URI embedding is a possible later
  feature.
- No batch/multi-file export, no directory walking.

## Changes

- `render.go`: `RenderStaticPage(body []byte, title string,
  includeMermaidJS bool) []byte` — sibling of `RenderPage`; reads
  `assets/base.css` (and `assets/mermaid.min.js` when needed) from the
  existing `assetsFS` embed; inlines user CSS via `os.ReadFile(cfg.CSS)`
  (unreadable file → skip silently, matching config never-fatal
  philosophy); same escaping discipline (`title` HTML-escaped,
  mermaid theme JS-escaped).
- `main.go`: `-html` bool flag. When set, after `cfg = LoadConfig()` and
  the renderer-flag override: read the file, `RenderBody`,
  `RenderStaticPage`, write to stdout, return before any
  listener/webview/reloader setup.
- `README.md`: Usage line + a short "Static export" note with the
  relative-links caveat.
- `examples/config` untouched (no new config keys).

## Testing

- `render_test.go` unit tests for `RenderStaticPage`:
  - contains `<style>` with base.css content; contains NO `/_assets/`
    hrefs and NO `/_events` reference;
  - `includeMermaidJS=false` → no `mermaid` script content;
    `true` → inlined script + `mermaid.initialize` with escaped theme;
  - `cfg.CSS` set to a temp file → its content inlined; unreadable path →
    page still renders without it;
  - `cfg.Theme = "dark"` → `<body class="dark">`.
- CLI verified by running (controller): native-only doc → small output
  containing `<svg class="mermaid-svg">` and no `mermaid.min` payload;
  gantt doc → large output containing the inlined payload and
  `<pre class="mermaid">`; `-html` + `-mermaid-renderer js` → no inline
  SVG; missing file → exit 1; output renders correctly in headless Chrome
  (screenshot, light + dark).
