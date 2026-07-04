# mdthing

A command-line Markdown viewer that opens a native window. Renders inline
Mermaid diagrams, follows links between notes, and live-reloads on save.
View-only.

## Usage

    mdthing notes.md

Close the window (or press Ctrl-C in the terminal) to quit.

## Build

    go build -o mdthing .

**Linux** requires WebKitGTK dev packages at build time, e.g. on Debian/Ubuntu:

    sudo apt install libwebkit2gtk-4.1-dev

**macOS** uses the system WebKit; no extra packages are needed.

## Features

- GitHub-flavored Markdown (tables, task lists, strikethrough)
- Syntax-highlighted code blocks
- Inline Mermaid diagrams — common flowcharts and sequence diagrams render
  natively to SVG (no JS); other diagram types fall back to the bundled
  mermaid.js
- `[[wikilinks]]` and relative links navigate, with back/forward
- Live reload when the displayed file changes on disk

## Configuration

Optional config file at `$XDG_CONFIG_HOME/mdthing/config`
(default `~/.config/mdthing/config`), ghostty-style `key = value` lines:

    # ~/.config/mdthing/config
    window-width = 1200
    window-height = 900
    # light (default) or dark
    theme = dark
    # extra stylesheet, loaded after the built-in one
    css = ~/notes/custom.css
    # default, dark, forest, neutral
    mermaid-theme = forest
    # disable live reload
    watch = false

Comments must be on their own line — everything after `=` is the value.

Every key is optional. Unknown keys or bad values print a warning and are
ignored; a missing file means all defaults. `mermaid-theme` follows `theme`
unless set explicitly.

[`examples/config`](examples/config) is a commented starting point with all
defaults; [`examples/user.css`](examples/user.css) is one for a `css`
stylesheet.
