package app

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/view"
)

// Key handling: prompt and filter bar line editing plus the action
// dispatch of the focused view.

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if m.confirm != nil {
		c := m.confirm
		m.confirm = nil
		uc := m.formUserCancel
		m.formUserCancel = nil
		if k == "y" || k == "Y" {
			// The user command stays registered so StopLoading can
			// cancel it; its result message deregisters the entry.
			m.setStatus("", false)
			return c.OnYes
		}
		if uc != nil && uc.id >= 0 {
			delete(m.userCancels, uc.id)
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
		if s, ok := f.(view.FilterStarter); ok && s.FilterOnOpen() {
			// Sticky bar: return to the list, filter kept.
			m.filterBar = nil
		}
		// No-op inside the main view's bar.
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
		// Cancel in-flight captured user commands; their result
		// messages deregister the entries when they come back.
		for _, cancel := range m.userCancels {
			cancel()
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

// processUserResult runs the result messages of a captured user command
// through the Update loop, splitting batches like the test driver does.
func (m *Model) processUserResult(msg tea.Msg) tea.Cmd {
	if b, ok := msg.(tea.BatchMsg); ok {
		var out tea.BatchMsg
		for _, c := range b {
			if c == nil {
				continue
			}
			out = append(out, m.processUserResult(c()))
		}
		if len(out) == 0 {
			return nil
		}
		// bubbletea unwraps a batch returned inside a command.
		return func() tea.Msg { return out }
	}
	_, cmd := m.update(msg)
	return cmd
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
		return m.runCapturedCmd(argv, "pager", strings.Join(argv, " "))
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
