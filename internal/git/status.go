package git

import (
	"context"
	"strings"
)

// FileKind classifies a status entry.
type FileKind int

const (
	FileChanged FileKind = iota
	FileRenamed
	FileUnmerged
	FileUntracked
)

// FileStatus is one entry of git status --porcelain=v2.
type FileStatus struct {
	Kind     FileKind
	X, Y     byte // index and worktree status ('.' means unmodified)
	Path     string
	OrigPath string
}

// Staged reports whether the entry has staged changes.
func (f FileStatus) Staged() bool {
	return (f.Kind == FileChanged || f.Kind == FileRenamed) && f.X != '.'
}

// Unstaged reports whether the entry has unstaged changes.
func (f FileStatus) Unstaged() bool {
	return f.Kind == FileUnmerged || ((f.Kind == FileChanged || f.Kind == FileRenamed) && f.Y != '.')
}

// Status is the parsed output of git status.
type Status struct {
	Branch   string
	OID      string
	Upstream string
	Ahead    string
	Behind   string
	Files    []FileStatus
}

// StatusArgs are the arguments for a full status listing.
var StatusArgs = []string{"status", "--porcelain=v2", "-z", "--branch", "--untracked-files=all"}

// ReadStatus runs git status and parses the result.
func ReadStatus(ctx context.Context, r Runner) (*Status, error) {
	out, err := r.Output(ctx, StatusArgs...)
	if err != nil {
		return nil, err
	}
	return ParseStatus(out), nil
}

// ParseStatus parses NUL separated porcelain v2 output.
func ParseStatus(out []byte) *Status {
	st := &Status{}
	fields := splitNUL(out)
	for i := 0; i < len(fields); i++ {
		e := fields[i]
		if e == "" {
			continue
		}
		switch e[0] {
		case '#':
			parseBranchHeader(st, e)
		case '1':
			p := strings.SplitN(e, " ", 9)
			if len(p) == 9 {
				st.Files = append(st.Files, FileStatus{Kind: FileChanged, X: p[1][0], Y: p[1][1], Path: p[8]})
			}
		case '2':
			p := strings.SplitN(e, " ", 10)
			if len(p) == 10 {
				f := FileStatus{Kind: FileRenamed, X: p[1][0], Y: p[1][1], Path: p[9]}
				if i+1 < len(fields) {
					i++
					f.OrigPath = fields[i]
				}
				st.Files = append(st.Files, f)
			}
		case 'u':
			p := strings.SplitN(e, " ", 11)
			if len(p) == 11 {
				st.Files = append(st.Files, FileStatus{Kind: FileUnmerged, X: p[1][0], Y: p[1][1], Path: p[10]})
			}
		case '?':
			st.Files = append(st.Files, FileStatus{Kind: FileUntracked, X: '?', Y: '?', Path: e[2:]})
		}
	}
	return st
}

func parseBranchHeader(st *Status, e string) {
	p := strings.Fields(e)
	if len(p) < 3 {
		return
	}
	switch p[1] {
	case "branch.oid":
		st.OID = p[2]
	case "branch.head":
		st.Branch = p[2]
	case "branch.upstream":
		st.Upstream = p[2]
	case "branch.ab":
		st.Ahead = strings.TrimPrefix(p[2], "+")
		if len(p) > 3 {
			st.Behind = strings.TrimPrefix(p[3], "-")
		}
	}
}

// IsDirty reports whether the worktree has tracked changes.
func IsDirty(ctx context.Context, r Runner) (bool, error) {
	out, err := r.Output(ctx, "status", "--porcelain=v2", "-z", "--untracked-files=no")
	if err != nil {
		return false, err
	}
	return len(out) > 0, nil
}
