package keymap

import "testing"

// TestTigDefaults checks the default bindings documented in the design issue.
func TestTigDefaults(t *testing.T) {
	km := Default()
	cases := []struct {
		view, key string
		want      Action
	}{
		{"main", "m", ViewMain}, {"main", "d", ViewDiff}, {"main", "l", ViewLog}, {"main", "p", ViewPager},
		{"main", "t", ViewTree}, {"main", "f", ViewBlob}, {"main", "g", ViewGrep}, {"main", "b", ViewBlame},
		{"main", "r", ViewRefs}, {"main", "y", ViewStash}, {"main", "h", ViewHelp}, {"main", "s", ViewStatus},
		{"main", "c", ViewStage}, {"main", "w", ViewWorktree},
		{"main", "q", ViewClose}, {"main", "Q", Quit}, {"main", "enter", Enter}, {"main", "tab", ViewNext},
		{"main", "R", Refresh}, {"main", "O", Maximize}, {"tree", ",", Parent},
		{"main", "k", MoveUp}, {"main", "j", MoveDown}, {"main", "up", Previous}, {"main", "down", Next}, {"main", "ctrl+p", Previous}, {"main", "ctrl+n", Next},
		{"main", "pgup", MovePageUp}, {"main", "-", MovePageUp}, {"main", "pgdown", MovePageDown}, {"main", "space", MovePageDown},
		{"main", "home", MoveFirstLine}, {"main", "end", MoveLastLine},
		{"main", "insert", ScrollLineUp}, {"main", "delete", ScrollLineDown},
		{"main", "left", ScrollLeft}, {"main", "right", ScrollRight}, {"main", "|", ScrollFirstCol},
		{"main", "/", Search}, {"main", "?", SearchBack}, {"main", "n", FindNext}, {"main", "N", FindPrev}, {"main", ":", Prompt},
		{"main", "#", ToggleLineNo}, {"main", "D", ToggleDate}, {"main", "A", ToggleAuthor}, {"main", "G", ToggleGraph},
		{"main", "~", ToggleLineGraphics}, {"main", "F", ToggleRefs}, {"main", "W", ToggleIgnoreSpace}, {"main", "X", ToggleID},
		{"main", "%", ToggleFileFilter}, {"main", "$", ToggleTitleOverflow}, {"main", "H", MoveHead}, {"main", "o", Options},
		{"main", "z", StopLoading}, {"main", "v", ShowVersion}, {"main", "ctrl+l", ScreenRedraw}, {"main", "T", ThemePicker},
		{"diff", "[", DiffContextDown}, {"diff", "]", DiffContextUp}, {"blob", "e", Edit},
		{"status", "u", StatusUpdate}, {"stage", "u", StatusUpdate}, {"stage", "1", StageUpdateLine}, {"stage", "@", StageNext},
		{"status", "!", StatusRevert}, {"status", "M", StatusMerge}, {"status", "C", StatusCommit},
		{"main", "C", CherryPick}, {"refs", "C", Checkout}, {"refs", "!", BranchDelete},
		{"stash", "A", StashApply}, {"stash", "P", StashPop}, {"stash", "!", StashDrop},
		{"worktree", "a", WorktreeAdd}, {"worktree", "!", WorktreeRemove}, {"worktree", "L", WorktreeLock},
		{"worktree", "P", WorktreePrune}, {"worktree", "o", WorktreeOpen}, {"worktree", "enter", Enter},
	}
	for _, c := range cases {
		got, ok := km.Lookup(c.view, c.key)
		if !ok || got != c.want {
			t.Errorf("%s %q = %q, want %q", c.view, c.key, got, c.want)
		}
	}
	for a := range Descriptions {
		if !Valid(a) {
			t.Errorf("%s not valid", a)
		}
	}
}

func TestApplyOverrides(t *testing.T) {
	km := Default()
	errs := km.Apply(map[string]map[string]string{
		"generic": {"ctrl-d": "move-half-page-down", "x": "bogus"},
		"main":    {"C": "none"},
	})
	if len(errs) != 1 {
		t.Errorf("errs = %v", errs)
	}
	if a, ok := km.Lookup("main", "ctrl+d"); !ok || a != MoveHalfPageDown {
		t.Errorf("ctrl+d = %q %v", a, ok)
	}
	if _, ok := km.Lookup("main", "C"); ok {
		t.Error("C must be unbound in main")
	}
	if a, _ := km.Lookup("refs", "C"); a != Checkout {
		t.Error("refs C must stay")
	}
}

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"ctrl-d": "ctrl+d", "Ctrl+D": "ctrl+d", "<C-L>": "", "PgUp": "pgup", "PageDown": "pgdown",
		"<Enter>": "enter", "Space": "space", " ": "space", "A": "A", "-": "-", "F5": "f5", "alt-shift-x": "alt+shift+x",
	}
	for in, want := range cases {
		got, err := NormalizeKey(in)
		if want == "" {
			if err == nil {
				t.Errorf("%q: expected error, got %q", in, got)
			}
			continue
		}
		if err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", in, got, err, want)
		}
	}
}
