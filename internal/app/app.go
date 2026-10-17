// Package app is knot's root Bubble Tea model: view stack, split layout,
// status bar, prompt and action dispatch.
package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
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

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		if k == "y" || k == "Y" {
			m.setStatus("", false)
			return c.OnYes
		}
		m.setStatus("Cancelled", false)
		return nil
	}
	if m.prompt != nil {
		return m.promptKey(msg)
	}
	if m.filterBar != nil {
		return m.filterKey(msg)
	}
	m.setStatus("", false)
	v := m.focused()
	if v == nil {
		return tea.Quit
	}
	a, ok := m.env.Keys.Lookup(v.Name(), k)
	if !ok {
		if k == "esc" {
			// Esc clears the filter of a filtered list.
			if f, ok := v.(view.Filterable); ok {
				if q, _, _, _ := f.FilterStatus(); q != "" {
					m.setFilter(f, "")
				}
			}
		}
		return nil
	}
	return m.dispatch(a)
}

func (m *Model) promptKey(msg tea.KeyPressMsg) tea.Cmd {
	p := m.prompt
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+g":
		m.prompt = nil
		m.setStatus("Cancelled", false)
		return nil
	case "enter":
		m.prompt = nil
		return p.onSubmit(string(p.input))
	default:
		editInput(&p.input, msg)
	}
	return nil
}

// editInput applies the line-editing keys shared by the prompt and the
// filter bar. It reports whether the input changed.
func editInput(input *[]rune, msg tea.KeyPressMsg) bool {
	switch msg.String() {
	case "backspace", "ctrl+h":
		if len(*input) > 0 {
			*input = (*input)[:len(*input)-1]
			return true
		}
	case "ctrl+u":
		if len(*input) > 0 {
			*input = nil
			return true
		}
	case "ctrl+w":
		s := strings.TrimRight(string(*input), " ")
		if i := strings.LastIndex(s, " "); i >= 0 {
			*input = []rune(s[:i+1])
		} else {
			*input = nil
		}
		return true
	case "space":
		*input = append(*input, ' ')
		return true
	default:
		if msg.Text != "" && msg.Mod&^tea.ModShift == 0 {
			*input = append(*input, []rune(msg.Text)...)
			return true
		}
	}
	return false
}

// enterFilter focuses the filter bar of the focused filterable view,
// showing the current query if one is active.
func (m *Model) enterFilter(f view.Filterable) tea.Cmd {
	q, _, _, _ := f.FilterStatus()
	m.filterBar = &filterBar{input: []rune(q)}
	return nil
}

// setFilter applies q to a filterable view.
func (m *Model) setFilter(f view.Filterable, q string) {
	f.SetFilter(m.env, q)
}

// filterKey handles keys while the filter bar is focused.
func (m *Model) filterKey(msg tea.KeyPressMsg) tea.Cmd {
	f, ok := m.focused().(view.Filterable)
	if !ok {
		m.filterBar = nil
		return nil
	}
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+g":
		// Clear the query and restore the full list; the cursor moves back
		// to the commit selected before entering (kept by SetFilter).
		m.filterBar = nil
		m.setStatus("", false)
		m.setFilter(f, "")
	case "enter", "ctrl+n", "down":
		// Focus the list with the filter kept, cursor on the first match.
		m.filterBar = nil
		f.B().SetCursor(0, f.Len())
	case "ctrl+p", "up":
		// No-op inside the bar.
	default:
		if editInput(&m.filterBar.input, msg) {
			m.setFilter(f, string(m.filterBar.input))
		}
	}
	return nil
}

// selectionViews are opened for the selection rather than brought to front.
var listViews = map[keymap.Action]func(*Model) view.View{
	keymap.ViewMain:     func(m *Model) view.View { return view.NewMain(m.env.Revs, m.env.Paths) },
	keymap.ViewLog:      func(m *Model) view.View { return view.NewLog(m.env.Revs, m.env.Paths) },
	keymap.ViewReflog:   func(m *Model) view.View { return view.NewReflog(nil) },
	keymap.ViewRefs:     func(m *Model) view.View { return view.NewRefs() },
	keymap.ViewStash:    func(m *Model) view.View { return view.NewStash() },
	keymap.ViewStatus:   func(m *Model) view.View { return view.NewStatus() },
	keymap.ViewHelp:     func(m *Model) view.View { return view.NewHelp() },
	keymap.ViewWorktree: func(m *Model) view.View { return view.NewWorktree() },
	keymap.ViewGrep:     func(m *Model) view.View { return view.NewGrep(nil) },
}

var listViewNames = map[keymap.Action]string{
	keymap.ViewMain: "main", keymap.ViewLog: "log", keymap.ViewReflog: "reflog", keymap.ViewRefs: "refs",
	keymap.ViewStash: "stash", keymap.ViewStatus: "status", keymap.ViewHelp: "help", keymap.ViewWorktree: "worktree",
	keymap.ViewGrep: "grep", keymap.ViewPager: "pager",
}

func (m *Model) dispatch(a keymap.Action) tea.Cmd {
	env := m.env
	v := m.focused()
	if strings.HasPrefix(string(a), keymap.CommandPrefix) {
		i, err := strconv.Atoi(strings.TrimPrefix(string(a), keymap.CommandPrefix))
		if err != nil || i < 0 || i >= len(env.Cfg.Commands) {
			return view.Errorf(fmt.Errorf("unknown command %s", a))
		}
		return m.runCommand(env.Cfg.Commands[i], v.Selection(env))
	}
	if cmd, ok := v.Do(env, a); ok {
		return cmd
	}
	sel := v.Selection(env)

	if name, ok := listViewNames[a]; ok {
		if cmd, ok := m.bringToFront(name); ok {
			return cmd
		}
		if a == keymap.ViewPager {
			return view.Status("No pager content")
		}
		return m.push(listViews[a](m), false)
	}

	switch a {
	case keymap.ViewDiff:
		if sel.Commit == "" {
			return view.Status("No commit selected")
		}
		return m.open(view.OpenMsg{View: view.NewDiff(sel.Commit, nil), Split: true})
	case keymap.ViewTree:
		rev := sel.Rev
		if rev == "" {
			rev = "HEAD"
		}
		return m.push(view.NewTree(rev, sel.Directory), false)
	case keymap.ViewBlob:
		if sel.File == "" {
			return view.Status("No file selected")
		}
		return m.open(view.OpenMsg{View: view.NewBlob(sel.Rev, sel.File, sel.Line), Split: true})
	case keymap.ViewBlame:
		if sel.File == "" {
			return view.Status("No file selected")
		}
		return m.push(view.NewBlame(sel.Rev, sel.File, sel.Line), false)
	case keymap.ViewStage:
		if v.Name() == "status" {
			return v.Enter(env)
		}
		if _, ok := m.bringToFront("stage"); ok {
			return nil
		}
		return view.Status("Open a file from the status view (s) first")
	case keymap.ViewClose:
		cmd := m.closeTop()
		if len(m.stack) == 0 {
			return tea.Batch(cmd, tea.Quit)
		}
		return cmd
	case keymap.Quit:
		for len(m.stack) > 0 {
			m.closeTop()
		}
		return tea.Quit
	case keymap.Enter:
		return v.Enter(env)
	case keymap.ViewNext:
		if m.split() {
			m.focusParent = !m.focusParent
		}
		return nil
	case keymap.Refresh:
		if err := env.ReloadRefs(context.Background()); err != nil {
			return view.Errorf(err)
		}
		return v.Load(env)
	case keymap.Maximize:
		if m.split() {
			if m.focusParent {
				// Maximize the parent: close the child.
				return m.closeTop()
			}
			m.maximized = true
		}
		return nil
	case keymap.Previous, keymap.Next:
		if m.split() && !m.focusParent {
			parent := m.stack[len(m.stack)-2].v
			view.Navigate(parent, a)
			// The parent's Enter replaces the child (see open).
			m.focusParent = true
			cmd := parent.Enter(env)
			if cmd == nil {
				m.focusParent = false
			}
			return cmd
		}
		if a == keymap.Previous {
			// Previous on the first row of a filterable view focuses the
			// filter bar (Ctrl-P / Up).
			if f, ok := v.(view.Filterable); ok && v.B().Cursor == 0 {
				return m.enterFilter(f)
			}
		}
		return m.navigate(v, a)
	case keymap.MoveUp, keymap.MoveDown, keymap.MovePageUp, keymap.MovePageDown, keymap.MoveHalfPageUp,
		keymap.MoveHalfPageDown, keymap.MoveFirstLine, keymap.MoveLastLine, keymap.ScrollLineUp,
		keymap.ScrollLineDown, keymap.ScrollLeft, keymap.ScrollRight, keymap.ScrollFirstCol:
		return m.navigate(v, a)
	case keymap.Search, keymap.SearchBack:
		dir := 1
		label := "/"
		if a == keymap.SearchBack {
			dir, label = -1, "?"
		}
		// Open the prompt synchronously so the following keystrokes go to it.
		m.prompt = &prompt{label: label, onSubmit: func(s string) tea.Cmd {
			if err := v.B().SetSearch(s); err != nil {
				return view.Errorf(err)
			}
			if s == "" {
				return nil
			}
			return m.find(v, dir)
		}}
		return nil
	case keymap.FindNext:
		return m.find(v, 1)
	case keymap.FindPrev:
		return m.find(v, -1)
	case keymap.Filter:
		if f, ok := v.(view.Filterable); ok {
			return m.enterFilter(f)
		}
		return view.Status(fmt.Sprintf("filter is not available in the %s view", v.Name()))
	case keymap.Prompt:
		m.prompt = &prompt{label: ":", onSubmit: m.runPrompt}
		return nil
	case keymap.ToggleLineNo:
		env.Opts.LineNumbers = !env.Opts.LineNumbers
	case keymap.ToggleDate:
		view.CycleDate(env.Opts)
		return view.Status("Date display: " + env.Opts.Date)
	case keymap.ToggleAuthor:
		view.CycleAuthor(env.Opts)
		return view.Status("Author display: " + env.Opts.Author)
	case keymap.ToggleGraph:
		env.Opts.Graph = !env.Opts.Graph
	case keymap.ToggleLineGraphics:
		env.Opts.ASCII = !env.Opts.ASCII
		return m.optionChanged("line-graphics")
	case keymap.ToggleRefs:
		env.Opts.Refs = !env.Opts.Refs
	case keymap.ToggleID:
		env.Opts.ID = !env.Opts.ID
	case keymap.ToggleIgnoreSpace:
		env.Opts.IgnoreSpace = !env.Opts.IgnoreSpace
		return tea.Batch(view.Status("Ignore whitespace: "+onOff(env.Opts.IgnoreSpace)), m.optionChanged("ignore-space"))
	case keymap.ToggleFileFilter:
		env.Opts.FileFilter = !env.Opts.FileFilter
		return tea.Batch(view.Status("File filter: "+onOff(env.Opts.FileFilter)), m.optionChanged("file-filter"))
	case keymap.ToggleTitleOverflow:
		env.Opts.TitleOverflow = !env.Opts.TitleOverflow
	case keymap.DiffContextUp, keymap.DiffContextDown:
		if a == keymap.DiffContextUp {
			env.Opts.DiffContext++
		} else if env.Opts.DiffContext > 0 {
			env.Opts.DiffContext--
		}
		return tea.Batch(view.Status("Diff context: "+strconv.Itoa(env.Opts.DiffContext)), m.optionChanged("diff-context"))
	case keymap.Options:
		return m.push(view.NewOptionsView(), false)
	case keymap.ThemePicker:
		return m.push(view.NewThemeView(), false)
	case keymap.StopLoading:
		for _, e := range m.stack {
			e.v.B().Stop()
		}
		return view.Status("Stopped loading")
	case keymap.ShowVersion:
		return view.Status("knot " + env.Version)
	case keymap.ScreenRedraw:
		return tea.ClearScreen
	case keymap.Edit:
		if sel.File == "" {
			return view.Status("No file selected")
		}
		return view.EditorCmd(env, sel.File, sel.Line)
	case keymap.Parent:
		return view.Status("This view has no parent")
	case keymap.MoveHead:
		return view.Status("HEAD is not in this view")
	default:
		if keymap.Valid(a) {
			return view.Status(fmt.Sprintf("%s is not available in the %s view", a, v.Name()))
		}
	}
	return nil
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (m *Model) navigate(v view.View, a keymap.Action) tea.Cmd {
	view.Navigate(v, a)
	if w, ok := v.(view.CursorWatcher); ok {
		return w.CursorMoved(m.env)
	}
	return nil
}

func (m *Model) find(v view.View, dir int) tea.Cmd {
	if err := view.Find(m.env, v, dir); err != nil {
		return view.Errorf(err)
	}
	if w, ok := v.(view.CursorWatcher); ok {
		return w.CursorMoved(m.env)
	}
	return nil
}

// runPrompt executes a ':' command.
func (m *Model) runPrompt(s string) tea.Cmd {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return nil
	case s == "q" || s == "quit":
		return m.dispatch(keymap.Quit)
	case strings.HasPrefix(s, "!"):
		argv, err := splitArgs(strings.TrimSpace(s[1:]))
		if err != nil {
			return view.Errorf(err)
		}
		if len(argv) == 0 {
			return nil
		}
		return m.runCaptured(argv, "pager", strings.Join(argv, " "))
	}
	if n, err := strconv.Atoi(s); err == nil {
		v := m.focused()
		v.B().SetCursor(n-1, v.Len())
		return nil
	}
	if a := keymap.Action(s); keymap.Valid(a) {
		return m.dispatch(a)
	}
	return view.Errorf(fmt.Errorf("unknown command %q", s))
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

func (m *Model) render() (string, *tea.Cursor) {
	th := m.env.Theme
	w, h := m.width, m.height
	avail := max(h-1, 2)
	m.panels = m.panels[:0]
	var body string
	var barCur *tea.Cursor
	if len(m.stack) == 0 {
		body = strings.Repeat("\n", avail-1)
	} else if m.split() {
		pi, ci := len(m.stack)-2, len(m.stack)-1
		ratio := m.env.Cfg.UI.SplitRatio
		dir := m.env.Cfg.UI.SplitDirection
		if dir == "auto" {
			dir = "horizontal"
			if w >= 160 {
				dir = "vertical"
			}
		}
		if dir == "vertical" {
			cw := int(float64(w) * ratio)
			pw := w - cw - 1
			left, c1 := m.renderPanel(pi, 0, 0, pw, avail)
			right, c2 := m.renderPanel(ci, pw+1, 0, cw, avail)
			barCur = c1
			if c2 != nil {
				barCur = c2
			}
			sep := th.Render(theme.TitleBlur, "│")
			if m.env.Opts.ASCII {
				sep = th.Render(theme.TitleBlur, "|")
			}
			l, r := strings.Split(left, "\n"), strings.Split(right, "\n")
			var sb strings.Builder
			for i := 0; i < avail; i++ {
				if i > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(view.Fit(l[i], pw) + sep + r[i])
			}
			body = sb.String()
		} else {
			ch := max(int(float64(avail)*ratio), 2)
			ph := avail - ch
			if ph < 2 {
				ph, ch = 2, avail-2
			}
			top, c1 := m.renderPanel(pi, 0, 0, w, ph)
			bottom, c2 := m.renderPanel(ci, 0, ph, w, ch)
			body = top + "\n" + bottom
			barCur = c1
			if c2 != nil {
				barCur = c2
			}
		}
	} else {
		var c *tea.Cursor
		body, c = m.renderPanel(len(m.stack)-1, 0, 0, w, avail)
		barCur = c
	}
	line, cursor := m.statusLine(w, h-1)
	if barCur != nil {
		cursor = barCur
	}
	return body + "\n" + line, cursor
}

// renderPanel draws a view with its filter bar (filterable views show it
// permanently, dimmed when unfocused) above the content and its title bar
// below the content.
func (m *Model) renderPanel(idx, x, y, w, h int) (string, *tea.Cursor) {
	v := m.stack[idx].v
	var cur *tea.Cursor
	bar := ""
	barH := 0
	if f, ok := v.(view.Filterable); ok {
		barH = 1
		bar, cur = m.filterBarRow(f, x, y, w, idx == m.focusedIdx())
	}
	m.panels = append(m.panels, panel{x: x, y: y + barH, w: w, h: h - 1 - barH, idx: idx})
	content := view.Render(m.env, v, w, h-1-barH)
	role := theme.TitleBlur
	if idx == m.focusedIdx() {
		role = theme.TitleFocus
	}
	title := v.Title(m.env)
	if v.B().Loading {
		title += "  " + m.spinner() + " Loading"
	}
	pos := view.Position(v)
	gap := w - ansi.StringWidth(title) - ansi.StringWidth(pos) - 2
	var bar2 string
	if gap >= 1 {
		bar2 = " " + title + strings.Repeat(" ", gap) + pos + " "
	} else {
		bar2 = view.Fit(" "+title, w)
	}
	parts := []string{content, m.env.Theme.Render(role, view.Fit(bar2, w))}
	if bar != "" {
		parts = append([]string{bar}, parts...)
	}
	return strings.Join(parts, "\n"), cur
}

// filterBarRow renders the one-row filter bar shown at the top of a
// filterable view's panel. It is always visible: dim while unfocused, with
// the "<matches>/<loaded>" count on the right ("…" appended while loading)
// and the text cursor while the bar is focused.
func (m *Model) filterBarRow(f view.Filterable, x, y, w int, focused bool) (string, *tea.Cursor) {
	th := m.env.Theme
	labelRole, inputRole, countRole := theme.LineNumber, theme.LineNumber, theme.LineNumber
	// Edited means the bar holds the focus (the text cursor is drawn).
	edited := focused && m.filterBar != nil
	if edited {
		labelRole, inputRole = theme.Prompt, theme.Default
	}
	q, matches, loaded, loading := f.FilterStatus()
	input := q
	if focused && m.filterBar != nil {
		input = string(m.filterBar.input)
	}
	right := fmt.Sprintf("%d/%d", matches, loaded)
	if loading {
		right += "…"
	}
	label := " Search:"
	in := input
	if input == "" && !edited {
		in = "(Ctrl-P to filter)"
	}
	in = " " + in
	gap := w - ansi.StringWidth(label) - ansi.StringWidth(in) - ansi.StringWidth(right) - 2
	var row string
	switch {
	case gap >= 1:
		row = th.Render(labelRole, label) + th.Render(inputRole, in) +
			strings.Repeat(" ", gap) + th.Render(countRole, right) + " "
	default:
		text := th.Render(labelRole, label) + th.Render(inputRole, in)
		curLen := ansi.StringWidth(text)
		if curLen < w {
			text = strings.TrimRight(ansi.Truncate(text, w-1, "…"), " ")
		} else {
			text = ansi.Truncate(text, w-1, "…")
		}
		row = text + strings.Repeat(" ", max(w-ansi.StringWidth(text), 0))
	}
	var cur *tea.Cursor
	if edited {
		cx := x + ansi.StringWidth(label) + ansi.StringWidth(in)
		cur = tea.NewCursor(min(cx, x+w-1), y)
	}
	return row, cur
}

func (m *Model) statusLine(w, y int) (string, *tea.Cursor) {
	th := m.env.Theme
	switch {
	case m.prompt != nil:
		text := m.prompt.label + string(m.prompt.input)
		vis := text
		if ansi.StringWidth(vis) >= w {
			vis = ansi.TruncateLeft(vis, ansi.StringWidth(vis)-w+1, "")
		}
		return th.Render(theme.Prompt, view.Fit(vis, w)), tea.NewCursor(min(ansi.StringWidth(vis), w-1), y)
	case m.confirm != nil:
		return th.Render(theme.Warning, view.Fit(m.confirm.Question+" [y/N]", w)), nil
	}
	right := th.Render(theme.LineNumber, m.env.WorktreeName())
	switch {
	case m.env.Head.Branch != "":
		right += " " + th.Render(theme.RefHead, "["+m.env.Head.Branch+"]")
	case m.env.Head.Hash != "":
		right += " " + th.Render(theme.RefHead, "[detached "+view.Short(m.env.Head.Hash, 7)+"]")
	}
	left := m.status
	role := theme.StatusBar
	if m.statusErr {
		role = theme.Error
	}
	room := w - ansi.StringWidth(right) - 1
	if room < 1 {
		return th.Render(role, view.Fit(left, w)), nil
	}
	return th.Render(role, view.Fit(left, room)) + " " + th.Render(theme.RefHead, right), nil
}

func (m *Model) panelAt(x, y int) (panel, bool) {
	for _, p := range m.panels {
		if x >= p.x && x < p.x+p.w && y >= p.y && y < p.y+p.h {
			return p, true
		}
	}
	return panel{}, false
}

func (m *Model) focusIdx(idx int) {
	if m.split() {
		m.focusParent = idx == len(m.stack)-2
	}
}

func (m *Model) wheel(ms tea.Mouse) tea.Cmd {
	p, ok := m.panelAt(ms.X, ms.Y)
	if !ok || m.prompt != nil || m.confirm != nil {
		return nil
	}
	v := m.stack[p.idx].v
	a := keymap.ScrollLineDown
	if ms.Button == tea.MouseWheelUp {
		a = keymap.ScrollLineUp
	}
	for i := 0; i < 3; i++ {
		view.Navigate(v, a)
	}
	return nil
}

func (m *Model) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft || m.prompt != nil || m.confirm != nil {
		return nil
	}
	p, ok := m.panelAt(ms.X, ms.Y)
	if !ok {
		return nil
	}
	m.focusIdx(p.idx)
	v := m.stack[p.idx].v
	b := v.B()
	line := b.Offset + ms.Y - p.y
	if line >= v.Len() {
		return nil
	}
	if line == b.Cursor {
		return v.Enter(m.env)
	}
	b.SetCursor(line, v.Len())
	if w, ok := v.(view.CursorWatcher); ok {
		return w.CursorMoved(m.env)
	}
	return nil
}

// ConfigWarnings converts configuration problems to strings.
func ConfigWarnings(problems []config.Problem) []string {
	out := make([]string, 0, len(problems))
	for _, p := range problems {
		out = append(out, p.String())
	}
	return out
}
