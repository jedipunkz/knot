package git

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
)

// DiffFile returns the diff of one file in the index (cached) or worktree.
func DiffFile(ctx context.Context, r Runner, path string, cached bool, contextLines int, ignoreSpace bool) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", unified(contextLines)}
	if cached {
		args = append(args, "--cached")
	}
	if ignoreSpace {
		args = append(args, "-w")
	}
	args = append(args, "--", path)
	out, err := r.Output(ctx, args...)
	return string(out), err
}

func unified(n int) string {
	if n < 0 {
		n = 0
	}
	return "-U" + strconv.Itoa(n)
}

// ApplyCached applies a patch to the index.
func ApplyCached(ctx context.Context, r Runner, patch string, reverse bool) error {
	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	if reverse {
		args = append(args, "--reverse")
	}
	args = append(args, "-")
	_, err := r.RunInput(ctx, []byte(patch), args...)
	return err
}

// StageFile adds a file (including untracked and deleted files) to the index.
func StageFile(ctx context.Context, r Runner, path string) error {
	_, err := r.Run(ctx, "add", "-A", "--", path)
	return err
}

// UnstageFile removes staged changes of a file from the index.
func UnstageFile(ctx context.Context, r Runner, path string) error {
	if RefExists(ctx, r, "HEAD") {
		_, err := r.Run(ctx, "reset", "-q", "HEAD", "--", path)
		return err
	}
	_, err := r.Run(ctx, "rm", "--cached", "-q", "--", path)
	return err
}

// DiscardFile resets the worktree copy of a tracked file to the index.
func DiscardFile(ctx context.Context, r Runner, path string) error {
	_, err := r.Run(ctx, "checkout", "--", path)
	return err
}

// DeleteUntracked removes an untracked file from the worktree.
func DeleteUntracked(worktree, path string) error {
	full := filepath.Join(worktree, path)
	rel, err := filepath.Rel(worktree, full)
	if err != nil || rel == ".." || filepath.IsAbs(rel) || len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator) {
		return os.ErrPermission
	}
	return os.Remove(full)
}
