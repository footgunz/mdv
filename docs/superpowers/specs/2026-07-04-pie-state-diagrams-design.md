# mdthing — native pie charts and state diagrams (sub-project 3)

Pie and state diagram support for the native mermaid engine. Pie is a
small self-contained arc pipeline; state diagrams are a parser that
targets the existing flowchart IR, reusing dagre layout and the SVG
emitter with two new node dressings.

## Roadmap context

Sub-project 3 (1: flowchart — shipped; 2: sequence — shipped; 4: `-html`
flag — future). Fallback contract, themes, escaping, determinism, and the
screenshot-comparison acceptance gate all carry over unchanged.

## Pie charts

### Subset

- Header `pie`, optionally `pie showData`.
- Optional `title <text>` line.
- Slice lines: `"label" : <number>` — number is a non-negative float;
  at least one slice with a positive total required.
- `%%` comments and blank lines. Anything else → `ErrUnsupported`.

### Rendering

- Fixed radius 90; slices as SVG arc paths from 12 o'clock, clockwise,
  ordered by value DESCENDING (mermaid.js behavior; confirmed against the
  JS renderer during screenshot acceptance).
- Percentage label (`NN%`, rounded) centered in each slice at ~60% radius,
  skipped for slices under 5% (avoid collisions).
- Legend right of the circle: one row per slice in the same order —
  colored swatch (12×12), label, and ` [value]` appended when `showData`.
  Legend text measured with `measureText` for canvas sizing.
- Optional title centered above the circle (bold).
- Single-slice pie renders as a full circle (arc math must not degenerate).

### Theme palette

`Theme` gains `Palette []string` — 8 categorical colors, cycled when
slices exceed 8. Light: muted chart tones on white; Dark: brighter
variants for `#0d1117`. Slices get a 2px `NodeFill` stroke as the
separator gap. Slice percentage text uses `Theme.Text`; legend text
`Theme.Text`.

### Files

`mermaid/pie.go` (parse + layout + emit in one file — the pipeline is
~200 lines total), `mermaid/pie_test.go`, goldens `pie-basic.mmd`
(with title + showData) under the existing `-update` harness.

## State diagrams

### Architecture

`mermaid/state.go` is a PARSER ONLY: `parseState(src string) (*Graph,
error)` builds the existing flowchart `Graph` IR. `measureGraph`,
`layout` (dagre, compound), and `emit` are reused as-is except for two
new shape cases in `emit`.

### Subset

- Headers `stateDiagram-v2` and `stateDiagram` (same parser).
- Transitions: `A --> B` and `A --> B : label`.
- `[*]` as source = start pseudo-state; as target = end pseudo-state.
  Every `[*]` occurrence maps to a synthetic node per role and context:
  one shared start node and one shared end node per scope (top level or
  composite) — ids `__start`/`__end` (suffixed per composite scope).
- Declarations: `state "Long Label" as s1`; descriptions `s1 : Some text`
  (sets the node label; last one wins).
- Composite states: `state Name { ... }` one level deep → subgraph with
  title `Name`; nested composites → `ErrUnsupported`.
- `direction TB` / `direction LR` at top level (default TB).
- `%%` comments, blank lines. Everything else (choice/fork/join/history,
  `<<...>>` stereotypes, notes, `--` concurrency) → `ErrUnsupported`.

### Dressing

- New shapes in the flowchart IR: `ShapeStateStart` (filled circle,
  `NodeStroke` fill, radius 7) and `ShapeStateEnd` (double circle:
  outer ring `NodeStroke` stroke, inner filled dot). Neither renders a
  text label. Sized as fixed 18×18 in `measureGraph`.
- Normal states render as the existing `ShapeRound` (rounded rect).
- Transition labels ride the existing edge-label path; edges are the
  existing curved directed edges.

### Files

`mermaid/state.go`, `mermaid/state_test.go`, `measureGraph`/`emit`
touch-ups in `text.go`/`svg.go`, golden `state-basic.mmd` (start/end,
labels, one composite, LR variant covered in unit tests).

## Dispatch

`Render` gains `case "pie"` and `case "stateDiagram", "stateDiagram-v2"`.
`detect` already returns these keywords. `render.go`/viewer untouched —
these types simply stop falling back.

## Error handling

Unchanged contract: any parse failure or out-of-subset construct returns
an `ErrUnsupported`-wrapped error → JS fallback; no panics on user input;
pie with zero/negative total errors.

## Testing

- Pie parser: title/showData/slices/comments; bad number, no slices,
  unknown line → error. Arc math: slice angles sum to 360°, single-slice
  full circle, percentages rounded, order descending.
- Pie SVG: well-formed, path-per-slice count, legend rows, palette
  cycling >8 slices, showData values, dark palette applied.
- State parser: each construct → expected Graph IR (nodes/shapes/labels,
  edges+labels, subgraphs, direction); every unsupported construct
  errors; `[*]` scoping (top-level vs composite get distinct nodes).
- State SVG: start dot + double-circle end present; goldens.
- Integration: `RenderBody` on pie and state fences yields `<svg` with
  no fallback; existing fallback tests keep their (still-unsupported)
  `gantt`/`classDiagram` fixtures.
- Acceptance: screenshot comparison vs mermaid.js in both themes
  (pie slice order/percentages and state shapes eyeballed side-by-side).
