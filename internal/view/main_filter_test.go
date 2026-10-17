package view

import (
	"strings"
	"testing"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/graph"
	"github.com/jedipunkz/knot/internal/theme"
)

func filterEnv(t *testing.T) *Env {
	t.Helper()
	return testEnv(t)
}

func testCommits() []git.Commit {
	return []git.Commit{
		{Hash: "111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorName: "Alice", Subject: "add the parser"},
		{Hash: "222222bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", AuthorName: "Bob", Subject: "fix the parser"},
		{Hash: "333333cccccccccccccccccccccccccccccccccc", AuthorName: "Alice", Subject: "refactor: cleanup"},
	}
}

// feedCommits simulates the arrival of a loaded batch, like HandleMsg does.
func feedCommits(env *Env, v *MainView, cs []git.Commit) {
	start := len(v.all)
	batch := make([]mainRow, 0, len(cs))
	for _, c := range cs {
		batch = append(batch, mainRow{commit: c, graph: v.builder.Add(c.Hash, nil)})
	}
	v.all = append(v.all, batch...)
	if v.query != "" {
		v.rows = append(v.rows, v.filterRows(env, v.all[start:])...)
	} else {
		v.rows = v.all
	}
}

func hasMatched(segs []Seg) bool {
	for _, s := range segs {
		if s.Role == theme.SearchMatch {
			return true
		}
	}
	return false
}

func TestMainViewSetFilter(t *testing.T) {
	env := filterEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	feedCommits(env, v, testCommits())

	// Fuzzy on the subject.
	v.SetFilter(env, "fx par")
	if n := v.Len(); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	if s := v.Selection(env).Commit; s != v.all[1].commit.Hash {
		t.Errorf("selection = %q, want the fix commit", s)
	}
	// Order is the original log order; "alice" matches the author of the
	// first and third commit.
	v.SetFilter(env, "alice")
	if n := v.Len(); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
	if got := Plain(env, v, 0); !strings.Contains(got, "add the parser") {
		t.Errorf("row 0 = %q", got)
	}
	if got := Plain(env, v, 1); !strings.Contains(got, "refactor") {
		t.Errorf("row 1 = %q", got)
	}

	// Commit ID prefix matches even without the ID column.
	v.SetFilter(env, "3333")
	if n := v.Len(); n != 1 || v.Selection(env).Commit != v.all[2].commit.Hash {
		t.Fatalf("prefix match failed: rows %d", v.Len())
	}
	// Fuzzy on hex IDs must not match: prefix only.
	v.SetFilter(env, "ccc")
	if n := v.Len(); n != 0 {
		t.Errorf("fuzzy hex matched: rows = %d, want 0", n)
	}
	v.SetFilter(env, "")

	// Space-separated tokens are ANDed.
	v.SetFilter(env, "add parser")
	if n := v.Len(); n != 1 {
		t.Fatalf("AND tokens rows = %d, want 1", n)
	}
	v.SetFilter(env, "")

	// Author matches.
	v.SetFilter(env, "bob")
	if n := v.Len(); n != 1 || v.Selection(env).Commit != v.all[1].commit.Hash {
		t.Fatalf("author match failed: rows = %d", n)
	}

	// Ref names match: the branch main points at the first commit.
	env.Refs = []git.Ref{{Name: "refs/heads/topic", Short: "topic", Kind: git.RefBranch, Target: v.all[0].commit.Hash}}
	env.Decor = git.Decorations(env.Refs)
	v.SetFilter(env, "topic")
	if n := v.Len(); n != 1 || v.Selection(env).Commit == "" {
		t.Fatalf("ref match rows = %d, want 1", n)
	}
	v.SetFilter(env, "zzz")
	if n := v.Len(); n != 0 {
		t.Fatalf("non-match rows = %d, want 0", n)
	}
	if got := Render(env, v, 40, 3); !strings.Contains(got, "(empty)") {
		t.Errorf("render of empty result = %q", got)
	}
}

func TestMainViewSetFilterCursor(t *testing.T) {
	env := filterEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	feedCommits(env, v, testCommits())
	// Cursor on the last commit.
	v.Cursor = 2
	// Narrowing keeps the selected commit visible.
	v.SetFilter(env, "alice")
	if n := v.Len(); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
	if v.Cursor != 1 {
		t.Errorf("cursor = %d, want 1 (second Alice)", v.Cursor)
	}
	// Clearing the filter keeps the selected commit.
	v.SetFilter(env, "")
	if v.Cursor != 2 || v.Selection(env).Commit != v.all[2].commit.Hash {
		t.Errorf("after clear: cursor %d selection %q", v.Cursor, v.Selection(env).Commit)
	}
}

func TestMainViewSetFilterHighlight(t *testing.T) {
	env := filterEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	feedCommits(env, v, testCommits())
	v.SetFilter(env, "fix")
	row := v.Row(env, 0)
	if !hasMatched(row) {
		t.Errorf("row has no matched segments: %+v", row)
	}
	// The graph column is hidden while the filter is active.
	for _, s := range row {
		if s.Text == "●" || s.Text == "*" {
			t.Errorf("graph cell %q shown while filtered", s.Text)
		}
	}
}

func TestMainViewPseudoRowsHidden(t *testing.T) {
	env := filterEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	feedCommits(env, v, testCommits())
	v.HandleMsg(env, changesMsg{unstaged: true, staged: true})
	if n := v.Len(); n != 5 {
		t.Fatalf("rows = %d, want 5 (2 pseudo + 3 commits)", n)
	}
	v.SetFilter(env, "parser")
	if n := v.Len(); n != 2 {
		t.Fatalf("filtered rows = %d, want 2 (pseudo hidden)", n)
	}
	v.SetFilter(env, "")
	if n := v.Len(); n != 5 {
		t.Fatalf("rows after clear = %d, want 5", n)
	}
}

func TestMainViewSetFilterBatchIncremental(t *testing.T) {
	env := filterEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	v.SetFilter(env, "parser")
	feedCommits(env, v, testCommits()[:2])
	if n := v.Len(); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
	later := []git.Commit{{Hash: "444444dddddddddddddddddddddddddddddddddd", AuthorName: "Cid", Subject: "rewrite the parser"}}
	feedCommits(env, v, later)
	if n := v.Len(); n != 3 {
		t.Fatalf("rows = %d, want 3 (batch filtered on arrival)", n)
	}
}
