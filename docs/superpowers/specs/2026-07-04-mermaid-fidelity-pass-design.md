# mdthing — native mermaid renderer fidelity pass

Close three visual gaps between the native SVG engine and mermaid.js,
identified in the 2026-07-03 screenshot comparison: angular edges, the
rectangular self-message loop, and cramped box sizing.

## Goals

- Flowchart edges render as smooth curves through dagre's routing points,
  visually equivalent to mermaid's `curveBasis` styling.
- Sequence self-messages render as a smooth right-bulging arc.
- Box sizing calibrated toward mermaid.js's proportions:
  - sequence participant boxes get minimums (measured text still grows
    them beyond),
  - sequence vertical rhythm loosens to mermaid-like row spacing,
  - flowchart node padding increases to mermaid-like interiors.
- Everything else — themes, fonts (14px page-matched, deliberate
  divergence), markers, escaping, fallback contract — unchanged.

## Non-goals (YAGNI)

- Autonumber badges and mirrored participant boxes (explicitly declined).
- Pixel-identical parity; the yardstick is the four-panel screenshot
  comparison reading as "same family".
- No new dependencies; curve math is inline arithmetic.

## Changes

### 1. Curved flowchart edges (`mermaid/svg.go`)

Replace the polyline `d` (`M … L … L …`) with midpoint-quadratic
smoothing: start at `p0`, `L` to the midpoint of `p0→p1`, then for each
interior point `p[i]` emit `Q p[i] midpoint(p[i], p[i+1])`, and close with
`L pn`. Two-point edges stay straight lines. The path still terminates
exactly at `pn`, so `marker-end` arrowheads and dotted/thick stroke
styling are unaffected. Edge label placement unchanged.

### 2. Curved self-messages (`mermaid/seq_svg.go`)

The three-segment rectangular loop becomes one cubic path:
`M (x, y) C (x+seqSelfW·1.6, y) (x+seqSelfW·1.6, y+seqSelfH) (x+6, y+seqSelfH)`
— out to the right, down, back to just right of the lifeline, arrowhead
via the existing marker. Label position unchanged (right of the bulge).

### 3. Box sizing calibration

- `mermaid/seq_layout.go`: new constants `seqActorMinW = 150`,
  `seqActorMinH = 50`; participant `W = max(measured+pad, seqActorMinW)`,
  `H = max(measured+pad, seqActorMinH)` (uniform-H rule unchanged — all
  boxes take the max H). `seqRowPad` 10 → 16 (message row ≈ 33px).
- `mermaid/text.go` (flowchart node measurement): `padX` 16 → 20,
  `padY` 10 → 15. Diamond (1.6×/1.8×) and circle (max-dimension) factors
  unchanged — they scale from the padded box.
- Horizontal gap logic, dagre nodesep/ranksep (50/50), and all frame/note
  constants unchanged.

## Compatibility

- Golden files change (geometry + path commands): regenerate with
  `-update`, eyeball at generation time.
- Layout invariant tests unaffected in *kind*; tests asserting exact
  constants (none currently do) would be updated by their own failure.

## Testing

- Layout: participant minimums applied (short label → 150×50; long label
  → wider than 150); row rhythm reflected in message Y spacing
  (delta ≥ lineheight + 16).
- SVG: flowchart edge path with ≥3 points contains `Q ` commands and ends
  at dagre's final point (marker anchoring); 2-point edges remain pure
  `M/L`. Self-message path contains `C `.
- Goldens regenerated for both flowchart and sequence fixtures.
- Final acceptance: re-run the four-panel headless-Chrome comparison
  (native/js × light/dark) and eyeball proportions side by side.
