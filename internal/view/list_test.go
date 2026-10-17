package view

import (
	"strings"
	"testing"
)

func TestPendingLineAppliedAfterLoad(t *testing.T) {
	env := testEnv(t)
	p := NewPager("t", nil) // the initial view has no rows yet
	p.B().PendingLine = 10

	// The first render (WindowSizeMsg) sees an empty view; the cursor is
	// clamped to 0 but the pending line is kept for later.
	Render(env, p, 40, 10)
	if p.Cursor != 0 {
		t.Fatalf("empty render cursor = %d, want 0", p.Cursor)
	}
	if p.B().PendingLine != 10 {
		t.Fatalf("empty render pending line = %d, want 10", p.B().PendingLine)
	}

	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, "row "+strings.Repeat("x", i))
	}
	p.Content = lines
	p.Load(env) // the load result arrives

	out := Render(env, p, 40, 10)
	if p.B().PendingLine != 0 {
		t.Fatalf("pending line %d not cleared", p.B().PendingLine)
	}
	if p.Cursor != 9 {
		t.Fatalf("cursor = %d, want 9 (line 10)", p.Cursor)
	}
	if p.Offset != 6 {
		t.Fatalf("offset = %d, want 6", p.Offset)
	}
	if !strings.Contains(out, "row xxxxxxxxx") {
		t.Error("line 10 is not visible in the render")
	}

	// Later renders must not reposition the cursor again.
	p.Content[9] = "row two"
	p.Load(env)
	out = Render(env, p, 40, 10)
	if p.Cursor != 9 || p.Offset != 6 {
		t.Fatalf("second render cursor/offset = %d/%d, want 9/6", p.Cursor, p.Offset)
	}
	if !strings.Contains(out, "row two") {
		t.Error("the second render does not show the updated row")
	}
}

func TestPendingLineClampedToContent(t *testing.T) {
	env := testEnv(t)
	p := NewPager("t", []string{"alpha", "beta", "gamma", "delta"})
	p.B().PendingLine = 100 // beyond the content
	p.Load(env)
	Render(env, p, 40, 10)
	if p.Cursor != 3 {
		t.Fatalf("cursor = %d, want 3 (last row)", p.Cursor)
	}
	if p.B().PendingLine != 0 {
		t.Fatalf("pending line = %d, want 0", p.B().PendingLine)
	}
}
