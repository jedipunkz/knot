// Command knot is a tig-compatible text-mode interface for Git.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/jedipunkz/knot/internal/app"
	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
	"github.com/jedipunkz/knot/internal/view"
)

// version is set with -ldflags "-X main.version=...".
var version = ""

// Minimum git version (git worktree list --porcelain -z needs 2.36).
const minGitMajor, minGitMinor = 2, 36

const usage = `knot - a tig-compatible text-mode interface for Git

Usage:
  knot [options] [revisions] [--] [paths]
  knot log    [options] [revisions] [--] [paths]
  knot show   [options] [revisions] [--] [paths]
  knot reflog [options] [revisions]
  knot blame  [options] [rev] [--] path
  knot grep   [options] [pattern]
  knot refs   [options]
  knot stash  [options]
  knot status
  knot worktree
  knot < [git command output]

Options:
  +<number>               Select line <number> in the first view
  -C <path>               Run as if knot was started in <path>
  --config <file>         Use an alternative configuration file
  --theme <name>          Override the theme for this run
  --print-default-config  Print the default configuration and exit
  --check-config          Validate the configuration file and exit
  -v, --version           Show version and exit
  -h, --help              Show this help and exit

Other options (e.g. --all) are passed to git.
`

type cli struct {
	sub         string
	dir         string
	configPath  string
	theme       string
	line        int
	printConfig bool
	checkConfig bool
	showVersion bool
	help        bool
	args        []string // remaining arguments for git
}

var subcommands = map[string]bool{
	"log": true, "show": true, "reflog": true, "blame": true, "grep": true,
	"refs": true, "stash": true, "status": true, "worktree": true,
}

func parseArgs(argv []string) (*cli, error) {
	c := &cli{}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		need := func() (string, error) {
			if i+1 >= len(argv) {
				return "", fmt.Errorf("option %s needs a value", a)
			}
			i++
			return argv[i], nil
		}
		var err error
		switch {
		case a == "--":
			c.args = append(c.args, argv[i:]...)
			i = len(argv)
		case a == "-C":
			c.dir, err = need()
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			c.dir = a[2:]
		case a == "--config":
			c.configPath, err = need()
		case strings.HasPrefix(a, "--config="):
			c.configPath = strings.TrimPrefix(a, "--config=")
		case a == "--theme":
			c.theme, err = need()
		case strings.HasPrefix(a, "--theme="):
			c.theme = strings.TrimPrefix(a, "--theme=")
		case a == "--print-default-config":
			c.printConfig = true
		case a == "--check-config":
			c.checkConfig = true
		case a == "-v" || a == "--version":
			c.showVersion = true
		case a == "-h" || a == "--help":
			c.help = true
		case len(a) > 1 && a[0] == '+':
			n, convErr := strconv.Atoi(a[1:])
			if convErr != nil || n < 1 {
				return nil, fmt.Errorf("invalid line number %q", a)
			}
			c.line = n
		case c.sub == "" && len(c.args) == 0 && subcommands[a]:
			c.sub = a
		default:
			c.args = append(c.args, a)
		}
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

// splitRevsPaths separates "revisions -- paths". Options stay with the
// revisions and are passed to git.
func splitRevsPaths(args []string) (revs, paths []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "knot:", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := parseArgs(os.Args[1:])
	if err != nil {
		return fmt.Errorf("%w\n\n%s", err, usage)
	}
	switch {
	case c.help:
		fmt.Print(usage)
		return nil
	case c.showVersion:
		fmt.Println("knot version", buildVersion())
		return nil
	case c.printConfig:
		_, err := io.WriteString(os.Stdout, config.DefaultFile)
		return err
	}

	path, found, err := config.Locate(c.configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if !found {
		path = ""
	}
	cfg, problems, err := config.Load(path)
	if err != nil {
		return err
	}
	km := keymap.Default()
	keyErrs := km.Apply(cfg.Keys)
	for i, cmd := range cfg.Commands {
		v := cmd.View
		if v == "" {
			v = keymap.Generic
		}
		k, err := keymap.NormalizeKey(cmd.Key)
		if err != nil {
			keyErrs = append(keyErrs, fmt.Sprintf("commands[%d]: %v", i, err))
			continue
		}
		km.Bind(v, k, keymap.Action(keymap.CommandPrefix+strconv.Itoa(i)))
	}
	warnings := app.ConfigWarnings(problems)
	for _, e := range keyErrs {
		warnings = append(warnings, path+": error: "+e)
	}

	themeName := cfg.Theme
	if c.theme != "" {
		themeName = c.theme
		cfg.Theme = c.theme
	}
	themeDir := config.Dir(path)
	if themeName == "auto" {
		themeName = "default"
	}
	th, err := theme.Load(themeDir, themeName)
	if err != nil {
		if c.theme != "" {
			return err
		}
		warnings = append(warnings, err.Error())
		th, _ = theme.Load("", "default")
	}

	if c.checkConfig {
		if path == "" {
			fmt.Println("No configuration file found; using defaults.")
		} else {
			fmt.Println("Checked", path)
		}
		for _, w := range warnings {
			fmt.Println(w)
		}
		if len(warnings) > 0 {
			return errors.New("configuration has problems")
		}
		return nil
	}

	ctx := context.Background()
	gr := git.Runner{Binary: cfg.Git.Binary}
	major, minor, err := gr.Version(ctx)
	if err != nil {
		return fmt.Errorf("cannot run %s: %w", cfg.Git.Binary, err)
	}
	if major < minGitMajor || major == minGitMajor && minor < minGitMinor {
		return fmt.Errorf("git %d.%d is too old; knot needs git %d.%d or newer", major, minor, minGitMajor, minGitMinor)
	}

	dir := c.dir
	if dir == "" {
		if dir, err = os.Getwd(); err != nil {
			return err
		}
	}
	repo, loc, err := git.Open(ctx, cfg.Git.Binary, dir)
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	env := &view.Env{
		Repo: repo, Loc: loc, Cfg: cfg, Theme: th, Keys: km, Opts: view.NewOptions(cfg),
		Version: buildVersion(), HomeDir: home, ThemeDir: themeDir,
	}
	if err := env.ReloadRefs(ctx); err != nil {
		return err
	}

	stdinPiped := !term.IsTerminal(os.Stdin.Fd())
	initial, err := initialView(c, env, stdinPiped)
	if err != nil {
		return err
	}
	if c.line > 0 {
		initial.B().Cursor = c.line - 1
	}

	opts := []tea.ProgramOption{}
	if stdinPiped {
		tty, err := os.Open("/dev/tty")
		if err != nil {
			return fmt.Errorf("stdin is not a terminal and /dev/tty is unavailable: %w", err)
		}
		defer tty.Close()
		opts = append(opts, tea.WithInput(tty))
	}
	m := app.New(env, []view.View{initial}, warnings)
	return app.Run(m, opts...)
}

func initialView(c *cli, env *view.Env, stdinPiped bool) (view.View, error) {
	revs, paths := splitRevsPaths(c.args)
	switch c.sub {
	case "log":
		env.Revs, env.Paths = revs, paths
		return view.NewLog(revs, paths), nil
	case "show":
		commit := "HEAD"
		for _, r := range revs {
			if !strings.HasPrefix(r, "-") {
				commit = r
			}
		}
		return view.NewDiff(commit, paths), nil
	case "reflog":
		return view.NewReflog(revs), nil
	case "blame":
		rev := ""
		file := ""
		switch {
		case len(paths) == 1:
			file = paths[0]
			if len(revs) > 0 {
				rev = revs[len(revs)-1]
			}
		case len(revs) == 1:
			file = revs[0]
		case len(revs) == 2:
			rev, file = revs[0], revs[1]
		default:
			return nil, errors.New("usage: knot blame [rev] [--] path")
		}
		return view.NewBlame(rev, file, 0), nil
	case "grep":
		return view.NewGrep(c.args), nil
	case "refs":
		return view.NewRefs(), nil
	case "stash":
		return view.NewStash(), nil
	case "status":
		return view.NewStatus(), nil
	case "worktree":
		return view.NewWorktree(), nil
	}
	if stdinPiped {
		lines, err := readLines(os.Stdin)
		if err != nil {
			return nil, err
		}
		return view.NewPager("stdin", lines), nil
	}
	env.Revs, env.Paths = revs, paths
	return view.NewMain(revs, paths), nil
}

func readLines(r io.Reader) ([]string, error) {
	var lines []string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		lines = append(lines, strings.TrimSuffix(sc.Text(), "\r"))
	}
	return lines, sc.Err()
}
