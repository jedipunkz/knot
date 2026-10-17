package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultFileMatchesDefault(t *testing.T) {
	cfg, problems, err := Parse("default", DefaultFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems in default file: %v", problems)
	}
	cfg.Path = ""
	want := Default()
	if cfg.Git.LogArgs != nil && len(cfg.Git.LogArgs) == 0 {
		cfg.Git.LogArgs = nil
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("default file differs from Default():\n got %+v\nwant %+v", cfg, want)
	}
}

func TestParseValuesAndProblems(t *testing.T) {
	src := `theme = "gruvbox"
bogus = 1

[ui]
split-direction = "diagonal"
tab-size = 4

[main]
author = "email"

[worktree]
on-switch = "status"

[keys.main]
"C" = "none"

[keys.nope]
"x" = "quit"

[[commands]]
key = "F"
view = "main"
argv = ["git", "fetch"]

[[commands]]
key = "G"
argv = ["git", "gc"]
output = "loud"
`
	cfg, problems, err := Parse("knot.toml", src)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "gruvbox" || cfg.UI.TabSize != 4 || cfg.Main.Author != "email" || cfg.Worktree.OnSwitch != "status" {
		t.Errorf("values not applied: %+v", cfg)
	}
	if cfg.UI.SplitDirection != "horizontal" {
		t.Errorf("invalid value must fall back to default, got %q", cfg.UI.SplitDirection)
	}
	if cfg.Keys["main"]["C"] != "none" {
		t.Errorf("keys.main not decoded: %v", cfg.Keys)
	}
	if _, ok := cfg.Keys["nope"]; ok {
		t.Error("unknown view must be removed")
	}
	if len(cfg.Commands) != 1 || cfg.Commands[0].Output != "status" {
		t.Errorf("commands = %+v", cfg.Commands)
	}
	var msgs []string
	for _, p := range problems {
		msgs = append(msgs, p.String())
	}
	joined := strings.Join(msgs, "\n")
	for _, want := range []string{
		"knot.toml:2: warning: unknown key \"bogus\"",
		"knot.toml:5: error: ui.split-direction",
		"knot.toml:17: error: unknown view \"nope\"",
		"output must be",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing problem %q in:\n%s", want, joined)
		}
	}
}

func TestParseSyntaxError(t *testing.T) {
	_, _, err := Parse("bad.toml", "a = \n")
	if err == nil || !strings.Contains(err.Error(), "bad.toml:1") {
		t.Errorf("err = %v", err)
	}
}

func TestLocateOrder(t *testing.T) {
	dir := t.TempDir()
	xdg := filepath.Join(dir, "xdg")
	if err := os.MkdirAll(filepath.Join(xdg, "knot"), 0o755); err != nil {
		t.Fatal(err)
	}
	xdgFile := filepath.Join(xdg, "knot", "knot.toml")
	if err := os.WriteFile(xdgFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(dir, "env.toml")
	if err := os.WriteFile(envFile, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", filepath.Join(dir, "home"))
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("KNOT_CONFIG", envFile)

	if p, ok, _ := Locate(""); !ok || p != envFile {
		t.Errorf("KNOT_CONFIG should win, got %q %v", p, ok)
	}
	t.Setenv("KNOT_CONFIG", "")
	if p, ok, _ := Locate(""); !ok || p != xdgFile {
		t.Errorf("XDG should be used, got %q %v", p, ok)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	if _, ok, _ := Locate(""); ok {
		t.Error("no file expected")
	}
	if _, ok, err := Locate(filepath.Join(dir, "missing.toml")); ok || err == nil {
		t.Error("--config with a missing file must fail")
	}
}
