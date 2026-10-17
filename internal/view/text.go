package view

import (
	"context"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/theme"
)

// textView is a base for views that show colored text lines (diff, log,
// pager, blob, stage).
type textView struct {
	Base
	lines []string
	roles []theme.Role
	rows  [][]Seg // cached segments; nil entries are built on demand
	tab   int
}

func (t *textView) Len() int { return len(t.lines) }

func (t *textView) Row(env *Env, i int) []Seg {
	if t.rows[i] == nil {
		t.rows[i] = []Seg{{t.roles[i], t.lines[i]}}
	}
	return t.rows[i]
}

func (t *textView) setLines(env *Env, lines []string, classify func(string) theme.Role) {
	t.tab = env.Cfg.UI.TabSize
	t.lines = make([]string, len(lines))
	t.roles = make([]theme.Role, len(lines))
	t.rows = make([][]Seg, len(lines))
	for i, l := range lines {
		t.lines[i] = ExpandTabs(l, t.tab)
		t.roles[i] = classify(l)
	}
	if env.Cfg.Diff.Highlight == "word" {
		t.wordHighlight()
	}
	if env.Cfg.Diff.Highlight == "off" {
		for i := range t.roles {
			if t.roles[i] == theme.DiffAdd || t.roles[i] == theme.DiffDel {
				t.roles[i] = theme.Default
			}
		}
	}
	t.styleStat()
}

var statLine = regexp.MustCompile(`^ (.*\S)\s+\|\s+(\d+ ?)?([+-]*)$`)

// styleStat colors +/- in diffstat lines.
func (t *textView) styleStat() {
	for i, l := range t.lines {
		if t.roles[i] != theme.Default {
			continue
		}
		m := statLine.FindStringSubmatchIndex(l)
		if m == nil || m[6] < 0 {
			continue
		}
		bars := l[m[6]:m[7]]
		segs := []Seg{{theme.Default, l[:m[6]]}}
		plus := strings.Count(bars, "+")
		if plus > 0 {
			segs = append(segs, Seg{theme.StatAdd, strings.Repeat("+", plus)})
		}
		if minus := len(bars) - plus; minus > 0 {
			segs = append(segs, Seg{theme.StatDel, strings.Repeat("-", minus)})
		}
		t.rows[i] = segs
	}
}

// wordHighlight highlights the changed part of paired -/+ lines.
func (t *textView) wordHighlight() {
	n := len(t.lines)
	for i := 0; i < n; {
		if t.roles[i] != theme.DiffDel {
			i++
			continue
		}
		start := i
		for i < n && t.roles[i] == theme.DiffDel {
			i++
		}
		mid := i
		for i < n && t.roles[i] == theme.DiffAdd {
			i++
		}
		if i-mid != mid-start {
			continue
		}
		for k := 0; k < mid-start; k++ {
			a, b := start+k, mid+k
			pre, suf := commonAffixes(t.lines[a][1:], t.lines[b][1:])
			t.rows[a] = splitHL(t.lines[a], pre+1, len(t.lines[a])-suf, theme.DiffDel, theme.DiffDelHL)
			t.rows[b] = splitHL(t.lines[b], pre+1, len(t.lines[b])-suf, theme.DiffAdd, theme.DiffAddHL)
		}
	}
}

func commonAffixes(a, b string) (int, int) {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	// Do not split UTF-8 sequences.
	for pre > 0 && pre < len(a) && a[pre]&0xC0 == 0x80 {
		pre--
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	for suf > 0 && len(a)-suf < len(a) && a[len(a)-suf]&0xC0 == 0x80 {
		suf--
	}
	return pre, suf
}

func splitHL(s string, from, to int, base, hl theme.Role) []Seg {
	if from >= to {
		return []Seg{{base, s}}
	}
	segs := []Seg{{base, s[:from]}, {hl, s[from:to]}}
	if to < len(s) {
		segs = append(segs, Seg{base, s[to:]})
	}
	return segs
}

// classifyDiff assigns a role to a line of git diff / show / log output.
func classifyDiff(l string) theme.Role {
	switch {
	case strings.HasPrefix(l, "diff --git"), strings.HasPrefix(l, "diff --cc"), strings.HasPrefix(l, "index "),
		strings.HasPrefix(l, "--- "), strings.HasPrefix(l, "+++ "), strings.HasPrefix(l, "new file mode"),
		strings.HasPrefix(l, "deleted file mode"), strings.HasPrefix(l, "similarity index"),
		strings.HasPrefix(l, "rename from"), strings.HasPrefix(l, "rename to"), strings.HasPrefix(l, "old mode"),
		strings.HasPrefix(l, "new mode"), strings.HasPrefix(l, "Binary files"):
		return theme.DiffHeader
	case strings.HasPrefix(l, "@@"):
		return theme.DiffHunk
	case strings.HasPrefix(l, "+"):
		return theme.DiffAdd
	case strings.HasPrefix(l, "-"):
		return theme.DiffDel
	case strings.HasPrefix(l, "commit "):
		return theme.CommitID
	case strings.HasPrefix(l, "Author:"), strings.HasPrefix(l, "AuthorDate:"), strings.HasPrefix(l, "Commit:"),
		strings.HasPrefix(l, "CommitDate:"), strings.HasPrefix(l, "Date:"), strings.HasPrefix(l, "Merge:"),
		strings.HasPrefix(l, "Refs:"):
		return theme.DiffMeta
	}
	return theme.Default
}

func classifyPlain(string) theme.Role { return theme.Default }

// gitText loads the output of a git command into the text view.
func (t *textView) gitText(env *Env, args []string, classify func(string) theme.Role) tea.Cmd {
	r := env.Git()
	return t.load(func(ctx context.Context) (any, error) {
		out, err := r.Output(ctx, args...)
		if err != nil {
			return nil, err
		}
		return splitText(string(out)), nil
	})
}

func splitText(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (t *textView) handleText(env *Env, msg tea.Msg, classify func(string) theme.Role) bool {
	m, ok := msg.(loadedMsg)
	if !ok {
		return false
	}
	t.finish(m.err)
	if m.err != nil {
		t.setLines(env, nil, classify)
		return true
	}
	lines, _ := m.data.([]string)
	t.setLines(env, lines, classify)
	return true
}

// scrollEnter implements tig's Enter in text views: scroll down one line.
func (t *textView) scrollEnter() tea.Cmd {
	t.SetCursor(t.Cursor+1, t.Len())
	return nil
}

// lineRef finds the file and new-side line number for a line in a diff.
func lineRef(lines []string, i int) (file string, line int) {
	if i >= len(lines) {
		return "", 0
	}
	hunk := -1
	for k := i; k >= 0; k-- {
		l := lines[k]
		if hunk < 0 && strings.HasPrefix(l, "@@ ") {
			hunk = k
		}
		if strings.HasPrefix(l, "+++ ") {
			file = strings.TrimPrefix(strings.TrimPrefix(l, "+++ "), "b/")
			break
		}
		if strings.HasPrefix(l, "diff --git ") {
			if f := strings.Fields(l); len(f) == 4 {
				file = strings.TrimPrefix(f[3], "b/")
			}
			break
		}
	}
	if file == "/dev/null" {
		file = ""
	}
	if hunk < 0 {
		return file, 0
	}
	h, ok := parseHunkStart(lines[hunk])
	if !ok {
		return file, 0
	}
	line = h
	for k := hunk + 1; k < i; k++ {
		if !strings.HasPrefix(lines[k], "-") && !strings.HasPrefix(lines[k], "\\") {
			line++
		}
	}
	return file, line
}

var hunkStart = regexp.MustCompile(`^@@+ [^+]*\+(\d+)`)

func parseHunkStart(l string) (int, bool) {
	m := hunkStart.FindStringSubmatch(l)
	if m == nil {
		return 0, false
	}
	n := 0
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n, true
}
