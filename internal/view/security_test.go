package view

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
)

// confirmRun executes a command, unwrapping the confirm form when the
// action is confirmation gated.
func confirmRun(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command returned")
	}
	msg := cmd()
	if cm, ok := msg.(ConfirmMsg); ok {
		msg = cm.OnYes()
	}
	return msg
}

// statusOf walks result messages and returns the status texts shown.
func statusOf(t *testing.T, msg tea.Msg) []StatusMsg {
	t.Helper()
	var out []StatusMsg
	var walk func(tea.Msg)
	walk = func(m tea.Msg) {
		switch m := m.(type) {
		case tea.BatchMsg:
			for _, c := range m {
				if c != nil {
					walk(c())
				}
			}
		case StatusMsg:
			out = append(out, m)
		}
	}
	walk(msg)
	return out
}

func assertOkStatus(t *testing.T, msg tea.Msg) {
	t.Helper()
	sts := statusOf(t, msg)
	if len(sts) != 1 || sts[0].Err {
		t.Fatalf("expected a success status, got %+v", sts)
	}
}

func TestRefsViewCheckoutCraftedDashedBranch(t *testing.T) {
	env := headerEnv(t)
	// git branch rejects names starting with "-", but update-ref does
	// not; a branch fetched from a remote can carry such a name.
	gitCmd(t, env.WorktreeDir(), "update-ref", "refs/heads/-c", "HEAD")
	hash := gitCmd(t, env.WorktreeDir(), "rev-parse", "refs/heads/-c")
	if ref, err := git.ListRefs(context.Background(), env.Git()); err != nil {
		t.Fatal(err)
	} else {
		for _, r := range ref {
			if !strings.Contains(r.Name, "-") {
				continue
			}
			v := NewRefs()
			v.refs = []git.Ref{r}
			v.Cursor = 0
			cmd, ok := v.Do(env, keymap.Checkout)
			if !ok {
				t.Fatal("checkout not handled")
			}
			assertOkStatus(t, confirmRun(t, cmd))
			if got := gitCmd(t, env.WorktreeDir(), "rev-parse", "HEAD"); got != hash {
				t.Errorf("HEAD = %s, want %s", got, hash)
			}
			gitCmd(t, env.WorktreeDir(), "checkout", "-q", "main")
		}
	}
}

func TestRefsViewDeleteCraftedDashedBranch(t *testing.T) {
	env := headerEnv(t)
	gitCmd(t, env.WorktreeDir(), "update-ref", "refs/heads/-d", "HEAD")
	ref, err := git.ListRefs(context.Background(), env.Git())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range ref {
		if r.Short != "-d" {
			continue
		}
		found = true
		v := NewRefs()
		v.refs = []git.Ref{r}
		v.Cursor = 0
		cmd, ok := v.Do(env, keymap.BranchDelete)
		if !ok {
			t.Fatal("delete not handled")
		}
		assertOkStatus(t, confirmRun(t, cmd))
	}
	if !found {
		t.Fatal("crafted branch not listed")
	}
	if _, err := env.Git().Run(context.Background(), "rev-parse", "--verify", "-q", "refs/heads/-d"); err == nil {
		t.Error("crafted branch still exists after delete")
	}
}

func TestRefsViewDeleteUsesDoubledash(t *testing.T) {
	env := headerEnv(t)
	gitCmd(t, env.WorktreeDir(), "branch", "victim", "HEAD~1")
	ref, err := git.ListRefs(context.Background(), env.Git())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range ref {
		if r.Short != "victim" {
			continue
		}
		found = true
		v := NewRefs()
		v.refs = []git.Ref{r}
		v.Cursor = 0
		cmd, ok := v.Do(env, keymap.BranchDelete)
		if !ok {
			t.Fatal("delete not handled")
		}
		assertOkStatus(t, confirmRun(t, cmd))
	}
	if !found {
		t.Fatal("branch not listed")
	}
	if _, err := env.Git().Run(context.Background(), "rev-parse", "--verify", "-q", "refs/heads/victim"); err == nil {
		t.Error("branch still exists after delete")
	}
}

func TestReadWorktreeFileSymlinkContainment(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.txt"), []byte("inner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink("ok.txt", filepath.Join(root, "inside")); err != nil {
		t.Fatal(err)
	}
	lines, err := readWorktreeFile(root, "ok.txt")
	if err != nil || len(lines) != 1 || lines[0] != "inner" {
		t.Errorf("plain file: %v, %v", lines, err)
	}
	lines, err = readWorktreeFile(root, "inside")
	if err != nil || len(lines) != 1 || lines[0] != "inner" {
		t.Errorf("symlink to a file inside the worktree must read: %v, %v", lines, err)
	}
	if _, err := readWorktreeFile(root, "escape"); err == nil || !strings.Contains(err.Error(), "outside the worktree") {
		t.Errorf("symlink outside the worktree: err = %v", err)
	}
	// Plain traversal is rejected before the symlink resolution.
	if _, err := readWorktreeFile(root, "../out.txt"); err == nil || !strings.Contains(err.Error(), "outside the worktree") {
		t.Errorf("traversal path: err = %v", err)
	}
}

func TestTreeViewCraftedEntryNamesRejected(t *testing.T) {
	env := testEnv(t)
	v := NewTree("HEAD", "")
	v.entries = []git.TreeEntry{
		{Mode: "040000", Type: "tree", Name: ".."},
		{Mode: "100644", Type: "blob", Name: "a/b"},
	}
	for i, e := range v.entries {
		v.Cursor = i
		dir, history := v.Dir, len(v.history)
		cmd := v.Enter(env)
		if cmd == nil {
			t.Fatalf("entry %q: no error command", e.Name)
		}
		if sm, ok := cmd().(StatusMsg); !ok || !sm.Err || !strings.Contains(sm.Text, "crafted") {
			t.Errorf("entry %q: no crafted-name error: %v", e.Name, cmd())
		}
		if v.Dir != dir || len(v.history) != history {
			t.Errorf("entry %q: navigation state changed: dir=%q history=%d", e.Name, v.Dir, len(v.history))
		}
	}
}

func TestTreeViewLegitimateNavigation(t *testing.T) {
	env := headerEnv(t)
	if err := os.MkdirAll(filepath.Join(env.WorktreeDir(), "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.WorktreeDir(), "sub", "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, env.WorktreeDir(), "add", "sub/f.txt")
	gitCmd(t, env.WorktreeDir(), "commit", "-q", "-m", "sub")

	v := NewTree("HEAD", "")
	v.entries = []git.TreeEntry{{Mode: "040000", Type: "tree", Name: "sub"}}
	v.Cursor = 0
	if cmd := v.Enter(env); cmd == nil {
		t.Fatal("tree navigation returned no command")
	} else if v.Dir != "sub" {
		t.Fatalf("dir = %q, want sub", v.Dir)
	}

	v2 := NewTree("HEAD", "sub")
	v2.entries = []git.TreeEntry{{Mode: "100644", Type: "blob", Name: "f.txt"}}
	v2.Cursor = 0
	if cmd := v2.Enter(env); cmd == nil {
		t.Fatal("blob navigation returned no command")
	}
}
