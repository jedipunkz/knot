package git

import (
	"context"
	"path/filepath"
	"strings"
)

// Worktree is one entry of git worktree list.
type Worktree struct {
	Path           string
	Head           string
	Branch         string // short name, empty when detached
	Detached       bool
	Bare           bool
	Locked         bool
	LockReason     string
	Prunable       bool
	PrunableReason string
	Main           bool // first entry of the list
}

// Name is a short display name of the worktree.
func (w Worktree) Name() string {
	return filepath.Base(w.Path)
}

// ListWorktrees runs git worktree list --porcelain -z.
func ListWorktrees(ctx context.Context, r Runner) ([]Worktree, error) {
	out, err := r.Output(ctx, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktrees(out), nil
}

// ParseWorktrees parses NUL separated porcelain output. Records are
// separated by an empty field.
func ParseWorktrees(out []byte) []Worktree {
	var list []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			list = append(list, *cur)
			cur = nil
		}
	}
	for _, f := range strings.Split(string(out), "\x00") {
		if f == "" {
			flush()
			continue
		}
		key, val, _ := strings.Cut(f, " ")
		if key == "worktree" {
			flush()
			cur = &Worktree{Path: val}
			continue
		}
		if cur == nil {
			continue
		}
		switch key {
		case "HEAD":
			cur.Head = val
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "detached":
			cur.Detached = true
		case "bare":
			cur.Bare = true
		case "locked":
			cur.Locked, cur.LockReason = true, val
		case "prunable":
			cur.Prunable, cur.PrunableReason = true, val
		}
	}
	flush()
	if len(list) > 0 {
		list[0].Main = true
	}
	return list
}

// AddWorktree creates a worktree. When newBranch is true, branch is created
// at HEAD. An empty branch creates a detached worktree.
func AddWorktree(ctx context.Context, r Runner, path, branch string, newBranch bool) ([]byte, error) {
	args := []string{"worktree", "add"}
	switch {
	case branch == "":
		args = append(args, "--detach", "--", path)
	case newBranch:
		args = append(args, "-b", branch, "--", path)
	default:
		args = append(args, "--", path, branch)
	}
	return r.Run(ctx, args...)
}

// RemoveWorktree removes a worktree, optionally with --force.
func RemoveWorktree(ctx context.Context, r Runner, path string, force bool) ([]byte, error) {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	return r.Run(ctx, append(args, "--", path)...)
}

// LockWorktree locks a worktree with an optional reason.
func LockWorktree(ctx context.Context, r Runner, path, reason string) ([]byte, error) {
	args := []string{"worktree", "lock"}
	if reason != "" {
		args = append(args, "--reason", reason)
	}
	return r.Run(ctx, append(args, "--", path)...)
}

// UnlockWorktree unlocks a worktree.
func UnlockWorktree(ctx context.Context, r Runner, path string) ([]byte, error) {
	return r.Run(ctx, "worktree", "unlock", "--", path)
}

// PruneWorktrees runs git worktree prune; dryRun only reports.
func PruneWorktrees(ctx context.Context, r Runner, dryRun bool) ([]byte, error) {
	args := []string{"worktree", "prune", "--verbose"}
	if dryRun {
		args = append(args, "--dry-run")
		return r.Output(ctx, args...)
	}
	return r.Run(ctx, args...)
}
