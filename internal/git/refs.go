package git

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
)

// RefKind classifies a reference.
type RefKind int

const (
	RefBranch RefKind = iota
	RefRemote
	RefTag
	RefStash
	RefOther
)

// Ref is a branch, remote branch, tag or other reference.
type Ref struct {
	Name         string // full name, e.g. refs/heads/main
	Short        string // e.g. main, origin/main, v1.0
	Kind         RefKind
	Hash         string // object the ref points at
	Target       string // commit (tags are peeled)
	Upstream     string
	Track        string // e.g. "[ahead 1, behind 2]"
	IsHead       bool   // checked out in the current worktree
	WorktreePath string // worktree that has this branch checked out
	Date         time.Time
	Author       string
	Subject      string
}

const refFormat = "%(refname)%1f%(objectname)%1f%(*objectname)%1f%(upstream:short)%1f%(upstream:track)%1f%(HEAD)%1f%(worktreepath)%1f%(committerdate:unix)%1f%(*committerdate:unix)%1f%(authorname)%1f%(*authorname)%1f%(subject)"

// ListRefs returns all references sorted by kind and name.
func ListRefs(ctx context.Context, r Runner) ([]Ref, error) {
	out, err := r.Output(ctx, "for-each-ref", "--format="+refFormat)
	if err != nil {
		return nil, err
	}
	var refs []Ref
	for _, line := range splitLines(out) {
		if ref, ok := parseRef(line); ok {
			refs = append(refs, ref)
		}
	}
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		return refs[i].Short < refs[j].Short
	})
	return refs, nil
}

func parseRef(line string) (Ref, bool) {
	f := strings.Split(line, "\x1f")
	if len(f) != 12 {
		return Ref{}, false
	}
	ref := Ref{
		Name:         f[0],
		Hash:         f[1],
		Target:       f[1],
		Upstream:     f[3],
		Track:        f[4],
		IsHead:       f[5] == "*",
		WorktreePath: f[6],
		Author:       f[9],
		Subject:      f[11],
	}
	if f[2] != "" {
		ref.Target = f[2]
	}
	date := f[7]
	if date == "" {
		date = f[8]
	}
	if ref.Author == "" {
		ref.Author = f[10]
	}
	if sec, err := strconv.ParseInt(date, 10, 64); err == nil {
		ref.Date = time.Unix(sec, 0)
	}
	switch {
	case strings.HasPrefix(ref.Name, "refs/heads/"):
		ref.Kind, ref.Short = RefBranch, strings.TrimPrefix(ref.Name, "refs/heads/")
	case strings.HasPrefix(ref.Name, "refs/remotes/"):
		ref.Kind, ref.Short = RefRemote, strings.TrimPrefix(ref.Name, "refs/remotes/")
		if strings.HasSuffix(ref.Short, "/HEAD") {
			return Ref{}, false
		}
	case strings.HasPrefix(ref.Name, "refs/tags/"):
		ref.Kind, ref.Short = RefTag, strings.TrimPrefix(ref.Name, "refs/tags/")
	case ref.Name == "refs/stash":
		ref.Kind, ref.Short = RefStash, "stash"
	default:
		ref.Kind, ref.Short = RefOther, strings.TrimPrefix(ref.Name, "refs/")
	}
	return ref, true
}

// Decorations maps a commit hash to the refs pointing at it.
func Decorations(refs []Ref) map[string][]Ref {
	m := make(map[string][]Ref)
	for _, ref := range refs {
		m[ref.Target] = append(m[ref.Target], ref)
	}
	return m
}
