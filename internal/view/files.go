package view

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// TreeView lists a directory at a revision.
type TreeView struct {
	Base
	Rev     string
	Dir     string
	entries []git.TreeEntry
	history []int // cursor positions of parent directories
}

// NewTree creates a tree view at rev (HEAD when empty) for dir.
func NewTree(rev, dir string) *TreeView {
	if rev == "" {
		rev = "HEAD"
	}
	return &TreeView{Rev: rev, Dir: strings.Trim(dir, "/")}
}

func (v *TreeView) Name() string { return "tree" }

func (v *TreeView) Title(env *Env) string {
	return fmt.Sprintf("[tree] %s:/%s", Short(v.Rev, 10), v.Dir)
}

func (v *TreeView) Len() int {
	if v.Dir != "" {
		return len(v.entries) + 1
	}
	return len(v.entries)
}

func (v *TreeView) entry(i int) *git.TreeEntry {
	if v.Dir != "" {
		if i == 0 {
			return nil
		}
		i--
	}
	if i < 0 || i >= len(v.entries) {
		return nil
	}
	return &v.entries[i]
}

func (v *TreeView) Load(env *Env) tea.Cmd {
	r := env.Git()
	rev, dir := v.Rev, v.Dir
	return v.load(func(ctx context.Context) (any, error) {
		list, err := git.ListTree(ctx, r, rev, dir)
		if err != nil {
			return nil, err
		}
		// Directories first, like tig.
		var dirs, files []git.TreeEntry
		for _, e := range list {
			if e.Type == "tree" {
				dirs = append(dirs, e)
			} else {
				files = append(files, e)
			}
		}
		return append(dirs, files...), nil
	})
}

func (v *TreeView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if m, ok := msg.(loadedMsg); ok && !v.stale(m.gen) {
		v.finish(m.err)
		v.entries, _ = m.data.([]git.TreeEntry)
		v.Clamp(v.Len())
	}
	return nil
}

func (v *TreeView) Row(env *Env, i int) []Seg {
	e := v.entry(i)
	if e == nil {
		return []Seg{{theme.Directory, ".."}}
	}
	if e.Type == "tree" {
		return []Seg{{theme.LineNumber, e.Mode + " "}, {theme.Directory, e.Name + "/"}}
	}
	if e.Type == "commit" {
		return []Seg{{theme.LineNumber, e.Mode + " "}, {theme.RefStash, e.Name + " @ " + Short(e.Hash, 7)}}
	}
	return []Seg{{theme.LineNumber, e.Mode + " "}, {theme.Default, e.Name}}
}

func (v *TreeView) path(name string) string {
	if v.Dir == "" {
		return name
	}
	return v.Dir + "/" + name
}

func (v *TreeView) Enter(env *Env) tea.Cmd {
	e := v.entry(v.Cursor)
	if e == nil {
		if v.Dir != "" {
			return v.parent(env)
		}
		return nil
	}
	switch e.Type {
	case "tree":
		v.history = append(v.history, v.Cursor)
		v.Dir = v.path(e.Name)
		v.Cursor, v.Offset = 0, 0
		return v.Load(env)
	case "blob":
		return Msg(OpenMsg{View: NewBlob(v.Rev, v.path(e.Name), 0), Split: true})
	}
	return nil
}

func (v *TreeView) parent(env *Env) tea.Cmd {
	if v.Dir == "" {
		return Status("Already at the root directory")
	}
	v.Dir = path.Dir(v.Dir)
	if v.Dir == "." {
		v.Dir = ""
	}
	v.Cursor = 0
	if n := len(v.history); n > 0 {
		v.Cursor = v.history[n-1]
		v.history = v.history[:n-1]
	}
	return v.Load(env)
}

func (v *TreeView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	if a == keymap.Parent {
		return v.parent(env), true
	}
	return nil, false
}

func (v *TreeView) Selection(env *Env) Selection {
	sel := Selection{Commit: v.Rev, Rev: v.Rev, Directory: v.Dir}
	if e := v.entry(v.Cursor); e != nil && e.Type == "blob" {
		sel.File = v.path(e.Name)
	}
	return sel
}

// BlobView shows a file at a revision ("" = the worktree copy).
type BlobView struct {
	textView
	Rev  string
	Path string
	Line int // initial line (1-based)
}

// NewBlob creates a blob view.
func NewBlob(rev, file string, line int) *BlobView {
	return &BlobView{Rev: rev, Path: file, Line: line}
}

func (v *BlobView) Name() string { return "blob" }

func (v *BlobView) Title(env *Env) string {
	if v.Rev == "" {
		return "[blob] " + v.Path
	}
	return fmt.Sprintf("[blob] %s:%s", Short(v.Rev, 10), v.Path)
}

func (v *BlobView) Load(env *Env) tea.Cmd {
	if v.Rev == "" {
		root := env.WorktreeDir()
		p := v.Path
		return v.load(func(ctx context.Context) (any, error) { return readWorktreeFile(root, p) })
	}
	return v.gitText(env, []string{"cat-file", "-p", v.Rev + ":" + v.Path}, classifyPlain)
}

// readWorktreeFile reads a file below the worktree root as lines.
func readWorktreeFile(root, p string) ([]string, error) {
	full := filepath.Join(root, filepath.FromSlash(p))
	if rel, err := filepath.Rel(root, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s is outside the worktree", p)
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return splitText(string(b)), nil
}

func (v *BlobView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if v.handleText(env, msg, classifyPlain) && v.Line > 0 {
		v.SetCursor(v.Line-1, len(v.lines))
		v.Offset = max(v.Cursor-3, 0)
		v.Line = 0
	}
	return nil
}

func (v *BlobView) Enter(env *Env) tea.Cmd { return v.scrollEnter() }

func (v *BlobView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	if a == keymap.Edit {
		return EditorCmd(env, v.Path, v.Cursor+1), true
	}
	return nil, false
}

func (v *BlobView) Selection(env *Env) Selection {
	return Selection{Commit: v.Rev, Rev: v.Rev, File: v.Path, Line: v.Cursor + 1, Directory: path.Dir(v.Path)}
}

// BlameView annotates a file with the commits that changed each line.
type BlameView struct {
	Base
	Rev   string // "" = worktree
	Path  string
	Line  int
	lines []git.BlameLine
}

// NewBlame creates a blame view.
func NewBlame(rev, file string, line int) *BlameView {
	return &BlameView{Rev: rev, Path: file, Line: line}
}

func (v *BlameView) Name() string { return "blame" }

func (v *BlameView) Title(env *Env) string {
	if v.Rev == "" {
		return "[blame] " + v.Path
	}
	return fmt.Sprintf("[blame] %s:%s", Short(v.Rev, 10), v.Path)
}

func (v *BlameView) Len() int { return len(v.lines) }

func (v *BlameView) Load(env *Env) tea.Cmd {
	r := env.Git()
	rev, p := v.Rev, v.Path
	return v.load(func(ctx context.Context) (any, error) { return git.Blame(ctx, r, rev, p) })
}

func (v *BlameView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if m, ok := msg.(loadedMsg); ok && !v.stale(m.gen) {
		v.finish(m.err)
		v.lines, _ = m.data.([]git.BlameLine)
		if v.Line > 0 {
			v.SetCursor(v.Line-1, len(v.lines))
			v.Offset = max(v.Cursor-3, 0)
			v.Line = 0
		}
	}
	return nil
}

func (v *BlameView) Row(env *Env, i int) []Seg {
	l := v.lines[i]
	var segs []Seg
	prevSame := i > 0 && v.lines[i-1].Commit == l.Commit
	for _, col := range env.Cfg.Blame.Columns {
		switch col {
		case "commit-id":
			n := env.Cfg.Main.CommitIDLength
			id := Short(l.Commit.Hash, n)
			if strings.Trim(l.Commit.Hash, "0") == "" {
				id = strings.Repeat(" ", n)
			}
			if prevSame {
				id = strings.Repeat(" ", n)
			}
			segs = append(segs, Seg{theme.CommitID, Fit(id, n) + " "})
		case "author":
			a := authorCol(env, l.Commit.Author, l.Commit.AuthorMail)
			if prevSame {
				a = strings.Repeat(" ", len([]rune(a)))
			}
			segs = append(segs, Seg{theme.Author, a})
		case "date":
			d := dateCol(env, l.Commit.AuthorTime)
			if prevSame {
				d = strings.Repeat(" ", len([]rune(d)))
			}
			segs = append(segs, Seg{theme.Date, d})
		case "line-number":
			segs = append(segs, Seg{theme.LineNumber, fmt.Sprintf("%*d ", len(fmt.Sprint(len(v.lines))), l.Line)})
		}
	}
	return append(segs, Seg{theme.Default, ExpandTabs(l.Text, env.Cfg.UI.TabSize)})
}

func (v *BlameView) current() *git.BlameLine {
	if !v.onRow(len(v.lines)) {
		return nil
	}
	return &v.lines[v.Cursor]
}

func (v *BlameView) Enter(env *Env) tea.Cmd {
	l := v.current()
	if l == nil {
		return nil
	}
	if strings.Trim(l.Commit.Hash, "0") == "" {
		return Msg(OpenMsg{View: NewDiffWorktree(false), Split: true})
	}
	return Msg(OpenMsg{View: NewDiff(l.Commit.Hash, []string{l.Filename}), Split: true})
}

func (v *BlameView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	switch a {
	case keymap.Parent:
		l := v.current()
		if l == nil {
			return nil, true
		}
		if l.Commit.Previous == "" {
			return Status("The line has no parent commit"), true
		}
		v.Rev, v.Path, v.Line = l.Commit.Previous, l.Commit.PreviousFile, l.OrigLine
		return v.Load(env), true
	case keymap.Edit:
		return EditorCmd(env, v.Path, v.Cursor+1), true
	}
	return nil, false
}

func (v *BlameView) Selection(env *Env) Selection {
	sel := Selection{Rev: v.Rev, File: v.Path, Line: v.Cursor + 1, Directory: path.Dir(v.Path)}
	if l := v.current(); l != nil && strings.Trim(l.Commit.Hash, "0") != "" {
		sel.Commit = l.Commit.Hash
	}
	return sel
}

// GrepView shows git grep results.
type GrepView struct {
	Base
	Args    []string
	matches []git.GrepMatch
	rows    []grepRow
}

type grepRow struct {
	file  bool
	match git.GrepMatch
}

// NewGrep creates a grep view. With no arguments it prompts for a pattern.
func NewGrep(args []string) *GrepView { return &GrepView{Args: args} }

func (v *GrepView) Name() string          { return "grep" }
func (v *GrepView) Title(env *Env) string { return "[grep] " + strings.Join(v.Args, " ") }
func (v *GrepView) Len() int              { return len(v.rows) }

func (v *GrepView) Load(env *Env) tea.Cmd {
	if len(v.Args) == 0 {
		id := v.ID
		return Msg(PromptMsg{Label: "grep: ", OnSubmit: func(s string) tea.Cmd {
			if strings.TrimSpace(s) == "" {
				return Msg(CloseMsg{ID: id})
			}
			return Msg(grepPatternMsg{id: id, pattern: s})
		}})
	}
	r := env.Git()
	args := append([]string{}, v.Args...)
	if !strings.HasPrefix(args[0], "-") && !containsArg(args, "-e") {
		args = append([]string{"-e"}, args...)
	}
	return v.load(func(ctx context.Context) (any, error) { return git.Grep(ctx, r, args) })
}

func containsArg(args []string, a string) bool {
	for _, x := range args {
		if x == a || x == "--" {
			return x == a
		}
	}
	return false
}

type grepPatternMsg struct {
	id      int
	pattern string
}

func (m grepPatternMsg) ViewID() int { return m.id }

func (v *GrepView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case grepPatternMsg:
		v.Args = []string{"-e", m.pattern}
		return v.Load(env)
	case loadedMsg:
		if v.stale(m.gen) {
			return nil
		}
		v.finish(m.err)
		v.matches, _ = m.data.([]git.GrepMatch)
		v.rows = v.rows[:0]
		last := ""
		for _, mt := range v.matches {
			if mt.File != last {
				v.rows = append(v.rows, grepRow{file: true, match: mt})
				last = mt.File
			}
			v.rows = append(v.rows, grepRow{match: mt})
		}
		if len(v.rows) == 0 && m.err == nil {
			return Status("No matches")
		}
	}
	return nil
}

func (v *GrepView) Row(env *Env, i int) []Seg {
	r := v.rows[i]
	if r.file {
		return []Seg{{theme.Directory, r.match.File}}
	}
	return []Seg{{theme.LineNumber, fmt.Sprintf("%6d  ", r.match.Line)}, {theme.Default, ExpandTabs(r.match.Text, env.Cfg.UI.TabSize)}}
}

func (v *GrepView) Enter(env *Env) tea.Cmd {
	if v.onRow(len(v.rows)) {
		m := v.rows[v.Cursor].match
		line := m.Line
		if v.rows[v.Cursor].file {
			line = 0
		}
		return Msg(OpenMsg{View: NewBlob("", m.File, line), Split: true})
	}
	return nil
}

func (v *GrepView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	if a == keymap.Edit && v.onRow(len(v.rows)) {
		m := v.rows[v.Cursor].match
		return EditorCmd(env, m.File, m.Line), true
	}
	return nil, false
}

func (v *GrepView) Selection(env *Env) Selection {
	if v.onRow(len(v.rows)) {
		m := v.rows[v.Cursor].match
		return Selection{File: m.File, Line: m.Line, Directory: path.Dir(m.File)}
	}
	return Selection{}
}
