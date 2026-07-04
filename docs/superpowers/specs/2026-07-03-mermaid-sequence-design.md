# mdthing — native mermaid sequence diagrams (sub-project 2)

Sequence diagram support for the native mermaid SVG engine. Deterministic
pure-Go layout — no dagre, no goja involvement. Reuses the engine's text
metrics, themes, SVG conventions, and the viewer's JS-fallback contract
unchanged.

## Roadmap context

Sub-project 2 of the native-mermaid roadmap (1: core + flowchart — shipped;
3: pie + state; 4: `-html` flag).

## Goals

- Render the common sequence-diagram subset to themed SVG at markdown-render
  time; unsupported constructs fall back to mermaid.js silently.
- Pure-Go deterministic layout: two passes (horizontal lifeline spacing,
  vertical item walk). No new dependencies.
- Same package (`mermaid/`), same `Render` entry point, same `Theme`s, same
  escaping discipline, same `class="mermaid-svg"` root.
- `render.go` and the viewer are untouched — the existing fallback plumbing
  already routes everything.

## Non-goals (YAGNI)

- Pixel parity with mermaid.js.
- `critical`, `break`, `box`, `create`/`destroy`, `rect`, participant links,
  actor menus, `par_over`, message numbering options beyond bare
  `autonumber` — all fall back.
- Bottom-mirrored actor boxes (mermaid's `mirrorActors`): top boxes only in
  v1. Ceiling noted in code.
- Long-label overflow between NON-adjacent lifelines: spacing constraints
  are applied to adjacent pairs only; a very long label spanning distant
  participants may overhang the gap. Accepted v1 ceiling.

## Supported subset

- Header: `sequenceDiagram`.
- Participants: `participant id`, `participant id as Label`, `actor id`,
  `actor id as Label`; implicit declaration on first use in a message.
  Declaration/first-use order fixes left-to-right order. `actor` renders the
  same box as `participant` in v1 (distinct stick figure is out of scope);
  the keyword is accepted and tracked.
- Messages: `A->>B: text`, `A-->>B: text` (dashed), `A->B: text` (solid, no
  arrowhead in v1 head style: open line end), `A-->B: text` (dashed, open),
  `A-xB: text` (cross head), `A--xB: text` (dashed, cross). Self-messages
  (`A->>A: text`) render as a right-side loop.
- Activations: `activate A` / `deactivate A` statements and `+`/`-` arrow
  suffixes (`A->>+B:`, `B-->>-A:`). Nested activations stack with a small x
  offset. Unmatched `deactivate` → parse error (fallback). Activations still
  open at the end auto-close at the diagram bottom.
- Notes: `Note left of A: text`, `Note right of A: text`,
  `Note over A: text`, `Note over A,B: text` (case-insensitive `Note`).
- Frames: `loop <label> ... end`, `opt <label> ... end`,
  `alt <label> ... else <label> ... end` (multiple `else`),
  `par <label> ... and <label> ... end` (multiple `and`). Frames nest.
- `autonumber` (bare form): numbers every message `1.`, `2.`, … in order.
- `%%` comments and blank lines.

Anything else → `ErrUnsupported` (wrapped) → JS fallback. Unrecognized
statements must error, never silently misrender.

## IR (new types in package mermaid)

- `SeqDiagram{ Participants []*Participant; Items []SeqItem; Autonumber bool; Width, Height float64 }`
- `Participant{ ID, Label string; Actor bool; X, W, H float64 }` — X is the
  lifeline center, set by layout.
- `SeqItem` — interface implemented by:
  - `SeqMessage{ From, To, Text string; Dashed bool; Head HeadStyle;
    ActivateTo, DeactivateFrom bool; Y float64; Num int }` (HeadStyle:
    HeadArrow, HeadCross, HeadNone)
  - `SeqNote{ Pos NotePos; A, B, Text string; X, Y, W, H float64 }`
    (NotePos: NoteLeft, NoteRight, NoteOver)
  - `SeqFrame{ Kind string; Sections []SeqSection; X, Y, W, H float64;
    DividerYs []float64 }` with `SeqSection{ Label string; Items []SeqItem }`

Frames-as-tree makes nested layout a plain recursion. Activation rectangles
are computed during layout into `SeqDiagram`-level
`Activations []SeqActivation{ P *Participant; Level int; Y0, Y1 float64 }`.

## Layout

Two passes, all sizes from `measureText` (existing):

- **Horizontal:** initial lifeline x from cumulative participant box widths
  + minimum gap; then for each adjacent pair, widen the gap until the widest
  label that crosses it fits (message labels between those lifelines, note
  widths, self-message bumps). `Note over A,B` and frames pad the outer
  margins as needed.
- **Vertical:** recursive walk over `Items` assigning `Y` top-down: actor
  boxes, then per item a fixed row height derived from measured text; frame
  entry adds header height, sections record divider y's, frame exit adds
  padding. Activation stack per participant records rectangles.

Invariants (tested): y strictly increases down the item stream; lifeline x's
strictly increase in participant order; every frame's box contains all its
children's geometry; activation rectangles nest properly (inner within
outer y-range, offset x).

## SVG

`emitSequence(d *SeqDiagram, t Theme) []byte`, matching flowchart
conventions (`fmt.Fprintf` building, `html.EscapeString` on ALL
user-sourced text, fixed-precision floats for deterministic goldens):

- Participant boxes at top (NodeFill/NodeStroke, centered label), dashed
  vertical lifelines beneath (EdgeStroke).
- Messages: horizontal line at `Y` (dashed via `stroke-dasharray` when
  `Dashed`), label centered above the line; arrowhead reuses the existing
  `#arrow` marker; new `#cross` marker for `-x`; open ends get no marker.
  Self-messages: a small three-segment loop out the right side, label to
  its right.
- Autonumber prefixes `N. ` to the rendered label text.
- Activations: narrow rects (10px wide, NodeFill, NodeStroke) over the
  lifeline, `Level` shifting x by +4px per nesting level.
- Notes: rect (SubgraphFill at full opacity, NodeStroke border) + centered
  text.
- Frames: rect outline (no fill), kind label in a small corner tab
  (`loop`, `alt`, `opt`, `par`), section labels (`[label]`) at their
  divider, dashed divider lines.
- Root `<svg>` identical conventions: xmlns, width/height, viewBox,
  `class="mermaid-svg"`, font attributes.

## Integration

`Render` in `mermaid/mermaid.go` gains a `case "sequenceDiagram"`. Nothing
in `render.go`/`server.go` changes. Viewer tests flip: a supported
`sequenceDiagram` fence now asserts native `<svg` and no fallback flag;
fallback fixtures use `gantt`.

## Error handling

Unchanged contract: every parse/layout failure returns an error; callers
fall back to mermaid.js; no path may panic on user input.

## Testing

- Parser: table tests per construct; unsupported constructs
  (`critical`, `box`, `create`, unknown lines) → `ErrUnsupported`.
- Layout: the invariants listed above, plus autonumber assignment and
  unmatched-deactivate erroring.
- SVG: well-formed XML for label-torture inputs, element counts (boxes,
  lifelines, message lines, note rects, frame rects), escaping, theme
  fills, deterministic output.
- Goldens: `mermaid/testdata/seq-*.mmd` → `.svg` via the existing `-update`
  flag, eyeballed at generation time.
- Integration: `RenderBody` on a supported sequence fence contains `<svg`
  with `usedFallback == false`.
