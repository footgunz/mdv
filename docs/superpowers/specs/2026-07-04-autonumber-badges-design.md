# mdthing — sequence autonumber badges

Render sequence-diagram autonumbers as mermaid.js-style circled badges at
the message line start, replacing the current `N. ` text prefix.

## Goals

- With `autonumber`, every message gets a filled circle (radius 8) centered
  on the message line's start point — the from-lifeline x at `msg.Y` for
  normal messages, the curve start for self-messages — with the number
  centered inside (font-size 10).
- Colors invert against the diagram: badge fill `Theme.Text`, number text
  `Theme.NodeFill`. Works in both light and dark without new Theme fields.
- The message label drops the `N. ` prefix; the badge carries the number.
- Badges overlap the lifeline start deliberately (mermaid's placement).

## Non-goals (YAGNI)

- No `autonumber <start> <step>` arguments (still unsupported → fallback).
- No badge on notes/frames (mermaid numbers messages only — unchanged).
- No new Theme fields or config.

## Changes

- `mermaid/seq_svg.go`: in the message branch, when `d.Autonumber`, emit
  `<circle class="autonumber" cx cy r="8">` + centered `<text>` (font-size
  10, fill NodeFill) at the line/curve start; label text becomes plain
  `v.Text` (no prefix).
- `mermaid/seq_layout.go`: `numbered()` reduces to returning `m.Text` for
  width measurement (rename or inline as fits; `Num` assignment unchanged).

## Testing

- Autonumbered fixture: `<circle class="autonumber"` count equals message
  count; badge number texts present; label text does NOT contain `1. `.
- Non-autonumber fixture: zero `class="autonumber"` occurrences.
- Dark theme: badge fill equals `Dark.Text`, number fill equals
  `Dark.NodeFill`.
- Goldens regenerated (`seq-basic.mmd` uses autonumber); eyeball at
  generation.
