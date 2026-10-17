package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
	"github.com/jedipunkz/knot/internal/view"
)

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupRepo creates a repository with two commits, a branch "topic"
// checked out in a linked worktree and a branch "other".
func setupRepo(t *testing.T) (main, wt string) {
	t.Helper()
	root := t.TempDir()
	main = filepath.Join(root, "repo")
	wt = filepath.Join(root, "repo-topic")
	if err := os.MkdirAll(main, 0o755); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, main, "init", "-q", "-b", "main")
	gitCmd(t, main, "config", "user.name", "T")
	gitCmd(t, main, "config", "user.email", "t@example.com")
	gitCmd(t, main, "config", "commit.gpgsign", "false")
	for i, c := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(main, "f.txt"), []byte(strings.Repeat(c+"\n", i+1)), 0o644); err != nil {
			t.Fatal(err)
		}
		gitCmd(t, main, "add", "f.txt")
		gitCmd(t, main, "commit", "-q", "-m", "commit "+c)
	}
	gitCmd(t, main, "branch", "topic", "HEAD~1")
	gitCmd(t, main, "branch", "other")
	gitCmd(t, main, "worktree", "add", "-q", wt, "topic")
	return main, wt
}

func newModel(t *testing.T, dir string, cfg *config.Config, initial view.View) *Model {
	t.Helper()
	if cfg == nil {
		cfg = config.Default()
	}
	repo, loc, err := git.Open(context.Background(), "git", dir)
	if err != nil {
		t.Fatal(err)
	}
	th, _ := theme.Load("", "default")
	env := &view.Env{Repo: repo, Loc: loc, Cfg: cfg, Theme: th, Keys: keymap.Default(), Opts: view.NewOptions(cfg), Version: "test"}
	if err := env.ReloadRefs(context.Background()); err != nil {
		t.Fatal(err)
	}
	m := New(env, []view.View{initial}, nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	drive(t, m, m.Init(), 0)
	return m
}

// drive runs commands synchronously and feeds their messages back.
func drive(t *testing.T, m *Model, cmd tea.Cmd, depth int) {
	t.Helper()
	if cmd == nil || depth > 100 {
		return
	}
	msg := cmd()
	switch msg := msg.(type) {
	case nil, tea.QuitMsg:
		return
	case tea.BatchMsg:
		for _, c := range msg {
			drive(t, m, c, depth+1)
		}
	default:
		_, next := m.Update(msg)
		drive(t, m, next, depth+1)
	}
}

func press(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		default:
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		_, cmd := m.Update(msg)
		drive(t, m, cmd, 0)
	}
}

func TestWorktreeSwitchOpensMainAtHead(t *testing.T) {
	main, wt := setupRepo(t)
	m := newModel(t, main, nil, view.NewMain(nil, nil))
	press(t, m, "s") // push another view so the stack reset is visible
	press(t, m, "w")
	if got := m.focused().Name(); got != "worktree" {
		t.Fatalf("focused = %s", got)
	}
	if n := m.focused().Len(); n != 2 {
		t.Fatalf("worktree rows = %d", n)
	}
	press(t, m, "j", "enter")
	if len(m.stack) != 1 || m.focused().Name() != "main" {
		t.Fatalf("stack after switch: %d views, focused %s", len(m.stack), m.focused().Name())
	}
	want, _ := filepath.EvalSymlinks(wt)
	got, _ := filepath.EvalSymlinks(m.env.Loc.TopLevel)
	if got != want {
		t.Errorf("active worktree = %s, want %s", got, want)
	}
	if m.env.Head.Branch != "topic" {
		t.Errorf("head branch = %q", m.env.Head.Branch)
	}
	sel := m.focused().Selection(m.env)
	if sel.Commit != m.env.Head.Hash {
		t.Errorf("cursor on %s, want HEAD %s", sel.Commit, m.env.Head.Hash)
	}
	line, _ := m.statusLine(100, 29)
	if !strings.Contains(line, "repo-topic [topic]") {
		t.Errorf("status bar = %q", line)
	}
}

func TestWorktreeSwitchOnSwitchStatus(t *testing.T) {
	main, _ := setupRepo(t)
	cfg := config.Default()
	cfg.Worktree.OnSwitch = "status"
	m := newModel(t, main, cfg, view.NewWorktree())
	press(t, m, "j", "enter")
	if m.focused().Name() != "status" {
		t.Errorf("focused = %s", m.focused().Name())
	}
}

func TestWorktreeSwitchRejectsPrunable(t *testing.T) {
	main, wt := setupRepo(t)
	if err := os.RemoveAll(wt); err != nil {
		t.Fatal(err)
	}
	m := newModel(t, main, nil, view.NewWorktree())
	press(t, m, "j", "enter")
	if m.focused().Name() != "worktree" || !m.statusErr || !strings.Contains(m.status, "prunable") {
		t.Errorf("focused %s, status %q (err=%v)", m.focused().Name(), m.status, m.statusErr)
	}
}

func TestSplitNavigation(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewMain(nil, nil))
	press(t, m, "enter")
	if !m.split() || m.focused().Name() != "diff" {
		t.Fatalf("expected diff split, stack=%d focused=%s", len(m.stack), m.focused().Name())
	}
	first := m.focused().(*view.DiffView).Commit
	press(t, m, "down")
	if !m.split() || m.focused().Name() != "diff" {
		t.Fatalf("after down: split=%v focused=%s", m.split(), m.focused().Name())
	}
	second := m.focused().(*view.DiffView).Commit
	if first == second || second != m.stack[0].v.Selection(m.env).Commit {
		t.Errorf("child not updated: %s -> %s", first, second)
	}
	if len(m.stack) != 2 {
		t.Errorf("stack = %d", len(m.stack))
	}
	press(t, m, "tab")
	if m.focused().Name() != "main" {
		t.Errorf("tab should focus parent, got %s", m.focused().Name())
	}
	press(t, m, "q")
	if len(m.stack) != 1 {
		t.Errorf("q should close the child, stack = %d", len(m.stack))
	}
	// Render at a tiny size and vertical split must not panic.
	m.env.Cfg.UI.SplitDirection = "vertical"
	press(t, m, "enter")
	m.Update(tea.WindowSizeMsg{Width: 10, Height: 4})
	_ = m.View()
}

func TestConfirmCancelAndAccept(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewRefs())
	// Rows: main(*), other, topic(+) — select "other".
	for i := 0; i < m.focused().Len(); i++ {
		if r := m.focused().Selection(m.env).Ref; r != nil && r.Short == "other" {
			break
		}
		press(t, m, "j")
	}
	press(t, m, "C")
	if m.confirm == nil {
		t.Fatal("expected confirmation")
	}
	press(t, m, "n")
	if h := git.ReadHead(context.Background(), m.env.Git()); h.Branch != "main" {
		t.Fatalf("checkout ran without confirmation: %s", h.Branch)
	}
	press(t, m, "C", "y")
	if h := git.ReadHead(context.Background(), m.env.Git()); h.Branch != "other" {
		t.Errorf("checkout not executed: %q (status %q)", h.Branch, m.status)
	}
}

func TestCheckoutBlockedForOtherWorktree(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewRefs())
	for i := 0; i < m.focused().Len(); i++ {
		if r := m.focused().Selection(m.env).Ref; r != nil && r.Short == "topic" {
			break
		}
		press(t, m, "j")
	}
	press(t, m, "C")
	if m.confirm != nil || !strings.Contains(m.status, "checked out in worktree") {
		t.Errorf("confirm=%v status=%q", m.confirm != nil, m.status)
	}
}

func TestUserCommandPlaceholders(t *testing.T) {
	values := map[string]string{"branch": "feat; rm -rf /", "commit": "abc"}
	t.Setenv("KNOT_TEST_VAR", "x")
	got := expandArgv([]string{"git", "rebase", "%(branch)", "%(commit)..HEAD", "$KNOT_TEST_VAR", "%(unknown)"}, values)
	want := []string{"git", "rebase", "feat; rm -rf /", "abc..HEAD", "x", "%(unknown)"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("expandArgv = %q", got)
	}
	// A placeholder value containing $VAR must not be expanded.
	got = expandArgv([]string{"%(branch)"}, map[string]string{"branch": "$KNOT_TEST_VAR"})
	if got[0] != "$KNOT_TEST_VAR" {
		t.Errorf("placeholder value was expanded: %q", got[0])
	}
}

func TestSplitArgs(t *testing.T) {
	got, err := splitArgs(`git log --format="%h %s" 'a b' c\ d`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"git", "log", "--format=%h %s", "a b", "c d"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("splitArgs = %q", got)
	}
	if _, err := splitArgs(`"open`); err == nil {
		t.Error("expected unterminated quote error")
	}
}

func TestUserCommandPager(t *testing.T) {
	main, _ := setupRepo(t)
	cfg := config.Default()
	cfg.Commands = []config.Command{{Key: "F", View: "main", Argv: []string{"git", "log", "--format=%s", "%(commit)"}, Output: "pager"}}
	m := newModel(t, main, cfg, view.NewMain(nil, nil))
	m.env.Keys.Bind("main", "F", keymap.Action(keymap.CommandPrefix+"0"))
	press(t, m, "F")
	if m.focused().Name() != "pager" {
		t.Fatalf("focused = %s (status %q)", m.focused().Name(), m.status)
	}
	if got := view.Plain(m.env, m.focused(), 0); got != "commit two" {
		t.Errorf("pager line 0 = %q", got)
	}
}
