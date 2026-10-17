package view

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
	for i, role := range valueRoles {
		segs := v.Row(env, i)
		if segs[0].Role != theme.LineNumber || segs[0].Text != labelRow(headerLabels[i]) {
			t.Errorf("row %d label = %v %q", i, segs[0].Role, segs[0].Text)
		}
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
