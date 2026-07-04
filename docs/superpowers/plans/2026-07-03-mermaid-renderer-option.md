# Mermaid Renderer Option Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `mermaid-renderer = native|js` config key plus a `-mermaid-renderer` CLI flag that force all mermaid fences through the original mermaid.js path when set to `js`.

**Architecture:** `Config` gains a validated `MermaidRenderer` field (default `native`). The mermaid branch in `renderFenced` skips the native engine when the value is `js` and emits the existing escaped `<pre class="mermaid">` fallback. `main.go` adopts the stdlib `flag` package; a non-empty flag value overwrites the config field after `LoadConfig()`.

**Tech Stack:** Go stdlib only (`flag`). No new dependencies.

## Global Constraints

- Config errors never fatal: bad `mermaid-renderer` value → one stderr warning (via the existing parseConfig warning mechanism), keep `native`.
- CLI errors ARE fatal: invalid flag value or wrong positional-arg count → usage to stderr, exit 2.
- `js` mode must reuse the existing fallback path byte-for-byte (escaped `<pre class="mermaid">`, fallback flag set) — no new rendering code.
- `native` mode behavior unchanged. No new dependencies. Branch: `mermaid-renderer-option` (already created).

---

### Task 1: Config key and js render path

**Files:**
- Modify: `config.go` (field, default, parse case)
- Modify: `render.go` (mermaid branch guard)
- Test: `config_test.go`, `render_test.go`

**Interfaces:**
- Consumes: existing `Config`, `parseConfig`, `defaultConfig`, `cfg`, `renderFenced`.
- Produces: `Config.MermaidRenderer string` (`"native"` default, `"js"` alternative) — Task 2's flag writes it.

- [ ] **Step 1: Write the failing tests**

Add to `config_test.go`:

```go
func TestParseConfigMermaidRenderer(t *testing.T) {
	if d := defaultConfig(); d.MermaidRenderer != "native" {
		t.Fatalf("default renderer %q, want native", d.MermaidRenderer)
	}
	c, warns := parseConfig([]byte("mermaid-renderer = js"))
	if c.MermaidRenderer != "js" || len(warns) != 0 {
		t.Fatalf("js: got %q warns %v", c.MermaidRenderer, warns)
	}
	c, warns = parseConfig([]byte("mermaid-renderer = native"))
	if c.MermaidRenderer != "native" || len(warns) != 0 {
		t.Fatalf("native: got %q warns %v", c.MermaidRenderer, warns)
	}
	c, warns = parseConfig([]byte("mermaid-renderer = webgl"))
	if c.MermaidRenderer != "native" || len(warns) != 1 || !strings.Contains(warns[0], "mermaid-renderer") {
		t.Fatalf("bad value: got %q warns %v", c.MermaidRenderer, warns)
	}
}
```

Add to `render_test.go`:

```go
func TestRenderBodyMermaidJSMode(t *testing.T) {
	defer func(old Config) { cfg = old }(cfg)
	cfg.MermaidRenderer = "js"
	out, fallback, err := RenderBody([]byte("```mermaid\ngraph TD\nA --> B\n```\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "<svg") {
		t.Fatalf("js mode must not render natively: %s", s)
	}
	if !strings.Contains(s, `<pre class="mermaid">`) || !fallback {
		t.Fatalf("js mode must use the fallback path (fallback=%v): %s", fallback, s)
	}
}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./ -run 'TestParseConfigMermaidRenderer|TestRenderBodyMermaidJSMode'`
Expected: FAIL — `MermaidRenderer` field undefined (compile error), then behavior.

- [ ] **Step 3: Implement**

`config.go` — add the field to `Config` (after `MermaidTheme`):

```go
	MermaidRenderer string // "native" (built-in SVG engine) or "js" (mermaid.js)
```

In `defaultConfig()` add:

```go
		MermaidRenderer: "native",
```

In `parseConfig`'s switch, after the `mermaid-theme` case:

```go
		case "mermaid-renderer":
			if val == "native" || val == "js" {
				c.MermaidRenderer = val
			} else {
				warns = append(warns, fmt.Sprintf("mermaid-renderer must be native or js, got %q", val))
			}
```

`render.go` — in `renderFenced`, wrap the native attempt. Current code:

```go
	if lang == "mermaid" {
		theme := mermaid.Light
		if cfg.Theme == "dark" {
			theme = mermaid.Dark
		}
		if svg, err := mermaid.Render(code, theme); err == nil {
			w.Write(svg)
			w.WriteString("\n")
			return ast.WalkSkipChildren, nil
		}
		r.mermaidFallback = true
		...
```

becomes:

```go
	if lang == "mermaid" {
		if cfg.MermaidRenderer != "js" {
			theme := mermaid.Light
			if cfg.Theme == "dark" {
				theme = mermaid.Dark
			}
			if svg, err := mermaid.Render(code, theme); err == nil {
				w.Write(svg)
				w.WriteString("\n")
				return ast.WalkSkipChildren, nil
			}
		}
		r.mermaidFallback = true
		...
```

(The fallback lines below are untouched.)

- [ ] **Step 4: Run the full suite**

Run: `go test ./... -race`
Expected: PASS (new tests included; every pre-existing native-path test still passes because the default stays `native`).

- [ ] **Step 5: Commit**

```bash
gofmt -w . && go vet ./...
git add config.go config_test.go render.go render_test.go
git commit -m "Add mermaid-renderer config key routing js mode to fallback"
```

---

### Task 2: CLI flag, docs, verification

**Files:**
- Modify: `main.go` (flag package, usage, override)
- Modify: `README.md` (Usage + Configuration)
- Modify: `examples/config`

**Interfaces:**
- Consumes: `Config.MermaidRenderer` (Task 1), `LoadConfig()` (existing).
- Produces: the CLI surface. No unit tests — `main()` glue verified by running (Step 3).

- [ ] **Step 1: Rework `main.go` argument handling**

Replace:

```go
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mdthing <file.md>")
		os.Exit(2)
	}
	abs, err := filepath.Abs(os.Args[1])
```

with:

```go
	rendererFlag := flag.String("mermaid-renderer", "", "mermaid renderer: native or js (overrides config)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdthing [-mermaid-renderer native|js] <file.md>")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	abs, err := filepath.Abs(flag.Arg(0))
```

Add `"flag"` to the import block. Then, immediately AFTER the existing `cfg = LoadConfig()` line:

```go
	switch *rendererFlag {
	case "":
		// defer to config
	case "native", "js":
		cfg.MermaidRenderer = *rendererFlag
	default:
		fmt.Fprintln(os.Stderr, "mdthing: -mermaid-renderer must be native or js")
		flag.Usage()
		os.Exit(2)
	}
```

Also update the later `os.Args[1]` reference in the stat-error message to `flag.Arg(0)`:

```go
	if info, err := os.Stat(abs); err != nil || info.IsDir() {
		fmt.Fprintf(os.Stderr, "mdthing: cannot read %s\n", flag.Arg(0))
		os.Exit(1)
	}
```

- [ ] **Step 2: Build and run the whole suite**

Run: `go build ./... && go vet ./... && go test ./... -race`
Expected: all PASS.

- [ ] **Step 3: Manual verification**

```bash
go build -o mdthing .
./mdthing -mermaid-renderer bogus README.md; echo "exit=$?"   # expect usage + exit=2
./mdthing 2>&1; echo "exit=$?"                                 # expect usage + exit=2
```

Then launch `./mdthing -mermaid-renderer js <file with a flowchart fence>` and confirm via `curl` that the served page contains `<pre class="mermaid">` and `mermaid.min.js` but NO inline `<svg class="mermaid-svg">`; relaunch without the flag and confirm the `<svg` returns. Close window, clean exit.

- [ ] **Step 4: Update `README.md`**

Usage section, after the existing `mdthing notes.md` line:

```markdown
    mdthing -mermaid-renderer js notes.md   # force mermaid.js rendering

`-mermaid-renderer native|js` overrides the config key below for one run.
```

Configuration block, add after the `mermaid-theme` line:

```markdown
    # mermaid renderer: native (built-in SVG) or js (bundled mermaid.js)
    mermaid-renderer = js
```

- [ ] **Step 5: Update `examples/config`**

After the mermaid-theme entry:

```
# Mermaid renderer: native (built-in SVG engine) or js (bundled mermaid.js)
#mermaid-renderer = native
```

- [ ] **Step 6: Commit**

```bash
git add main.go README.md examples/config
git commit -m "Add -mermaid-renderer flag overriding config"
```

---

## Self-Review

**Spec coverage:** config key + validation + default (Task 1); js mode reuses fallback path byte-for-byte, fallback flag set, mermaid.min.js ships via existing conditional (Task 1, no new render code); flag with defer-to-config default, override after LoadConfig, loud exit-2 on bad value/argc (Task 2); README + examples/config (Task 2); tests per spec incl. cfg-restore pattern (Task 1); flag paths verified by running (Task 2 Step 3). ✓

**Placeholder scan:** clean.

**Type consistency:** `Config.MermaidRenderer string` used identically in both tasks; `flag.Arg(0)` replaces both `os.Args[1]` references.
