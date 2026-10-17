// Tests of the main view plumbing not covered by the filter tests:
// metadata, Enter/Do actions, ref decorations and column rendering.
package view

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/graph"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// msgOf runs a command and reports its message.
func msgOf(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("command is nil")
	}
	return cmd()
}

func TestMainViewMeta(t *testing.T) {
	env := testEnv(t)
	v := NewMain([]string{"v2.0"}, []string{"docs"})
	if v.Name() != "main" {
		t.Errorf("name = %q", v.Name())
	}
	if got := v.Title(env); got != "[main] v2.0 -- docs" {
		t.Errorf("title = %q", got)
	}
	v = NewMain(nil, nil)
	if got := v.Title(env); got != "[main] HEAD" {
		t.Errorf("title = %q", got)
	}
}

func TestMainViewEnter(t *testing.T) {
	env := testEnv(t)
	v := NewMain(nil, nil)
	if cmd := v.Enter(env); cmd != nil {
		t.Error("Enter on an empty view opens something")
	}
	hash := strings.Repeat("a", 40)
	v.all = []mainRow{
		{special: "unstaged", commit: git.Commit{Subject: "Unstaged changes"}},
		{special: "staged"},
		{commit: git.Commit{Hash: hash, Subject: "one"}},
	}
	v.rows = v.all
	// Nothing is selected below the last row.
	v.Cursor = -1
	if cmd := v.Enter(env); cmd != nil {
		t.Error("Enter without a selected row opens something")
	}
	// Pseudo rows open the worktree diff, commits their own diff.
	enter := []struct {
		cursor   int
		worktree bool
		cached   bool
		commit   string
	}{
		{0, true, false, ""},
		{1, true, true, ""},
		{2, false, false, hash},
	}
	for _, c := range enter {
		v.Cursor = c.cursor
		msg := msgOf(t, v.Enter(env))
		om, ok := msg.(OpenMsg)
		if !ok {
			t.Fatalf("Enter returned %T", msg)
		}
		if !om.Split {
			t.Error("Enter does not open as a split")
		}
		dv, ok := om.View.(*DiffView)
		if !ok {
			t.Fatalf("Enter opens %T", om.View)
		}
		if dv.Worktree != c.worktree || dv.Cached != c.cached || dv.Commit != c.commit {
			t.Errorf("Enter cursor %d = worktree %v cached %v commit %q", c.cursor, dv.Worktree, dv.Cached, dv.Commit)
		}
	}
}

func TestMainViewDo(t *testing.T) {
	env := testEnv(t)
	env.Repo = &git.Repo{Binary: "git"}
	v := NewMain(nil, nil)
	hash := strings.Repeat("a", 40)
	v.all = []mainRow{
		{commit: git.Commit{Hash: hash, Subject: "one"}},
		{special: "unstaged"},
	}
	v.rows = v.all

	// MoveHead moves to HEAD's commit.
	env.Head.Hash = hash
	if cmd, ok := v.Do(env, keymap.MoveHead); cmd != nil || !ok || v.Cursor != 0 {
		t.Errorf("MoveHead = %v %v cursor %d", cmd, ok, v.Cursor)
	}
	env.Head.Hash = strings.Repeat("f", 40)
	cmd, ok := v.Do(env, keymap.MoveHead)
	if !ok {
		t.Fatal("MoveHead is not handled")
	}
	if msg := msgOf(t, cmd); msg != (StatusMsg{Text: "HEAD is not in this view"}) {
		t.Errorf("MoveHead message = %+v", msg)
	}

	// Cherry-pick works on commits only.
	v.Cursor = 1
	if c, ok := v.Do(env, keymap.CherryPick); c != nil || !ok {
		t.Errorf("Cherry-pick on a pseudo row = %v %v", c, ok)
	}
	v.Cursor = 0
	c2, ok2 := v.Do(env, keymap.CherryPick)
	if !ok2 {
		t.Fatal("Cherry-pick is not handled")
	}
	cm, isConfirm := msgOf(t, c2).(ConfirmMsg)
	if !isConfirm || !strings.Contains(cm.Question, "Cherry-pick") || cm.OnYes == nil {
		t.Errorf("Cherry-pick confirm = %+v %v", cm, isConfirm)
	}

	// Toggling the line graphics reloads the view.
	if cmd, ok := v.Do(env, keymap.ToggleLineGraphics); !ok || cmd == nil {
		t.Errorf("ToggleLineGraphics = %v %v", cmd, ok)
	}
}

func TestMainViewRefDecorations(t *testing.T) {
	env := testEnv(t)
	hash := strings.Repeat("2", 40)
	env.Loc.TopLevel = "/work/repo"
	env.Head.Hash = hash
	env.Refs = []git.Ref{
		{Name: "refs/heads/main", Short: "main", Kind: git.RefBranch, Target: hash, IsHead: true},
		{Name: "refs/heads/topic", Short: "topic", Kind: git.RefBranch, Target: hash, WorktreePath: "/work/other"},
		{Name: "refs/remotes/o/main", Short: "o/main", Kind: git.RefRemote, Target: hash},
		{Name: "refs/tags/v1", Short: "v1", Kind: git.RefTag, Target: hash},
		{Name: "refs/stash", Short: "refs/stash", Kind: git.RefStash, Target: hash},
		{Name: "refs/notes/x", Short: "notes/x", Kind: git.RefOther, Target: hash},
	}
	env.Decor = git.Decorations(env.Refs)
	text := refText(env, hash)
	for _, want := range []string{"[HEAD] ", "[main] ", "{wt:other} ", "{o/main} ", "<v1> ", "(stash) ", "(notes/x) "} {
		if !strings.Contains(text, want) {
			t.Errorf("ref decorations %q miss %q", text, want)
		}
	}
	// A named branch is not decorated with [HEAD].
	env.Head.Branch = "main"
	text = refText(env, hash)
	if strings.Contains(text, "[HEAD] ") {
		t.Errorf("named branch carries [HEAD]: %q", text)
	}
	// A branch checked out in the current worktree gets no {wt:} piece.
	env.Loc.TopLevel = "/work/other"
	if text = refText(env, hash); strings.Contains(text, "{wt:") {
		t.Errorf("current worktree branch has a worktree piece: %q", text)
	}
}

func TestMainViewRowColumns(t *testing.T) {
	env := testEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	feedCommits(env, v, testCommits())
	c0 := testCommits()[0]
	env.Head.Hash = c0.Hash
	env.Refs = []git.Ref{{Name: "refs/heads/main", Short: "main", Kind: git.RefBranch, Target: c0.Hash, IsHead: true}}
	env.Decor = git.Decorations(env.Refs)
	env.Opts.Graph = true
	env.Opts.ID = true
	env.Opts.Refs = true
	env.Opts.Date = "relative-compact"
	env.Opts.Author = "full"
	join := func(segs []Seg) string {
		var sb strings.Builder
		for _, s := range segs {
			sb.WriteString(s.Text)
		}
		return sb.String()
	}
	row := join(v.Row(env, 0))
	for _, want := range []string{"[main] ", "Alice", c0.Hash[:env.Cfg.Main.CommitIDLength], "add the parser"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q misses %q", row, want)
		}
	}

	// Author mode off hides the column.
	env.Opts.Author = "off"
	if row = join(v.Row(env, 0)); strings.Contains(row, "Alice") {
		t.Errorf("author off still renders the author: %q", row)
	}
	env.Opts.Author = "full"

	// Title overflow: a subject over the configured limit splits the row
	// into a default part and an overflow part.
	subject := "add the parser"
	env.Opts.TitleOverflow = true
	env.Cfg.UI.TitleOverflow = len([]rune(subject)) + 1
	for i, r := range v.rows {
		if r.commit.Subject == subject {
			v.Cursor = i
			break
		}
	}
	for _, s := range v.Row(env, v.Cursor) {
		if s.Role == theme.Overflow {
			t.Errorf("overflow segment rendered although the subject fits: %+v", v.Row(env, v.Cursor))
			break
		}
	}
	env.Cfg.UI.TitleOverflow = len([]rune(subject)) - 2
	overflow := false
	for _, s := range v.Row(env, v.Cursor) {
		if s.Role == theme.Overflow {
			overflow = true
			if s.Text != string([]rune(subject)[env.Cfg.UI.TitleOverflow:]) {
				t.Errorf("overflow segment = %q", s.Text)
			}
		}
	}
	if !overflow {
		t.Errorf("no overflow segment: %s", join(v.Row(env, v.Cursor)))
	}
	env.Opts.TitleOverflow = false

	// Pseudo rows keep the worktree mark and a plain subject.
	v.SetFilter(env, "")
	v.HandleMsg(env, changesMsg{unstaged: true, staged: true})
	if v.Len() != 5 {
		t.Fatalf("rows = %d, want 5", v.Len())
	}
	mark := join(v.Row(env, 0))
	if !strings.Contains(mark, "●") || !strings.Contains(mark, "Unstaged changes") {
		t.Errorf("pseudo row = %q", mark)
	}
	staged := join(v.Row(env, 1))
	if !strings.Contains(staged, "Staged changes") {
		t.Errorf("staged pseudo row = %q", staged)
	}
}

func TestMainHelpers(t *testing.T) {
	for _, s := range []string{"--topo-order", "--date-order", "--author-date-order", "--reverse"} {
		if !hasOrder([]string{"x", s}) {
			t.Errorf("hasOrder(%q) is false", s)
		}
	}
	if hasOrder(nil) || hasOrder([]string{"--reverse-x"}) {
		t.Error("hasOrder matched a non order flag")
	}
	if got := baseName("/a/b/"); got != "b" {
		t.Errorf("baseName(/a/b/) = %q", got)
	}
	if got := baseName("C:\\x\\file"); got != "file" {
		t.Errorf("baseName = %q", got)
	}
	if got := baseName("plain"); got != "plain" {
		t.Errorf("baseName = %q", got)
	}
}
