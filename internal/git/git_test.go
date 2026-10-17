package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type testRepo struct {
	t   *testing.T
	dir string
	r   Runner
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	dir := t.TempDir()
	tr := &testRepo{t: t, dir: dir, r: Runner{Dir: dir}}
	tr.git("init", "-q", "-b", "main")
	tr.git("config", "user.name", "Knot Tester")
	tr.git("config", "user.email", "knot@example.com")
	tr.git("config", "commit.gpgsign", "false")
	return tr
}

func (tr *testRepo) git(args ...string) string {
	tr.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", tr.dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2024-01-02T03:04:05Z", "GIT_COMMITTER_DATE=2024-01-02T03:04:05Z")
	out, err := cmd.CombinedOutput()
	if err != nil {
		tr.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func (tr *testRepo) write(name, content string) {
	tr.t.Helper()
	p := filepath.Join(tr.dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		tr.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		tr.t.Fatal(err)
	}
}

func (tr *testRepo) commit(name, content, msg string) string {
	tr.t.Helper()
	tr.write(name, content)
	tr.git("add", name)
	tr.git("commit", "-q", "-m", msg)
	return strings.TrimSpace(tr.git("rev-parse", "HEAD"))
}

func TestOpenAndHead(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("a.txt", "a\n", "first")
	repo, loc, err := Open(context.Background(), "", filepath.Join(tr.dir))
	if err != nil {
		t.Fatal(err)
	}
	if repo.Bare {
		t.Error("expected non-bare repo")
	}
	want, _ := filepath.EvalSymlinks(tr.dir)
	got, _ := filepath.EvalSymlinks(loc.TopLevel)
	if got != want {
		t.Errorf("toplevel = %q, want %q", got, want)
	}
	h := ReadHead(context.Background(), repo.Runner(loc))
	if h.Branch != "main" || len(h.Hash) != 40 {
		t.Errorf("head = %+v", h)
	}
}

func TestStreamLog(t *testing.T) {
	tr := newTestRepo(t)
	first := tr.commit("a.txt", "a\n", "first")
	second := tr.commit("a.txt", "b\n", "second subject")
	var all []Commit
	err := StreamLog(context.Background(), tr.r, LogArgs(nil, nil, nil), 1, func(cs []Commit) error {
		all = append(all, cs...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d commits", len(all))
	}
	if all[0].Hash != second || all[0].Subject != "second subject" || all[0].Parents[0] != first {
		t.Errorf("unexpected first commit %+v", all[0])
	}
	if all[1].Parents != nil {
		t.Errorf("root commit has parents %v", all[1].Parents)
	}
	if all[0].AuthorName != "Knot Tester" || all[0].AuthorDate.UTC().Year() != 2024 {
		t.Errorf("author fields: %+v", all[0])
	}
}

func TestRefsAndWorktrees(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("a.txt", "a\n", "first")
	tr.git("tag", "-a", "v1", "-m", "tag v1")
	tr.git("branch", "feature")
	wt := filepath.Join(t.TempDir(), "wt-feature")
	tr.git("worktree", "add", "-q", wt, "feature")
	tr.git("worktree", "lock", "--reason", "busy", wt)
	gone := filepath.Join(t.TempDir(), "wt-gone")
	tr.git("worktree", "add", "-q", "--detach", gone)
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	refs, err := ListRefs(context.Background(), tr.r)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Ref{}
	for _, r := range refs {
		byName[r.Short] = r
	}
	if !byName["main"].IsHead || byName["main"].Kind != RefBranch {
		t.Errorf("main: %+v", byName["main"])
	}
	if byName["feature"].WorktreePath == "" {
		t.Errorf("feature should record its worktree: %+v", byName["feature"])
	}
	if tag := byName["v1"]; tag.Kind != RefTag || tag.Target != byName["main"].Hash || tag.Hash == tag.Target {
		t.Errorf("annotated tag must be peeled: %+v", tag)
	}

	wts, err := ListWorktrees(context.Background(), tr.r)
	if err != nil {
		t.Fatal(err)
	}
	if len(wts) != 3 {
		t.Fatalf("got %d worktrees: %+v", len(wts), wts)
	}
	if !wts[0].Main || wts[0].Branch != "main" {
		t.Errorf("main worktree: %+v", wts[0])
	}
	if wts[1].Branch != "feature" || !wts[1].Locked || wts[1].LockReason != "busy" {
		t.Errorf("feature worktree: %+v", wts[1])
	}
	if !wts[2].Detached || !wts[2].Prunable {
		t.Errorf("gone worktree: %+v", wts[2])
	}

	out, err := PruneWorktrees(context.Background(), tr.r, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = out
	if _, err := UnlockWorktree(context.Background(), tr.r, wt); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveWorktree(context.Background(), tr.r, wt, false); err != nil {
		t.Fatal(err)
	}
	wtNew := filepath.Join(t.TempDir(), "wt-new")
	if _, err := AddWorktree(context.Background(), tr.r, wtNew, "topic", true); err != nil {
		t.Fatal(err)
	}
	if !RefExists(context.Background(), tr.r, "refs/heads/topic") {
		t.Error("new branch not created")
	}
}

func TestStatus(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("a.txt", "a\n", "first")
	tr.commit("old.txt", "old\n", "second")
	tr.write("a.txt", "changed\n")
	tr.write("new file.txt", "new\n")
	tr.git("mv", "old.txt", "renamed.txt")
	tr.write("staged.txt", "s\n")
	tr.git("add", "staged.txt")

	st, err := ReadStatus(context.Background(), tr.r)
	if err != nil {
		t.Fatal(err)
	}
	if st.Branch != "main" {
		t.Errorf("branch = %q", st.Branch)
	}
	got := map[string]FileStatus{}
	for _, f := range st.Files {
		got[f.Path] = f
	}
	if f := got["a.txt"]; !f.Unstaged() || f.Staged() {
		t.Errorf("a.txt: %+v", f)
	}
	if f := got["renamed.txt"]; f.Kind != FileRenamed || f.OrigPath != "old.txt" || !f.Staged() {
		t.Errorf("renamed.txt: %+v", f)
	}
	if f := got["new file.txt"]; f.Kind != FileUntracked {
		t.Errorf("untracked: %+v", f)
	}
	if f := got["staged.txt"]; !f.Staged() || f.X != 'A' {
		t.Errorf("staged.txt: %+v", f)
	}
	dirty, err := IsDirty(context.Background(), tr.r)
	if err != nil || !dirty {
		t.Errorf("IsDirty = %v, %v", dirty, err)
	}
}

func TestStashReflogTreeGrepBlame(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("dir/a.txt", "hello\nworld\n", "first")
	h2 := tr.commit("dir/a.txt", "hello\nthere\n", "second")
	tr.write("dir/a.txt", "stashed\n")
	tr.git("stash", "push", "-q", "-m", "wip stuff")
	ctx := context.Background()

	stashes, err := ListStashes(ctx, tr.r)
	if err != nil || len(stashes) != 1 || stashes[0].Ref != "stash@{0}" || !strings.Contains(stashes[0].Subject, "wip stuff") {
		t.Errorf("stashes = %+v, %v", stashes, err)
	}
	rl, err := ListReflog(ctx, tr.r, nil)
	if err != nil || len(rl) < 2 || rl[0].Hash != h2 {
		t.Errorf("reflog = %+v, %v", rl, err)
	}
	root, err := ListTree(ctx, tr.r, "HEAD", "")
	if err != nil || len(root) != 1 || root[0].Type != "tree" || root[0].Name != "dir" {
		t.Errorf("root tree = %+v, %v", root, err)
	}
	sub, err := ListTree(ctx, tr.r, "HEAD", "dir")
	if err != nil || len(sub) != 1 || sub[0].Name != "a.txt" {
		t.Errorf("sub tree = %+v, %v", sub, err)
	}
	m, err := Grep(ctx, tr.r, []string{"-e", "there"})
	if err != nil || len(m) != 1 || m[0].File != "dir/a.txt" || m[0].Line != 2 {
		t.Errorf("grep = %+v, %v", m, err)
	}
	none, err := Grep(ctx, tr.r, []string{"-e", "nomatch-xyz"})
	if err != nil || len(none) != 0 {
		t.Errorf("grep no match = %+v, %v", none, err)
	}
	bl, err := Blame(ctx, tr.r, "HEAD", "dir/a.txt")
	if err != nil || len(bl) != 2 {
		t.Fatalf("blame = %+v, %v", bl, err)
	}
	if bl[1].Commit.Hash != h2 || bl[1].Text != "there" || bl[1].Commit.Summary != "second" {
		t.Errorf("blame line 2 = %+v %+v", bl[1], bl[1].Commit)
	}
	if bl[0].Commit.Hash == h2 || bl[0].Filename != "dir/a.txt" {
		t.Errorf("blame line 1 = %+v", bl[0])
	}
}

func TestLineAndHunkStaging(t *testing.T) {
	tr := newTestRepo(t)
	tr.commit("f.txt", "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n", "first")
	tr.write("f.txt", "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n10\nELEVEN\n12\n13\n")
	ctx := context.Background()

	text, err := DiffFile(ctx, tr.r, "f.txt", false, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	fd := ParseFileDiff(text)
	if len(fd.Hunks) != 2 {
		t.Fatalf("expected 2 hunks, got %d:\n%s", len(fd.Hunks), text)
	}
	// Stage only the "+TWO" line.
	plus := -1
	for i, l := range fd.Lines {
		if l == "+TWO" {
			plus = i
		}
	}
	patch, err := fd.LinePatch(plus, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyCached(ctx, tr.r, patch, false); err != nil {
		t.Fatalf("apply line patch: %v\n%s", err, patch)
	}
	cached := tr.git("diff", "--cached", "-U0", "--", "f.txt")
	if !strings.Contains(cached, "+TWO") || strings.Contains(cached, "-2\n") {
		t.Errorf("after line staging, cached diff:\n%s", cached)
	}

	// Stage the second hunk completely.
	text, _ = DiffFile(ctx, tr.r, "f.txt", false, 1, false)
	fd = ParseFileDiff(text)
	last := len(fd.Hunks) - 1
	if err := ApplyCached(ctx, tr.r, fd.HunkPatch(last), false); err != nil {
		t.Fatal(err)
	}
	cached = tr.git("diff", "--cached", "--", "f.txt")
	if !strings.Contains(cached, "+ELEVEN") || !strings.Contains(cached, "+13") {
		t.Errorf("after hunk staging, cached diff:\n%s", cached)
	}

	// Unstage the "+ELEVEN" line again (reverse line patch).
	text, _ = DiffFile(ctx, tr.r, "f.txt", true, 1, false)
	fd = ParseFileDiff(text)
	idx := -1
	for i, l := range fd.Lines {
		if l == "+ELEVEN" {
			idx = i
		}
	}
	patch, err = fd.LinePatch(idx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyCached(ctx, tr.r, patch, true); err != nil {
		t.Fatalf("reverse apply: %v\n%s", err, patch)
	}
	cached = tr.git("diff", "--cached", "--", "f.txt")
	if strings.Contains(cached, "+ELEVEN") || !strings.Contains(cached, "+13") || !strings.Contains(cached, "+TWO") {
		t.Errorf("after line unstaging, cached diff:\n%s", cached)
	}

	if err := UnstageFile(ctx, tr.r, "f.txt"); err != nil {
		t.Fatal(err)
	}
	if out := tr.git("diff", "--cached"); out != "" {
		t.Errorf("expected empty index diff, got:\n%s", out)
	}
	if err := DiscardFile(ctx, tr.r, "f.txt"); err != nil {
		t.Fatal(err)
	}
	if out := tr.git("status", "--porcelain"); out != "" {
		t.Errorf("expected clean tree, got %q", out)
	}
}

func TestDeleteUntrackedRejectsEscape(t *testing.T) {
	dir := t.TempDir()
	if err := DeleteUntracked(dir, "../outside"); err == nil {
		t.Error("expected error for path escaping the worktree")
	}
}

func TestParseWorktreesBare(t *testing.T) {
	out := "worktree /r.git\x00bare\x00\x00worktree /w\x00HEAD abc\x00branch refs/heads/x\x00\x00"
	wts := ParseWorktrees([]byte(out))
	if len(wts) != 2 || !wts[0].Bare || !wts[0].Main || wts[1].Branch != "x" {
		t.Errorf("parsed %+v", wts)
	}
}
