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
- Inline Mermaid diagrams (vendored, works offline)
- `[[wikilinks]]` and relative links navigate, with back/forward
- Live reload when the displayed file changes on disk

## Configuration

Optional config file at `$XDG_CONFIG_HOME/mdthing/config`
(default `~/.config/mdthing/config`), ghostty-style `key = value` lines:

    # ~/.config/mdthing/config
    window-width = 1200
    window-height = 900
    theme = dark              # light (default) or dark
    css = ~/notes/custom.css  # extra stylesheet, loaded after the built-in one
    mermaid-theme = forest    # default, dark, forest, neutral
    watch = false             # disable live reload

Every key is optional. Unknown keys or bad values print a warning and are
ignored; a missing file means all defaults. `mermaid-theme` follows `theme`
unless set explicitly.

[`examples/user.css`](examples/user.css) is a commented starting point for a
`css` stylesheet.
