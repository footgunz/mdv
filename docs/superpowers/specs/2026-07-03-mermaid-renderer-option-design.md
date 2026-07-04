# mdthing — mermaid-renderer config option and CLI flag

A switch between the native SVG mermaid engine and the original mermaid.js
rendering path, settable in the config file and overridable per-invocation
with a CLI flag.

## Goals

- Config key `mermaid-renderer = native | js`, default `native` (current
  behavior). Follows existing config rules: unknown/bad value → one stderr
  warning, keep the default, never fatal.
- CLI flag `-mermaid-renderer native|js` overriding the config for one run.
  Usage becomes `mdthing [-mermaid-renderer native|js] <file.md>`.
- `js` mode routes every mermaid fence through the existing fallback path
  (`<pre class="mermaid">` + bundled mermaid.js), exactly as diagrams
  rendered before the native engine existed. `mermaid-theme` config applies
  as before; pages ship mermaid.min.js via the existing conditional.
- `native` mode is untouched current behavior (native for supported
  diagrams, JS fallback otherwise).

## Non-goals (YAGNI)

- No per-diagram or per-file override, no third renderer value.
- No flag mirroring of any other config key — this is not the start of a
  general flags-for-config system.

## Behavior

- `Config` gains `MermaidRenderer string`; `defaultConfig()` sets `"native"`;
  `parseConfig` accepts exactly `native`/`js` and warns otherwise (keeping
  the default), consistent with the `theme` key's validation style.
- `main.go` adopts the stdlib `flag` package: `-mermaid-renderer` string
  flag, default `""` meaning "defer to config". After `cfg = LoadConfig()`,
  a non-empty flag value overwrites `cfg.MermaidRenderer`. An invalid flag
  value (not `native`/`js`) prints usage to stderr and exits 2 — CLI typos
  are loud, unlike config typos, because the user just typed them.
  Positional argument handling moves to `flag.Arg(0)`; zero or multiple
  positional args → usage + exit 2 (same as today's behavior for wrong
  argc).
- `render.go` `renderFenced` mermaid branch: when
  `cfg.MermaidRenderer == "js"`, skip `mermaid.Render` and emit the escaped
  `<pre class="mermaid">` fallback with the fallback flag set. No other
  render changes.

## Documentation

- README: flag in the Usage section, `mermaid-renderer` row in the
  Configuration block.
- `examples/config`: commented `#mermaid-renderer = native` entry with the
  value list.

## Testing

- `config_test.go`: `mermaid-renderer = js` parses; `native` parses; bad
  value warns and keeps `native`; default is `native`.
- `render_test.go`: with `cfg.MermaidRenderer = "js"` (cfg restored via the
  existing defer pattern), a natively-supported flowchart fence emits
  `<pre class="mermaid">` (no `<svg`) and reports `usedFallback == true`.
- Flag precedence and usage-error paths are `main()` glue, verified by
  running: `mdthing -mermaid-renderer js file.md` serves a page with no
  inline mermaid SVG; `-mermaid-renderer bogus` exits 2 with usage.
