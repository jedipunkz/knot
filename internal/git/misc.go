package git

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// Stash is one entry of git stash list.
type Stash struct {
	Ref     string // stash@{0}
	Hash    string
	Date    time.Time
	Author  string
	Subject string
}

// ListStashes returns all stash entries.
func ListStashes(ctx context.Context, r Runner) ([]Stash, error) {
	out, err := r.Output(ctx, "stash", "list", "--format=%gd%x1f%H%x1f%ct%x1f%an%x1f%gs")
	if err != nil {
		return nil, err
	}
	var list []Stash
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\x1f")
		if len(f) != 5 {
			continue
		}
		s := Stash{Ref: f[0], Hash: f[1], Author: f[3], Subject: f[4]}
		s.Date = parseUnix(f[2])
		list = append(list, s)
	}
	return list, nil
}

// ReflogEntry is one entry of git reflog.
type ReflogEntry struct {
	Hash     string
	Selector string // HEAD@{0}
	Subject  string // reflog message
	Date     time.Time
	Author   string
}

// ListReflog returns reflog entries for the given revisions (HEAD when empty).
func ListReflog(ctx context.Context, r Runner, revs []string) ([]ReflogEntry, error) {
	args := []string{"reflog", "show", "--no-color", "--format=%H%x1f%gd%x1f%gs%x1f%ct%x1f%an"}
	args = append(args, revs...)
	out, err := r.Output(ctx, args...)
	if err != nil {
		return nil, err
	}
	var list []ReflogEntry
	for _, line := range splitLines(out) {
		f := strings.Split(line, "\x1f")
		if len(f) != 5 {
			continue
		}
		list = append(list, ReflogEntry{Hash: f[0], Selector: f[1], Subject: f[2], Date: parseUnix(f[3]), Author: f[4]})
	}
	return list, nil
}

// TreeEntry is one entry of git ls-tree.
type TreeEntry struct {
	Mode string
	Type string // blob, tree, commit
	Hash string
	Name string
}

// ListTree lists the tree at rev:dir.
func ListTree(ctx context.Context, r Runner, rev, dir string) ([]TreeEntry, error) {
	out, err := r.Output(ctx, "ls-tree", "-z", rev+":"+dir)
	if err != nil {
		return nil, err
	}
	var list []TreeEntry
	for _, rec := range splitNUL(out) {
		meta, name, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		p := strings.Fields(meta)
		if len(p) != 3 {
			continue
		}
		list = append(list, TreeEntry{Mode: p[0], Type: p[1], Hash: p[2], Name: name})
	}
	return list, nil
}

// GrepMatch is one line matched by git grep.
type GrepMatch struct {
	File string
	Line int
	Text string
}

// Grep runs git grep. No match is not an error.
func Grep(ctx context.Context, r Runner, args []string) ([]GrepMatch, error) {
	full := append([]string{"grep", "-n", "-z", "-I", "--no-color"}, args...)
	out, err := r.Output(ctx, full...)
	if err != nil {
		if ExitCode(err) == 1 {
			return nil, nil
		}
		return nil, err
	}
	var list []GrepMatch
	for _, line := range splitLines(out) {
		f := strings.SplitN(line, "\x00", 3)
		if len(f) != 3 {
			continue
		}
		n, _ := strconv.Atoi(f[1])
		list = append(list, GrepMatch{File: f[0], Line: n, Text: f[2]})
	}
	return list, nil
}

func parseUnix(s string) time.Time {
	sec, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
