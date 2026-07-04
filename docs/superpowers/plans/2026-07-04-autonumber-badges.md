# Autonumber Badges Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sequence autonumbers render as mermaid.js-style circled badges at the message line start instead of a `N. ` label prefix.

**Architecture:** One task: `seq_svg.go` gains a badge helper called from both message branches; the label uses plain `v.Text`; the now-pointless `numbered()` helper is deleted (its layout call site measures `v.Text` directly). Goldens regenerate.

**Tech Stack:** Go stdlib only.

## Global Constraints

- Badge: `<circle class="autonumber" r="8">` centered on the line/curve start, fill `Theme.Text`; number centered inside, font-size 10, fill `Theme.NodeFill`. Emitted only when `d.Autonumber`.
- Labels lose the `N. ` prefix everywhere; `SeqMessage.Num` assignment in layout is unchanged.
- Deterministic `%.1f`; goldens regenerated with `-update`; escaping/theming conventions untouched.
- Branch: `autonumber-badges` (already created).
- **If plan code conflicts with plan tests, the tests govern.**

---

### Task 1: Badge rendering

**Files:**
- Modify: `mermaid/seq_svg.go` (badge helper + both message branches)
- Modify: `mermaid/seq_layout.go` (delete `numbered`, measure `v.Text`)
- Test: `mermaid/seq_test.go` (new test + update `TestSeqSVGElements`)

**Interfaces:**
- Consumes: `SeqMessage.Num`, `SeqDiagram.Autonumber`, `Theme`.
- Produces: `func autonumBadge(b *bytes.Buffer, x, y float64, n int, t Theme)` (unexported, seq_svg.go only).

- [ ] **Step 1: Write the failing test**

Add to `mermaid/seq_test.go`:

```go
func TestSeqSVGAutonumberBadges(t *testing.T) {
	out := renderSeq(t, "sequenceDiagram\nautonumber\na->>b: one\nb->>b: two", Light)
	if c := strings.Count(out, `class="autonumber"`); c != 2 {
		t.Fatalf("badges %d, want 2\n%s", c, out)
	}
	for _, want := range []string{">1<", ">2<"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing badge number %s", want)
		}
	}
	if strings.Contains(out, "1. one") || strings.Contains(out, "2. two") {
		t.Fatal("N. prefix must be gone from labels")
	}
	if !strings.Contains(out, ">one<") {
		t.Fatal("plain label missing")
	}

	plain := renderSeq(t, "sequenceDiagram\na->>b: x", Light)
	if strings.Contains(plain, "autonumber") {
		t.Fatal("badges emitted without autonumber")
	}

	dark := renderSeq(t, "sequenceDiagram\nautonumber\na->>b: x", Dark)
	if !strings.Contains(dark, `r="8" fill="`+Dark.Text+`"`) {
		t.Fatalf("dark badge fill must be Dark.Text:\n%s", dark)
	}
}
```

Update `TestSeqSVGElements` (its `seqFixture` has `autonumber`): in its `for _, want := range` list, replace `"1. solid request"` with `">solid request<"` and `"5. again"` with `">again<"`; add before that loop:

```go
	if c := strings.Count(out, `class="autonumber"`); c != 5 {
		t.Fatalf("autonumber badges %d, want 5", c)
	}
```

- [ ] **Step 2: Run tests to verify failure**

Run: `go test ./mermaid/ -run 'TestSeqSVGAutonumberBadges|TestSeqSVGElements'`
Expected: FAIL — no `class="autonumber"` in output; labels still prefixed.

- [ ] **Step 3: Implement**

`mermaid/seq_svg.go` — add:

```go
// autonumBadge draws a mermaid-style circled message number at the line start.
func autonumBadge(b *bytes.Buffer, x, y float64, n int, t Theme) {
	fmt.Fprintf(b, `<circle class="autonumber" cx="%.1f" cy="%.1f" r="8" fill="%s"/>`, x, y, t.Text)
	fmt.Fprintf(b, `<text x="%.1f" y="%.1f" text-anchor="middle" dominant-baseline="central" font-size="10" fill="%s">%d</text>`, x, y, t.NodeFill, n)
}
```

In the `*SeqMessage` case, change `text := numbered(v, d.Autonumber)` to `text := v.Text`. In the self-message branch, after the label `Fprintf`, add:

```go
					if d.Autonumber {
						autonumBadge(&b, from, v.Y, v.Num, t)
					}
```

before the `continue`. In the normal-message branch, after the label `Fprintf`, add the same three lines (badge at `from, v.Y` — the line start).

`mermaid/seq_layout.go` — in `scanH`'s message case, change `measureText(numbered(v, d.Autonumber), t.FontSize)` to `measureText(v.Text, t.FontSize)`, and DELETE the whole `numbered` function (both call sites are now gone).

- [ ] **Step 4: Regenerate goldens, full suite**

Run: `go test ./mermaid/ -update && go test ./... -race`
Expected: PASS. Textually confirm `seq-basic.svg` contains `class="autonumber"` circles and plain (unprefixed) labels.

- [ ] **Step 5: Commit**

```bash
gofmt -w mermaid/ && go vet ./mermaid/
git add mermaid/seq_svg.go mermaid/seq_layout.go mermaid/seq_test.go mermaid/testdata
git commit -m "Render autonumbers as circled badges like mermaid.js"
```

---

## Self-Review

**Spec coverage:** badge geometry/colors/conditionality (Step 3 + dark test); prefix removal incl. layout measurement (Step 3, both call sites); Num assignment untouched; goldens (Step 4); non-goals respected (no autonumber args, no note badges, no Theme fields). ✓

**Placeholder scan:** clean.

**Type consistency:** `autonumBadge(*bytes.Buffer, float64, float64, int, Theme)` defined and used only within seq_svg.go; `numbered` deleted with both call sites accounted for.
