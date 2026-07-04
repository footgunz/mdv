package mermaid

import (
	"errors"
	"strings"
	"testing"
)

// mustParseSeq is shared by Tasks 2-4 tests.
func mustParseSeq(t *testing.T, src string) *SeqDiagram {
	t.Helper()
	d, err := parseSequence(src)
	if err != nil {
		t.Fatalf("parseSequence: %v", err)
	}
	return d
}

func TestRenderDispatchesSequence(t *testing.T) {
	// While parseSequence is a stub this must still be ErrUnsupported;
	// after Task 2 lands the error goes away and the second assertion
	// (still-unsupported kind) keeps guarding dispatch.
	_, err := Render([]byte("gantt\ntitle X"), Light)
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("gantt must stay unsupported, got %v", err)
	}
	out, err := Render([]byte("sequenceDiagram\nA->>B: hi"), Light)
	if err != nil {
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("sequence dispatch must route to sequence pipeline: %v", err)
		}
		t.Skip("sequence pipeline still stubbed (pre-Task 4)")
	}
	if !strings.Contains(string(out), "<svg") {
		t.Fatalf("expected svg, got: %.80s", out)
	}
}
