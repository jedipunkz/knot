package theme

// BuiltinNames lists built-in themes in display order.
var BuiltinNames = []string{"default", "tig-classic", "dark", "light", "solarized-dark", "solarized-light", "gruvbox", "catppuccin-mocha"}

type palette struct {
	fg, dim, red, green, yellow, blue, magenta, cyan, orange string
	cursorFg, cursorBg, barFg, barBg, blurBg, addBg, delBg   string
}

func fromPalette(p palette) map[Role]Spec {
	s := map[Role]Spec{
		Default:         {Fg: p.fg},
		Cursor:          {Fg: p.cursorFg, Bg: p.cursorBg, Attrs: []string{"bold"}},
		TitleFocus:      {Fg: p.barFg, Bg: p.barBg, Attrs: []string{"bold"}},
		TitleBlur:       {Fg: p.barFg, Bg: p.blurBg},
		StatusBar:       {Fg: p.fg},
		LineNumber:      {Fg: p.dim},
		Date:            {Fg: p.blue},
		Author:          {Fg: p.green},
		CommitID:        {Fg: p.magenta},
		RefHead:         {Fg: p.cyan, Attrs: []string{"bold"}},
		RefBranch:       {Fg: p.cyan},
		RefRemote:       {Fg: p.yellow},
		RefTag:          {Fg: p.magenta, Attrs: []string{"bold"}},
		RefStash:        {Fg: p.dim},
		RefWorktree:     {Fg: p.orange},
		DiffAdd:         {Fg: p.green},
		DiffDel:         {Fg: p.red},
		DiffAddHL:       {Fg: p.green, Bg: p.addBg, Attrs: []string{"bold"}},
		DiffDelHL:       {Fg: p.red, Bg: p.delBg, Attrs: []string{"bold"}},
		DiffHunk:        {Fg: p.magenta},
		DiffHeader:      {Fg: p.yellow},
		DiffMeta:        {Fg: p.blue},
		StatAdd:         {Fg: p.green},
		StatDel:         {Fg: p.red},
		StatusStaged:    {Fg: p.green},
		StatusUnstaged:  {Fg: p.red},
		StatusUntracked: {Fg: p.dim},
		Section:         {Fg: p.yellow, Attrs: []string{"bold"}},
		SearchMatch:     {Attrs: []string{"reverse"}},
		Prompt:          {Fg: p.fg},
		Error:           {Fg: p.red, Attrs: []string{"bold"}},
		Warning:         {Fg: p.yellow},
		Overflow:        {Fg: p.orange, Attrs: []string{"underline"}},
		Directory:       {Fg: p.blue, Attrs: []string{"bold"}},
		Marker:          {Fg: p.orange, Attrs: []string{"bold"}},
	}
	lanes := []string{p.blue, p.yellow, p.magenta, p.cyan, p.green, p.red}
	for i, c := range lanes {
		s[GraphRole(i)] = Spec{Fg: c}
	}
	return s
}

var builtin = map[string]map[Role]Spec{
	// default uses the 16 ANSI colors so it follows the terminal palette.
	// The cursor uses reverse video so it stays visible with NO_COLOR.
	"default": withCursor(fromPalette(palette{
		fg: "default", dim: "bright-black", red: "red", green: "green", yellow: "yellow", blue: "blue",
		magenta: "magenta", cyan: "cyan", orange: "bright-yellow",
		barFg: "bright-white", barBg: "blue", blurBg: "bright-black",
		addBg: "default", delBg: "default",
	}), Spec{Attrs: []string{"reverse", "bold"}}),
	// tig-classic mimics tig's default colors.
	"tig-classic": withCursor(fromPalette(palette{
		fg: "default", dim: "bright-black", red: "red", green: "green", yellow: "yellow", blue: "blue",
		magenta: "magenta", cyan: "cyan", orange: "yellow",
		barFg: "white", barBg: "blue", blurBg: "blue",
		addBg: "default", delBg: "default",
	}), Spec{Fg: "white", Bg: "green", Attrs: []string{"bold"}}),
	"dark": fromPalette(palette{
		fg: "#d0d0d0", dim: "#6c6c6c", red: "#ff5f5f", green: "#87d75f", yellow: "#ffd75f", blue: "#5fafff",
		magenta: "#d787ff", cyan: "#5fd7d7", orange: "#ffaf5f",
		cursorFg: "#ffffff", cursorBg: "#3a3a5a", barFg: "#ffffff", barBg: "#005f87", blurBg: "#3a3a3a",
		addBg: "#1f3a1f", delBg: "#3a1f1f",
	}),
	"light": fromPalette(palette{
		fg: "#303030", dim: "#8a8a8a", red: "#c0392b", green: "#2e7d32", yellow: "#9a6700", blue: "#1f5fbf",
		magenta: "#8e24aa", cyan: "#00838f", orange: "#d35400",
		cursorFg: "#000000", cursorBg: "#d0e0ff", barFg: "#ffffff", barBg: "#1f5fbf", blurBg: "#8a8a8a",
		addBg: "#dff5df", delBg: "#fbe1e1",
	}),
	"solarized-dark": fromPalette(palette{
		fg: "#839496", dim: "#586e75", red: "#dc322f", green: "#859900", yellow: "#b58900", blue: "#268bd2",
		magenta: "#d33682", cyan: "#2aa198", orange: "#cb4b16",
		cursorFg: "#fdf6e3", cursorBg: "#073642", barFg: "#fdf6e3", barBg: "#268bd2", blurBg: "#586e75",
		addBg: "#073642", delBg: "#073642",
	}),
	"solarized-light": fromPalette(palette{
		fg: "#657b83", dim: "#93a1a1", red: "#dc322f", green: "#859900", yellow: "#b58900", blue: "#268bd2",
		magenta: "#d33682", cyan: "#2aa198", orange: "#cb4b16",
		cursorFg: "#002b36", cursorBg: "#eee8d5", barFg: "#fdf6e3", barBg: "#268bd2", blurBg: "#93a1a1",
		addBg: "#eee8d5", delBg: "#eee8d5",
	}),
	"gruvbox": fromPalette(palette{
		fg: "#ebdbb2", dim: "#928374", red: "#fb4934", green: "#b8bb26", yellow: "#fabd2f", blue: "#83a598",
		magenta: "#d3869b", cyan: "#8ec07c", orange: "#fe8019",
		cursorFg: "#fbf1c7", cursorBg: "#504945", barFg: "#282828", barBg: "#83a598", blurBg: "#665c54",
		addBg: "#32361a", delBg: "#3c1f1e",
	}),
	"catppuccin-mocha": fromPalette(palette{
		fg: "#cdd6f4", dim: "#6c7086", red: "#f38ba8", green: "#a6e3a1", yellow: "#f9e2af", blue: "#89b4fa",
		magenta: "#cba6f7", cyan: "#94e2d5", orange: "#fab387",
		cursorFg: "#cdd6f4", cursorBg: "#45475a", barFg: "#1e1e2e", barBg: "#89b4fa", blurBg: "#585b70",
		addBg: "#2b3b2e", delBg: "#3e2833",
	}),
}

func withCursor(s map[Role]Spec, cursor Spec) map[Role]Spec {
	s[Cursor] = cursor
	return s
}
