# mdthing — native mermaid SVG engine (core + flowchart)

A server-side, no-browser-JS render engine for mermaid diagrams, emitting
inline SVG. Sub-project 1 of the native-mermaid roadmap: the engine core and
flowchart support. Anything the engine cannot handle falls back to the
existing mermaid.js path, so no diagram that renders today ever regresses.

## Roadmap context (not in this spec's scope)

1. **Engine core + flowchart — this spec.**
2. Sequence diagrams (deterministic layout; reuses text/SVG/theme infra).
3. Pie + state diagrams (pie is arcs; state re-dresses flowchart layout).
4. `-html` flag emitting static self-contained HTML with inline SVG.

## Goals

- Render the common flowchart subset to inline `<svg>` at markdown-render
  time — no browser JS involved for supported diagrams.
- Use the engine everywhere it can be used: the viewer emits inline SVG for
  supported diagrams; `mermaid.min.js` is loaded by a page only when that
  page contains at least one fallback block.
- Layout matches mermaid's own: dagre.js (the layout library mermaid uses,
  MIT) vendored and executed in goja, a pure-Go JS interpreter. No DOM, no
  browser, still a single binary.
- Diagrams follow the app theme: light/dark palettes keyed to `base.css` /
  `body.dark` values, selected by `cfg.Theme`.
- The engine lives in its own package (`mermaid/`) with a one-function API,
  importable independently of the viewer later.

## Non-goals (YAGNI)

- Pixel-identical output to mermaid.js. Visually equivalent is the bar.
- Diagram types beyond flowchart (they fall back; later sub-projects).
- The full flowchart grammar (interaction/click/class/style directives,
  markdown-in-labels, htmlLabels). Subset below; the rest falls back.
- `forest`/`neutral` native themes — native engine knows light/dark only;
  `mermaid-theme` config continues to affect only the JS fallback path.
- No dependency on TyphonHill/go-mermaid (generates mermaid text from Go —
  the opposite direction — and GPL-3.0, incompatible with clean licensing).
  The IR is original work. goja, dagre.js, mermaid.js are MIT.

## Package layout

```
mermaid/
  mermaid.go    — API: Render(src []byte, theme Theme) ([]byte, error).
                  Detects diagram type from the header line; non-flowchart
                  or unparseable → ErrUnsupported.
  parse.go      — flowchart parser → IR.
  layout.go     — IR → dagre.js in goja → node positions + edge polylines.
  dagre.js      — vendored MIT dagre build, go:embed.
  svg.go        — positioned IR → SVG bytes.
  text.go       — string measurement: golang.org/x/image/font + embedded
                  golang.org/x/image/font/gofont/goregular TTF.
  theme.go      — Theme struct; Light and Dark palettes.
```

Pipeline: `source → parse → IR → dagre (goja) → positioned IR → SVG`.

## Flowchart subset (v1)

Supported:
- Headers: `graph` / `flowchart` with direction `TD`, `TB`, `LR`, `RL`, `BT`.
- Node shapes: `id[rect]`, `id(round)`, `id([stadium])`, `id{diamond}`,
  `id((circle))`, and bare `id` (defaults to rect). Quoted labels
  (`id["text"]`) and labels with spaces.
- Edges: `-->`, `---`, `-.->`, `==>`; edge labels via `|label|` and
  `-- label -->` forms. Chained statements (`a --> b --> c`) and fan-out
  (`a --> b & c`).
- `subgraph [title] ... end` (one level of nesting minimum).
- Comments (`%%`), blank lines, `;` statement separators.

Parsed and ignored (harmless): `classDef`, `class`, `linkStyle`, `style`
statements — layout/geometry is unaffected; custom colors are dropped in v1.

`ErrUnsupported` (→ JS fallback): any other diagram type, `click`/`href`
interaction, `htmlLabels`/markdown labels, `direction` statements inside
subgraphs, multi-line labels, anything the parser does not recognize.

## Rendering

- Nodes: `<rect>` (rounded for `(round)`/`([stadium])`), `<polygon>` for
  diamonds, `<circle>`/`<ellipse>` for circles, with centered `<text>`.
- Edges: `<path>` through dagre's edge points, solid/dotted/thick per edge
  type, arrowhead marker for directed edges, `<text>` label at the dagre
  label position.
- Subgraphs: a background `<rect>` + title behind member nodes.
- Root element carries `viewBox`, an explicit width/height, and
  `class="mermaid-svg"` for page CSS to center it (same slot the current
  `<pre class="mermaid">` occupies).

## Text measurement

`text.go` measures label strings with `golang.org/x/image/font` against the
embedded `goregular` face at the theme's font size; node boxes are sized
from measurement plus padding. Emitted `<text>` uses a font stack beginning
with the metrics-compatible family and generic fallbacks; padding absorbs
the small cross-font drift. Exact pixel fidelity is a non-goal.

## Theming

`theme.go`: `type Theme struct` with node fill/stroke, edge stroke, text
color, subgraph fill, font size/family. `Light` and `Dark` values match the
page palette (`base.css` and its `body.dark` block). The call site picks
`mermaid.Dark` when `cfg.Theme == "dark"`.

## Viewer integration (render.go)

In `renderFenced`, the `mermaid` branch becomes:

1. Try `mermaid.Render(code, themeFromCfg())`.
2. Success → write the SVG bytes into the page (inline block).
3. Any error → current behavior exactly: `<pre class="mermaid">` with
   escaped source, counted as a fallback.

`RenderBody` reports whether any fallback block was emitted; `RenderPage`
includes the `mermaid.min.js` script tags only when it was. Pages whose
diagrams all render natively ship no mermaid JS.

## Error handling

- Every engine failure (parse, unsupported, goja/dagre, SVG) returns an
  error; the caller falls back to mermaid.js silently. No engine error can
  break a page.
- Malformed mermaid source behaves as today: falls back, and mermaid.js
  shows its own error box.
- dagre.js and goja run per-render with no shared mutable state across
  renders (a fresh goja VM or per-call lock — implementation's choice, but
  concurrent renders must be safe: the HTTP server renders on request).

## Testing

- **Parser:** table-driven: each supported construct → expected IR;
  representative unsupported constructs → `ErrUnsupported`.
- **Layout:** invariants, not pixels: every node positioned, no NaN/Inf,
  rank order respects edge direction for `TD` and `LR`, positive bounding
  box containing all nodes.
- **SVG:** parses as XML; shape-per-node and path-per-edge counts; labels
  present; dark theme changes fill values.
- **Golden files:** small set of `.mmd` → `.svg` goldens under
  `mermaid/testdata/`, regenerated with `go test -update`; failures show
  the diff for review.
- **Integration:** supported fence → `<svg` in `RenderBody` output and no
  mermaid.min.js script in the page; unsupported fence → `<pre
  class="mermaid">` present and script included.

## Dependencies

- `github.com/dop251/goja` (MIT) — JS interpreter for dagre.
- `golang.org/x/image` (BSD) — font metrics + embedded goregular TTF.
- Vendored `dagre.js` (MIT), `go:embed`.
- No GPL code; no browser; binary stays self-contained.
