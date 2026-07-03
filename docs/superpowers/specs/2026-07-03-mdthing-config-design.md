# mdthing — config system

An extensible configuration file for mdthing, ghostty-style: flat `key = value`
lines in `$XDG_CONFIG_HOME/mdthing/config`.

## Goals

- One config file at `$XDG_CONFIG_HOME/mdthing/config`, defaulting to
  `~/.config/mdthing/config` when the env var is unset.
- Ghostty-style format: `key = value` per line, `#` comments, blank lines
  ignored. No sections, no quoting rules.
- Extending the schema later = add a struct field + a parser case. Nothing else.
- A broken or missing config never prevents viewing a document.

## Non-goals (YAGNI)

- No TOML/YAML/JSON, no dependency.
- No CLI flags mirroring config keys.
- No live reload of the config file (restart to apply).
- No `os.UserConfigDir()` — on macOS it returns `~/Library/Application
  Support`; we explicitly follow XDG instead.

## Format

```
# ~/.config/mdthing/config
window-width = 1200
window-height = 900
theme = dark
css = ~/notes/custom.css
mermaid-theme = forest
watch = false
```

Parsing rules: split each line at the first `=`; trim whitespace around key and
value; skip blank lines and lines starting with `#`. Keys are case-sensitive
kebab-case.

## Schema (initial)

| key | type | default | effect |
|---|---|---|---|
| `window-width` | int | 900 | initial window width (px) |
| `window-height` | int | 1000 | initial window height (px) |
| `theme` | `light` \| `dark` | `light` | page palette (adds `dark` class to `<body>`), chroma highlight style (`github` vs `github-dark`), and the mermaid default theme |
| `css` | file path | unset | extra user stylesheet, served at `/_user.css` and linked after `base.css`; leading `~` expands to `$HOME` |
| `mermaid-theme` | string | `default`, or `dark` when `theme = dark` | passed to `mermaid.initialize({theme: ...})`; mermaid accepts `default`/`dark`/`forest`/`neutral` |
| `watch` | bool (`true`/`false`) | `true` | `false` skips creating the file watcher; live reload off |

## Error handling

- Missing config file → all defaults, silent.
- Unknown key → one warning line to stderr (`mdthing: config: unknown key "x"`),
  continue.
- Unparseable value (bad int, bad bool, theme not light/dark) → warning to
  stderr, keep that key's default, continue.
- Config errors are never fatal and never open a dialog.

## Architecture

One new file, `config.go`:

- `type Config struct { WindowWidth, WindowHeight int; Theme string; CSS string; MermaidTheme string; Watch bool }`
- `func defaultConfig() Config` — the defaults above.
- `func parseConfig(src []byte) (Config, []string)` — pure function, returns
  the config and a slice of warning strings. All unit tests target this.
- `func LoadConfig() Config` — resolves the XDG path, reads the file (missing
  file → defaults), prints warnings to stderr, resolves `~` in `css`, applies
  the mermaid-theme-follows-theme default.

The app is a single `package main`, so the loaded config lives in one
package-level `var cfg Config` (initialized to `defaultConfig()`), set by
`main()` before the server or window starts. Consumers read it directly:

- `main.go` — window size (`SetSize`), `watch` (skip `NewReloader` wiring when false).
- `render.go` — `RenderPage` emits `<body class="dark">` for the dark theme,
  links `/_user.css` when `css` is set, and passes the mermaid theme to
  `mermaid.initialize`; the code renderer picks `github` or `github-dark`.
- `server.go` — serves `/_user.css` from the configured path when set.
- `assets/base.css` — gains a `body.dark { ... }` palette block.

## Testing

`config_test.go` targets `parseConfig`: defaults on empty input, each key
parses, comments and whitespace skipped, unknown key warns, bad int/bool warns
and keeps the default. One render test confirms `theme = dark` marks the page.
`LoadConfig`'s path resolution and `main.go` wiring stay thin glue, verified by
running the tool.
