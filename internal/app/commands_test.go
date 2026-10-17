package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/view"
)

func TestUserCommandCompletesAndDeregisters(t *testing.T) {
	_, wt := setupRepo(t)
	cfg := config.Default()
	cfg.Commands = []config.Command{{Argv: []string{"echo", "hello"}, Output: "status"}}
	m := newModel(t, wt, cfg, view.NewMain(nil, nil))

	cmd := m.runCommand(cfg.Commands[0], view.Selection{})
	if len(m.userCancels) != 1 {
		t.Fatalf("registered entries = %d, want 1", len(m.userCancels))
	}
	drive(t, m, cmd, 0)
	if len(m.userCancels) != 0 {
		t.Errorf("entries after completion = %d, want 0", len(m.userCancels))
	}
	// The status shows the last output line of the command, not the
	// label, when there is output.
	if !strings.Contains(m.status, "hello") {
		t.Errorf("status = %q", m.status)
	}
}

func TestStopLoadingCancelsUserCommands(t *testing.T) {
	_, wt := setupRepo(t)
	cfg := config.Default()
	cfg.Commands = []config.Command{{Argv: []string{"echo", "hello"}, Output: "status"}}
	m := newModel(t, wt, cfg, view.NewMain(nil, nil))

	ctx, id := m.registerCaptured()
	if len(m.userCancels) != 1 {
		t.Fatalf("registered entries = %d, want 1", len(m.userCancels))
	}
	msg := m.dispatch(keymap.StopLoading)()
	_ = msg
	if err := ctx.Err(); err != context.Canceled {
		t.Errorf("in-flight user command not canceled: %v", err)
	}
	if _, next := m.Update(userDoneMsg{id: id, msg: view.Status("stopped")}); next != nil {
		t.Errorf("completion produced %v", next)
	}
	if len(m.userCancels) != 0 {
		t.Error("entry not deregistered after completion")
	}
}

func TestUserCommandConfirmDeclineDeregisters(t *testing.T) {
	_, wt := setupRepo(t)
	cfg := config.Default()
	cfg.Commands = []config.Command{{Argv: []string{"touch", "executed"}, Output: "status", Confirm: true}}
	m := newModel(t, wt, cfg, view.NewMain(nil, nil))

	cmd := m.runCommand(cfg.Commands[0], view.Selection{})
	if len(m.userCancels) != 1 {
		t.Fatalf("registered entries = %d, want 1", len(m.userCancels))
	}
	drive(t, m, cmd, 0)
	if m.confirm == nil {
		t.Fatal("no confirmation form")
	}
	if len(m.userCancels) != 1 {
		t.Errorf("entries while the form is open = %d, want 1", len(m.userCancels))
	}
	press(t, m, "n")
	if m.confirm != nil {
		t.Error("form still open")
	}
	if len(m.userCancels) != 0 {
		t.Error("declined entry kept registered")
	}
	if _, err := os.Stat(filepath.Join(wt, "executed")); !os.IsNotExist(err) {
		t.Error("declined command was executed")
	}
}

func TestUserCommandConfirmAcceptRunsAndDeregisters(t *testing.T) {
	_, wt := setupRepo(t)
	cfg := config.Default()
	cfg.Commands = []config.Command{{Argv: []string{"touch", "executed"}, Output: "status", Confirm: true}}
	m := newModel(t, wt, cfg, view.NewMain(nil, nil))

	cmd := m.runCommand(cfg.Commands[0], view.Selection{})
	drive(t, m, cmd, 0)
	press(t, m, "y")
	if _, err := os.Stat(filepath.Join(wt, "executed")); err != nil {
		t.Errorf("accepted command did not run: %v", err)
	}
	if len(m.userCancels) != 0 {
		t.Error("entry not deregistered after completion")
	}
}
