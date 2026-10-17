package view

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// Status sections.
const (
	secStaged    = "staged"
	secUnstaged  = "unstaged"
	secUntracked = "untracked"
)

type statusRow struct {
	header  string // non-empty for header lines
	section string
	file    *git.FileStatus
}

// StatusView shows the working tree status.
type StatusView struct {
	Base
	st   *git.Status
	rows []statusRow
}

// NewStatus creates a status view.
func NewStatus() *StatusView { return &StatusView{} }

func (v *StatusView) Name() string          { return "status" }
func (v *StatusView) Title(env *Env) string { return "[status] " + env.WorktreeName() }
func (v *StatusView) Len() int              { return len(v.rows) }

func (v *StatusView) Load(env *Env) tea.Cmd {
	if env.Loc.TopLevel == "" {
		v.Err = fmt.Errorf("the status view needs a work tree")
		return nil
	}
	r := env.Git()
	return v.load(func(ctx context.Context) (any, error) { return git.ReadStatus(ctx, r) })
}

func (v *StatusView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	m, ok := msg.(loadedMsg)
	if !ok {
		return nil
	}
	v.finish(m.err)
	st, _ := m.data.(*git.Status)
	v.st = st
	v.rows = nil
	if st == nil {
		return nil
	}
	head := "On branch " + st.Branch
	if st.Branch == "(detached)" {
		head = "HEAD detached at " + Short(st.OID, 7)
	}
	if st.Upstream != "" {
		head += fmt.Sprintf("  (upstream %s, ahead %s, behind %s)", st.Upstream, st.Ahead, st.Behind)
	}
	v.rows = append(v.rows, statusRow{header: head})
	add := func(section, title string, pick func(git.FileStatus) bool) {
		v.rows = append(v.rows, statusRow{header: title, section: section})
		n := 0
		for i := range st.Files {
			if pick(st.Files[i]) {
				v.rows = append(v.rows, statusRow{section: section, file: &st.Files[i]})
				n++
			}
		}
		if n == 0 {
			v.rows = append(v.rows, statusRow{header: "  (no files)", section: section})
		}
	}
	add(secStaged, "Changes to be committed:", git.FileStatus.Staged)
	add(secUnstaged, "Changes not staged for commit:", git.FileStatus.Unstaged)
	add(secUntracked, "Untracked files:", func(f git.FileStatus) bool { return f.Kind == git.FileUntracked })
	v.Clamp(len(v.rows))
	if v.Cursor == 0 && len(v.rows) > 2 {
		// Start on the first file like tig.
		for i, r := range v.rows {
			if r.file != nil {
				v.Cursor = i
				break
			}
		}
	}
	return nil
}

func (v *StatusView) Row(env *Env, i int) []Seg {
	r := v.rows[i]
	if r.file == nil {
		if r.section == "" {
			return []Seg{{theme.Default, r.header}}
		}
		if strings.HasPrefix(r.header, "  ") {
			return []Seg{{theme.LineNumber, r.header}}
		}
		return []Seg{{theme.Section, r.header}}
	}
	f := r.file
	var code byte
	role := theme.StatusUnstaged
	switch r.section {
	case secStaged:
		code, role = f.X, theme.StatusStaged
	case secUnstaged:
		code = f.Y
		if f.Kind == git.FileUnmerged {
			code = 'U'
		}
	default:
		code, role = '?', theme.StatusUntracked
	}
	name := f.Path
	if f.Kind == git.FileRenamed && r.section == secStaged {
		name = f.OrigPath + " -> " + f.Path
	}
	return []Seg{{role, "  " + string(code) + " "}, {theme.Default, name}}
}

func (v *StatusView) current() *statusRow {
	if v.Cursor < len(v.rows) {
		return &v.rows[v.Cursor]
	}
	return nil
}

func (v *StatusView) Enter(env *Env) tea.Cmd {
	r := v.current()
	if r == nil || r.file == nil {
		return nil
	}
	return Msg(OpenMsg{View: NewStage(r.file.Path, r.section), Split: true})
}

func (v *StatusView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	r := v.current()
	switch a {
	case keymap.StatusCommit:
		return GitExec(env, "commit"), true
	}
	if r == nil {
		return nil, false
	}
	switch a {
	case keymap.StatusUpdate:
		return v.update(env, r), true
	case keymap.StatusRevert:
		if r.file == nil {
			return nil, true
		}
		return revertFile(env, *r.file, r.section), true
	case keymap.StatusMerge:
		if r.file == nil || r.file.Kind != git.FileUnmerged {
			return Status("The file is not unmerged"), true
		}
		return GitExec(env, "mergetool", "--", r.file.Path), true
	}
	return nil, false
}

// update stages or unstages the file, or all files of a section header.
func (v *StatusView) update(env *Env, r *statusRow) tea.Cmd {
	gr := env.Git()
	var files []string
	if r.file != nil {
		files = []string{r.file.Path}
	} else if r.section != "" {
		for _, row := range v.rows {
			if row.section == r.section && row.file != nil {
				files = append(files, row.file.Path)
			}
		}
	}
	if len(files) == 0 {
		return nil
	}
	section := r.section
	return func() tea.Msg {
		ctx := context.Background()
		for _, f := range files {
			var err error
			if section == secStaged {
				err = git.UnstageFile(ctx, gr, f)
			} else {
				err = git.StageFile(ctx, gr, f)
			}
			if err != nil {
				return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
			}
		}
		return ReloadMsg{}
	}
}

func revertFile(env *Env, f git.FileStatus, section string) tea.Cmd {
	gr := env.Git()
	root := env.WorktreeDir()
	switch {
	case section == secStaged:
		return Status("Unstage the file first (u), then discard its changes")
	case f.Kind == git.FileUntracked:
		return Confirm(env, keymap.StatusRevert, "Delete untracked file "+f.Path+"?", func() tea.Msg {
			if err := git.DeleteUntracked(root, f.Path); err != nil {
				return StatusMsg{Text: err.Error(), Err: true}
			}
			return tea.BatchMsg{Status("Deleted " + f.Path), Msg(ReloadMsg{})}
		})
	case f.Kind == git.FileUnmerged:
		return Status("Resolve the conflict first (M)")
	}
	return Confirm(env, keymap.StatusRevert, "Discard unstaged changes of "+f.Path+"?", func() tea.Msg {
		if err := git.DiscardFile(context.Background(), gr, f.Path); err != nil {
			return StatusMsg{Text: err.Error(), Err: true}
		}
		return tea.BatchMsg{Status("Discarded changes of " + f.Path), Msg(ReloadMsg{})}
	})
}

func (v *StatusView) Selection(env *Env) Selection {
	if r := v.current(); r != nil && r.file != nil {
		return Selection{File: r.file.Path}
	}
	return Selection{}
}

// StageView shows the diff of one file in the index or worktree and stages
// chunks or single lines.
type StageView struct {
	textView
	Path    string
	Section string
	fd      *git.FileDiff
}

// NewStage creates a stage view for a file of a status section.
func NewStage(file, section string) *StageView {
	return &StageView{Path: file, Section: section}
}

func (v *StageView) Name() string { return "stage" }

func (v *StageView) Title(env *Env) string {
	switch v.Section {
	case secStaged:
		return "[stage] Staged changes to " + v.Path
	case secUntracked:
		return "[stage] Untracked file " + v.Path
	}
	return "[stage] Unstaged changes to " + v.Path
}

func (v *StageView) Load(env *Env) tea.Cmd {
	r := env.Git()
	root := env.WorktreeDir()
	file, section, ctxLines, ws := v.Path, v.Section, env.Opts.DiffContext, env.Opts.IgnoreSpace
	return v.load(func(ctx context.Context) (any, error) {
		if section == secUntracked {
			return readWorktreeFile(root, file)
		}
		text, err := git.DiffFile(ctx, r, file, section == secStaged, ctxLines, ws)
		if err != nil {
			return nil, err
		}
		return git.ParseFileDiff(text), nil
	})
}

func (v *StageView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	m, ok := msg.(loadedMsg)
	if !ok {
		return nil
	}
	v.finish(m.err)
	switch d := m.data.(type) {
	case *git.FileDiff:
		v.fd = d
		v.setLines(env, d.Lines, classifyDiff)
	case []string:
		v.fd = nil
		v.setLines(env, d, classifyPlain)
	default:
		v.fd = nil
		v.setLines(env, nil, classifyPlain)
	}
	if len(v.lines) == 0 && m.err == nil {
		// Like tig, close the stage view when nothing is left.
		return tea.Batch(Msg(CloseMsg{ID: v.ID}), Status("No more changes in "+v.Path))
	}
	return nil
}

func (v *StageView) Enter(env *Env) tea.Cmd { return v.scrollEnter() }

func (v *StageView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	gr := env.Git()
	reverse := v.Section == secStaged
	switch a {
	case keymap.StatusUpdate:
		if v.Section == secUntracked || v.fd == nil {
			return v.wholeFile(env), true
		}
		hi := v.fd.HunkAt(v.Cursor)
		if hi < 0 {
			return v.wholeFile(env), true
		}
		patch := v.fd.HunkPatch(hi)
		return applyPatch(gr, patch, reverse), true
	case keymap.StageUpdateLine:
		if v.fd == nil {
			return Status("Line staging needs a diff"), true
		}
		patch, err := v.fd.LinePatch(v.Cursor, reverse)
		if err != nil {
			return Errorf(err), true
		}
		return applyPatch(gr, patch, reverse), true
	case keymap.StageNext:
		if v.fd == nil {
			return nil, true
		}
		for _, h := range v.fd.Hunks {
			if h.Start > v.Cursor {
				v.SetCursor(h.Start, len(v.lines))
				v.Offset = h.Start
				v.Clamp(len(v.lines))
				return nil, true
			}
		}
		return Status("No more chunks"), true
	case keymap.DiffContextUp:
		env.Opts.DiffContext++
		return v.Load(env), true
	case keymap.DiffContextDown:
		if env.Opts.DiffContext > 0 {
			env.Opts.DiffContext--
		}
		return v.Load(env), true
	case keymap.StatusRevert:
		f := git.FileStatus{Path: v.Path, Kind: git.FileChanged}
		if v.Section == secUntracked {
			f.Kind = git.FileUntracked
		}
		return revertFile(env, f, v.Section), true
	case keymap.StatusCommit:
		return GitExec(env, "commit"), true
	case keymap.Edit:
		_, line := lineRef(v.lines, v.Cursor)
		if v.Section == secUntracked {
			line = v.Cursor + 1
		}
		return EditorCmd(env, v.Path, line), true
	}
	return nil, false
}

func (v *StageView) wholeFile(env *Env) tea.Cmd {
	gr := env.Git()
	file, staged := v.Path, v.Section == secStaged
	return func() tea.Msg {
		var err error
		if staged {
			err = git.UnstageFile(context.Background(), gr, file)
		} else {
			err = git.StageFile(context.Background(), gr, file)
		}
		if err != nil {
			return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
		}
		return ReloadMsg{}
	}
}

func applyPatch(gr git.Runner, patch string, reverse bool) tea.Cmd {
	return func() tea.Msg {
		if err := git.ApplyCached(context.Background(), gr, patch, reverse); err != nil {
			return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
		}
		return ReloadMsg{}
	}
}

func (v *StageView) Selection(env *Env) Selection {
	_, line := lineRef(v.lines, v.Cursor)
	return Selection{File: v.Path, Line: line}
}
