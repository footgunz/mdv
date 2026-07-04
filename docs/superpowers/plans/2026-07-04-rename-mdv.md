# Rename mdthing → mdv Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete rename to `mdv`: module path, binary, CLI strings, config path, build files, living docs.

**Architecture:** One mechanical task. The config path is the only behavior change and goes test-first; the rest is a sweep guided by `grep -rn mdthing` until clean (outside `docs/superpowers/` and `.git`).

**Tech Stack:** Go stdlib, `go mod edit`.

## Global Constraints

- Historical specs/plans under `docs/superpowers/` are NOT edited (except the rename spec/plan themselves being added).
- No behavior change beyond `configPath` → `$XDG_CONFIG_HOME/mdv/config` (no legacy fallback).
- Branch: `rename-mdv` (already created).

---

### Task 1: The rename

**Files:**
- Modify: `go.mod`, `render.go` (import), `config.go`, `config_test.go`, `main.go`, `Taskfile.yml`, `.gitignore`, `README.md`, `examples/config`, `examples/user.css`

**Interfaces:**
- Consumes/Produces: no API changes; module path becomes `github.com/dgunther/mdv`.

- [ ] **Step 1: Config path test first**

In `config_test.go` `TestConfigPathXDG`, change both expectations from `"mdthing"` to `"mdv"`:

```go
	if got := configPath(); got != filepath.Join("/xdg", "mdv", "config") {
```
and
```go
	if got := configPath(); got != filepath.Join(home, ".config", "mdv", "config") {
```

Run: `go test ./ -run TestConfigPathXDG` — expected FAIL (paths still mdthing).

- [ ] **Step 2: Fix `configPath` in `config.go`**

Change `filepath.Join(dir, "mdthing", "config")` → `filepath.Join(dir, "mdv", "config")`. Re-run the test — PASS.

- [ ] **Step 3: Module path + import**

```bash
go mod edit -module github.com/dgunther/mdv
```

In `render.go`, change the import `"github.com/dgunther/mdthing/mermaid"` → `"github.com/dgunther/mdv/mermaid"`. Run `go build ./...` — must compile.

- [ ] **Step 4: CLI strings**

- `main.go`: usage string → `"usage: mdv [-html] [-mermaid-renderer native|js] <file.md>"`; every `"mdthing:"` stderr prefix → `"mdv:"` (there are 6: Abs error, cannot-read, renderer-flag error, reloader error, listen error, html-mode read/render errors).
- `config.go`: `"mdthing: config:"` → `"mdv: config:"` in `LoadConfig`.

- [ ] **Step 5: Build files + docs**

- `Taskfile.yml`: `go build -o mdv .` in build; `rm -f mdv` in clean (desc lines mentioning mdthing → mdv).
- `.gitignore`: `/mdthing` → `/mdv`.
- `README.md`: title `# mdv`, every usage example and prose mention (`mdthing notes.md` → `mdv notes.md`, config path `~/.config/mdthing/config` → `~/.config/mdv/config`, build output `-o mdv`).
- `examples/config`: comment paths → `~/.config/mdv/config`, `~/.config/mdv/user.css`.
- `examples/user.css`: comment paths likewise.

- [ ] **Step 6: Verify sweep + suite + smoke**

```bash
grep -rn mdthing . --exclude-dir=.git --exclude-dir=docs && echo "LEFTOVERS" || echo "clean"
go build ./... && go vet ./... && go test ./... -race
rm -f mdthing && go build -o mdv . && ls -la mdv
./mdv -x 2>&1 | head -2          # usage says mdv
./mdv -html README.md | head -c 200   # renders
task clean && task build && ls mdv
task install && ls ~/go/bin/mdv && rm -f ~/go/bin/mdthing
```

Expected: grep clean; suite green; usage/error strings say `mdv`; `~/go/bin/mdv` exists; stale `~/go/bin/mdthing` removed.

- [ ] **Step 7: Commit**

```bash
git add -A && git status --short
git commit -m "Rename mdthing to mdv"
```

(The deleted `mdthing` build artifact was never tracked; `git add -A` picks up all edited files only.)

---

## Self-Review

**Spec coverage:** module+import (Step 3), binary via Taskfile/install (Steps 5–6), CLI strings incl. config warnings (Step 4), config path test-first (Steps 1–2), .gitignore (Step 5), README+examples (Step 5), stale binary cleanup (Step 6), docs/superpowers untouched, grep-clean verification (Step 6). ✓

**Placeholder scan:** clean. **Type consistency:** no signatures change.
