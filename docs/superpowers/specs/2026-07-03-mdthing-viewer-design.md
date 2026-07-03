# mdthing — Markdown viewer

A command-line tool that opens a native window rendering a Markdown file, with
inline Mermaid diagrams, clickable links to other notes, and live reload on
save. View-only — no editing. macOS is the primary target; Linux is supported.

```
mdthing notes.md
```

## Goals

- Render a `.md` file in a dedicated OS window (not a terminal, not a browser tab).
- Render fenced `mermaid` code blocks as diagrams, inline.
- GitHub-flavored Markdown: tables, strikethrough, task lists, autolinks.
- Syntax highlighting for code blocks.
- Follow links: clicking a relative link or an Obsidian-style `[[wikilink]]` to
  another `.md` navigates to it, with working back/forward.
- Live reload: editing the displayed file in another app re-renders the window.
- Works fully offline (no CDN dependencies).
- Single self-contained binary; `mdthing file.md` is the whole interface.

## Non-goals (YAGNI)

- No editing capability.
- No sidebar / file-tree / vault browser.
- No config file, themes, or plugin system.
- No auto-timeout / idle shutdown — the window *is* the session.
- No terminal rendering.

## Stack

- **Language:** Go — compiles to a small single binary, easiest distribution.
- **Window:** `github.com/webview/webview_go` — WKWebView on macOS, WebKitGTK on Linux.
- **Markdown:** `github.com/yuin/goldmark` with the GFM extension and
  `goldmark-highlighting` (chroma) for code blocks.
- **File watching:** `github.com/fsnotify/fsnotify`.
- **Mermaid:** `mermaid.min.js` vendored and embedded via `go:embed`.

## Architecture

A tiny local HTTP server renders Markdown to HTML on demand, and the webview
window browses it like a private mini-site on `127.0.0.1:<random-port>`.

```
mdthing file.md
      │
      ├─ webview window ──────► 127.0.0.1:PORT   (WKWebView / WebKitGTK)
      │                              │
      └─ fsnotify watcher            │
                                     ▼
                           Go HTTP server
                           ├─ GET /<path>.md → goldmark render → HTML page
                           ├─ static assets (images, css) from the file's dir
                           ├─ GET /_events   → SSE reload channel
                           └─ GET /_assets/mermaid.min.js (embedded)
```

**Why a local server rather than pushing HTML strings into the window:** link
navigation and back/forward come for free. A relative link or a rewritten
`[[wikilink]]` is just another URL the webview fetches, and webview history
handles back/forward. Relative images and assets work because the server roots
static files at the displayed file's directory.

The server binds to `127.0.0.1` only, on an OS-assigned free port (listener
created before the server starts; the port is read from the listener).

## Lifecycle & shutdown

The main goroutine blocks on `w.Run()`, which returns only when the user closes
the window. On return, the process shuts the HTTP server down and exits; the
watcher goroutine dies with the process.

```go
w := webview.New(false)
defer w.Destroy()
go serve(ln)                 // HTTP server on the pre-bound listener
go watch(...)                // fsnotify → SSE reload
w.Navigate("http://" + ln.Addr().String() + "/" + relPath)
w.Run()                      // blocks until window closed
srv.Shutdown(ctx)            // window gone → tear down
```

**SIGINT handling:** if launched from a terminal and interrupted with Ctrl-C,
catch SIGINT and terminate the webview (`w.Terminate()`) so shutdown is clean.

## Rendering

One HTML template wraps every rendered page: readable GitHub-ish CSS, the
rendered body, the Mermaid bootstrap, and the SSE reload subscription.

- **goldmark** configured with the GFM extension and `goldmark-highlighting`.
- **Wikilinks:** `[[note]]` is rewritten to `<a href="note.md">note</a>` and
  `[[note|label]]` to `<a href="note.md">label</a>`. Start with a regex prepass
  on the raw Markdown; upgrade to a goldmark AST transform only if the prepass
  misbehaves (e.g. matches inside code spans).
- **Mermaid:** fenced ` ```mermaid ` blocks are emitted as
  `<pre class="mermaid">…</pre>` (raw diagram source preserved). The template
  loads the embedded `mermaid.min.js` and calls `mermaid.run()` after load.
- **Links:** relative `.md` links resolve against the server and navigate.
  External `http(s)` links open normally in the webview.

## Live reload

The server tracks which file is currently displayed. When the user navigates to
another note, the watcher re-points to that file.

Each rendered page subscribes to `GET /_events` (Server-Sent Events, stdlib
`net/http` — no extra dependency). When fsnotify reports the current file
changed, the server writes one `reload` event; the page calls
`location.reload()`. SSE is used rather than a Go-driven `w.Eval` because it
survives link navigation cleanly — every freshly loaded page just resubscribes.

Debounce watcher events briefly (editors often emit several writes per save) so
one save triggers one reload.

## Module layout

```
main.go        — flags, listener, webview lifecycle, SIGINT handling
server.go      — http.ServeMux: render .md, static assets, /_events (SSE)
render.go      — goldmark setup + wikilink prepass + HTML template
watch.go       — fsnotify wrapper, tracks current file, fans out reload events
assets/        — mermaid.min.js, base.css  (go:embed)
```

Four small files, one responsibility each.

## Error handling

- No file argument, or file does not exist / is not readable → print usage/error
  to stderr and exit non-zero (before opening a window).
- Requested `.md` path that does not exist while navigating → render a simple
  "not found" page in the window rather than crashing.
- Malformed Mermaid → Mermaid renders its own error box in-page; not fatal.

## Testing

`render.go` holds the only real logic and gets unit tests (Markdown in →
HTML out):

- a `mermaid` fenced block survives as `<pre class="mermaid">` with source intact,
- `[[wikilink]]` and `[[note|label]]` are rewritten to the correct anchors,
- a GFM table renders to a `<table>`.

The server, watcher, and webview are thin glue, verified by running the tool
against a sample file (open, edit-and-see-reload, click a link, view a diagram).

## Build & distribution

- `go build` produces a single binary. The embedded assets mean no runtime
  files to ship.
- Linux requires WebKitGTK dev packages at build time (documented in the README);
  macOS uses the system WKWebView with no extra dependency.
