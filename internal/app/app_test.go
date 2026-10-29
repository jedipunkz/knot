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
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		case "ctrl+p":
			msg = tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}
		case "ctrl+n":
			msg = tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl}
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
	press(t, m, "ctrl+n") // out of the filter bar, onto the list
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
	if !strings.Contains(line, "repo-topic") || !strings.Contains(line, "[topic]") {
		t.Errorf("status bar = %q", line)
	}
}

func TestWorktreeSwitchOnSwitchStatus(t *testing.T) {
	main, _ := setupRepo(t)
	cfg := config.Default()
	cfg.Worktree.OnSwitch = "status"
	m := newModel(t, main, cfg, view.NewWorktree())
	press(t, m, "ctrl+n", "j", "enter")
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
	press(t, m, "ctrl+n", "j", "enter")
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

func TestMainFilterBar(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewMain(nil, nil))
	if n := m.focused().Len(); n != 2 {
		t.Fatalf("rows = %d, want 2", n)
	}
	// The filter bar is visible from the start, without focus.
	out0, cur0 := m.render()
	if !strings.Contains(out0, "Search:") || !strings.Contains(out0, "(Ctrl-P to filter)") {
		t.Errorf("initial render has no filter bar: %q", firstLine(out0))
	}
	if !strings.Contains(out0, "2/2") {
		t.Errorf("initial render has no match count: %q", firstLine(out0))
	}
	if cur0 != nil {
		t.Error("the initial filter bar must not hold the cursor")
	}
	// k at row 0 does not enter the bar.
	press(t, m, "k")
	if m.filterBar != nil {
		t.Fatal("k entered the filter bar")
	}
	// Ctrl-P at row 0 enters the bar.
	press(t, m, "ctrl+p")
	if m.filterBar == nil {
		t.Fatal("ctrl+p did not focus the filter bar")
	}
	// Typing filters live: "cmone" matches only "commit one" (the subject;
	// "tw" would also fuzzy-match the {wt:...} ref of the other row).
	press(t, m, "c", "m", "o", "n", "e")
	if n := m.focused().Len(); n != 1 {
		t.Fatalf("filtered rows = %d, want 1", n)
	}
	out, cur := m.render()
	if !strings.Contains(out, "Search: cmone") {
		t.Errorf("render = %q", firstLine(out))
	}
	if !strings.Contains(out, "1/2") {
		t.Errorf("render has no match count: %q", firstLine(out))
	}
	if cur == nil {
		t.Error("no cursor on the filter bar")
	}
	// Ctrl-P inside the bar does nothing.
	press(t, m, "ctrl+p")
	if m.filterBar == nil || string(m.filterBar.input) != "cmone" {
		t.Fatalf("ctrl+p inside the bar changed state: %q", string(m.filterBar.input))
	}
	// Enter returns to the list with the filter kept, cursor on the match.
	press(t, m, "enter")
	if m.filterBar != nil {
		t.Error("enter did not leave the bar")
	}
	if n := m.focused().Len(); n != 1 {
		t.Fatalf("rows after enter = %d, want 1", n)
	}
	one := m.focused().Selection(m.env).Commit
	if one == "" {
		t.Fatal("no selection")
	}
	// Esc in the filtered list clears the filter and keeps the commit.
	press(t, m, "esc")
	if n := m.focused().Len(); n != 2 {
		t.Fatalf("rows after esc = %d, want 2", n)
	}
	if sel := m.focused().Selection(m.env); sel.Commit != one {
		t.Errorf("selection after esc = %q, want %q", sel.Commit, one)
	}
	// Up reaches row 0, then the next Up enters the bar.
	press(t, m, "up")
	if m.filterBar != nil {
		t.Fatal("up above row 0 entered the bar")
	}
	press(t, m, "up")
	if m.filterBar == nil {
		t.Error("up at row 0 did not enter the bar")
	}
	// Ctrl-N returns to the list keeping the filter.
	press(t, m, "c", "m", "o", "n", "e", "ctrl+n")
	if m.filterBar != nil || m.focused().Len() != 1 {
		t.Fatalf("after ctrl+n: bar=%v rows=%d", m.filterBar != nil, m.focused().Len())
	}
	// Esc in the bar clears the query and restores the full list.
	press(t, m, "up", "esc")
	if m.filterBar != nil || m.focused().Len() != 2 {
		t.Fatalf("esc in the bar: bar=%v rows=%d", m.filterBar != nil, m.focused().Len())
	}
	// The filter action is unbound by default but bindable.
	m.env.Keys.Bind("main", "P", keymap.Filter)
	press(t, m, "P")
	if m.filterBar == nil {
		t.Fatal("bound filter key did not open the bar")
	}
	press(t, m, "ctrl+c")
	if m.filterBar != nil || m.focused().Len() != 2 {
		t.Fatalf("ctrl+c in the bar: bar=%v rows=%d", m.filterBar != nil, m.focused().Len())
	}
}

func TestKeyBar(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewMain(nil, nil))
	// The curated entries resolve against the live keymap.
	out, _ := m.render()
	for _, want := range []string{"head", "cherry-pick"} {
		if !strings.Contains(out, want) {
			t.Errorf("keybar missing %q", want)
		}
	}
	// Deep entries appear when the bar has room.
	if !strings.Contains(m.keybarRow(200), "prompt") {
		t.Error("keybar entry for the prompt missing")
	}
	// The keybar sits on its own row directly above the status bar.
	lines := strings.Split(out, "\n")
	bar, status := lines[len(lines)-2], lines[len(lines)-1]
	if !strings.Contains(bar, "close") || strings.Contains(status, "close") {
		t.Errorf("keybar = %q, status = %q", bar, status)
	}
	// While the filter bar holds the focus no keybar is shown.
	press(t, m, "ctrl+p")
	out, _ = m.render()
	if strings.Contains(out, "cherry-pick") {
		t.Error("keybar shown while the filter bar is focused")
	}
	press(t, m, "esc")
	// ui.keybar = false hides the row.
	m.env.Cfg.UI.KeyBar = false
	out, _ = m.render()
	if strings.Contains(out, "cherry-pick") {
		t.Error("keybar shown with ui.keybar = false")
	}
}

// Quitting with q empties the view stack; Bubble Tea still renders a
// final frame, which must not reach into a missing view.
func TestRenderAfterLastViewClosed(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewMain(nil, nil))
	press(t, m, "q")
	if len(m.stack) != 0 {
		t.Fatalf("stack = %d, want 0", len(m.stack))
	}
	m.View()
}

func TestLoadingSpinner(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewMain(nil, nil))
	// The title shows the spinner while the view loads.
	m.focused().B().Loading = true
	m.frame = 1
	if got := m.spinner(); got != spinnerFrames[1] {
		t.Errorf("frame = %q, want %q", got, spinnerFrames[1])
	}
	out, _ := m.render()
	if !strings.Contains(out, spinnerFrames[1]+" Loading") {
		t.Errorf("render has no spinner: %q", firstLine(out))
	}
	if _, cmd := m.Update(spinnerMsg{}); cmd == nil {
		t.Error("spinner stopped while the view is still loading")
	}
	// When loading finished the spinner stops.
	m.focused().B().Loading = false
	if _, cmd := m.Update(spinnerMsg{}); cmd != nil {
		t.Error("spinner kept running after loading finished")
	}
}

// firstLine returns the first line of a rendered screen.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func TestWorktreeFilterBar(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewWorktree())
	// The filter bar is focused right after opening the worktree view.
	if m.filterBar == nil {
		t.Fatal("filter bar not focused on open")
	}
	if name := m.focused().Name(); name != "worktree" {
		t.Fatalf("focused = %s", name)
	}
	// Typing filters live: "z" appears in none of the worktree paths,
	// branches or subjects with this fixture.
	press(t, m, "z")
	if n := m.focused().Len(); n != 0 {
		t.Fatalf("filtered rows = %d, want 0", n)
	}
	if m.filterBar == nil {
		t.Error("the bar lost focus while typing")
	}
	// Backspace restores the full list without leaving the bar.
	press(t, m, "backspace")
	if n := m.focused().Len(); n != 2 {
		t.Fatalf("rows after backspace = %d, want 2", n)
	}
	// Ctrl-N moves to the list with the filter kept; the key bar shows
	// the selected worktree's info.
	press(t, m, "ctrl+n")
	if m.filterBar != nil {
		t.Error("ctrl+n did not move to the list")
	}
	if sel := m.focused().Selection(m.env); sel.Worktree == "" {
		t.Errorf("selection empty")
	}
	out, _ := m.render()
	// The selected worktree's info is shown above the status bar.
	split := strings.Split(out, "\n")
	if !strings.Contains(split[len(split)-2], "main") {
		t.Errorf("keybar info = %q", split[len(split)-2])
	}
	press(t, m, "ctrl+p")
	if m.filterBar == nil {
		t.Error("ctrl+p did not focus the filter bar")
	}
	press(t, m, "enter")
	if m.filterBar != nil {
		t.Error("enter did not move to the list")
	}
	if sel := m.focused().Selection(m.env); sel.Worktree == "" {
		t.Errorf("selection after enter empty")
	}
	// Enter in the list resets the view stack (switching the worktree).
	press(t, m, "enter")
	if len(m.stack) != 1 {
		t.Fatalf("stack after switch = %d, want 1", len(m.stack))
	}
	if m.filterBar != nil {
		t.Error("the switch kept the filter bar focused")
	}
	if m.focused().Name() != "main" {
		t.Errorf("focused after switch = %s", m.focused().Name())
	}
}

func TestWorktreeEscClearsFilter(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewWorktree())
	press(t, m, "z", "ctrl+n")
	if n := m.focused().Len(); n != 0 {
		t.Fatal("filter did not narrow the list")
	}
	press(t, m, "esc")
	if m.focused().Len() != 2 {
		t.Fatalf("esc did not clear the filter: %d rows", m.focused().Len())
	}
	if sel := m.focused().Selection(m.env); sel.Worktree == "" {
		t.Errorf("selection after esc empty")
	}
	// Ctrl-P on the (cleared) list re-focuses the bar.
	press(t, m, "ctrl+p")
	if m.filterBar == nil || string(m.filterBar.input) != "" {
		t.Errorf("ctrl+p after esc: bar=%v input=%q", m.filterBar != nil, string(m.filterBar.input))
	}
}

func TestWorktreeCtrlNavMovesCursor(t *testing.T) {
	main, _ := setupRepo(t)
	m := newModel(t, main, nil, view.NewWorktree())
	// Ctrl-N first exits the filter bar, then moves down the list.
	press(t, m, "ctrl+n")
	if m.filterBar != nil {
		t.Fatal("ctrl+n did not move to the list")
	}
	cur := m.focused().(*view.WorktreeView).Cursor
	press(t, m, "ctrl+n")
	if got := m.focused().(*view.WorktreeView).Cursor; got != cur+1 {
		t.Fatalf("ctrl+n cursor = %d, want %d", got, cur+1)
	}
	press(t, m, "ctrl+p")
	if got := m.focused().(*view.WorktreeView).Cursor; got != cur {
		t.Fatalf("ctrl+p cursor = %d, want %d", got, cur)
	}
	// Ctrl-P on the first row re-focuses the filter bar.
	press(t, m, "ctrl+p")
	if m.filterBar == nil {
		t.Error("ctrl+p on the first row did not focus the filter bar")
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
