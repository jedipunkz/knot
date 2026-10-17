package main

import (
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	c, err := parseArgs([]string{"-C", "/repo", "--theme=dark", "+12", "log", "--all", "main", "--", "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	if c.dir != "/repo" || c.theme != "dark" || c.line != 12 || c.sub != "log" {
		t.Errorf("parsed %+v", c)
	}
	revs, paths := splitRevsPaths(c.args)
	if strings.Join(revs, " ") != "--all main" || strings.Join(paths, " ") != "a.go" {
		t.Errorf("revs=%q paths=%q", revs, paths)
	}

	// A subcommand name after a revision is a revision, not a subcommand.
	c, err = parseArgs([]string{"main", "status"})
	if err != nil || c.sub != "" || strings.Join(c.args, " ") != "main status" {
		t.Errorf("parsed %+v, %v", c, err)
	}
	if _, err := parseArgs([]string{"+x"}); err == nil {
		t.Error("expected error for invalid line number")
	}
	if _, err := parseArgs([]string{"--config"}); err == nil {
		t.Error("expected error for missing value")
	}
}
