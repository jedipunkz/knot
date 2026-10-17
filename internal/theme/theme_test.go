package theme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinsResolve(t *testing.T) {
	for _, name := range BuiltinNames {
		th, err := Load("", name)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, r := range AllRoles() {
			if _, ok := th.specs[r]; !ok {
				t.Errorf("%s: role %s missing", name, r)
			}
		}
	}
}

func TestParseColor(t *testing.T) {
	for _, ok := range []string{"#fff", "#a6e3a1", "0", "255", "green", "bright-black", "default", ""} {
		if _, err := ParseColor(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"#12", "#zzzzzz", "256", "-1", "chartreuse"} {
		if _, err := ParseColor(bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestUserThemeExtends(t *testing.T) {
	dir := t.TempDir()
	themes := filepath.Join(dir, "themes")
	if err := os.MkdirAll(themes, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(themes, name+".toml"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("mine", `extends = "gruvbox"
[styles]
diff-add = { fg = "#00ff00", attrs = ["bold"] }
`)
	write("loop-a", `extends = "loop-b"`)
	write("loop-b", `extends = "loop-a"`)
	write("badrole", `[styles]
nonsense = { fg = "red" }
`)

	th, err := Load(dir, "mine")
	if err != nil {
		t.Fatal(err)
	}
	if th.specs[DiffAdd].Fg != "#00ff00" {
		t.Errorf("override not applied: %+v", th.specs[DiffAdd])
	}
	if th.specs[DiffDel].Fg != builtin["gruvbox"][DiffDel].Fg {
		t.Errorf("extends not applied: %+v", th.specs[DiffDel])
	}
	if _, err := Load(dir, "loop-a"); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got %v", err)
	}
	if _, err := Load(dir, "badrole"); err == nil {
		t.Error("expected unknown role error")
	}
	if _, err := Load(dir, "../escape"); err == nil {
		t.Error("expected error for path-like name")
	}
	names := Names(dir)
	if names[len(names)-1] != "mine" && !contains(names, "mine") {
		t.Errorf("user theme not listed: %v", names)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestDimVariants(t *testing.T) {
	for _, name := range BuiltinNames {
		th, err := Load("", name)
		if err != nil {
			t.Fatal(err)
		}
		cases := map[Role]Role{AuthorDim: Author, DateDim: Date, CommitIDDim: CommitID, RefDim: RefBranch}
		for dim, val := range cases {
			valSpec := th.specs[val]
			dimSpec := th.specs[dim]
			if dimSpec.Fg != valSpec.Fg {
				t.Errorf("%s: %s fg = %q, want the %s color %q", name, dim, dimSpec.Fg, val, valSpec.Fg)
			}
			if !contains(dimSpec.Attrs, "dim") {
				t.Errorf("%s: %s attrs = %v, want dim attr", name, dim, dimSpec.Attrs)
			}
		}
	}
}
