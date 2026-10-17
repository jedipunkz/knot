// Package config loads knot.toml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the whole configuration.
type Config struct {
	Theme     string                       `toml:"theme"`
	ThemeAuto ThemeAuto                    `toml:"theme-auto"`
	UI        UI                           `toml:"ui"`
	Main      Main                         `toml:"main"`
	Diff      Diff                         `toml:"diff"`
	Blame     Blame                        `toml:"blame"`
	Worktree  Worktree                     `toml:"worktree"`
	Git       Git                          `toml:"git"`
	Keys      map[string]map[string]string `toml:"keys"`
	Commands  []Command                    `toml:"commands"`
	Confirm   map[string]bool              `toml:"confirm"`

	// Path is the file the configuration was loaded from ("" for defaults).
	Path string `toml:"-"`
}

// ThemeAuto selects themes when theme = "auto".
type ThemeAuto struct {
	Dark  string `toml:"dark"`
	Light string `toml:"light"`
}

// UI holds general display options.
type UI struct {
	LineGraphics    string  `toml:"line-graphics"`
	SplitDirection  string  `toml:"split-direction"`
	SplitRatio      float64 `toml:"split-ratio"`
	ShowLineNumbers bool    `toml:"show-line-numbers"`
	Mouse           bool    `toml:"mouse"`
	TabSize         int     `toml:"tab-size"`
	TitleOverflow   int     `toml:"title-overflow"`
	KeyBar          bool    `toml:"keybar"`
}

// Main holds main view options.
type Main struct {
	Columns        []string `toml:"columns"`
	DateFormat     string   `toml:"date-format"`
	DateCustom     string   `toml:"date-custom"`
	Author         string   `toml:"author"`
	CommitIDLength int      `toml:"commit-id-length"`
	Graph          bool     `toml:"graph"`
}

// Diff holds diff options.
type Diff struct {
	Context          int    `toml:"context"`
	IgnoreWhitespace bool   `toml:"ignore-whitespace"`
	Highlight        string `toml:"highlight"`
}

// Blame holds blame view options.
type Blame struct {
	Columns []string `toml:"columns"`
}

// Worktree holds worktree options.
type Worktree struct {
	OnSwitch              string   `toml:"on-switch"`
	ShowDirty             bool     `toml:"show-dirty"`
	DirtyCheckConcurrency int      `toml:"dirty-check-concurrency"`
	OpenCommand           []string `toml:"open-command"`
}

// Git holds git invocation options.
type Git struct {
	Binary  string   `toml:"binary"`
	LogArgs []string `toml:"log-args"`
}

// Command is a user defined command bound to a key.
type Command struct {
	Key     string   `toml:"key"`
	View    string   `toml:"view"`
	Argv    []string `toml:"argv"`
	Confirm bool     `toml:"confirm"`
	Output  string   `toml:"output"`
}

// Problem is a configuration warning or error.
type Problem struct {
	File  string
	Line  int // 0 when unknown
	Msg   string
	Error bool // false for warnings (e.g. unknown keys)
}

func (p Problem) String() string {
	kind := "warning"
	if p.Error {
		kind = "error"
	}
	loc := p.File
	if p.Line > 0 {
		loc = fmt.Sprintf("%s:%d", p.File, p.Line)
	}
	if loc == "" {
		return fmt.Sprintf("%s: %s", kind, p.Msg)
	}
	return fmt.Sprintf("%s: %s: %s", loc, kind, p.Msg)
}

// Views that can be targeted by keys, commands and on-switch.
var Views = []string{"main", "diff", "log", "reflog", "tree", "blob", "blame", "refs", "status", "stage", "stash", "grep", "pager", "help", "worktree"}

// Default returns the built-in configuration.
func Default() *Config {
	return &Config{
		Theme:     "default",
		ThemeAuto: ThemeAuto{Dark: "dark", Light: "light"},
		UI: UI{
			LineGraphics:   "utf-8",
			SplitDirection: "horizontal",
			SplitRatio:     0.66,
			Mouse:          true,
			TabSize:        8,
			TitleOverflow:  50,
			KeyBar:         true,
		},
		Main: Main{
			Columns:        []string{"date", "author", "commit-id", "graph", "refs", "subject"},
			DateFormat:     "absolute",
			DateCustom:     "%Y-%m-%d %H:%M",
			Author:         "full",
			CommitIDLength: 7,
			Graph:          true,
		},
		Diff:     Diff{Context: 3, Highlight: "line"},
		Blame:    Blame{Columns: []string{"commit-id", "author", "date", "line-number"}},
		Worktree: Worktree{OnSwitch: "main", ShowDirty: true, DirtyCheckConcurrency: 4, OpenCommand: []string{"$SHELL"}},
		Git:      Git{Binary: "git"},
		Keys:     map[string]map[string]string{},
		Confirm:  map[string]bool{},
	}
}

// Locate returns the configuration path using the documented lookup order.
// flagPath is the value of --config. ok is false when no file exists.
func Locate(flagPath string) (path string, ok bool, err error) {
	if flagPath != "" {
		if _, err := os.Stat(flagPath); err != nil {
			return flagPath, false, err
		}
		return flagPath, true, nil
	}
	var candidates []string
	if p := os.Getenv("KNOT_CONFIG"); p != "" {
		candidates = append(candidates, p)
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		candidates = append(candidates, filepath.Join(x, "knot", "knot.toml"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "knot", "knot.toml"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return c, false, err
		}
	}
	return "", false, nil
}

// Dir returns the knot configuration directory (for themes).
func Dir(configPath string) string {
	if configPath != "" {
		return filepath.Dir(configPath)
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "knot")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "knot")
	}
	return ""
}

// Load reads the file at path on top of the defaults. A missing path ("")
// returns the defaults. Problems are returned for unknown keys and invalid
// values; invalid values fall back to their defaults.
func Load(path string) (*Config, []Problem, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil, nil
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil, err
	}
	return parse(path, string(src))
}

// Parse decodes configuration text (used for tests and the default file).
func Parse(name, src string) (*Config, []Problem, error) {
	return parse(name, src)
}

func parse(path, src string) (*Config, []Problem, error) {
	cfg := Default()
	md, err := toml.Decode(src, cfg)
	if err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			return Default(), nil, fmt.Errorf("%s:%d: %s", path, pe.Position.Line, pe.Message)
		}
		return Default(), nil, fmt.Errorf("%s: %w", path, err)
	}
	cfg.Path = path
	var problems []Problem
	for _, k := range md.Undecoded() {
		problems = append(problems, Problem{File: path, Line: lineOf(src, k), Msg: fmt.Sprintf("unknown key %q", k.String())})
	}
	v := &validator{cfg: cfg, def: Default(), file: path, src: src}
	v.validate()
	problems = append(problems, v.problems...)
	return cfg, problems, nil
}

type validator struct {
	cfg, def *Config
	file     string
	src      string
	problems []Problem
}

func (v *validator) fail(key []string, format string, args ...any) {
	v.problems = append(v.problems, Problem{File: v.file, Line: lineOf(v.src, key), Msg: fmt.Sprintf(format, args...), Error: true})
}

func oneOf(s string, allowed ...string) bool {
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

func (v *validator) enum(key []string, val *string, def string, allowed ...string) {
	if !oneOf(*val, allowed...) {
		v.fail(key, "%s must be one of %s, got %q", strings.Join(key, "."), strings.Join(allowed, " | "), *val)
		*val = def
	}
}

func (v *validator) validate() {
	c, d := v.cfg, v.def
	v.enum([]string{"ui", "line-graphics"}, &c.UI.LineGraphics, d.UI.LineGraphics, "utf-8", "ascii", "default")
	v.enum([]string{"ui", "split-direction"}, &c.UI.SplitDirection, d.UI.SplitDirection, "horizontal", "vertical", "auto")
	if c.UI.SplitRatio <= 0.1 || c.UI.SplitRatio >= 0.9 {
		v.fail([]string{"ui", "split-ratio"}, "ui.split-ratio must be between 0.1 and 0.9, got %v", c.UI.SplitRatio)
		c.UI.SplitRatio = d.UI.SplitRatio
	}
	if c.UI.TabSize < 1 || c.UI.TabSize > 16 {
		v.fail([]string{"ui", "tab-size"}, "ui.tab-size must be between 1 and 16, got %d", c.UI.TabSize)
		c.UI.TabSize = d.UI.TabSize
	}
	if c.UI.TitleOverflow < 0 {
		v.fail([]string{"ui", "title-overflow"}, "ui.title-overflow must not be negative")
		c.UI.TitleOverflow = d.UI.TitleOverflow
	}
	for _, col := range c.Main.Columns {
		if !oneOf(col, "date", "author", "commit-id", "graph", "refs", "subject") {
			v.fail([]string{"main", "columns"}, "unknown main column %q", col)
			c.Main.Columns = d.Main.Columns
			break
		}
	}
	v.enum([]string{"main", "date-format"}, &c.Main.DateFormat, d.Main.DateFormat, "off", "absolute", "relative", "relative-compact", "custom")
	v.enum([]string{"main", "author"}, &c.Main.Author, d.Main.Author, "off", "full", "abbreviated", "email", "email-user")
	if c.Main.CommitIDLength < 4 || c.Main.CommitIDLength > 40 {
		v.fail([]string{"main", "commit-id-length"}, "main.commit-id-length must be between 4 and 40, got %d", c.Main.CommitIDLength)
		c.Main.CommitIDLength = d.Main.CommitIDLength
	}
	if c.Diff.Context < 0 {
		v.fail([]string{"diff", "context"}, "diff.context must not be negative")
		c.Diff.Context = d.Diff.Context
	}
	v.enum([]string{"diff", "highlight"}, &c.Diff.Highlight, d.Diff.Highlight, "off", "line", "word")
	for _, col := range c.Blame.Columns {
		if !oneOf(col, "commit-id", "author", "date", "line-number") {
			v.fail([]string{"blame", "columns"}, "unknown blame column %q", col)
			c.Blame.Columns = d.Blame.Columns
			break
		}
	}
	v.enum([]string{"worktree", "on-switch"}, &c.Worktree.OnSwitch, d.Worktree.OnSwitch, Views...)
	if c.Worktree.DirtyCheckConcurrency < 1 {
		v.fail([]string{"worktree", "dirty-check-concurrency"}, "worktree.dirty-check-concurrency must be at least 1")
		c.Worktree.DirtyCheckConcurrency = d.Worktree.DirtyCheckConcurrency
	}
	if len(c.Worktree.OpenCommand) == 0 {
		v.fail([]string{"worktree", "open-command"}, "worktree.open-command must not be empty")
		c.Worktree.OpenCommand = d.Worktree.OpenCommand
	}
	if c.Git.Binary == "" {
		c.Git.Binary = d.Git.Binary
	}
	for view := range c.Keys {
		if view != "generic" && !oneOf(view, Views...) {
			v.fail([]string{"keys", view}, "unknown view %q in [keys]", view)
			delete(c.Keys, view)
		}
	}
	valid := c.Commands[:0]
	for i, cmd := range c.Commands {
		key := []string{"commands"}
		switch {
		case cmd.Key == "":
			v.fail(key, "commands[%d]: key is required", i)
		case len(cmd.Argv) == 0:
			v.fail(key, "commands[%d]: argv is required", i)
		case cmd.View != "" && cmd.View != "generic" && !oneOf(cmd.View, Views...):
			v.fail(key, "commands[%d]: unknown view %q", i, cmd.View)
		default:
			if cmd.Output == "" {
				cmd.Output = "status"
			}
			if !oneOf(cmd.Output, "status", "pager", "none", "suspend") {
				v.fail(key, "commands[%d]: output must be status | pager | none | suspend, got %q", i, cmd.Output)
				continue
			}
			valid = append(valid, cmd)
		}
	}
	c.Commands = valid
}

// lineOf finds the line of a key in the source. It is a best-effort search:
// the section header is located first, then the key below it.
func lineOf(src string, key toml.Key) int {
	if len(key) == 0 {
		return 0
	}
	lines := strings.Split(src, "\n")
	start := 0
	if len(key) > 1 {
		section := regexp.MustCompile(`^\s*\[\[?\s*` + regexp.QuoteMeta(strings.Join(key[:len(key)-1], ".")) + `\s*\]\]?`)
		for i, l := range lines {
			if section.MatchString(l) {
				start = i
				break
			}
		}
	}
	last := key[len(key)-1]
	re := regexp.MustCompile(`^\s*(\[\[?\s*([\w.-]+\.)?|"?)` + regexp.QuoteMeta(last) + `"?\s*(=|\]\]?)`)
	for i := start; i < len(lines); i++ {
		if re.MatchString(lines[i]) {
			return i + 1
		}
	}
	if start > 0 {
		return start + 1
	}
	return 0
}
