// Package app is knot's root Bubble Tea model: view stack, split layout,
// status bar, prompt and action dispatch.
package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/theme"
	"github.com/jedipunkz/knot/internal/view"
)

type entry struct {
	v     view.View
	child bool // opened as a split child of the entry below
}

type prompt struct {
	label    string
	input    []rune
	onSubmit func(string) tea.Cmd
}

// filterBar holds the state of the fuzzy filter bar of the focused view.
type filterBar struct {
	input []rune
}

type panel struct {
	x, y, w, h int
	idx        int // index into the stack
}

// Model is the root model.
type Model struct {
	env         *view.Env
	stack       []entry
	focusParent bool
	maximized   bool
	width       int
	height      int
	nextID      int

	status      string
	statusErr   bool
	prompt      *prompt
	filterBar   *filterBar
	confirm     *view.ConfirmMsg
	frame       int // loading spinner frame
	spinRunning bool
	autoTheme   bool
	panels      []panel
	initCmds    []tea.Cmd
}

type execDoneMsg struct{ err error }

// New creates the model with the initial views.
func New(env *view.Env, initial []view.View, warnings []string) *Model {
	m := &Model{env: env, autoTheme: env.Cfg.Theme == "auto"}
	for _, v := range initial {
		m.initCmds = append(m.initCmds, m.push(v, false))
	}
	if len(warnings) > 0 {
		m.status = warnings[0]
		if len(warnings) > 1 {
			m.status += fmt.Sprintf(" (+%d more; run knot --check-config)", len(warnings)-1)
		}
		m.statusErr = true
	}
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	cmds := m.initCmds
	m.initCmds = nil
	if m.autoTheme {
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	return tea.Batch(cmds...)
}

func (m *Model) top() *entry {
	if len(m.stack) == 0 {
		return nil
	}
	return &m.stack[len(m.stack)-1]
}

func (m *Model) split() bool {
	t := m.top()
	return t != nil && t.child && !m.maximized && len(m.stack) >= 2
}

// focused returns the stack index of the focused view.
func (m *Model) focusedIdx() int {
	if m.split() && m.focusParent {
		return len(m.stack) - 2
	}
	return len(m.stack) - 1
}

func (m *Model) focused() view.View {
	if len(m.stack) == 0 {
		return nil
	}
	return m.stack[m.focusedIdx()].v
}

func (m *Model) push(v view.View, child bool) tea.Cmd {
	m.nextID++
	v.B().ID = m.nextID
	m.stack = append(m.stack, entry{v: v, child: child && len(m.stack) > 0})
	m.focusParent = false
	m.maximized = false
	// Views that ask for it start with the filter bar focused.
	if s, ok := v.(view.FilterStarter); ok && s.FilterOnOpen() {
		m.filterBar = &filterBar{}
	}
	return v.Load(m.env)
}

func (m *Model) closeTop() tea.Cmd {
	t := m.top()
	if t == nil {
		return nil
	}
	t.v.B().Stop()
	var cmd tea.Cmd
	if c, ok := t.v.(view.Closer); ok {
		cmd = c.Closed(m.env)
	}
	m.stack = m.stack[:len(m.stack)-1]
	m.focusParent = false
	m.maximized = false
	return cmd
}

func (m *Model) closeID(id int) tea.Cmd {
	for i := len(m.stack) - 1; i >= 0; i-- {
		if m.stack[i].v.B().ID == id {
			var cmds []tea.Cmd
			for len(m.stack) > i {
				cmds = append(cmds, m.closeTop())
			}
			if len(m.stack) == 0 {
				cmds = append(cmds, tea.Quit)
			}
			return tea.Batch(cmds...)
		}
	}
	return nil
}

// open handles OpenMsg.
func (m *Model) open(msg view.OpenMsg) tea.Cmd {
	var cmds []tea.Cmd
	switch {
	case msg.Split && m.split() && m.focusParent:
		// The parent asks again: replace the child.
		cmds = append(cmds, m.closeTop())
		cmds = append(cmds, m.push(msg.View, true))
	case msg.Replace && len(m.stack) > 0:
		child := m.top().child
		cmds = append(cmds, m.closeTop())
		cmds = append(cmds, m.push(msg.View, child))
	default:
		cmds = append(cmds, m.push(msg.View, msg.Split))
	}
	return tea.Batch(cmds...)
}

// bringToFront switches to an open view with the given name, closing the
// views above it. It returns false when no such view is open.
func (m *Model) bringToFront(name string) (tea.Cmd, bool) {
	for i := len(m.stack) - 1; i >= 0; i-- {
		if m.stack[i].v.Name() != name {
			continue
		}
		var cmds []tea.Cmd
		for len(m.stack) > i+1 {
			cmds = append(cmds, m.closeTop())
		}
		m.stack[i].child = m.stack[i].child && i > 0
		return tea.Batch(cmds...), true
	}
	return nil, false
}

func (m *Model) setStatus(text string, isErr bool) {
	m.status, m.statusErr = text, isErr
}

// spinnerInterval controls the loading spinner frame rate.
var spinnerInterval = 120 * time.Millisecond

// spinnerFrames animates the loading indicator; the ASCII variant is used
// when the user turned ASCII line graphics on.
var (
	spinnerFrames      = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinnerFramesASCII = []string{"|", "/", "-", "\\"}
)

// spinnerMsg advances the spinner while a view is loading.
type spinnerMsg struct{}

// anyLoading reports whether any open view is still loading.
func (m *Model) anyLoading() bool {
	for _, e := range m.stack {
		if e.v.B().Loading {
			return true
		}
	}
	return false
}

// spinner returns the current spinner frame.
func (m *Model) spinner() string {
	frames := spinnerFrames
	if m.env.Opts.ASCII {
		frames = spinnerFramesASCII
	}
	return frames[m.frame%len(frames)]
}

// spinTick schedules the next spinner frame.
func (m *Model) spinTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerMsg{} })
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(spinnerMsg); ok {
		if !m.anyLoading() {
			m.spinRunning = false
			return m, nil
		}
		m.frame++
		return m, m.spinTick()
	}
	model, cmd := m.update(msg)
	// Start the spinner when a message turned on a background load.
	if !m.spinRunning && m.anyLoading() {
		m.spinRunning = true
		cmd = tea.Batch(cmd, m.spinTick())
	}
	return model, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.BackgroundColorMsg:
		if m.autoTheme {
			name := m.env.Cfg.ThemeAuto.Light
			if msg.IsDark() {
				name = m.env.Cfg.ThemeAuto.Dark
			}
			m.applyTheme(name)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m, m.key(msg)
	case tea.PasteMsg:
		content := []rune(strings.ReplaceAll(msg.Content, "\n", " "))
		switch {
		case m.prompt != nil:
			m.prompt.input = append(m.prompt.input, content...)
		case m.filterBar != nil:
			m.filterBar.input = append(m.filterBar.input, content...)
			if f, ok := m.focused().(view.Filterable); ok {
				m.setFilter(f, string(m.filterBar.input))
			}
		}
		return m, nil
	case tea.MouseWheelMsg:
		return m, m.wheel(msg.Mouse())
	case tea.MouseClickMsg:
		return m, m.click(msg.Mouse())
	case view.OpenMsg:
		return m, m.open(msg)
	case view.StatusMsg:
		m.setStatus(msg.Text, msg.Err)
		return m, nil
	case view.ConfirmMsg:
		c := msg
		m.confirm = &c
		return m, nil
	case view.PromptMsg:
		m.prompt = &prompt{label: msg.Label, input: []rune(msg.Initial), onSubmit: msg.OnSubmit}
		return m, nil
	case view.ReloadMsg:
		return m, m.reloadAll()
	case view.CloseMsg:
		return m, m.closeID(msg.ID)
	case view.SwitchWorktreeMsg:
		return m, m.switchWorktree(msg.Path)
	case view.PreviewThemeMsg:
		m.applyTheme(msg.Name)
		return m, nil
	case view.OptionChangedMsg:
		return m, m.optionChanged(msg.Name)
	case view.ExecMsg:
		return m, tea.ExecProcess(msg.Cmd, func(err error) tea.Msg { return execDoneMsg{err: err} })
	case execDoneMsg:
		if msg.err != nil {
			m.setStatus(msg.err.Error(), true)
		}
		return m, m.reloadAll()
	case view.ViewMsg:
		for _, e := range m.stack {
			if e.v.B().ID == msg.ViewID() {
				return m, e.v.HandleMsg(m.env, msg)
			}
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) applyTheme(name string) {
	th, err := theme.Load(m.env.ThemeDir, name)
	if err != nil {
		m.setStatus(err.Error(), true)
		return
	}
	m.env.Theme = th
}

func (m *Model) reloadAll() tea.Cmd {
	if err := m.env.ReloadRefs(context.Background()); err != nil {
		m.setStatus(err.Error(), true)
	}
	cmds := make([]tea.Cmd, 0, len(m.stack))
	for _, e := range m.stack {
		cmds = append(cmds, e.v.Load(m.env))
	}
	return tea.Batch(cmds...)
}

func (m *Model) optionChanged(name string) tea.Cmd {
	var cmds []tea.Cmd
	for _, e := range m.stack {
		switch e.v.Name() {
		case "diff", "stage":
			if name == "ignore-space" || name == "diff-context" || name == "file-filter" {
				cmds = append(cmds, e.v.Load(m.env))
			}
		case "main":
			if name == "line-graphics" {
				cmds = append(cmds, e.v.Load(m.env))
			}
		}
	}
	return tea.Batch(cmds...)
}

// switchWorktree implements Enter in the worktree view.
func (m *Model) switchWorktree(path string) tea.Cmd {
	ctx := context.Background()
	repo, loc, err := git.Open(ctx, m.env.Cfg.Git.Binary, path)
	if err != nil {
		m.setStatus(err.Error(), true)
		return nil
	}
	if loc.TopLevel == "" {
		m.setStatus(fmt.Sprintf("%s is not a work tree", path), true)
		return nil
	}
	same := loc.TopLevel == m.env.Loc.TopLevel
	for len(m.stack) > 0 {
		m.closeTop()
	}
	if !same {
		m.env.Repo, m.env.Loc = repo, loc
		m.env.Revs, m.env.Paths = nil, nil
		if err := m.env.ReloadRefs(ctx); err != nil {
			m.setStatus(err.Error(), true)
		}
		m.setStatus("Switched to worktree "+m.env.Abbrev(loc.TopLevel), false)
	}
	return m.push(m.switchView(m.env.Cfg.Worktree.OnSwitch), false)
}

// switchView creates the view opened after a worktree switch.
func (m *Model) switchView(name string) view.View {
	switch name {
	case "status":
		return view.NewStatus()
	case "refs":
		return view.NewRefs()
	case "log":
		return view.NewLog(nil, nil)
	case "stash":
		return view.NewStash()
	case "reflog":
		return view.NewReflog(nil)
	case "tree":
		return view.NewTree("HEAD", "")
	case "worktree":
		return view.NewWorktree()
	case "help":
		return view.NewHelp()
	case "diff":
		return view.NewDiffWorktree(false)
	case "grep":
		return view.NewGrep(nil)
	}
	mv := view.NewMain(nil, nil)
	mv.SelectHead = true
	return mv
}

// Run starts the program.
func Run(m *Model, opts ...tea.ProgramOption) error {
	p := tea.NewProgram(m, opts...)
	_, err := p.Run()
	return err
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	var v tea.View
	v.AltScreen = true
	if m.env.Cfg.UI.Mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	v.WindowTitle = "knot - " + m.env.WorktreeName()
	if m.width == 0 || m.height == 0 {
		return v
	}
	content, cursor := m.render()
	v.SetContent(content)
	v.Cursor = cursor
	return v
}

// ConfigWarnings converts configuration problems to strings.
func ConfigWarnings(problems []config.Problem) []string {
	out := make([]string, 0, len(problems))
	for _, p := range problems {
		out = append(out, p.String())
	}
	return out
}
