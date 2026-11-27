# mdv

A command-line Markdown viewer that opens a native window. Renders inline
Mermaid diagrams, follows links between notes, and live-reloads on save.
View-only.

## Usage

    mdv notes.md                        # open the viewer window
    mdv -mermaid-renderer js notes.md   # force mermaid.js rendering
    mdv -html notes.md > notes.html     # static self-contained HTML export

`-mermaid-renderer native|js` overrides the config key below for one run.

Close the window (or press Ctrl-C in the terminal) to quit.

## Build

    go build -tags desktop,production -o mdv ./cmd/mdv

**Linux** requires GTK3 and WebKitGTK dev packages at build time, plus the
`webkit2_41` build tag for the current `webkit2gtk-4.1` ABI, e.g. on
Debian/Ubuntu:

    sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
    go build -tags desktop,production,webkit2_41 -o mdv ./cmd/mdv

**macOS** uses the system WebKit; no extra packages are needed.

**Windows** uses the WebView2 runtime (preinstalled on Windows 11 and
current Windows 10). Wails' Windows backend is pure Go, so no C compiler
is needed:

    go build -tags desktop,production -o mdv.exe ./cmd/mdv    # or: task build

### macOS app bundle

    task app            # -> ./mdv.app
    task install-app    # -> /Applications/mdv.app + `mdv` on PATH

`mdv.app` shows up under Finder's **Open With** for `.md` files, with a
proper icon and no terminal. It registers as an alternate handler, so it
won't take over as your default editor for Markdown — pick "Always Open
With" yourself if you want that.

`task install-app` is the one to use if you launch mdv both ways: it copies
the bundle to `/Applications` and symlinks `$(go env GOPATH)/bin/mdv` at the
binary inside it, so a single copy serves the command line and Finder and
the two can't drift. Run through the symlink, `mdv` behaves exactly as
before — usage on no arguments, a window on `mdv file.md`. (`task
install-cli-only` installs the bare binary instead, replacing that symlink.)

Opening a second file reuses the running window. Launching `mdv.app` with no
document shows a placeholder until you open one.

`task icons` regenerates `cmd/mdv/icon.png` and `build/darwin/mdv.icns` from
`cmd/mdv/icon.svg`; it needs `rsvg-convert` (`brew install librsvg`).

### Windows: command line and "Open with"

    task install-windows    # or: powershell -ExecutionPolicy Bypass -File build\windows\install.ps1

This `go install`s `mdv.exe` into `$(go env GOPATH)\bin` (put
that on your PATH for `mdv file.md` in cmd or PowerShell) and registers it
per-user (HKCU, no admin) under Explorer's **Open with** for `.md` and
`.markdown`, and in **Settings > Default apps**. It never takes over as the
default handler; pick mdv under "Open with > Choose another app" and tick
**Always** for that. `-Uninstall` removes the registration.

Started from Explorer, mdv hides the console Windows gives it and shows
errors in a message box instead. Double-clicked with no document, it opens
a file picker. From a terminal it behaves exactly as on other platforms.
Opening a second file from Explorer reuses that window, as Finder does;
`mdv file.md` from a terminal always gets a window of its own.

`task winres` rebuilds the embedded exe icon, manifest (per-monitor DPI
awareness) and version info (`cmd/mdv/rsrc_windows_*.syso`) from
`cmd/mdv/icon.png` and `build/windows/`.

## Features

- GitHub-flavored Markdown (tables, task lists, strikethrough)
- Syntax-highlighted code blocks
- Inline Mermaid diagrams — common flowcharts, sequence diagrams, pie
  charts, and state diagrams render natively to SVG (no JS); other diagram
  types fall back to the bundled mermaid.js
- `[[wikilinks]]` and relative links navigate, with back/forward: Cmd-[ /
  Cmd-] or Cmd-Left / Cmd-Right on macOS, Alt-Left / Alt-Right on Windows
  and Linux, and the mouse's back/forward buttons
- Live reload when the displayed file changes on disk

## Configuration

Optional config file at `$XDG_CONFIG_HOME/mdv/config`
(default `~/.config/mdv/config`, i.e. `%USERPROFILE%\.config\mdv\config`
on Windows), ghostty-style `key = value` lines:

    # ~/.config/mdv/config
    window-width = 1200
    window-height = 900
    # light (default) or dark
    theme = dark
    # extra stylesheet, loaded after the built-in one
    css = ~/notes/custom.css
    # default, dark, forest, neutral
    mermaid-theme = forest
    # mermaid renderer: native (built-in SVG) or js (bundled mermaid.js)
    mermaid-renderer = js
    # disable live reload
    watch = false

Comments must be on their own line — everything after `=` is the value.

Every key is optional. Unknown keys or bad values print a warning and are
ignored; a missing file means all defaults. `mermaid-theme` follows `theme`
unless set explicitly.

[`examples/config`](examples/config) is a commented starting point with all
defaults; [`examples/user.css`](examples/user.css) is one for a `css`
stylesheet.

## Static export

`-html` renders once to stdout: native diagrams as inline SVG, styles
inlined, and mermaid.js embedded only when a diagram needs the JS fallback.
Relative image and `[[wikilink]]` targets are kept as-is — they resolve as
long as the HTML file stays next to the sources it references.
