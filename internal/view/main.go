package view

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/graph"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

type mainRow struct {
	commit  git.Commit
	graph   []graph.Cell
	special string // "unstaged" / "staged" pseudo commits
}

// MainView lists commits, like tig's main view.
type MainView struct {
	Base
	Revs  []string
	Paths []string
	// SelectHead moves the cursor to HEAD once it is loaded.
	SelectHead bool

	rows        []mainRow
	builder     *graph.Builder
	authorWidth int
	moved       bool // the cursor was placed on a commit (SelectHead)
}

// NewMain creates a main view for revisions and paths.
func NewMain(revs, paths []string) *MainView {
	return &MainView{Revs: revs, Paths: paths}
}

func (v *MainView) Name() string { return "main" }

func (v *MainView) Title(env *Env) string {
	t := "[main] "
	if len(v.Revs) > 0 {
		t += strings.Join(v.Revs, " ")
	} else {
		t += "HEAD"
	}
	if len(v.Paths) > 0 {
		t += " -- " + strings.Join(v.Paths, " ")
	}
	return t
}

func (v *MainView) Len() int { return len(v.rows) }

type commitBatchMsg struct {
	id      int
	commits []git.Commit
	next    tea.Cmd
	done    bool
	err     error
}

func (m commitBatchMsg) ViewID() int { return m.id }

type changesMsg struct {
	id               int
	unstaged, staged bool
}

func (m changesMsg) ViewID() int { return m.id }

func (v *MainView) Load(env *Env) tea.Cmd {
	ctx := v.startLoad()
	v.rows = nil
	sym := graph.Unicode
	if env.Opts.ASCII {
		sym = graph.ASCII
	}
	v.builder = graph.New(sym)
	v.authorWidth = 0
	v.moved = false
	r := env.Git()
	id := v.ID
	extra := env.Cfg.Git.LogArgs
	if env.Opts.Graph && !hasOrder(extra) {
		// Like tig's commit-order=auto: topological order for the graph.
		extra = append([]string{"--topo-order"}, extra...)
	}
	args := git.LogArgs(extra, v.Revs, v.Paths)
	ch := make(chan commitBatchMsg, 4)
	go func() {
		defer close(ch)
		err := git.StreamLog(ctx, r, args, 500, func(cs []git.Commit) error {
			select {
			case ch <- commitBatchMsg{id: id, commits: cs}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		select {
		case ch <- commitBatchMsg{id: id, done: true, err: err}:
		case <-ctx.Done():
		}
	}()
	var wait tea.Cmd
	wait = func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return commitBatchMsg{id: id, done: true, err: context.Canceled}
		}
		if !m.done {
			m.next = wait
		}
		return m
	}
	cmds := []tea.Cmd{wait}
	if len(v.Revs) == 0 && env.Loc.TopLevel != "" {
		cmds = append(cmds, func() tea.Msg {
			st, err := git.ReadStatus(ctx, r)
			if err != nil {
				return changesMsg{id: id}
			}
			var m changesMsg
			m.id = id
			for _, f := range st.Files {
				if f.Unstaged() {
					m.unstaged = true
				}
				if f.Staged() {
					m.staged = true
				}
			}
			return m
		})
	}
	return tea.Batch(cmds...)
}

func (v *MainView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case commitBatchMsg:
		for _, c := range m.commits {
			v.rows = append(v.rows, mainRow{commit: c, graph: v.builder.Add(c.Hash, c.Parents)})
			v.authorWidth = max(v.authorWidth, len([]rune(FormatAuthor(c.AuthorName, c.AuthorEmail, env.Opts.Author))))
		}
		if v.SelectHead {
			for i, r := range v.rows {
				if r.special == "" && r.commit.Hash == env.Head.Hash {
					v.SetCursor(i, len(v.rows))
					v.SelectHead = false
					v.moved = true
					break
				}
			}
		}
		if m.done {
			v.finish(m.err)
			return nil
		}
		return m.next
	case changesMsg:
		var pre []mainRow
		if m.unstaged {
			pre = append(pre, mainRow{special: "unstaged", commit: git.Commit{Subject: "Unstaged changes"}})
		}
		if m.staged {
			pre = append(pre, mainRow{special: "staged", commit: git.Commit{Subject: "Staged changes"}})
		}
		if len(pre) > 0 {
			if v.Cursor > 0 || v.moved {
				v.Cursor += len(pre)
			}
			v.rows = append(pre, v.rows...)
		}
	}
	return nil
}

func (v *MainView) Row(env *Env, i int) []Seg {
	r := v.rows[i]
	c := r.commit
	var segs []Seg
	opts := env.Opts
	for _, col := range env.Cfg.Main.Columns {
		switch col {
		case "date":
			if w := DateWidth(opts.Date, env.Cfg.Main.DateCustom); w > 0 {
				segs = append(segs, Seg{theme.Date, Fit(FormatDate(c.AuthorDate, opts.Date, env.Cfg.Main.DateCustom), w) + " "})
			}
		case "author":
			if opts.Author != "off" {
				w := min(max(v.authorWidth, 6), 20)
				segs = append(segs, Seg{theme.Author, Fit(FormatAuthor(c.AuthorName, c.AuthorEmail, opts.Author), w) + " "})
			}
		case "commit-id":
			if opts.ID {
				n := env.Cfg.Main.CommitIDLength
				segs = append(segs, Seg{theme.CommitID, Fit(Short(c.Hash, n), n) + " "})
			}
		case "graph":
			if !opts.Graph {
				continue
			}
			if r.special != "" {
				sym := "●"
				if opts.ASCII {
					sym = "*"
				}
				segs = append(segs, Seg{theme.StatusUnstaged, sym + " "})
				continue
			}
			for _, cell := range r.graph {
				segs = append(segs, Seg{theme.GraphRole(cell.Lane), string(cell.Ch)})
			}
			segs = append(segs, Seg{theme.Default, " "})
		case "refs":
			if opts.Refs && r.special == "" {
				segs = append(segs, refSegs(env, c.Hash)...)
			}
		case "subject":
			if r.special != "" {
				segs = append(segs, Seg{theme.StatusUnstaged, c.Subject})
				continue
			}
			subj := ExpandTabs(c.Subject, 1)
			limit := env.Cfg.UI.TitleOverflow
			if opts.TitleOverflow && limit > 0 && len([]rune(subj)) > limit {
				rs := []rune(subj)
				segs = append(segs, Seg{theme.Default, string(rs[:limit])}, Seg{theme.Overflow, string(rs[limit:])})
			} else {
				segs = append(segs, Seg{theme.Default, subj})
			}
		}
	}
	return segs
}

// refSegs renders tig-style ref decorations for a commit.
func refSegs(env *Env, hash string) []Seg {
	var segs []Seg
	for _, ref := range env.Decor[hash] {
		switch ref.Kind {
		case git.RefBranch:
			role := theme.RefBranch
			if ref.IsHead {
				role = theme.RefHead
			}
			segs = append(segs, Seg{role, "[" + ref.Short + "]"}, Seg{theme.Default, " "})
			if ref.WorktreePath != "" && ref.WorktreePath != env.Loc.TopLevel {
				segs = append(segs, Seg{theme.RefWorktree, "{wt:" + baseName(ref.WorktreePath) + "}"}, Seg{theme.Default, " "})
			}
		case git.RefRemote:
			segs = append(segs, Seg{theme.RefRemote, "{" + ref.Short + "}"}, Seg{theme.Default, " "})
		case git.RefTag:
			segs = append(segs, Seg{theme.RefTag, "<" + ref.Short + ">"}, Seg{theme.Default, " "})
		case git.RefStash:
			segs = append(segs, Seg{theme.RefStash, "(stash)"}, Seg{theme.Default, " "})
		default:
			segs = append(segs, Seg{theme.RefStash, "(" + ref.Short + ")"}, Seg{theme.Default, " "})
		}
	}
	if env.Head.Branch == "" && env.Head.Hash == hash {
		segs = append([]Seg{{theme.RefHead, "[HEAD]"}, {theme.Default, " "}}, segs...)
	}
	return segs
}

func (v *MainView) current() *mainRow {
	if v.Cursor < 0 || v.Cursor >= len(v.rows) {
		return nil
	}
	return &v.rows[v.Cursor]
}

func (v *MainView) Enter(env *Env) tea.Cmd {
	r := v.current()
	if r == nil {
		return nil
	}
	switch r.special {
	case "unstaged":
		return Msg(OpenMsg{View: NewDiffWorktree(false), Split: true})
	case "staged":
		return Msg(OpenMsg{View: NewDiffWorktree(true), Split: true})
	}
	return Msg(OpenMsg{View: NewDiff(r.commit.Hash, v.Paths), Split: true})
}

func (v *MainView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	switch a {
	case keymap.MoveHead:
		for i, r := range v.rows {
			if r.special == "" && r.commit.Hash == env.Head.Hash {
				v.SetCursor(i, len(v.rows))
				return nil, true
			}
		}
		return Status("HEAD is not in this view"), true
	case keymap.CherryPick:
		r := v.current()
		if r == nil || r.special != "" {
			return nil, true
		}
		hash := r.commit.Hash
		return Confirm(env, a, "Cherry-pick "+Short(hash, 7)+" "+r.commit.Subject+"?",
			RunGit(env, "Cherry-picked "+Short(hash, 7), "cherry-pick", hash)), true
	case keymap.ToggleLineGraphics:
		// The graph symbols are computed while loading.
		return v.Load(env), true
	}
	return nil, false
}

func (v *MainView) Selection(env *Env) Selection {
	r := v.current()
	if r == nil || r.special != "" {
		return Selection{}
	}
	sel := Selection{Commit: r.commit.Hash, Rev: r.commit.Hash}
	for _, ref := range env.Decor[r.commit.Hash] {
		if ref.Kind == git.RefBranch {
			rf := ref
			sel.Ref = &rf
			break
		}
	}
	if len(v.Paths) == 1 {
		sel.File = v.Paths[0]
	}
	return sel
}

func hasOrder(args []string) bool {
	for _, a := range args {
		switch a {
		case "--topo-order", "--date-order", "--author-date-order", "--reverse":
			return true
		}
	}
	return false
}

func baseName(p string) string {
	p = strings.TrimRight(p, "/")
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
