package view

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// HelpView lists the effective key bindings.
type HelpView struct {
	Base
	rows [][]Seg
}

// NewHelp creates a help view.
func NewHelp() *HelpView { return &HelpView{} }

func (v *HelpView) Name() string          { return "help" }
func (v *HelpView) Title(env *Env) string { return "[help] knot " + env.Version }
func (v *HelpView) Len() int              { return len(v.rows) }
func (v *HelpView) Row(env *Env, i int) []Seg {
	return v.rows[i]
}

func (v *HelpView) Load(env *Env) tea.Cmd {
	v.rows = nil
	add := func(segs ...Seg) { v.rows = append(v.rows, segs) }
	for _, view := range env.Keys.Views() {
		bs := env.Keys.Bindings(view)
		if len(bs) == 0 {
			continue
		}
		if len(v.rows) > 0 {
			add()
		}
		add(Seg{theme.Section, fmt.Sprintf("[%s]", view)})
		// Group keys by action.
		var order []keymap.Action
		keys := map[keymap.Action][]string{}
		for _, b := range bs {
			if _, ok := keys[b.Action]; !ok {
				order = append(order, b.Action)
			}
			keys[b.Action] = append(keys[b.Action], displayKey(b.Key))
		}
		for _, a := range order {
			desc := keymap.Descriptions[a]
			if strings.HasPrefix(string(a), keymap.CommandPrefix) {
				desc = commandDesc(env, string(a))
			}
			add(Seg{theme.RefHead, Fit(strings.Join(keys[a], ", "), 18) + " "},
				Seg{theme.Author, Fit(string(a), 24) + " "},
				Seg{theme.Default, desc})
		}
	}
	return nil
}

func commandDesc(env *Env, a string) string {
	i, err := strconv.Atoi(strings.TrimPrefix(a, keymap.CommandPrefix))
	if err != nil || i < 0 || i >= len(env.Cfg.Commands) {
		return "user command"
	}
	return "Run: " + strings.Join(env.Cfg.Commands[i].Argv, " ")
}

func displayKey(k string) string {
	switch k {
	case "space":
		return "Space"
	case "enter":
		return "Enter"
	case "tab":
		return "Tab"
	}
	if len(k) > 1 {
		return "<" + k + ">"
	}
	return k
}

func (v *HelpView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd      { return nil }
func (v *HelpView) Enter(env *Env) tea.Cmd                       { return nil }
func (v *HelpView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) { return nil, false }
func (v *HelpView) Selection(env *Env) Selection                 { return Selection{} }

// option is one entry of the option menu.
type option struct {
	name  string
	value func(env *Env) string
	next  func(env *Env)
}

func cycle(cur string, values ...string) string {
	for i, v := range values {
		if v == cur {
			return values[(i+1)%len(values)]
		}
	}
	return values[0]
}

// CycleDate cycles the date display like tig's D.
func CycleDate(o *Options) {
	o.Date = cycle(o.Date, "off", "absolute", "relative", "relative-compact", "custom")
}

// CycleAuthor cycles the author display like tig's A.
func CycleAuthor(o *Options) {
	o.Author = cycle(o.Author, "off", "full", "abbreviated", "email", "email-user")
}

func boolText(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

var options = []option{
	{"show-line-numbers", func(e *Env) string { return boolText(e.Opts.LineNumbers) }, func(e *Env) { e.Opts.LineNumbers = !e.Opts.LineNumbers }},
	{"date", func(e *Env) string { return e.Opts.Date }, func(e *Env) { CycleDate(e.Opts) }},
	{"author", func(e *Env) string { return e.Opts.Author }, func(e *Env) { CycleAuthor(e.Opts) }},
	{"show-graph", func(e *Env) string { return boolText(e.Opts.Graph) }, func(e *Env) { e.Opts.Graph = !e.Opts.Graph }},
	{"line-graphics", func(e *Env) string {
		if e.Opts.ASCII {
			return "ascii"
		}
		return "utf-8"
	}, func(e *Env) { e.Opts.ASCII = !e.Opts.ASCII }},
	{"show-refs", func(e *Env) string { return boolText(e.Opts.Refs) }, func(e *Env) { e.Opts.Refs = !e.Opts.Refs }},
	{"show-id", func(e *Env) string { return boolText(e.Opts.ID) }, func(e *Env) { e.Opts.ID = !e.Opts.ID }},
	{"ignore-space", func(e *Env) string { return boolText(e.Opts.IgnoreSpace) }, func(e *Env) { e.Opts.IgnoreSpace = !e.Opts.IgnoreSpace }},
	{"file-filter", func(e *Env) string { return boolText(e.Opts.FileFilter) }, func(e *Env) { e.Opts.FileFilter = !e.Opts.FileFilter }},
	{"title-overflow", func(e *Env) string { return boolText(e.Opts.TitleOverflow) }, func(e *Env) { e.Opts.TitleOverflow = !e.Opts.TitleOverflow }},
	{"diff-context", func(e *Env) string { return strconv.Itoa(e.Opts.DiffContext) }, func(e *Env) { e.Opts.DiffContext = (e.Opts.DiffContext + 1) % 11 }},
	{"split-direction", func(e *Env) string { return e.Cfg.UI.SplitDirection }, func(e *Env) {
		e.Cfg.UI.SplitDirection = cycle(e.Cfg.UI.SplitDirection, "horizontal", "vertical", "auto")
	}},
}

// OptionsView is tig's option menu (o). Enter changes the value.
type OptionsView struct{ Base }

// NewOptionsView creates the option menu.
func NewOptionsView() *OptionsView { return &OptionsView{} }

func (v *OptionsView) Name() string          { return "options" }
func (v *OptionsView) Title(env *Env) string { return "[options] Enter changes the value" }
func (v *OptionsView) Len() int              { return len(options) }
func (v *OptionsView) Row(env *Env, i int) []Seg {
	o := options[i]
	return []Seg{{theme.Default, Fit(o.name, 20) + " "}, {theme.RefHead, o.value(env)}}
}
func (v *OptionsView) Load(env *Env) tea.Cmd                        { return nil }
func (v *OptionsView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd      { return nil }
func (v *OptionsView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) { return nil, false }
func (v *OptionsView) Selection(env *Env) Selection                 { return Selection{} }

// OptionChangedMsg tells the app that an option changed and views showing
// git output may need to reload.
type OptionChangedMsg struct{ Name string }

func (v *OptionsView) Enter(env *Env) tea.Cmd {
	if v.Cursor < len(options) {
		o := options[v.Cursor]
		o.next(env)
		return Msg(OptionChangedMsg{Name: o.name})
	}
	return nil
}

// ThemeView lists themes and previews the one under the cursor.
type ThemeView struct {
	Base
	names    []string
	original string
	applied  bool
}

// NewThemeView creates the theme picker.
func NewThemeView() *ThemeView { return &ThemeView{} }

func (v *ThemeView) Name() string          { return "theme" }
func (v *ThemeView) Title(env *Env) string { return "[theme] Enter applies, q cancels" }
func (v *ThemeView) Len() int              { return len(v.names) }

func (v *ThemeView) Load(env *Env) tea.Cmd {
	v.names = theme.Names(env.ThemeDir)
	v.original = env.Theme.Name
	for i, n := range v.names {
		if n == env.Theme.Name {
			v.Cursor = i
		}
	}
	return nil
}

func (v *ThemeView) Row(env *Env, i int) []Seg {
	mark := "  "
	if v.names[i] == v.original {
		mark = "* "
	}
	return []Seg{{theme.Marker, mark}, {theme.Default, v.names[i]}}
}

func (v *ThemeView) CursorMoved(env *Env) tea.Cmd {
	if v.Cursor < len(v.names) {
		return Msg(PreviewThemeMsg{Name: v.names[v.Cursor]})
	}
	return nil
}

func (v *ThemeView) Enter(env *Env) tea.Cmd {
	if v.Cursor >= len(v.names) {
		return nil
	}
	v.applied = true
	name := v.names[v.Cursor]
	return tea.Batch(Msg(PreviewThemeMsg{Name: name}), Msg(CloseMsg{ID: v.ID}),
		Status("Theme "+name+" applied for this session (set theme = \""+name+"\" in knot.toml to keep it)"))
}

// Closed restores the original theme when the picker is cancelled.
func (v *ThemeView) Closed(env *Env) tea.Cmd {
	if v.applied {
		return nil
	}
	return Msg(PreviewThemeMsg{Name: v.original, Restore: true})
}

func (v *ThemeView) HandleMsg(env *Env, msg tea.Msg) tea.Cmd      { return nil }
func (v *ThemeView) Do(env *Env, a keymap.Action) (tea.Cmd, bool) { return nil, false }
func (v *ThemeView) Selection(env *Env) Selection                 { return Selection{} }

// Compile-time interface checks.
var (
	_ View = (*MainView)(nil)
	_ View = (*DiffView)(nil)
	_ View = (*LogView)(nil)
	_ View = (*PagerView)(nil)
	_ View = (*ReflogView)(nil)
	_ View = (*StashView)(nil)
	_ View = (*RefsView)(nil)
	_ View = (*TreeView)(nil)
	_ View = (*BlobView)(nil)
	_ View = (*BlameView)(nil)
	_ View = (*GrepView)(nil)
	_ View = (*StatusView)(nil)
	_ View = (*StageView)(nil)
	_ View = (*WorktreeView)(nil)
	_ View = (*HelpView)(nil)
	_ View = (*OptionsView)(nil)
	_ View = (*ThemeView)(nil)

	_ CursorWatcher = (*ThemeView)(nil)
	_ Closer        = (*ThemeView)(nil)
)
