package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Commit is one entry of the main view.
type Commit struct {
	Hash        string
	Parents     []string
	AuthorName  string
	AuthorEmail string
	AuthorDate  time.Time
	Subject     string
}

const logFormat = "%H%x1f%P%x1f%an%x1f%ae%x1f%at%x1f%s"

// LogArgs builds the git log arguments for the main view.
func LogArgs(extra, revs, paths []string) []string {
	args := []string{"log", "--no-color", "--format=" + logFormat}
	args = append(args, extra...)
	args = append(args, revs...)
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	return args
}

// ParseCommit parses one line produced with logFormat.
func ParseCommit(line string) (Commit, error) {
	f := strings.Split(line, "\x1f")
	if len(f) != 6 {
		return Commit{}, fmt.Errorf("malformed log line: %q", line)
	}
	c := Commit{Hash: f[0], AuthorName: f[2], AuthorEmail: f[3], Subject: f[5]}
	if f[1] != "" {
		c.Parents = strings.Fields(f[1])
	}
	if sec, err := strconv.ParseInt(f[4], 10, 64); err == nil {
		c.AuthorDate = time.Unix(sec, 0)
	}
	return c, nil
}

// StreamLog streams commits in batches to fn.
func StreamLog(ctx context.Context, r Runner, args []string, batch int, fn func([]Commit) error) error {
	buf := make([]Commit, 0, batch)
	err := r.StreamLines(ctx, func(line string) error {
		c, err := ParseCommit(line)
		if err != nil {
			return err
		}
		buf = append(buf, c)
		if len(buf) >= batch {
			if err := fn(buf); err != nil {
				return err
			}
			buf = make([]Commit, 0, batch)
		}
		return nil
	}, args...)
	if err != nil {
		return err
	}
	if len(buf) > 0 {
		return fn(buf)
	}
	return nil
}
