// Package keymap maps keys to named actions. Defaults follow tig.
package keymap

import (
	"fmt"
	"sort"
	"strings"
)

// Action is a named command.
type Action string

// Actions. Names follow tig's action names where one exists.
const (
	None Action = "none"

	ViewMain     Action = "view-main"
	ViewDiff     Action = "view-diff"
	ViewLog      Action = "view-log"
	ViewReflog   Action = "view-reflog"
	ViewPager    Action = "view-pager"
	ViewTree     Action = "view-tree"
	ViewBlob     Action = "view-blob"
	ViewGrep     Action = "view-grep"
	ViewBlame    Action = "view-blame"
	ViewRefs     Action = "view-refs"
	ViewStash    Action = "view-stash"
	ViewHelp     Action = "view-help"
	ViewStatus   Action = "view-status"
	ViewStage    Action = "view-stage"
	ViewWorktree Action = "view-worktree"

	ViewClose Action = "view-close"
	Quit      Action = "quit"
	Enter     Action = "enter"
	ViewNext  Action = "view-next"
	Refresh   Action = "refresh"
	Maximize  Action = "maximize"
	Parent    Action = "parent"
	Previous  Action = "previous"
	Next      Action = "next"

	MoveUp           Action = "move-up"
	MoveDown         Action = "move-down"
	MovePageUp       Action = "move-page-up"
	MovePageDown     Action = "move-page-down"
	MoveHalfPageUp   Action = "move-half-page-up"
	MoveHalfPageDown Action = "move-half-page-down"
	MoveFirstLine    Action = "move-first-line"
	MoveLastLine     Action = "move-last-line"
	ScrollLineUp     Action = "scroll-line-up"
	ScrollLineDown   Action = "scroll-line-down"
	ScrollLeft       Action = "scroll-left"
	ScrollRight      Action = "scroll-right"
	ScrollFirstCol   Action = "scroll-first-col"

	Search     Action = "search"
	SearchBack Action = "search-back"
	FindNext   Action = "find-next"
	FindPrev   Action = "find-prev"
	Prompt     Action = "prompt"
	Filter     Action = "filter"

	ToggleLineNo        Action = "toggle-lineno"
	ToggleDate          Action = "toggle-date"
	ToggleAuthor        Action = "toggle-author"
	ToggleGraph         Action = "toggle-graph"
	ToggleLineGraphics  Action = "toggle-line-graphics"
	ToggleRefs          Action = "toggle-refs"
	ToggleIgnoreSpace   Action = "toggle-ignore-space"
	ToggleID            Action = "toggle-id"
	ToggleFileFilter    Action = "toggle-file-filter"
	ToggleTitleOverflow Action = "toggle-title-overflow"
	MoveHead            Action = "move-head"
	Options             Action = "options"
	StopLoading         Action = "stop-loading"
	ShowVersion         Action = "show-version"
	ScreenRedraw        Action = "screen-redraw"
	ThemePicker         Action = "theme-picker"
	DiffContextDown     Action = "diff-context-down"
	DiffContextUp       Action = "diff-context-up"
	Edit                Action = "edit"

	StatusUpdate    Action = "status-update"
	StageUpdateLine Action = "stage-update-line"
	StageNext       Action = "stage-next"
	StatusRevert    Action = "status-revert"
	StatusMerge     Action = "status-merge"
	StatusCommit    Action = "status-commit"
	CherryPick      Action = "cherry-pick"
	Checkout        Action = "checkout"
	BranchDelete    Action = "branch-delete"
	StashApply      Action = "stash-apply"
	StashPop        Action = "stash-pop"
	StashDrop       Action = "stash-drop"
	WorktreeAdd     Action = "worktree-add"
	WorktreeRemove  Action = "worktree-remove"
	WorktreeLock    Action = "worktree-lock"
	WorktreePrune   Action = "worktree-prune"
	WorktreeOpen    Action = "worktree-open"
)

// CommandPrefix marks actions that run user commands ("command:<index>").
const CommandPrefix = "command:"

// Descriptions of actions for the help view.
var Descriptions = map[Action]string{
	ViewMain: "Show main view", ViewDiff: "Show diff view", ViewLog: "Show log view",
	ViewReflog: "Show reflog view", ViewPager: "Show pager view", ViewTree: "Show tree view",
	ViewBlob: "Show blob view", ViewGrep: "Show grep view", ViewBlame: "Show blame view",
	ViewRefs: "Show refs view", ViewStash: "Show stash view", ViewHelp: "Show help view",
	ViewStatus: "Show status view", ViewStage: "Show stage view", ViewWorktree: "Show worktree view",
	ViewClose: "Close the current view", Quit: "Quit", Enter: "Open the selected line",
	ViewNext: "Focus the next view", Refresh: "Reload the current view", Maximize: "Maximize the current view",
	Parent: "Go to parent (directory, commit)", Previous: "Move up (in a split: move the parent view)",
	Next:   "Move down (in a split: move the parent view)",
	MoveUp: "Move cursor one line up", MoveDown: "Move cursor one line down",
	MovePageUp: "Move cursor one page up", MovePageDown: "Move cursor one page down",
	MoveHalfPageUp: "Move cursor half a page up", MoveHalfPageDown: "Move cursor half a page down",
	MoveFirstLine: "Move cursor to the first line", MoveLastLine: "Move cursor to the last line",
	ScrollLineUp: "Scroll one line up", ScrollLineDown: "Scroll one line down",
	ScrollLeft: "Scroll one column left", ScrollRight: "Scroll one column right", ScrollFirstCol: "Scroll to the first column",
	Search: "Search forward", SearchBack: "Search backward", FindNext: "Find next match", FindPrev: "Find previous match",
	Prompt: "Open the command prompt", Filter: "Open the fuzzy filter bar",
	ToggleLineNo: "Toggle line numbers", ToggleDate: "Cycle date display", ToggleAuthor: "Cycle author display",
	ToggleGraph: "Toggle revision graph", ToggleLineGraphics: "Toggle line graphics (utf-8 / ascii)",
	ToggleRefs: "Toggle reference display", ToggleIgnoreSpace: "Toggle ignoring whitespace in diffs",
	ToggleID: "Toggle commit ID display", ToggleFileFilter: "Toggle file filtering in diffs",
	ToggleTitleOverflow: "Toggle commit title overflow highlight", MoveHead: "Move to the HEAD commit",
	Options: "Open the option menu", StopLoading: "Stop all background loading", ShowVersion: "Show version",
	ScreenRedraw: "Redraw the screen", ThemePicker: "Open the theme picker",
	DiffContextDown: "Decrease diff context", DiffContextUp: "Increase diff context", Edit: "Open file in $EDITOR",
	StatusUpdate: "Stage / unstage file or chunk", StageUpdateLine: "Stage / unstage a single line",
	StageNext: "Jump to the next chunk", StatusRevert: "Discard changes of the file",
	StatusMerge: "Resolve conflict with git mergetool", StatusCommit: "Commit staged changes",
	CherryPick: "Cherry-pick the commit", Checkout: "Checkout the branch", BranchDelete: "Delete the branch",
	StashApply: "Apply the stash", StashPop: "Pop the stash", StashDrop: "Drop the stash",
	WorktreeAdd: "Add a worktree", WorktreeRemove: "Remove the worktree", WorktreeLock: "Lock / unlock the worktree",
	WorktreePrune: "Prune stale worktrees", WorktreeOpen: "Open a shell in the worktree",
}

// Destructive lists actions that ask for confirmation by default.
var Destructive = map[Action]bool{
	StatusRevert: true, CherryPick: true, Checkout: true, BranchDelete: true,
	StashApply: true, StashPop: true, StashDrop: true, WorktreeRemove: true, WorktreePrune: true,
}

// Valid reports whether a is a known action name.
func Valid(a Action) bool {
	if a == None {
		return true
	}
	_, ok := Descriptions[a]
	return ok
}

// Keymap holds bindings per view; "generic" applies to every view.
type Keymap struct {
	bindings map[string]map[string]Action
}

// Generic is the pseudo view for bindings shared by all views.
const Generic = "generic"

// Default returns tig's default bindings plus knot's additions.
func Default() *Keymap {
	km := &Keymap{bindings: map[string]map[string]Action{}}
	g := map[string]Action{
		"m": ViewMain, "d": ViewDiff, "l": ViewLog, "p": ViewPager, "t": ViewTree, "f": ViewBlob,
		"g": ViewGrep, "b": ViewBlame, "r": ViewRefs, "y": ViewStash, "h": ViewHelp, "s": ViewStatus,
		"c": ViewStage, "w": ViewWorktree,
		"q": ViewClose, "Q": Quit, "ctrl+c": Quit, "enter": Enter, "tab": ViewNext, "R": Refresh, "O": Maximize,
		",": Parent, "up": Previous, "down": Next, "ctrl+p": Previous, "ctrl+n": Next,
		"k": MoveUp, "j": MoveDown, "pgup": MovePageUp, "-": MovePageUp, "pgdown": MovePageDown, "space": MovePageDown,
		"home": MoveFirstLine, "end": MoveLastLine,
		"insert": ScrollLineUp, "delete": ScrollLineDown, "left": ScrollLeft, "right": ScrollRight, "|": ScrollFirstCol,
		"/": Search, "?": SearchBack, "n": FindNext, "N": FindPrev, ":": Prompt,
		"#": ToggleLineNo, "D": ToggleDate, "A": ToggleAuthor, "G": ToggleGraph, "~": ToggleLineGraphics,
		"F": ToggleRefs, "W": ToggleIgnoreSpace, "X": ToggleID, "%": ToggleFileFilter, "$": ToggleTitleOverflow,
		"H": MoveHead, "o": Options, "z": StopLoading, "v": ShowVersion, "ctrl+l": ScreenRedraw, "T": ThemePicker,
		"[": DiffContextDown, "]": DiffContextUp, "e": Edit,
	}
	km.bindings[Generic] = g
	km.bindings["status"] = map[string]Action{"u": StatusUpdate, "!": StatusRevert, "M": StatusMerge, "C": StatusCommit}
	km.bindings["stage"] = map[string]Action{"u": StatusUpdate, "1": StageUpdateLine, "@": StageNext, "!": StatusRevert, "C": StatusCommit}
	km.bindings["main"] = map[string]Action{"C": CherryPick}
	km.bindings["refs"] = map[string]Action{"C": Checkout, "!": BranchDelete}
	km.bindings["stash"] = map[string]Action{"A": StashApply, "P": StashPop, "!": StashDrop}
	km.bindings["worktree"] = map[string]Action{"a": WorktreeAdd, "!": WorktreeRemove, "L": WorktreeLock, "P": WorktreePrune, "o": WorktreeOpen}
	return km
}

// Lookup resolves a key: view-specific bindings win over generic ones.
func (km *Keymap) Lookup(view, key string) (Action, bool) {
	if a, ok := km.bindings[view][key]; ok {
		return a, a != None
	}
	if a, ok := km.bindings[Generic][key]; ok {
		return a, a != None
	}
	return "", false
}

// Bind sets a binding. Binding None removes the key in that view.
func (km *Keymap) Bind(view, key string, a Action) {
	if km.bindings[view] == nil {
		km.bindings[view] = map[string]Action{}
	}
	km.bindings[view][key] = a
}

// Apply applies configuration overrides. It returns messages for invalid
// entries, which are skipped.
func (km *Keymap) Apply(keys map[string]map[string]string) []string {
	var errs []string
	views := make([]string, 0, len(keys))
	for v := range keys {
		views = append(views, v)
	}
	sort.Strings(views)
	for _, view := range views {
		for key, name := range keys[view] {
			a := Action(name)
			if !Valid(a) && !strings.HasPrefix(name, CommandPrefix) {
				errs = append(errs, fmt.Sprintf("keys.%s: unknown action %q for key %q", view, name, key))
				continue
			}
			nk, err := NormalizeKey(key)
			if err != nil {
				errs = append(errs, fmt.Sprintf("keys.%s: %v", view, err))
				continue
			}
			km.Bind(view, nk, a)
		}
	}
	sort.Strings(errs)
	return errs
}

var keyNames = map[string]string{
	"enter": "enter", "return": "enter", "tab": "tab", "space": "space", "esc": "esc", "escape": "esc",
	"backspace": "backspace", "up": "up", "down": "down", "left": "left", "right": "right",
	"pgup": "pgup", "pageup": "pgup", "pgdown": "pgdown", "pagedown": "pgdown",
	"home": "home", "end": "end", "insert": "insert", "delete": "delete", "del": "delete",
}

// NormalizeKey converts a config key name ("ctrl-d", "PgUp", "<Enter>") to
// the key string reported by the terminal library ("ctrl+d", "pgup",
// "enter"). Single characters are kept as they are (case sensitive).
func NormalizeKey(k string) (string, error) {
	orig := k
	if len([]rune(k)) == 1 {
		if k == " " {
			return "space", nil
		}
		return k, nil
	}
	if strings.HasPrefix(k, "<") && strings.HasSuffix(k, ">") {
		k = k[1 : len(k)-1]
	}
	lower := strings.ToLower(k)
	var mods []string
	for {
		found := false
		for _, m := range []string{"ctrl", "alt", "shift"} {
			for _, sep := range []string{"-", "+"} {
				if p := m + sep; strings.HasPrefix(lower, p) && len(lower) > len(p) {
					mods = append(mods, m)
					lower = lower[len(p):]
					k = k[len(p):]
					found = true
				}
			}
		}
		if !found {
			break
		}
	}
	base := ""
	switch {
	case len([]rune(k)) == 1:
		base = k
		if len(mods) > 0 {
			base = strings.ToLower(k)
		}
	case keyNames[lower] != "":
		base = keyNames[lower]
	case len(lower) >= 2 && lower[0] == 'f' && isDigits(lower[1:]):
		base = lower
	default:
		return "", fmt.Errorf("unknown key %q", orig)
	}
	order := map[string]int{"ctrl": 0, "alt": 1, "shift": 2}
	sort.Slice(mods, func(i, j int) bool { return order[mods[i]] < order[mods[j]] })
	if len(mods) == 0 {
		return base, nil
	}
	return strings.Join(mods, "+") + "+" + base, nil
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// Binding is one key → action entry for display.
type Binding struct {
	Key    string
	Action Action
}

// Bound reports whether any view binds the action.
func (km *Keymap) Bound(a Action) bool {
	for _, vb := range km.bindings {
		for _, x := range vb {
			if x == a {
				return true
			}
		}
	}
	return false
}

// Bindings returns the effective bindings of a view (excluding generic),
// sorted by action then key. Removed bindings are omitted.
func (km *Keymap) Bindings(view string) []Binding {
	var out []Binding
	for k, a := range km.bindings[view] {
		if a != None {
			out = append(out, Binding{k, a})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Action != out[j].Action {
			return out[i].Action < out[j].Action
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Views returns the views that have specific bindings, sorted, with
// generic first.
func (km *Keymap) Views() []string {
	var vs []string
	for v := range km.bindings {
		if v != Generic {
			vs = append(vs, v)
		}
	}
	sort.Strings(vs)
	return append([]string{Generic}, vs...)
}
