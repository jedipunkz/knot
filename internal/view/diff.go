package view

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// DiffView shows a commit (message, diffstat, patch) or worktree changes.
type DiffView struct {
	textView
	Commit string
	Paths  []string // file filter (toggle with %)
	// Worktree diffs: Cached selects index vs HEAD.
	Worktree bool
	Cached   bool
	// Stash shows a stash entry instead of a commit.
	Stash string
	Label string
}

// NewDiff shows a commit.
func NewDiff(commit string, paths []string) *DiffView {
	return &DiffView{Commit: commit, Paths: paths}
}

// NewDiffWorktree shows unstaged (or staged) changes.
func NewDiffWorktree(cached bool) *DiffView {
	return &DiffView{Worktree: true, Cached: cached}
}

func (v *DiffView) Name() string { return "diff" }

func (v *DiffView) Title(env *Env) string {
	switch {
	case v.Label != "":
		return "[diff] " + v.Label
	case v.Worktree && v.Cached:
		return "[diff] Staged changes"
	case v.Worktree:
		return "[diff] Unstaged changes"
	}
	t := "[diff] " + Short(v.Commit, 10)
	if len(v.Paths) > 0 && env.Opts.FileFilter {
		t += " -- " + strings.Join(v.Paths, " ")
	}
	return t
}

func (v *DiffView) args(env *Env) []string {
	ctxArg := "-U" + strconv.Itoa(env.Opts.DiffContext)
	var args []string
	switch {
	case v.Stash != "":
		args = []string{"stash", "show", "--no-color", "--no-ext-diff", "-p", "--stat", "-M", ctxArg}
		if env.Opts.IgnoreSpace {
			args = append(args, "-w")
		}
		return append(args, v.Stash)
	case v.Worktree:
		args = []string{"diff", "--no-color", "--no-ext-diff", "--patch-with-stat", "-M", ctxArg}
		if v.Cached {
			args = append(args, "--cached")
		}
	default:
		args = []string{"show", "--no-color", "--no-ext-diff", "--patch-with-stat", "--format=fuller", "--no-decorate", "-M", "--root", ctxArg, v.Commit}
	}
	if env.Opts.IgnoreSpace {
		args = append(args, "-w")
	}
	if len(v.Paths) > 0 && env.Opts.FileFilter {
		args = append(args, "--")
		args = append(args, v.Paths...)
	}
	return args
}

// Load streams the diff line by line with the chunked ingestion the log
// view uses, so a huge commit's patch renders progressively instead of
// buffering the whole output. The commit-header restyle waits for the done
// message: it must see the complete header block once, not each batch.
func (v *DiffView) Load(env *Env) tea.Cmd {
	return v.streamText(env, streamBatch, classifyDiff, v.args(env)...)
}

func (v *DiffView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	cmd, handled := v.handleChunk(env, msg, classifyDiff)
	if !handled {
		return nil
	}
	if v.Commit == "" {
		return cmd
	}
	if m, ok := msg.(textChunkMsg); ok && m.done && !v.stale(m.gen) {
		v.restyleCommitHeader(env)
	}
	return cmd
}

// headerFields are the labeled fields of git show --format=fuller output,
// the theme role of their value and the dim variant of that value used as
// the label color.
type headerField struct {
	value theme.Role
	label theme.Role
}

var headerFields = map[string]headerField{
	"Merge":      {theme.CommitID, theme.CommitIDDim},
	"Author":     {theme.Author, theme.AuthorDim},
	"AuthorDate": {theme.Date, theme.DateDim},
	"Commit":     {theme.Author, theme.AuthorDim},
	"CommitDate": {theme.Date, theme.DateDim},
}

// isCommitHeaderLine reports whether l belongs to the git show
// --format=fuller header block.
func isCommitHeaderLine(l string) bool {
	if strings.HasPrefix(l, "commit ") {
		return true
	}
	field, _, ok := strings.Cut(l, ":")
	if !ok {
		return false
	}
	_, ok = headerFields[field]
	return ok
}

// headerLabelWidth aligns the header field labels; "CommitDate:" is the
// longest one.
const headerLabelWidth = 12

// restyleCommitHeader rebuilds the git show header block of a commit diff
// as segmented theme-colored rows in the order commit, Refs, Merge,
// Author, AuthorDate, Commit, CommitDate. Refs are drawn like the main
// view's ref decorations; everything below the header (message, diffstat,
// patch) is untouched.
func (v *DiffView) restyleCommitHeader(env *Env) {
	t := &v.textView
	if len(t.lines) == 0 || !strings.HasPrefix(t.lines[0], "commit ") {
		return
	}
	end := 1
	for end < len(t.lines) && isCommitHeaderLine(t.lines[end]) {
		end++
	}
	if end >= len(t.lines) || t.lines[end] != "" {
		return // not the expected header block
	}
	fields := strings.SplitN(t.lines[0], " ", 2)
	if len(fields) != 2 {
		return
	}
	sha := fields[1]

	var lines []string
	var roles []theme.Role
	var rows [][]Seg
	add := func(field string, labelRole theme.Role, value string, valueSegs []Seg) {
		lines = append(lines, Fit(field+":", headerLabelWidth)+" "+value)
		roles = append(roles, theme.Default)
		rows = append(rows, append([]Seg{{labelRole, Fit(field+":", headerLabelWidth)}}, valueSegs...))
	}
	add("commit", theme.CommitIDDim, sha, []Seg{{theme.CommitID, sha}})
	refs := refSegs(env, sha, nil)
	var refText strings.Builder
	for _, s := range refs {
		refText.WriteString(s.Text)
	}
	add("Refs", theme.RefDim, refText.String(), refs)
	for _, l := range t.lines[1:end] {
		field, value, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		hf := headerFields[field]
		if hf.value == "" {
			hf = headerField{theme.DiffMeta, theme.LineNumber}
		}
		add(field, hf.label, value, []Seg{{hf.value, value}})
	}
	t.lines = append(lines, t.lines[end:]...)
	t.roles = append(roles, t.roles[end:]...)
	t.rows = append(rows, t.rows[end:]...)
}

// Enter on a diffstat line jumps to the file's diff; otherwise it scrolls.
func (v *DiffView) Enter(env *Env) tea.Cmd {
	if v.Cursor < len(v.lines) {
		if m := statLine.FindStringSubmatch(v.lines[v.Cursor]); m != nil {
			name := strings.TrimSpace(m[1])
			for i, l := range v.lines {
				if strings.HasPrefix(l, "diff --git ") && (strings.HasSuffix(l, "/"+name) || strings.Contains(l, " b/"+name)) {
					v.SetCursor(i, len(v.lines))
					v.Offset = i
					v.Clamp(len(v.lines))
					return nil
				}
			}
		}
	}
	return v.scrollEnter()
}

func (v *DiffView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) {
	switch a {
	case keymap.DiffContextUp:
		env.Opts.DiffContext++
		return v.Load(env), true
	case keymap.DiffContextDown:
		if env.Opts.DiffContext > 0 {
			env.Opts.DiffContext--
		}
		return v.Load(env), true
	case keymap.Edit:
		file, line := lineRef(v.lines, v.Cursor)
		if file == "" {
			return Status("No file under the cursor"), true
		}
		return EditorCmd(env, file, line), true
	}
	return nil, false
}

func (v *DiffView) Selection(env *Env) Selection {
	file, line := lineRef(v.lines, v.Cursor)
	sel := Selection{Commit: v.Commit, Rev: v.Commit, File: file, Line: line}
	if v.Worktree {
		sel.Rev = ""
	}
	return sel
}

// LogView shows full commit messages with diffstat, streamed in as the git
// log output arrives so huge histories render progressively.
type LogView struct {
	textView
	Revs  []string
	Paths []string
}

// NewLog creates a log view.
func NewLog(revs, paths []string) *LogView { return &LogView{Revs: revs, Paths: paths} }

func (v *LogView) Name() string { return "log" }

func (v *LogView) Title(env *Env) string {
	if len(v.Revs) > 0 {
		return "[log] " + strings.Join(v.Revs, " ")
	}
	return "[log] HEAD"
}

func (v *LogView) Load(env *Env) tea.Cmd {
	args := []string{"log", "--no-color", "--stat", "--decorate=short", "--format=medium"}
	args = append(args, env.Cfg.Git.LogArgs...)
	args = append(args, v.Revs...)
	if len(v.Paths) > 0 {
		args = append(append(args, "--"), v.Paths...)
	}
	return v.streamText(env, streamBatch, classifyDiff, args...)
}

func (v *LogView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd {
	if cmd, ok := v.handleChunk(env, msg, classifyDiff); ok {
		return cmd
	}
	return nil
}

func (v *LogView) commitAt(i int) string {
	for k := min(i, len(v.lines)-1); k >= 0; k-- {
		if f := strings.Fields(v.lines[k]); len(f) >= 2 && f[0] == "commit" && len(f[1]) >= 40 {
			return f[1]
		}
	}
	return ""
}

func (v *LogView) Enter(env *Env) tea.Cmd {
	if c := v.commitAt(v.Cursor); c != "" {
		return Msg(OpenMsg{View: NewDiff(c, v.Paths), Split: true})
	}
	return nil
}

func (v *LogView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) { return nil, false }

func (v *LogView) Selection(env *Env) Selection {
	c := v.commitAt(v.Cursor)
	return Selection{Commit: c, Rev: c}
}

// PagerView shows stdin or the output of a command.
type PagerView struct {
	textView
	Label   string
	Content []string
}

// NewPager shows fixed text.
func NewPager(label string, content []string) *PagerView {
	return &PagerView{Label: label, Content: content}
}

func (v *PagerView) Name() string          { return "pager" }
func (v *PagerView) Title(env *Env) string { return "[pager] " + v.Label }

func (v *PagerView) Load(env *Env) tea.Cmd {
	v.setLines(env, v.Content, classifyDiff)
	return nil
}

func (v *PagerView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd { return nil }

func (v *PagerView) Enter(env *Env) tea.Cmd {
	// Like tig, Enter on a "commit <id>" line opens the commit.
	if v.Cursor < len(v.lines) {
		if f := strings.Fields(v.lines[v.Cursor]); len(f) >= 2 && f[0] == "commit" && len(f[1]) >= 7 {
			return Msg(OpenMsg{View: NewDiff(f[1], nil), Split: true})
		}
	}
	return v.scrollEnter()
}

func (v *PagerView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) { return nil, false }
func (v *PagerView) Selection(env *Env) Selection                 { return Selection{} }
