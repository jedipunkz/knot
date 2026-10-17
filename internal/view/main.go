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
	hl      *rowHighlight
}

// rowHighlight holds the fuzzy-filter match positions of one row. Positions
// are rune indices into the corresponding displayed text.
type rowHighlight struct {
	subject []int
	author  []int // positions in the formatted author; nil if it did not match
	id      int   // number of leading commit ID runes matched
	refs    []int // positions in the reference decoration text
}

// MainView lists commits, like tig's main view.
type MainView struct {
	Base
	Revs  []string
	Paths []string
	// SelectHead moves the cursor to HEAD once it is loaded.
	SelectHead bool

	all         []mainRow // everything loaded, in log order
	rows        []mainRow // visible rows: all, filtered by the query
	query       string
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
	gen     int // load generation captured at load start
	commits []git.Commit
	next    tea.Cmd
	done    bool
	err     error
}

func (m commitBatchMsg) ViewID() int { return m.id }

type changesMsg struct {
	id, gen          int
	unstaged, staged bool
}

func (m changesMsg) ViewID() int { return m.id }

func (v *MainView) Load(env *Env) tea.Cmd {
	ctx, gen := v.startLoad()
	v.all = nil
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
			case ch <- commitBatchMsg{id: id, gen: gen, commits: cs}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
		select {
		case ch <- commitBatchMsg{id: id, gen: gen, done: true, err: err}:
		case <-ctx.Done():
		}
	}()
	var wait tea.Cmd
	wait = func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return commitBatchMsg{id: id, gen: gen, done: true, err: context.Canceled}
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
				return changesMsg{id: id, gen: gen}
			}
			var m changesMsg
			m.id, m.gen = id, gen
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
		if v.stale(m.gen) {
			// A superseded load's result: drop the batch along with
			// the rest of its wait chain.
			return nil
		}
		batch := make([]mainRow, 0, len(m.commits))
		for _, c := range m.commits {
			v.authorWidth = max(v.authorWidth, len([]rune(FormatAuthor(c.AuthorName, c.AuthorEmail, env.Opts.Author))))
			batch = append(batch, mainRow{commit: c, graph: v.builder.Add(c.Hash, c.Parents)})
		}
		v.all = append(v.all, batch...)
		var newRows []mainRow
		if v.query != "" {
			// Each batch is filtered on arrival and appended.
			newRows = v.filterRows(env, batch)
			v.rows = append(v.rows, newRows...)
		} else {
			v.rows = v.all
			newRows = batch
		}
		if v.SelectHead {
			// Only the newly arrived rows can carry HEAD: the
			// rows loaded before this batch were already scanned
			// when they arrived, so one pass over all rows shared
			// by every batch becomes one pass in total.
			from := len(v.rows) - len(newRows)
			for i := from; i < len(v.rows); i++ {
				if r := v.rows[i]; r.special == "" && r.commit.Hash == env.Head.Hash {
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
		if v.stale(m.gen) {
			return nil
		}
		var pre []mainRow
		if m.unstaged {
			pre = append(pre, mainRow{special: "unstaged", commit: git.Commit{Subject: "Unstaged changes"}})
		}
		if m.staged {
			pre = append(pre, mainRow{special: "staged", commit: git.Commit{Subject: "Staged changes"}})
		}
		if len(pre) > 0 {
			v.all = append(pre, v.all...)
			if v.query != "" {
				// While a filter is active the pseudo rows stay hidden.
				return nil
			}
			if v.Cursor > 0 || v.moved {
				v.Cursor += len(pre)
			}
			v.rows = v.all
		}
	}
	return nil
}

func (v *MainView) Row(env *Env, i int) []Seg {
	r := v.rows[i]
	c := r.commit
	hl := r.hl
	if v.query == "" {
		hl = nil
	}
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
				auth := Fit(FormatAuthor(c.AuthorName, c.AuthorEmail, opts.Author), w)
				if hl != nil && len(hl.author) > 0 {
					off := 0
					segs = append(segs, splitMatched(env.Theme, theme.Author, auth, hl.author, &off)...)
				} else {
					segs = append(segs, Seg{theme.Author, auth})
				}
				segs = append(segs, Seg{theme.Default, " "})
			}
		case "commit-id":
			if opts.ID {
				n := env.Cfg.Main.CommitIDLength
				id := Short(c.Hash, n)
				if hl != nil && hl.id > 0 {
					k := min(hl.id, len(id))
					segs = append(segs, Seg{theme.SearchMatch, id[:k]}, Seg{theme.CommitID, id[k:]})
				} else {
					segs = append(segs, Seg{theme.CommitID, Fit(id, n)})
				}
				segs = append(segs, Seg{theme.Default, " "})
			}
		case "graph":
			// The graph is meaningless for a non-contiguous subset: hide it
			// while a filter is active.
			if !opts.Graph || v.query != "" {
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
				var hlRefs []int
				if hl != nil {
					hlRefs = hl.refs
				}
				segs = append(segs, refSegs(env, c.Hash, hlRefs)...)
			}
		case "subject":
			if r.special != "" {
				segs = append(segs, Seg{theme.StatusUnstaged, c.Subject})
				continue
			}
			subj := ExpandTabs(c.Subject, 1)
			if hl != nil && len(hl.subject) > 0 {
				off := 0
				segs = append(segs, splitMatched(env.Theme, theme.Default, subj, hl.subject, &off)...)
				continue
			}
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

// refPieces is one styled piece of the reference decoration, in the order
// the pieces are emitted.
type refPiece struct {
	role theme.Role
	text string
}

// refPieces builds the reference decoration pieces of a commit.
func refPieces(env *Env, hash string) []refPiece {
	var pieces []refPiece
	if env.Head.Branch == "" && env.Head.Hash == hash {
		pieces = append(pieces, refPiece{theme.RefHead, "[HEAD]"}, refPiece{theme.Default, " "})
	}
	for _, ref := range env.Decor[hash] {
		switch ref.Kind {
		case git.RefBranch:
			role := theme.RefBranch
			if ref.IsHead {
				role = theme.RefHead
			}
			pieces = append(pieces, refPiece{role, "[" + ref.Short + "]"}, refPiece{theme.Default, " "})
			if ref.WorktreePath != "" && ref.WorktreePath != env.Loc.TopLevel {
				pieces = append(pieces, refPiece{theme.RefWorktree, "{wt:" + baseName(ref.WorktreePath) + "}"}, refPiece{theme.Default, " "})
			}
		case git.RefRemote:
			pieces = append(pieces, refPiece{theme.RefRemote, "{" + ref.Short + "}"}, refPiece{theme.Default, " "})
		case git.RefTag:
			pieces = append(pieces, refPiece{theme.RefTag, "<" + ref.Short + ">"}, refPiece{theme.Default, " "})
		case git.RefStash:
			pieces = append(pieces, refPiece{theme.RefStash, "(stash)"}, refPiece{theme.Default, " "})
		default:
			pieces = append(pieces, refPiece{theme.RefStash, "(" + ref.Short + ")"}, refPiece{theme.Default, " "})
		}
	}
	return pieces
}

// refText is refPieces' plain text, used for fuzzy matching of ref names.
func refText(env *Env, hash string) string {
	var sb strings.Builder
	for _, p := range refPieces(env, hash) {
		sb.WriteString(p.text)
	}
	return sb.String()
}

// refSegs renders tig-style ref decorations for a commit. hl holds optional
// rune positions to highlight, referring to refText's output.
func refSegs(env *Env, hash string, hl []int) []Seg {
	var segs []Seg
	off := 0
	for _, p := range refPieces(env, hash) {
		segs = append(segs, splitMatched(env.Theme, p.role, p.text, hl, &off)...)
	}
	return segs
}

func (v *MainView) current() *mainRow {
	if !v.onRow(len(v.rows)) {
		return nil
	}
	return &v.rows[v.Cursor]
}

// SetFilter applies a fuzzy query ("" clears it). Filtering keeps the
// original log order; pseudo rows are hidden while the query is non-empty.
// The cursor stays on the selected commit while it still matches. When the
// new query extends the previous one, only the previous result set is
// scanned.
func (v *MainView) SetFilter(env *Env, q string) {
	if len(strings.Fields(q)) == 0 {
		q = ""
	}
	var hash, special string
	if r := v.current(); r != nil {
		hash, special = r.commit.Hash, r.special
	}
	src := v.all
	if v.query != "" && strings.HasPrefix(q, v.query) {
		src = v.rows
	}
	v.query = q
	v.rows = v.filterRows(env, src)
	v.Cursor = 0
	for i, r := range v.rows {
		if r.special == special && (special != "" || r.commit.Hash == hash) {
			v.Cursor = i
			break
		}
	}
	v.Clamp(len(v.rows))
	if v.SelectHead && q == "" {
		// The batch scan only covers rows that arrived after the
		// pending selection; a row hidden by the filter needs the
		// rebuilt list checked once before those batches arrive.
		for i, r := range v.rows {
			if r.special == "" && r.commit.Hash == env.Head.Hash {
				v.SetCursor(i, len(v.rows))
				v.SelectHead = false
				v.moved = true
				break
			}
		}
	}
}

// FilterStatus implements Filterable.
func (v *MainView) FilterStatus() (string, int, int, bool) {
	return v.query, len(v.rows), len(v.all), v.Loading
}

// filterRows returns the rows matching the active query, with their match
// positions computed for highlighting. Pseudo rows are hidden while a
// filter is active.
func (v *MainView) filterRows(env *Env, in []mainRow) []mainRow {
	return filterRowsOf(in, strings.Fields(v.query),
		func(r mainRow) []filterField {
			if r.special != "" {
				return nil
			}
			return mainFilterFields(env, r.commit)
		},
		func(r mainRow, m [][]int) mainRow {
			nr := r
			nr.hl = mainHighlight(m)
			return nr
		})
}

// mainFilterFields is the commit's filterable fields: subject, the raw
// author name (matched without positions; the highlighted author text is
// the formatted one), the formatted author, the commit ID (prefix only)
// and the reference decoration text.
func mainFilterFields(env *Env, c git.Commit) []filterField {
	return []filterField{
		{text: ExpandTabs(c.Subject, 1)},
		{text: c.AuthorName},
		{text: FormatAuthor(c.AuthorName, c.AuthorEmail, env.Opts.Author)},
		{text: c.Hash, prefix: true},
		{text: refText(env, c.Hash)},
	}
}

// mainHighlight packs the matched field positions into a rowHighlight.
// The author field positions refer to the formatted author text.
func mainHighlight(m [][]int) *rowHighlight {
	return &rowHighlight{subject: m[0], author: m[2], id: len(m[3]), refs: m[4]}
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
