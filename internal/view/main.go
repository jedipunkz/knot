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
		batch := make([]mainRow, 0, len(m.commits))
		for _, c := range m.commits {
			v.authorWidth = max(v.authorWidth, len([]rune(FormatAuthor(c.AuthorName, c.AuthorEmail, env.Opts.Author))))
			batch = append(batch, mainRow{commit: c, graph: v.builder.Add(c.Hash, c.Parents)})
		}
		v.all = append(v.all, batch...)
		if v.query != "" {
			// Each batch is filtered on arrival and appended.
			v.rows = append(v.rows, v.filterRows(env, batch)...)
		} else {
			v.rows = v.all
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
	if v.Cursor < 0 || v.Cursor >= len(v.rows) {
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
}

// FilterStatus implements Filterable.
func (v *MainView) FilterStatus() (string, int, int, bool) {
	return v.query, len(v.rows), len(v.all), v.Loading
}

// filterRows returns the rows matching the active query, with their match
// positions computed for highlighting.
func (v *MainView) filterRows(env *Env, in []mainRow) []mainRow {
	tokens := strings.Fields(v.query)
	if len(tokens) == 0 {
		return in
	}
	out := make([]mainRow, 0, len(in))
	for _, r := range in {
		if r.special != "" {
			continue
		}
		hl := matchRow(env, r.commit, tokens)
		if hl == nil {
			continue
		}
		nr := r
		nr.hl = hl
		out = append(out, nr)
	}
	return out
}

// matchRow matches all tokens against the filterable fields of a commit:
// subject, ref names and author (fuzzy) and the commit ID (prefix only).
// Every token must match at least one field. It returns nil when the row
// does not match.
func matchRow(env *Env, c git.Commit, tokens []string) *rowHighlight {
	refs := refText(env, c.Hash)
	displayed := FormatAuthor(c.AuthorName, c.AuthorEmail, env.Opts.Author)
	var hl *rowHighlight
	for _, tok := range tokens {
		var h rowHighlight
		matched := false
		if pos, ok := fuzzyMatch(tok, ExpandTabs(c.Subject, 1)); ok {
			matched = true
			h.subject = pos
		}
		if _, ok := fuzzyMatch(tok, c.AuthorName); ok {
			matched = true
		}
		// Highlight positions refer to the formatted author; they exist
		// only when the token matches it too.
		if pos, ok := fuzzyMatch(tok, displayed); ok {
			matched = true
			h.author = pos
		}
		if len(c.Hash) >= len(tok) && strings.HasPrefix(strings.ToLower(c.Hash), strings.ToLower(tok)) {
			matched = true
			h.id = len(tok)
		}
		if pos, ok := fuzzyMatch(tok, refs); ok {
			matched = true
			h.refs = pos
		}
		if !matched {
			return nil
		}
		if hl == nil {
			hl = &rowHighlight{}
		}
		hl.subject = unionPos(hl.subject, h.subject)
		hl.author = unionPos(hl.author, h.author)
		hl.id = max(hl.id, h.id)
		hl.refs = unionPos(hl.refs, h.refs)
	}
	return hl
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
