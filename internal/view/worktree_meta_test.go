// Tests of the worktree view plumbing not covered by the filter tests:
// metadata, the key-bar info row and the Enter/switch guards.
package view

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/jedipunkz/knot/internal/git"
)

func TestWorktreeViewMeta(t *testing.T) {
	env := testEnv(t)
	env.Repo = &git.Repo{Binary: "git", CommonDir: "/repo/.git"}
	v := NewWorktree()
	if v.Name() != "worktree" {
		t.Errorf("name = %q", v.Name())
	}
	if got := v.Title(env); got != "[worktree] /repo/.git" {
		t.Errorf("title = %q", got)
	}
	if !v.FilterOnOpen() {
		t.Error("the worktree view does not focus the filter bar on open")
	}
	if got := v.Info(env); got != nil {
		t.Errorf("info without rows = %+v", got)
	}
	feedWorktrees(env, v, testWorktrees())
	if got := v.Info(env); got == nil {
		t.Error("no info row although a worktree is selected")
	}
}

func TestWorktreeViewInfoRow(t *testing.T) {
	env := testEnv(t)
	v := NewWorktree()
	rows := testWorktrees()
	rows[0].dirty = 1
	feedWorktrees(env, v, rows)
	join := func(segs []Seg) string {
		var sb strings.Builder
		for _, s := range segs {
			sb.WriteString(s.Text)
		}
		return sb.String()
	}
	// The main worktree is marked @, the bare one *; the current
	// worktree path, branch and flags appear.
	if got := join(v.Info(env)); !strings.Contains(got, "@ ") || !strings.Contains(got, "/x/repo") || !strings.Contains(got, "dirty") || !strings.Contains(got, "111111a") {
		t.Errorf("info row = %q", got)
	}
	v.Cursor = 1
	if got := join(v.Info(env)); !strings.Contains(got, "commit two") {
		t.Errorf("info row = %q", got)
	}
	// The bare worktree renders the (bare) branch placeholder.
	v.Cursor = 2
	if got := join(v.Info(env)); !strings.Contains(got, "(bare)") || !strings.Contains(got, "* ") {
		t.Errorf("bare info row = %q", got)
	}
}

func TestWorktreeViewEnter(t *testing.T) {
	env := testEnv(t)
	env.Repo = &git.Repo{Binary: "git"}
	v := NewWorktree()
	// No selection: nothing happens.
	if cmd := v.Enter(env); cmd != nil {
		t.Error("Enter on an empty view switches")
	}
	dir := t.TempDir()
	cases := []struct {
		name   string
		row    worktreeRow
		status string // expected error text; empty = a switch command
		want   string // SwitchWorktreeMsg path on success
	}{
		{
			name:   "prunable",
			row:    worktreeRow{wt: git.Worktree{Path: "/gone", Branch: "pk", Head: "aaaa", Prunable: true}, subject: "s"},
			status: "prunable",
		},
		{
			name:   "bare",
			row:    worktreeRow{wt: git.Worktree{Path: "/b.git", Bare: true}, subject: "s"},
			status: "bare; cannot switch",
		},
		{
			name:   "missing",
			row:    worktreeRow{wt: git.Worktree{Path: filepath.Join(dir, "missing"), Branch: "m", Head: "aaaa"}, subject: "s"},
			status: "worktree",
		},
		{
			name: "valid",
			row:  worktreeRow{wt: git.Worktree{Path: dir, Branch: "b", Head: "aaaa"}, subject: "s"},
			want: dir,
		},
	}
	for _, c := range cases {
		v.all = []worktreeRow{c.row}
		v.refilter(env)
		v.Cursor = 0
		cmd := v.Enter(env)
		if c.status == "" {
			msg, ok := msgOf(t, cmd).(SwitchWorktreeMsg)
			if !ok || msg.Path != c.want {
				t.Errorf("%s: switch = %v %v", c.name, msg, ok)
			}
			continue
		}
		if msg, ok := msgOf(t, cmd).(StatusMsg); !ok || !msg.Err || !strings.Contains(msg.Text, c.status) {
			t.Errorf("%s: status = %+v, want %q", c.name, msg, c.status)
		}
	}
}
