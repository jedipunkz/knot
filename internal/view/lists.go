package view

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// ReflogView lists reflog entries.
type ReflogView struct {
	Base
	Revs    []string
	entries []git.ReflogEntry
}

// NewReflog creates a reflog view.
func NewReflog(revs []string) *ReflogView { return &ReflogView{Revs: revs} }

func (v *ReflogView) Name() string { return "reflog" }
func (v *ReflogView) Title(env *Env) string {
	if len(v.Revs) == 0 {
		return "[reflog] HEAD"
	}
	return "[reflog] " + strings.Join(v.Revs, " ")
}
func (v *ReflogView) Len() int { return len(v.entries) }

func (v *ReflogView) Load(env *Env) tea.Cmd {
	r := env.Git()
	revs := v.Revs
	return v.load(func(ctx context.Context) (any, error) { return git.ListReflog(ctx, r, revs) })
}

func (v *ReflogView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if m, ok := msg.(loadedMsg); ok && !v.stale(m.gen) {
		v.finish(m.err)
		v.entries, _ = m.data.([]git.ReflogEntry)
	}
	return nil
}

func (v *ReflogView) Row(env *Env, i int) []Seg {
	e := v.entries[i]
	return []Seg{
		{theme.CommitID, Fit(Short(e.Hash, env.Cfg.Main.CommitIDLength), env.Cfg.Main.CommitIDLength) + " "},
		{theme.RefHead, Fit(e.Selector, 12) + " "},
		{theme.Date, dateCol(env, e.Date)},
		{theme.Author, authorCol(env, e.Author, "")},
		{theme.Default, e.Subject},
	}
}

func (v *ReflogView) Enter(env *Env) tea.Cmd {
	if v.onRow(len(v.entries)) {
		return Msg(OpenMsg{View: NewDiff(v.entries[v.Cursor].Hash, nil), Split: true})
	}
	return nil
}

func (v *ReflogView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) { return nil, false }

func (v *ReflogView) Selection(env *Env) Selection {
	if v.onRow(len(v.entries)) {
		h := v.entries[v.Cursor].Hash
		return Selection{Commit: h, Rev: h}
	}
	return Selection{}
}

// dateCol renders a date column using the current date mode.
func dateCol(env *Env, t time.Time) string {
	w := DateWidth(env.Opts.Date, env.Cfg.Main.DateCustom)
	if w == 0 {
		return ""
	}
	return Fit(FormatDate(t, env.Opts.Date, env.Cfg.Main.DateCustom), w) + " "
}

// authorCol renders an author column using the current author mode.
func authorCol(env *Env, name, email string) string {
	if env.Opts.Author == "off" {
		return ""
	}
	return Fit(FormatAuthor(name, email, env.Opts.Author), 16) + " "
}

// StashView lists stashes.
type StashView struct {
	Base
	stashes []git.Stash
}

// NewStash creates a stash view.
func NewStash() *StashView { return &StashView{} }

func (v *StashView) Name() string          { return "stash" }
func (v *StashView) Title(env *Env) string { return "[stash]" }
func (v *StashView) Len() int              { return len(v.stashes) }

func (v *StashView) Load(env *Env) tea.Cmd {
	r := env.Git()
	return v.load(func(ctx context.Context) (any, error) { return git.ListStashes(ctx, r) })
}

func (v *StashView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if m, ok := msg.(loadedMsg); ok && !v.stale(m.gen) {
		v.finish(m.err)
		v.stashes, _ = m.data.([]git.Stash)
	}
	return nil
}

func (v *StashView) Row(env *Env, i int) []Seg {
	s := v.stashes[i]
	return []Seg{
		{theme.Date, dateCol(env, s.Date)},
		{theme.Author, authorCol(env, s.Author, "")},
		{theme.RefStash, Fit(s.Ref, 11) + " "},
		{theme.Default, s.Subject},
	}
}

func (v *StashView) current() *git.Stash {
	if !v.onRow(len(v.stashes)) {
		return nil
	}
	return &v.stashes[v.Cursor]
}

func (v *StashView) Enter(env *Env) tea.Cmd {
	if s := v.current(); s != nil {
		return Msg(OpenMsg{View: &DiffView{Stash: s.Ref, Commit: s.Hash, Label: s.Ref}, Split: true})
	}
	return nil
}

func (v *StashView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	s := v.current()
	switch a {
	case keymap.StashApply, keymap.StashPop, keymap.StashDrop:
		if s == nil {
			return nil, true
		}
		verb := map[keymap.Action]string{keymap.StashApply: "apply", keymap.StashPop: "pop", keymap.StashDrop: "drop"}[a]
		q := fmt.Sprintf("%s %s (%s)?", strings.ToUpper(verb[:1])+verb[1:], s.Ref, s.Subject)
		return Confirm(env, a, q, RunGit(env, "Stash "+verb+" done: "+s.Ref, "stash", verb, "-q", s.Ref)), true
	}
	return nil, false
}

func (v *StashView) Selection(env *Env) Selection {
	if s := v.current(); s != nil {
		return Selection{Commit: s.Hash, Stash: s.Ref, Rev: s.Hash}
	}
	return Selection{}
}

// RefsView lists branches, remotes and tags.
type RefsView struct {
	Base
	refs []git.Ref
}

// NewRefs creates a refs view.
func NewRefs() *RefsView { return &RefsView{} }

func (v *RefsView) Name() string          { return "refs" }
func (v *RefsView) Title(env *Env) string { return "[refs]" }
func (v *RefsView) Len() int              { return len(v.refs) }

func (v *RefsView) Load(env *Env) tea.Cmd {
	r := env.Git()
	return v.load(func(ctx context.Context) (any, error) { return git.ListRefs(ctx, r) })
}

func (v *RefsView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if m, ok := msg.(loadedMsg); ok && !v.stale(m.gen) {
		v.finish(m.err)
		refs, _ := m.data.([]git.Ref)
		v.refs = v.refs[:0]
		for _, r := range refs {
			if r.Kind != git.RefStash {
				v.refs = append(v.refs, r)
			}
		}
	}
	return nil
}

func (v *RefsView) Row(env *Env, i int) []Seg {
	r := v.refs[i]
	mark := "  "
	role := theme.RefBranch
	switch r.Kind {
	case git.RefRemote:
		role = theme.RefRemote
	case git.RefTag:
		role = theme.RefTag
	case git.RefOther:
		role = theme.RefStash
	}
	if r.IsHead {
		mark, role = "* ", theme.RefHead
	} else if r.WorktreePath != "" {
		mark = "+ "
	}
	segs := []Seg{
		{theme.Date, dateCol(env, r.Date)},
		{theme.Author, authorCol(env, r.Author, "")},
		{theme.Marker, mark},
		{role, Fit(r.Short, 30) + " "},
	}
	if r.WorktreePath != "" && !r.IsHead {
		segs = append(segs, Seg{theme.RefWorktree, "{wt:" + env.Abbrev(r.WorktreePath) + "} "})
	}
	if r.Track != "" {
		segs = append(segs, Seg{theme.Warning, r.Track + " "})
	}
	return append(segs, Seg{theme.Default, r.Subject})
}

func (v *RefsView) current() *git.Ref {
	if !v.onRow(len(v.refs)) {
		return nil
	}
	return &v.refs[v.Cursor]
}

func (v *RefsView) Enter(env *Env) tea.Cmd {
	if r := v.current(); r != nil {
		return Msg(OpenMsg{View: NewMain([]string{r.Name}, nil)})
	}
	return nil
}

func (v *RefsView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	r := v.current()
	if r == nil {
		return nil, false
	}
	switch a {
	case keymap.Checkout:
		if r.IsHead {
			return Status(r.Short + " is already checked out"), true
		}
		if r.Kind == git.RefBranch && r.WorktreePath != "" {
			return Errorf(fmt.Errorf("branch %s is checked out in worktree %s; switch to it with the worktree view (w)", r.Short, env.Abbrev(r.WorktreePath))), true
		}
		target := r.Short
		if r.Kind == git.RefTag {
			target = r.Name
		}
		return Confirm(env, a, "Checkout "+r.Short+"?", RunGit(env, "Checked out "+r.Short, "checkout", "-q", target)), true
	case keymap.BranchDelete:
		if r.Kind != git.RefBranch {
			return Status("Only local branches can be deleted"), true
		}
		if r.IsHead || r.WorktreePath != "" {
			return Errorf(fmt.Errorf("branch %s is checked out and cannot be deleted", r.Short)), true
		}
		return Confirm(env, a, "Delete branch "+r.Short+"? (git branch -d)", RunGit(env, "Deleted branch "+r.Short, "branch", "-d", r.Short)), true
	}
	return nil, false
}

func (v *RefsView) Selection(env *Env) Selection {
	if r := v.current(); r != nil {
		rf := *r
		return Selection{Commit: r.Target, Ref: &rf, Rev: r.Target}
	}
	return Selection{}
}
