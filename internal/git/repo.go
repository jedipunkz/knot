package git

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Repo describes the repository shared by all worktrees.
type Repo struct {
	Binary    string
	CommonDir string // absolute path of the common git dir
	Bare      bool
}

// Location describes where knot is currently operating.
type Location struct {
	GitDir   string // absolute git dir of the worktree
	TopLevel string // absolute worktree root, empty for a bare repository
}

// Open resolves the repository and worktree containing dir.
func Open(ctx context.Context, binary, dir string) (*Repo, Location, error) {
	r := Runner{Binary: binary, Dir: dir}
	out, err := r.Output(ctx, "rev-parse", "--path-format=absolute", "--git-common-dir", "--git-dir", "--is-bare-repository")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && strings.Contains(ge.Stderr, "not a git repository") {
			return nil, Location{}, fmt.Errorf("%s is not inside a git repository", dir)
		}
		return nil, Location{}, err
	}
	lines := splitLines(out)
	if len(lines) != 3 {
		return nil, Location{}, fmt.Errorf("unexpected rev-parse output: %q", out)
	}
	repo := &Repo{Binary: binary, CommonDir: filepath.Clean(lines[0]), Bare: lines[2] == "true"}
	loc := Location{GitDir: filepath.Clean(lines[1])}
	if !repo.Bare {
		top, err := r.Output(ctx, "rev-parse", "--show-toplevel")
		if err == nil {
			loc.TopLevel = filepath.Clean(strings.TrimSpace(string(top)))
		}
	}
	return repo, loc, nil
}

// Runner returns a runner for the given location.
func (repo *Repo) Runner(loc Location) Runner {
	dir := loc.TopLevel
	if dir == "" {
		dir = loc.GitDir
	}
	return Runner{Binary: repo.Binary, Dir: dir}
}

// Head describes HEAD of a worktree.
type Head struct {
	Hash   string // empty for an unborn branch
	Branch string // short branch name, empty when detached
}

// ReadHead returns HEAD of the worktree the runner points at.
func ReadHead(ctx context.Context, r Runner) Head {
	var h Head
	if out, err := r.Output(ctx, "symbolic-ref", "-q", "--short", "HEAD"); err == nil {
		h.Branch = strings.TrimSpace(string(out))
	}
	if out, err := r.Output(ctx, "rev-parse", "-q", "--verify", "HEAD^{commit}"); err == nil {
		h.Hash = strings.TrimSpace(string(out))
	}
	return h
}

// RefExists reports whether ref resolves to an object.
func RefExists(ctx context.Context, r Runner, ref string) bool {
	_, err := r.Output(ctx, "rev-parse", "-q", "--verify", ref)
	return err == nil
}
