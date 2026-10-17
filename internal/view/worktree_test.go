package view

import (
	"strings"
	"testing"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/theme"
)

func testWorktrees() []worktreeRow {
	return []worktreeRow{
		{wt: git.Worktree{Path: "/x/repo", Branch: "main", Head: "111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Main: true}, subject: "commit one"},
		{wt: git.Worktree{Path: "/x/repo-topic", Branch: "topic", Head: "222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, subject: "commit two"},
		{wt: git.Worktree{Path: "/x/bare.git", Bare: true}, subject: ""},
	}
}

func feedWorktrees(env *Env, v *WorktreeView, rows []worktreeRow) {
	v.all = rows
	v.refilter(env)
	v.Clamp(len(v.rows))
}

func TestWorktreeViewSetFilter(t *testing.T) {
	env := testEnv(t)
	v := NewWorktree()
	feedWorktrees(env, v, testWorktrees())
	if n := v.Len(); n != 3 {
		t.Fatalf("rows = %d, want 3", n)
	}

	// Fuzzy on the branch.
	v.SetFilter(env, "tpc")
	if n := v.Len(); n != 1 || v.Selection(env).Worktree != "/x/repo-topic" {
		t.Fatalf("branch filter: %d rows %q", n, v.Selection(env).Worktree)
	}
	for _, s := range v.Row(env, 0) {
		_ = s
	}
	// Prefix only on the commit ID.
	v.SetFilter(env, "2222")
	if n := v.Len(); n != 1 {
		t.Fatalf("id filter: %d rows", n)
	}
	// Path matches.
	v.SetFilter(env, "br")
	if n := v.Len(); n != 1 || !strings.Contains(plainRow(env, v, 0), "bare") {
		t.Fatalf("path filter: %d rows %q", n, plainRow(env, v, 0))
	}
	// Fuzzy on hex IDs does not match: prefix only.
	v.SetFilter(env, "aaa")
	if n := v.Len(); n != 0 {
		t.Fatalf("fuzzy hex: %d rows", n)
	}
	v.SetFilter(env, "")
	if n := v.Len(); n != 3 {
		t.Fatalf("rows after clear = %d", n)
	}
	// Space-separated tokens are ANDed.
	v.SetFilter(env, "repo on")
	if n := v.Len(); n != 1 {
		t.Fatalf("AND filter: %d rows", n)
	}
}

func TestWorktreeViewCursorPreserved(t *testing.T) {
	env := testEnv(t)
	v := NewWorktree()
	feedWorktrees(env, v, testWorktrees())
	v.Cursor = 1
	v.SetFilter(env, "tpc")
	if v.Cursor != 0 || v.Selection(env).Worktree != "/x/repo-topic" {
		t.Errorf("after filter: cursor %d selection %q", v.Cursor, v.Selection(env).Worktree)
	}
	v.SetFilter(env, "")
	if v.Cursor != 1 || v.Selection(env).Worktree != "/x/repo-topic" {
		t.Errorf("after clear: cursor %d selection %q", v.Cursor, v.Selection(env).Worktree)
	}
}

func TestWorktreeViewHighlight(t *testing.T) {
	env := testEnv(t)
	v := NewWorktree()
	feedWorktrees(env, v, testWorktrees())
	v.SetFilter(env, "topic")
	row := v.Row(env, 0)
	matched := false
	for _, s := range row {
		if s.Role == theme.SearchMatch {
			matched = true
		}
	}
	if !matched {
		t.Errorf("row %d has no matched segments: %+v", 0, row)
	}
	// The graph markers stay visible; no pseudo rows exist here.
	if got := plainRow(env, v, 0); !strings.Contains(got, "repo-topic") {
		t.Errorf("row = %q", got)
	}
}
