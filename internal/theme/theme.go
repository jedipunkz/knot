// Package theme maps semantic color roles to styles.
package theme

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/BurntSushi/toml"
)

// Role is a semantic color role.
type Role string

// Roles used by views. Views never use raw colors.
const (
	Default         Role = "default"
	Cursor          Role = "cursor"
	TitleFocus      Role = "title-focus"
	TitleBlur       Role = "title-blur"
	StatusBar       Role = "status-bar"
	LineNumber      Role = "line-number"
	Date            Role = "date"
	Author          Role = "author"
	CommitID        Role = "commit-id"
	RefHead         Role = "ref-head"
	RefBranch       Role = "ref-branch"
	RefRemote       Role = "ref-remote"
	RefTag          Role = "ref-tag"
	RefStash        Role = "ref-stash"
	RefWorktree     Role = "ref-worktree"
	DiffAdd         Role = "diff-add"
	DiffDel         Role = "diff-del"
	DiffAddHL       Role = "diff-add-highlight"
	DiffDelHL       Role = "diff-del-highlight"
	DiffHunk        Role = "diff-hunk"
	DiffHeader      Role = "diff-header"
	DiffMeta        Role = "diff-meta"
	StatAdd         Role = "stat-add"
	StatDel         Role = "stat-del"
	StatusStaged    Role = "status-staged"
	StatusUnstaged  Role = "status-unstaged"
	StatusUntracked Role = "status-untracked"
	Section         Role = "section"
	SearchMatch     Role = "search-match"
	Prompt          Role = "prompt"
	Error           Role = "error"
	Warning         Role = "warning"
	Overflow        Role = "title-overflow"
	Directory       Role = "directory"
	Marker          Role = "marker"
)

// GraphColors is the number of graph lane roles (graph-1 .. graph-N).
const GraphColors = 6

// AllRoles lists every role, including graph roles.
func AllRoles() []Role {
	roles := []Role{Default, Cursor, TitleFocus, TitleBlur, StatusBar, LineNumber, Date, Author, CommitID,
		RefHead, RefBranch, RefRemote, RefTag, RefStash, RefWorktree,
		DiffAdd, DiffDel, DiffAddHL, DiffDelHL, DiffHunk, DiffHeader, DiffMeta, StatAdd, StatDel,
		StatusStaged, StatusUnstaged, StatusUntracked, Section, SearchMatch, Prompt, Error, Warning,
		Overflow, Directory, Marker}
	for i := 1; i <= GraphColors; i++ {
		roles = append(roles, GraphRole(i-1))
	}
	return roles
}

// GraphRole returns the role for a graph lane.
func GraphRole(lane int) Role {
	return Role(fmt.Sprintf("graph-%d", lane%GraphColors+1))
}

// Spec is a style as written in a theme file.
type Spec struct {
	Fg    string   `toml:"fg"`
	Bg    string   `toml:"bg"`
	Attrs []string `toml:"attrs"`
}

// File is the on-disk theme format.
type File struct {
	Extends string          `toml:"extends"`
	Styles  map[string]Spec `toml:"styles"`
}

// Theme is a resolved theme.
type Theme struct {
	Name   string
	specs  map[Role]Spec
	styles map[Role]lipgloss.Style
}

// Style returns the style of a role (the default style for unknown roles).
func (t *Theme) Style(r Role) lipgloss.Style {
	if s, ok := t.styles[r]; ok {
		return s
	}
	return t.styles[Default]
}

// Render renders s with the style of r.
func (t *Theme) Render(r Role, s string) string {
	if s == "" {
		return ""
	}
	return t.Style(r).Render(s)
}

var namedColors = map[string]int{
	"black": 0, "red": 1, "green": 2, "yellow": 3, "blue": 4, "magenta": 5, "cyan": 6, "white": 7,
	"bright-black": 8, "gray": 8, "grey": 8, "bright-red": 9, "bright-green": 10, "bright-yellow": 11,
	"bright-blue": 12, "bright-magenta": 13, "bright-cyan": 14, "bright-white": 15,
}

// ParseColor parses a hex color, an ANSI 256 index or an ANSI color name.
// "default" and "" return nil (terminal default).
func ParseColor(s string) (color.Color, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "default" {
		return nil, nil
	}
	if strings.HasPrefix(s, "#") {
		if len(s) != 4 && len(s) != 7 {
			return nil, fmt.Errorf("invalid hex color %q", s)
		}
		if _, err := strconv.ParseUint(s[1:], 16, 32); err != nil {
			return nil, fmt.Errorf("invalid hex color %q", s)
		}
		return lipgloss.Color(s), nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 || n > 255 {
			return nil, fmt.Errorf("color index %d out of range 0-255", n)
		}
		return lipgloss.Color(s), nil
	}
	if n, ok := namedColors[s]; ok {
		return lipgloss.Color(strconv.Itoa(n)), nil
	}
	return nil, fmt.Errorf("unknown color %q", s)
}

func buildStyle(sp Spec) (lipgloss.Style, error) {
	st := lipgloss.NewStyle()
	fg, err := ParseColor(sp.Fg)
	if err != nil {
		return st, err
	}
	if fg != nil {
		st = st.Foreground(fg)
	}
	bg, err := ParseColor(sp.Bg)
	if err != nil {
		return st, err
	}
	if bg != nil {
		st = st.Background(bg)
	}
	for _, a := range sp.Attrs {
		switch strings.ToLower(a) {
		case "bold":
			st = st.Bold(true)
		case "dim", "faint":
			st = st.Faint(true)
		case "italic":
			st = st.Italic(true)
		case "underline":
			st = st.Underline(true)
		case "reverse":
			st = st.Reverse(true)
		case "strikethrough":
			st = st.Strikethrough(true)
		default:
			return st, fmt.Errorf("unknown attribute %q", a)
		}
	}
	return st, nil
}

// resolve builds a theme from specs layered over the default theme.
func resolve(name string, specs map[Role]Spec) (*Theme, error) {
	t := &Theme{Name: name, specs: map[Role]Spec{}, styles: map[Role]lipgloss.Style{}}
	for _, r := range AllRoles() {
		sp, ok := specs[r]
		if !ok {
			sp = builtin["default"][r]
		}
		st, err := buildStyle(sp)
		if err != nil {
			return nil, fmt.Errorf("theme %s: %s: %w", name, r, err)
		}
		t.specs[r] = sp
		t.styles[r] = st
	}
	return t, nil
}

// Names returns all available theme names: built-ins and user themes.
func Names(dir string) []string {
	seen := map[string]bool{}
	var names []string
	for _, n := range BuiltinNames {
		seen[n] = true
		names = append(names, n)
	}
	if dir != "" {
		entries, _ := os.ReadDir(filepath.Join(dir, "themes"))
		var user []string
		for _, e := range entries {
			n, ok := strings.CutSuffix(e.Name(), ".toml")
			if ok && !e.IsDir() && !seen[n] {
				user = append(user, n)
			}
		}
		sort.Strings(user)
		names = append(names, user...)
	}
	return names
}

// Load returns the theme with the given name. User themes in
// <dir>/themes/<name>.toml take precedence over built-ins and may extend
// another theme.
func Load(dir, name string) (*Theme, error) {
	specs, err := loadSpecs(dir, name, map[string]bool{})
	if err != nil {
		return nil, err
	}
	return resolve(name, specs)
}

func loadSpecs(dir, name string, visiting map[string]bool) (map[Role]Spec, error) {
	if visiting[name] {
		return nil, fmt.Errorf("theme %s: extends cycle", name)
	}
	visiting[name] = true
	if dir != "" && validName(name) {
		path := filepath.Join(dir, "themes", name+".toml")
		var f File
		md, err := toml.DecodeFile(path, &f)
		switch {
		case err == nil:
			if und := md.Undecoded(); len(und) > 0 {
				return nil, fmt.Errorf("%s: unknown key %q", path, und[0].String())
			}
			base := map[Role]Spec{}
			if f.Extends != "" {
				if base, err = loadSpecs(dir, f.Extends, visiting); err != nil {
					return nil, err
				}
			}
			out := map[Role]Spec{}
			for r, sp := range base {
				out[r] = sp
			}
			known := map[Role]bool{}
			for _, r := range AllRoles() {
				known[r] = true
			}
			for k, sp := range f.Styles {
				if !known[Role(k)] {
					return nil, fmt.Errorf("%s: unknown role %q", path, k)
				}
				out[Role(k)] = sp
			}
			return out, nil
		case !errors.Is(err, fs.ErrNotExist):
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if specs, ok := builtin[name]; ok {
		return specs, nil
	}
	return nil, fmt.Errorf("unknown theme %q", name)
}

func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, `/\`) && name != "." && name != ".."
}
