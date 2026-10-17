package view

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// headerEnv creates a repository with two commits: "one" (branch topic)
// and "two" (branch main = HEAD).
func headerEnv(t *testing.T) *Env {
	t.Helper()
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "main")
	gitCmd(t, dir, "config", "user.name", "T")
	gitCmd(t, dir, "config", "user.email", "t@example.com")
	gitCmd(t, dir, "config", "commit.gpgsign", "false")
	for i, c := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Repeat(c+"\n", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		gitCmd(t, dir, "add", "f.txt")
		gitCmd(t, dir, "commit", "-q", "-m", "commit "+c)
	}
	gitCmd(t, dir, "branch", "topic", "HEAD~1")
	repo, loc, err := git.Open(context.Background(), "git", dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	th, err := theme.Load("", "default")
	if err != nil {
		t.Fatal(err)
	}
	env := &Env{Repo: repo, Loc: loc, Cfg: cfg, Theme: th, Keys: keymap.Default(), Opts: NewOptions(cfg)}
	if err := env.ReloadRefs(context.Background()); err != nil {
		t.Fatal(err)
	}
	return env
}

// plainRow joins the text of a row's segments.
func plainRow(env *Env, v View, i int) string {
	var sb strings.Builder
	for _, s := range v.Row(env, i) {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// loadDiff loads a commit diff synchronously.
func loadDiff(t *testing.T, env *Env, commit string) *DiffView {
	t.Helper()
	v := NewDiff(commit, nil)
	if cmd := v.Load(env); cmd != nil {
		v.HandleMsg(env, cmd())
	}
	if v.Err != nil {
		t.Fatalf("load: %v", v.Err)
	}
	return v
}

// headerLabels is the field order required from the diff header.
var headerLabels = []string{"commit", "Refs", "Author", "AuthorDate", "Commit", "CommitDate"}

func labelRow(field string) string {
	return field + ":" + strings.Repeat(" ", headerLabelWidth-len(field)-1)
}

func TestDiffCommitHeader(t *testing.T) {
	env := headerEnv(t)
	v := loadDiff(t, env, env.Head.Hash)
	if v.Len() == 0 {
		t.Fatal("no lines")
	}
	// Field order: commit, Refs, Author, AuthorDate, Commit, CommitDate.
	for i, field := range headerLabels {
		got := plainRow(env, v, i)
		if !strings.HasPrefix(got, labelRow(field)) {
			t.Errorf("row %d = %q, want prefix %q", i, got, labelRow(field))
		}
	}
	// Every label is dimmed (line-number), every value keeps its field role.
	valueRoles := map[int]theme.Role{
		0: theme.CommitID,
		2: theme.Author,
		3: theme.Date,
		4: theme.Author,
		5: theme.Date,
	}
	// Labels use the dim variant of their field's value color; values
	// keep the value role.
	labelRoles := map[int]theme.Role{
		0: theme.CommitIDDim,
		1: theme.RefDim,
		2: theme.AuthorDim,
		3: theme.DateDim,
		4: theme.AuthorDim,
		5: theme.DateDim,
	}
	for i, role := range labelRoles {
		if s := v.Row(env, i)[0]; s.Role != role || s.Text != labelRow(headerLabels[i]) {
			t.Errorf("row %d label = %v %q, want %v", i, s.Role, s.Text, role)
		}
	}
	for i, role := range valueRoles {
		segs := v.Row(env, i)
		if len(segs) < 2 || segs[1].Role != role {
			t.Errorf("row %d value = %+v, want role %v", i, segs[1], role)
		}
	}
	// The commit row shows the full commit ID.
	if s := v.Row(env, 0)[1]; s.Text != env.Head.Hash {
		t.Errorf("commit value = %q, want %q", s.Text, env.Head.Hash)
	}
	// Refs come from the live decorations like in the main view; the
	// branch main = HEAD is drawn with the ref-head role.
	segs := v.Row(env, 1)
	if len(segs) < 2 {
		t.Fatalf("refs row = %+v", segs)
	}
	if s := segs[1]; s.Role != theme.RefHead || s.Text != "[main]" {
		t.Errorf("refs value = %v %q", s.Role, s.Text)
	}
	// The message and the patch below are unchanged.
	if !strings.Contains(plainRow(env, v, 7), "commit two") {
		t.Errorf("message row 7 = %q", plainRow(env, v, 7))
	}
	found := false
	for i := 0; i < v.Len(); i++ {
		if strings.HasPrefix(plainRow(env, v, i), "diff --git") {
			found = true
		}
	}
	if !found {
		t.Error("no diff --git in the body")
	}
}

func TestDiffCommitHeaderMerge(t *testing.T) {
	env := headerEnv(t)
	// Create a merge commit on top.
	gitCmd(t, env.WorktreeDir(), "checkout", "-q", "-b", "side", "HEAD~1")
	if err := os.WriteFile(filepath.Join(env.WorktreeDir(), "g.txt"), []byte("side\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, env.WorktreeDir(), "add", "g.txt")
	gitCmd(t, env.WorktreeDir(), "commit", "-q", "-m", "side")
	gitCmd(t, env.WorktreeDir(), "checkout", "-q", "main")
	gitCmd(t, env.WorktreeDir(), "merge", "-q", "--no-ff", "-m", "merge side", "side")
	sha := strings.TrimSpace(gitCmd(t, env.WorktreeDir(), "rev-parse", "HEAD"))
	v := loadDiff(t, env, sha)
	// The Merge field keeps its place between commit+Refs and Author.
	for i, field := range []string{"commit", "Refs", "Merge", "Author", "AuthorDate", "Commit", "CommitDate"} {
		if got := plainRow(env, v, i); !strings.HasPrefix(got, labelRow(field)) {
			t.Errorf("row %d = %q, want prefix %q", i, got, labelRow(field))
		}
	}
	if segs := v.Row(env, 2); len(segs) < 2 || segs[1].Role != theme.CommitID {
		t.Errorf("merge row = %+v", v.Row(env, 2))
	}
}

func TestLogViewStreamedIngest(t *testing.T) {
	env := testEnv(t)
	v := NewLog(nil, nil)
	_, gen := v.startLoad()

	// A synthetic wait chain: chunks hand off to wait, like the real
	// load's continuation commands do.
	wait := func() tea.Msg { return textChunkMsg{gen: gen, done: true} }

	chunk1 := []string{
		"commit aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"Author:     Alice <alice@example.com>",
		"    first commit",
		"",
		" file.go | 3 +++",
		" 3 files changed, 5 insertions(+) (bogus)",
	}
	// Deliver the first chunk and render: the lines are visible already.
	if cmd := v.HandleMsg(env, textChunkMsg{gen: gen, lines: chunk1, next: wait}); cmd == nil {
		t.Fatal("a live chunk must return the wait command")
	}
	if n := v.Len(); n != len(chunk1) {
		t.Fatalf("after chunk 1: len = %d, want %d", n, len(chunk1))
	}
	out := Render(env, v, 60, 8)
	if !strings.Contains(out, "first commit") {
		t.Errorf("chunk 1 not rendered: %q", out)
	}
	if s := v.Row(env, 0)[0]; s.Role != theme.CommitID {
		t.Errorf("commit line role = %v, want commit-id", s.Role)
	}
	// The diffstat line keeps its whole-buffer coloring.
	stat := v.Row(env, 4)
	if len(stat) < 2 || stat[1].Role != theme.StatAdd {
		t.Errorf("stat segs = %+v", stat)
	}

	// A second chunk appends without disturbing the first lines.
	chunk2 := []string{
		"commit bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"Author:     Bob <bob@example.com>",
		"    second commit",
		"+added line\\nnot really",
	}
	if cmd := v.HandleMsg(env, textChunkMsg{gen: gen, lines: chunk2, next: wait}); cmd == nil {
		t.Fatal("a live chunk must return the wait command")
	}
	if n := v.Len(); n != len(chunk1)+len(chunk2) {
		t.Fatalf("after chunk 2: len = %d", n)
	}
	if got := v.lines[len(chunk1)]; got != "commit bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("chunk 2 line = %q", got)
	}

	// Chunks from a superseded load are dropped.
	_, gen2 := v.startLoad()
	if cmd := v.HandleMsg(env, textChunkMsg{gen: gen, lines: []string{"stale"}, done: true}); cmd != nil {
		t.Fatal("stale chunk returned a command")
	}
	if n := v.Len(); n != len(chunk1)+len(chunk2) {
		t.Fatalf("stale chunk applied: len = %d", n)
	}

	// The current load finishes: the loading flag clears.
	if cmd := v.HandleMsg(env, textChunkMsg{gen: gen2, lines: []string{"commit cccccccccccccccccccccccccccccccccccccccc"}, done: true}); cmd != nil {
		t.Fatal("done chunk returned a command")
	}
	if v.Loading {
		t.Error("still loading after the done chunk")
	}
	if n := v.Len(); n != 1 {
		t.Fatalf("new generation did not replace old content: len = %d", n)
	}

	// An error on the done chunk keeps what streamed and is reported.
	_, gen3 := v.startLoad()
	v.HandleMsg(env, textChunkMsg{gen: gen3, lines: chunk1})
	v.HandleMsg(env, textChunkMsg{gen: gen3, done: true, err: errors.New("git failed")})
	if v.Err == nil {
		t.Error("the load error was not reported")
	}
	if v.Loading {
		t.Error("still loading after the failed done chunk")
	}
	if n := v.Len(); n != len(chunk1) {
		t.Errorf("partial content lost: len = %d, want %d", n, len(chunk1))
	}

	// An error with nothing streamed replaces the old content.
	v2 := NewLog(nil, nil)
	_, gen4 := v2.startLoad()
	v2.handleChunk(env, textChunkMsg{gen: gen4, lines: chunk1}, classifyDiff)
	_, gen5 := v2.startLoad()
	v2.handleChunk(env, textChunkMsg{gen: gen5, done: true, err: errors.New("boom")}, classifyDiff)
	if v2.Len() != 0 {
		t.Errorf("content not replaced on an early error: len = %d", v2.Len())
	}
	if v2.Err == nil || v2.Err.Error() != "boom" {
		t.Errorf("err = %v, want boom", v2.Err)
	}
}

func TestLogViewStreamsFromGit(t *testing.T) {
	env := headerEnv(t)
	v := NewLog(nil, nil)
	args := []string{"log", "--no-color", "--stat", "--decorate=short", "--format=medium"}
	cmd := v.streamText(env, 3, classifyDiff, args...)
	for i := 0; cmd != nil && i < 1000; i++ {
		cmd = v.HandleMsg(env, cmd())
	}
	if v.Err != nil {
		t.Fatalf("load error: %v", v.Err)
	}
	want := gitCmd(t, env.WorktreeDir(), "log", "--no-color", "--stat", "--decorate=short", "--format=medium")
	wantLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	if n := v.Len(); n != len(wantLines) {
		t.Fatalf("streamed %d lines, want %d", n, len(wantLines))
	}
	for i, w := range wantLines {
		if got := v.lines[i]; got != w {
			t.Fatalf("line %d = %q, want %q", i, got, w)
		}
	}
	// Enter on the first commit line walks up to "commit <sha>".
	v.SetCursor(0, v.Len())
	c := v.commitAt(v.Cursor)
	if c == "" {
		t.Fatal("commitAt found no hash")
	}
	if c != env.Head.Hash {
		t.Errorf("commitAt = %q, want %q", c, env.Head.Hash)
	}
}
