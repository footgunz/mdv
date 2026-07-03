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
