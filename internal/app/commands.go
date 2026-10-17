package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/view"
)

// placeholders returns the values for %(name) substitution.
func placeholders(env *view.Env, sel view.Selection) map[string]string {
	p := map[string]string{
		"commit":        sel.Commit,
		"head":          env.Head.Hash,
		"branch":        env.Head.Branch,
		"file":          sel.File,
		"directory":     sel.Directory,
		"stash":         sel.Stash,
		"repo:worktree": env.Loc.TopLevel,
		"worktree":      sel.Worktree,
		"lineno":        "",
		"remote":        "",
		"tag":           "",
	}
	if sel.Line > 0 {
		p["lineno"] = strconv.Itoa(sel.Line)
	}
	if p["worktree"] == "" {
		p["worktree"] = env.Loc.TopLevel
	}
	if sel.Ref != nil {
		switch sel.Ref.Kind {
		case git.RefBranch:
			p["branch"] = sel.Ref.Short
		case git.RefRemote:
			p["branch"] = sel.Ref.Short
			if r, _, ok := strings.Cut(sel.Ref.Short, "/"); ok {
				p["remote"] = r
			}
		case git.RefTag:
			p["tag"] = sel.Ref.Short
		}
	}
	return p
}

// expandArgv substitutes environment variables first and placeholders
// second, per argument. Placeholder values (branch names, file names, commit
// messages) are never re-interpreted.
func expandArgv(argv []string, values map[string]string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		a = os.ExpandEnv(a)
		var sb strings.Builder
		for {
			start := strings.Index(a, "%(")
			if start < 0 {
				sb.WriteString(a)
				break
			}
			end := strings.Index(a[start:], ")")
			if end < 0 {
				sb.WriteString(a)
				break
			}
			name := a[start+2 : start+end]
			sb.WriteString(a[:start])
			if v, ok := values[name]; ok {
				sb.WriteString(v)
			} else {
				sb.WriteString(a[start : start+end+1])
			}
			a = a[start+end+1:]
		}
		out[i] = sb.String()
	}
	return out
}

func (m *Model) runCommand(c config.Command, sel view.Selection) tea.Cmd {
	argv := expandArgv(c.Argv, placeholders(m.env, sel))
	label := strings.Join(argv, " ")
	run := func() tea.Msg {
		if c.Output == "suspend" {
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Dir = m.env.WorktreeDir()
			return view.ExecMsg{Cmd: cmd}
		}
		return m.runCaptured(argv, c.Output, label)()
	}
	if c.Confirm {
		return view.Msg(view.ConfirmMsg{Question: "Run " + label + "?", OnYes: run})
	}
	return run
}

// runCaptured runs argv without a terminal and shows the output according
// to the output mode (status | pager | none).
func (m *Model) runCaptured(argv []string, output, label string) tea.Cmd {
	dir := m.env.WorktreeDir()
	return func() tea.Msg {
		cmd := exec.CommandContext(context.Background(), argv[0], argv[1:]...)
		cmd.Dir = dir
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		err := cmd.Run()
		text := strings.TrimRight(out.String(), "\n")
		if err != nil {
			var ee *exec.ExitError
			msg := err.Error()
			if errors.As(err, &ee) && text != "" {
				msg = lastLine(text)
			}
			if output == "pager" && text != "" {
				return tea.BatchMsg{view.Msg(view.OpenMsg{View: view.NewPager(label, strings.Split(text, "\n"))}),
					view.Errorf(fmt.Errorf("%s: %s", label, msg))}
			}
			return tea.BatchMsg{view.Errorf(fmt.Errorf("%s: %s", label, msg)), view.Msg(view.ReloadMsg{})}
		}
		switch output {
		case "pager":
			var lines []string
			if text != "" {
				lines = strings.Split(text, "\n")
			}
			return view.OpenMsg{View: view.NewPager(label, lines)}
		case "none":
			return view.ReloadMsg{}
		default:
			msg := "Done: " + label
			if text != "" {
				msg = lastLine(text)
			}
			return tea.BatchMsg{view.Status(msg), view.Msg(view.ReloadMsg{})}
		}
	}
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// splitArgs splits a command line into arguments, honoring single and
// double quotes and backslash escapes. No shell expansion happens.
func splitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	var quote rune
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && quote != '\'':
			escaped = true
			inArg = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case r == ' ' || r == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}
