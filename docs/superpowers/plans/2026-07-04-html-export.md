# HTML Export Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `mdthing -html notes.md > notes.html` renders one file to self-contained HTML on stdout — native SVG inline, mermaid.js inlined only for fallback diagrams, no server/window.

**Architecture:** `RenderStaticPage` in render.go assembles the standalone document from the existing `assetsFS` embed; `main.go` gains a `-html` branch that runs read → `RenderBody` → `RenderStaticPage` → stdout before any listener/webview setup.

**Tech Stack:** Go stdlib only.

## Global Constraints

- `-html` never touches webview/server/watcher code paths (must work headless).
- mermaid payload inlined ONLY when `RenderBody`'s fallback flag is true; no `/_assets/` hrefs, no `/_events` in static output.
- Escaping: title HTML-escaped, mermaid theme JS-escaped (same as `RenderPage`). Unreadable user CSS skipped silently (config never-fatal philosophy).
- Errors → stderr + exit 1. Usage: `mdthing [-html] [-mermaid-renderer native|js] <file.md>`.
- Branch: `html-export` (already created). Tests govern over plan code.

---

### Task 1: RenderStaticPage

**Files:**
- Modify: `render.go`
- Test: `render_test.go`

**Interfaces:**
- Consumes: `assetsFS` (assets.go), `cfg` (Theme/CSS/MermaidTheme).
- Produces: `func RenderStaticPage(body []byte, title string, includeMermaidJS bool) []byte` — Task 2 calls it.

- [ ] **Step 1: Write the failing tests**

Add to `render_test.go`:

```go
func TestRenderStaticPage(t *testing.T) {
	page := string(RenderStaticPage([]byte("<p>hi</p>"), "Doc", false))
	for _, want := range []string{"<title>Doc</title>", "<p>hi</p>", "<style>", ".markdown-body"} {
		if !strings.Contains(page, want) {
			t.Fatalf("missing %q", want)
		}
	}
	for _, bad := range []string{"/_assets/", "/_events", "mermaid.initialize"} {
		if strings.Contains(page, bad) {
			t.Fatalf("static page must not contain %q", bad)
		}
	}
}

func TestRenderStaticPageMermaidJS(t *testing.T) {
	page := string(RenderStaticPage([]byte("x"), "t", true))
	if !strings.Contains(page, "mermaid.initialize") {
		t.Fatal("fallback static page must bootstrap mermaid")
	}
	if len(page) < 1_000_000 {
		t.Fatalf("mermaid payload not inlined (len=%d)", len(page))
	}
	if strings.Contains(page, "/_assets/") {
		t.Fatal("static page must not reference served assets")
	}
}

func TestRenderStaticPageUserCSSAndTheme(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)
	dir := t.TempDir()
	css := filepath.Join(dir, "u.css")
	if err := os.WriteFile(css, []byte(".markdown-body{letter-spacing:9px}"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.CSS = css
	cfg.Theme = "dark"
	page := string(RenderStaticPage([]byte("x"), "t", false))
	if !strings.Contains(page, "letter-spacing:9px") {
		t.Fatal("user css not inlined")
	}
	if !strings.Contains(page, `<body class="dark">`) {
		t.Fatal("dark body class missing")
	}

	cfg.CSS = filepath.Join(dir, "missing.css")
	page = string(RenderStaticPage([]byte("x"), "t", false))
	if !strings.Contains(page, "<style>") || strings.Contains(page, "letter-spacing") {
		t.Fatal("unreadable user css must be skipped silently")
	}
}
```

Add `"os"` and `"path/filepath"` to render_test.go's imports if absent.

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./ -run TestRenderStaticPage`
Expected: FAIL — `undefined: RenderStaticPage`.

- [ ] **Step 3: Implement in `render.go`**

```go
// RenderStaticPage wraps a body fragment as a self-contained document for
// -html export: styles inlined, mermaid.js inlined only when a diagram fell
// back, no live-reload plumbing.
func RenderStaticPage(body []byte, title string, includeMermaidJS bool) []byte {
	var b bytes.Buffer
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>`)
	template.HTMLEscape(&b, []byte(title))
	b.WriteString(`</title><style>`)
	css, _ := assetsFS.ReadFile("assets/base.css") // embedded, cannot fail
	b.Write(css)
	if cfg.CSS != "" {
		if user, err := os.ReadFile(cfg.CSS); err == nil {
			b.Write(user)
		}
	}
	b.WriteString(`</style>`)
	if cfg.Theme == "dark" {
		b.WriteString(`</head><body class="dark">`)
	} else {
		b.WriteString(`</head><body>`)
	}
	b.WriteString(`<article class="markdown-body">`)
	b.Write(body)
	b.WriteString(`</article>`)
	if includeMermaidJS {
		js, _ := assetsFS.ReadFile("assets/mermaid.min.js")
		b.WriteString(`<script>`)
		b.Write(js)
		b.WriteString("</script>")
		fmt.Fprintf(&b, `<script>mermaid.initialize({startOnLoad:true,theme:'%s'});</script>`, template.JSEscapeString(cfg.MermaidTheme))
	}
	b.WriteString(`</body></html>`)
	return b.Bytes()
}
```

Add `"os"` to render.go's imports.

- [ ] **Step 4: Run tests**

Run: `go test ./... -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add render.go render_test.go
git commit -m "Add RenderStaticPage for self-contained HTML export"
```

---

### Task 2: -html flag, README, CLI verification

**Files:**
- Modify: `main.go`, `README.md`

**Interfaces:**
- Consumes: `RenderStaticPage` (Task 1), `RenderBody`, `LoadConfig`, existing flags.
- Produces: the `-html` CLI surface. No unit tests — verified by running (Step 3, headless-safe).

- [ ] **Step 1: Implement the flag in `main.go`**

Add beside the existing flag declaration:

```go
	htmlFlag := flag.Bool("html", false, "render self-contained HTML to stdout and exit")
```

Update the usage string:

```go
		fmt.Fprintln(os.Stderr, "usage: mdthing [-html] [-mermaid-renderer native|js] <file.md>")
```

Immediately AFTER the `-mermaid-renderer` override switch (so config + flag are settled) and BEFORE `baseDir := ...`:

```go
	if *htmlFlag {
		src, err := os.ReadFile(abs)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdthing:", err)
			os.Exit(1)
		}
		body, fallback, err := RenderBody(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mdthing:", err)
			os.Exit(1)
		}
		os.Stdout.Write(RenderStaticPage(body, filepath.Base(abs), fallback))
		return
	}
```

- [ ] **Step 2: Build + suite**

Run: `go build ./... && go vet ./... && go test ./... -race`
Expected: PASS.

- [ ] **Step 3: CLI verification (headless-safe — run all of it)**

```bash
go build -o mdthing .
printf '# T\n\n```mermaid\ngraph TD\na-->b\n```\n' > /tmp/native.md
printf '# T\n\n```mermaid\ngantt\ntitle X\n```\n' > /tmp/fb.md
./mdthing -html /tmp/native.md > /tmp/native.html && wc -c /tmp/native.html
grep -c 'mermaid-svg' /tmp/native.html          # >=1
grep -c 'mermaid.initialize' /tmp/native.html   # expect 0 (grep exits 1)
./mdthing -html /tmp/fb.md > /tmp/fb.html && wc -c /tmp/fb.html   # ~2.7MB
grep -c 'mermaid.initialize' /tmp/fb.html       # 1
./mdthing -html -mermaid-renderer js /tmp/native.md | grep -c '<svg class="mermaid-svg"' # 0 (exit 1)
./mdthing -html /tmp/nope.md; echo "exit=$?"    # stderr + exit=1
```

Expected: native.html small (tens of KB) with inline SVG and no mermaid bootstrap; fb.html ~2.7MB with bootstrap; js-mode export has no native SVG; missing file exits 1.

- [ ] **Step 4: README**

Usage section — replace the flag line block with:

```markdown
    mdthing notes.md                        # open the viewer window
    mdthing -mermaid-renderer js notes.md   # force mermaid.js rendering
    mdthing -html notes.md > notes.html     # static self-contained HTML export
```

After the Configuration section, add:

```markdown
## Static export

`-html` renders once to stdout: native diagrams as inline SVG, styles
inlined, and mermaid.js embedded only when a diagram needs the JS fallback.
Relative image and `[[wikilink]]` targets are kept as-is — they resolve as
long as the HTML file stays next to the sources it references.
```

- [ ] **Step 5: Commit**

```bash
git add main.go README.md
git commit -m "Add -html flag for static self-contained export"
```

---

### Task 3 (controller): visual acceptance

Export the comparison fixture with `-html` (light + dark configs), open/screenshot via headless Chrome from the FILE (no server), eyeball both, confirm byte sizes tell the lean-vs-fallback story.

---

## Self-Review

**Spec coverage:** stdout-only flag + usage (Task 2); RenderStaticPage inlining rules, escaping, silent-skip user CSS, dark class, conditional mermaid payload (Task 1, all tested); never touches webview path (Task 2 branch placement, before any listener/webview construction); composes with -mermaid-renderer (Task 2 Step 3 check); errors exit 1 (Task 2); README caveat (Task 2); no new config keys. ✓

**Placeholder scan:** clean.

**Type consistency:** `RenderStaticPage([]byte, string, bool) []byte` defined Task 1, called Task 2 with `(body, filepath.Base(abs), fallback)` — matches `RenderBody`'s `(body, fallback, err)` returns.
