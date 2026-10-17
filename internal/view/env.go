// Package view implements knot's tig-style views.
package view

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// Options are runtime display toggles, initialized from the configuration.
type Options struct {
	LineNumbers   bool
	Date          string // off | absolute | relative | relative-compact | custom
	Author        string // off | full | abbreviated | email | email-user
	Graph         bool
	ASCII         bool // line graphics
	Refs          bool
	ID            bool
	IgnoreSpace   bool
	FileFilter    bool
	TitleOverflow bool
	DiffContext   int
}

// NewOptions builds options from the configuration.
func NewOptions(cfg *config.Config) *Options {
	id := false
	for _, c := range cfg.Main.Columns {
		if c == "commit-id" {
			id = true
		}
	}
	return &Options{
		LineNumbers:   cfg.UI.ShowLineNumbers,
		Date:          cfg.Main.DateFormat,
		Author:        cfg.Main.Author,
		Graph:         cfg.Main.Graph,
		ASCII:         cfg.UI.LineGraphics == "ascii",
		Refs:          true,
		ID:            id,
		IgnoreSpace:   cfg.Diff.IgnoreWhitespace,
		FileFilter:    true,
		TitleOverflow: cfg.UI.TitleOverflow > 0,
		DiffContext:   cfg.Diff.Context,
	}
}

// Env is the context every view runs against. Switching worktree replaces
// Loc, Head, Refs and Decor.
type Env struct {
	Repo     *git.Repo
	Loc      git.Location
	Head     git.Head
	Cfg      *config.Config
	Theme    *theme.Theme
	Keys     *keymap.Keymap
	Opts     *Options
	Refs     []git.Ref
	Decor    map[string][]git.Ref
	Revs     []string // revisions from the command line
	Paths    []string // paths from the command line
	Version  string
	HomeDir  string
	ThemeDir string
}

// Git returns a runner for the active worktree.
func (e *Env) Git() git.Runner {
	return e.Repo.Runner(e.Loc)
}

// WorktreeDir is the active worktree root (git dir for bare repositories).
func (e *Env) WorktreeDir() string {
	if e.Loc.TopLevel != "" {
		return e.Loc.TopLevel
	}
	return e.Loc.GitDir
}

// WorktreeName is the display name of the active worktree.
func (e *Env) WorktreeName() string {
	return filepath.Base(e.WorktreeDir())
}

// ReloadRefs refreshes HEAD and references.
func (e *Env) ReloadRefs(ctx context.Context) error {
	r := e.Git()
	e.Head = git.ReadHead(ctx, r)
	refs, err := git.ListRefs(ctx, r)
	if err != nil {
		return err
	}
	e.Refs = refs
	e.Decor = git.Decorations(refs)
	return nil
}

// Abbrev shortens a path below the home directory with ~.
func (e *Env) Abbrev(p string) string {
	if e.HomeDir != "" && (p == e.HomeDir || strings.HasPrefix(p, e.HomeDir+string(filepath.Separator))) {
		return "~" + p[len(e.HomeDir):]
	}
	return p
}

// Selection describes what is under the cursor; it feeds placeholders and
// view switching.
type Selection struct {
	Commit    string
	Ref       *git.Ref
	Stash     string
	File      string
	Line      int
	Directory string
	Worktree  string
	Rev       string // revision the file belongs to ("" = worktree)
}

// Messages sent from views to the application.

// OpenMsg asks the app to open a view. Split opens it as a child of the
// focused view; Replace replaces the focused view.
type OpenMsg struct {
	View    View
	Split   bool
	Replace bool
}

// StatusMsg shows a message in the status bar.
type StatusMsg struct {
	Text string
	Err  bool
}

// ConfirmMsg asks a yes/no question and runs OnYes when confirmed.
type ConfirmMsg struct {
	Question string
	OnYes    tea.Cmd
}

// PromptMsg asks for a line of input.
type PromptMsg struct {
	Label    string
	Initial  string
	OnSubmit func(string) tea.Cmd
}

// ReloadMsg reloads all open views (after the repository changed).
type ReloadMsg struct{}

// CloseMsg closes the view with the given id.
type CloseMsg struct{ ID int }

// SwitchWorktreeMsg switches the active worktree.
type SwitchWorktreeMsg struct{ Path string }

// PreviewThemeMsg previews or applies a theme. Restore discards a preview.
type PreviewThemeMsg struct {
	Name    string
	Restore bool
}

// ExecMsg runs a process with the terminal (TUI suspended), then reloads.
type ExecMsg struct {
	Cmd *exec.Cmd
}

// Status returns a command that shows a status message.
func Status(text string) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: text} }
}

// Errorf returns a command that shows an error.
func Errorf(err error) tea.Cmd {
	return func() tea.Msg { return StatusMsg{Text: err.Error(), Err: true} }
}

// Msg wraps a message into a command.
func Msg(m tea.Msg) tea.Cmd {
	return func() tea.Msg { return m }
}

// Confirm asks the user to confirm action a unless confirmation is disabled.
func Confirm(env *Env, a keymap.Action, question string, onYes tea.Cmd) tea.Cmd {
	if v, ok := env.Cfg.Confirm[string(a)]; ok && !v {
		return onYes
	}
	return Msg(ConfirmMsg{Question: question, OnYes: onYes})
}

// RunGit runs a modifying git command, then reports and reloads.
func RunGit(env *Env, done string, args ...string) tea.Cmd {
	r := env.Git()
	return func() tea.Msg {
		if _, err := r.Run(context.Background(), args...); err != nil {
			return tea.BatchMsg{Errorf(err), Msg(ReloadMsg{})}
		}
		return tea.BatchMsg{Status(done), Msg(ReloadMsg{})}
	}
}

// EditorCmd builds a command that opens file at line in $VISUAL/$EDITOR.
func EditorCmd(env *Env, file string, line int) tea.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	argv := strings.Fields(editor)
	if line > 0 {
		argv = append(argv, "+"+strconv.Itoa(line))
	}
	argv = append(argv, file)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = env.WorktreeDir()
	return Msg(ExecMsg{Cmd: cmd})
}

// GitExec runs an interactive git command (commit, mergetool) with the
// terminal.
func GitExec(env *Env, args ...string) tea.Cmd {
	cmd := env.Git().Command(context.Background(), false, args...)
	cmd.Dir = env.WorktreeDir()
	return Msg(ExecMsg{Cmd: cmd})
}
