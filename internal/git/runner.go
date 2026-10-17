// Package git wraps the git command line. Every command is executed with an
// argv slice (never through a shell), with -C pointing at the active worktree
// and with the pager and colors disabled.
package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Runner executes git commands in one directory.
type Runner struct {
	Binary string // git executable, "git" when empty
	Dir    string // passed as -C
}

// Error is returned when git exits with a non-zero status.
type Error struct {
	Args   []string
	Stderr string
	Code   int
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.Code)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// ExitCode returns the exit code of a git error, or -1.
func ExitCode(err error) int {
	var ge *Error
	if errors.As(err, &ge) {
		return ge.Code
	}
	return -1
}

func (r Runner) binary() string {
	if r.Binary == "" {
		return "git"
	}
	return r.Binary
}

// Command builds an *exec.Cmd for git. readOnly commands run with
// GIT_OPTIONAL_LOCKS=0 so browsing never takes the index lock.
func (r Runner) Command(ctx context.Context, readOnly bool, args ...string) *exec.Cmd {
	full := make([]string, 0, len(args)+6)
	if r.Dir != "" {
		full = append(full, "-C", r.Dir)
	}
	full = append(full, "--no-pager", "-c", "color.ui=never", "-c", "core.quotepath=off")
	full = append(full, args...)
	cmd := exec.CommandContext(ctx, r.binary(), full...)
	env := os.Environ()
	env = append(env, "GIT_TERMINAL_PROMPT=0", "LC_MESSAGES=C")
	if readOnly {
		env = append(env, "GIT_OPTIONAL_LOCKS=0")
	}
	cmd.Env = env
	return cmd
}

func (r Runner) run(ctx context.Context, readOnly bool, stdin []byte, args ...string) ([]byte, error) {
	cmd := r.Command(ctx, readOnly, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.Bytes(), &Error{Args: args, Stderr: stderr.String(), Code: ee.ExitCode()}
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// Output runs a read-only git command and returns its stdout.
func (r Runner) Output(ctx context.Context, args ...string) ([]byte, error) {
	return r.run(ctx, true, nil, args...)
}

// Run runs a git command that may modify the repository.
func (r Runner) Run(ctx context.Context, args ...string) ([]byte, error) {
	return r.run(ctx, false, nil, args...)
}

// RunInput runs a modifying git command with stdin.
func (r Runner) RunInput(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	return r.run(ctx, false, stdin, args...)
}

// StreamLines runs a read-only git command and calls fn for every line of
// stdout (without the trailing newline) as soon as it is read.
func (r Runner) StreamLines(ctx context.Context, fn func(line string) error, args ...string) error {
	cmd := r.Command(ctx, true, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var fnErr error
	for sc.Scan() {
		if fnErr = fn(sc.Text()); fnErr != nil {
			break
		}
	}
	if fnErr != nil {
		_, _ = io.Copy(io.Discard, out)
	}
	waitErr := cmd.Wait()
	if fnErr != nil {
		return fnErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			return &Error{Args: args, Stderr: stderr.String(), Code: ee.ExitCode()}
		}
		return waitErr
	}
	return nil
}

// Version returns the git version as (major, minor).
func (r Runner) Version(ctx context.Context) (int, int, error) {
	out, err := Runner{Binary: r.Binary}.Output(ctx, "version")
	if err != nil {
		return 0, 0, err
	}
	var major, minor int
	s := strings.TrimSpace(string(out))
	s = strings.TrimPrefix(s, "git version ")
	if _, err := fmt.Sscanf(s, "%d.%d", &major, &minor); err != nil {
		return 0, 0, fmt.Errorf("cannot parse git version %q", s)
	}
	return major, minor, nil
}

func splitNUL(b []byte) []string {
	s := string(b)
	s = strings.TrimSuffix(s, "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}

func splitLines(b []byte) []string {
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}
