package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
	"github.com/jedipunkz/knot/internal/view"
)

// Screen composition: view stack layout, per-panel rendering, the filter
// bar row, the key hints row and the status line.

func (m *Model) render() (string, *tea.Cursor) {
	th := m.env.Theme
	w, h := m.width, m.height
	keybarH := 0
	if m.env.Cfg.UI.KeyBar && h >= 5 && m.prompt == nil && m.confirm == nil {
		// While the filter bar is focused the hints belong to the input;
		// views with an info line (e.g. worktrees) still show the info.
		if m.filterBar == nil {
			keybarH = 1
		} else if _, ok := m.focused().(view.InfoLine); ok {
			keybarH = 1
		}
	}
	avail := max(h-1-keybarH, 2)
	m.panels = m.panels[:0]
	var body string
	var barCur *tea.Cursor
	if len(m.stack) == 0 {
		body = strings.Repeat("\n", avail-1)
	} else if m.split() {
		pi, ci := len(m.stack)-2, len(m.stack)-1
		ratio := m.env.Cfg.UI.SplitRatio
		dir := m.env.Cfg.UI.SplitDirection
		if dir == "auto" {
			dir = "horizontal"
			if w >= 160 {
				dir = "vertical"
			}
		}
		if dir == "vertical" {
			cw := int(float64(w) * ratio)
			pw := w - cw - 1
			left, c1 := m.renderPanel(pi, 0, 0, pw, avail)
			right, c2 := m.renderPanel(ci, pw+1, 0, cw, avail)
			barCur = c1
			if c2 != nil {
				barCur = c2
			}
			sep := th.Render(theme.TitleBlur, "│")
			if m.env.Opts.ASCII {
				sep = th.Render(theme.TitleBlur, "|")
			}
			l, r := strings.Split(left, "\n"), strings.Split(right, "\n")
			var sb strings.Builder
			for i := 0; i < avail; i++ {
				if i > 0 {
					sb.WriteByte('\n')
				}
				sb.WriteString(view.Fit(l[i], pw) + sep + r[i])
			}
			body = sb.String()
		} else {
			ch := max(int(float64(avail)*ratio), 2)
			ph := avail - ch
			if ph < 2 {
				ph, ch = 2, avail-2
			}
			top, c1 := m.renderPanel(pi, 0, 0, w, ph)
			bottom, c2 := m.renderPanel(ci, 0, ph, w, ch)
			body = top + "\n" + bottom
			barCur = c1
			if c2 != nil {
				barCur = c2
			}
		}
	} else {
		var c *tea.Cursor
		body, c = m.renderPanel(len(m.stack)-1, 0, 0, w, avail)
		barCur = c
	}
	line, cursor := m.statusLine(w, h-1)
	if barCur != nil {
		cursor = barCur
	}
	if keybarH == 1 {
		body += "\n" + m.keybarRow(w)
	}
	return body + "\n" + line, cursor
}

// keybarRow renders the row above the status bar: the focused view's info
// line first (e.g. the selected worktree), then the curated key hints.
// Hints are resolved against the live keymap (unbound actions are
// skipped); the row is cut at the screen width.
func (m *Model) keybarRow(w int) string {
	th := m.env.Theme
	var parts []string
	budget := w
	if il, ok := m.focused().(view.InfoLine); ok {
		infoW := 0
		for _, s := range il.Info(m.env) {
			parts = append(parts, th.Render(s.Role, s.Text))
			infoW += ansi.StringWidth(s.Text)
		}
		budget -= infoW + 2
	}
	if m.filterBar == nil {
		name := m.focused().Name()
		var out []string
		total := 0
		for _, e := range keymap.Bar(name) {
			keys := m.env.Keys.KeysOf(name, e.Action)
			if len(keys) == 0 {
				continue
			}
			key := keybarKey(keys[0])
			width := ansi.StringWidth(key) + 1 + ansi.StringWidth(e.Text) + 2
			if total+width > budget {
				break
			}
			out = append(out, th.Render(theme.Prompt, key)+" "+th.Render(theme.LineNumber, e.Text))
			total += width
		}
		parts = append(parts, out...)
	}
	row := strings.Join(parts, th.Render(theme.LineNumber, "  "))
	for ansi.StringWidth(row) > w {
		row = ansi.Truncate(row, w-1, "…")
	}
	return row
}

// keybarKey formats a key for the key bar.
func keybarKey(k string) string {
	switch k {
	case "space":
		return "Space"
	case "enter":
		return "Enter"
	}
	if len(k) > 1 {
		return "<" + k + ">"
	}
	return k
}

// renderPanel draws a view with its filter bar (filterable views show it
// permanently, dimmed when unfocused) above the content and its title bar
// below the content.
func (m *Model) renderPanel(idx, x, y, w, h int) (string, *tea.Cursor) {
	v := m.stack[idx].v
	var cur *tea.Cursor
	bar := ""
	barH := 0
	if f, ok := v.(view.Filterable); ok {
		barH = 1
		bar, cur = m.filterBarRow(f, x, y, w, idx == m.focusedIdx())
	}
	m.panels = append(m.panels, panel{x: x, y: y + barH, w: w, h: h - 1 - barH, idx: idx})
	content := view.Render(m.env, v, w, h-1-barH)
	role := theme.TitleBlur
	if idx == m.focusedIdx() {
		role = theme.TitleFocus
	}
	title := v.Title(m.env)
	if v.B().Loading {
		title += "  " + m.spinner() + " Loading"
	}
	pos := view.Position(v)
	gap := w - ansi.StringWidth(title) - ansi.StringWidth(pos) - 2
	var bar2 string
	if gap >= 1 {
		bar2 = " " + title + strings.Repeat(" ", gap) + pos + " "
	} else {
		bar2 = view.Fit(" "+title, w)
	}
	parts := []string{content, m.env.Theme.Render(role, view.Fit(bar2, w))}
	if bar != "" {
		parts = append([]string{bar}, parts...)
	}
	return strings.Join(parts, "\n"), cur
}

// filterBarRow renders the one-row filter bar shown at the top of a
// filterable view's panel. It is always visible: dim while unfocused, with
// the "<matches>/<loaded>" count on the right ("…" appended while loading)
// and the text cursor while the bar is focused.
func (m *Model) filterBarRow(f view.Filterable, x, y, w int, focused bool) (string, *tea.Cursor) {
	th := m.env.Theme
	labelRole, inputRole, countRole := theme.LineNumber, theme.LineNumber, theme.LineNumber
	// Edited means the bar holds the focus (the text cursor is drawn).
	edited := focused && m.filterBar != nil
	if edited {
		labelRole, inputRole = theme.Prompt, theme.Default
	}
	q, matches, loaded, loading := f.FilterStatus()
	input := q
	if focused && m.filterBar != nil {
		input = string(m.filterBar.input)
	}
	right := fmt.Sprintf("%d/%d", matches, loaded)
	if loading {
		right += "…"
	}
	label := " Search:"
	in := input
	if input == "" && !edited {
		in = "(Ctrl-P to filter)"
	}
	in = " " + in
	gap := w - ansi.StringWidth(label) - ansi.StringWidth(in) - ansi.StringWidth(right) - 2
	var row string
	switch {
	case gap >= 1:
		row = th.Render(labelRole, label) + th.Render(inputRole, in) +
			strings.Repeat(" ", gap) + th.Render(countRole, right) + " "
	default:
		text := th.Render(labelRole, label) + th.Render(inputRole, in)
		curLen := ansi.StringWidth(text)
		if curLen < w {
			text = strings.TrimRight(ansi.Truncate(text, w-1, "…"), " ")
		} else {
			text = ansi.Truncate(text, w-1, "…")
		}
		row = text + strings.Repeat(" ", max(w-ansi.StringWidth(text), 0))
	}
	var cur *tea.Cursor
	if edited {
		cx := x + ansi.StringWidth(label) + ansi.StringWidth(in)
		cur = tea.NewCursor(min(cx, x+w-1), y)
	}
	return row, cur
}

func (m *Model) statusLine(w, y int) (string, *tea.Cursor) {
	th := m.env.Theme
	switch {
	case m.prompt != nil:
		text := m.prompt.label + string(m.prompt.input)
		vis := text
		if ansi.StringWidth(vis) >= w {
			vis = ansi.TruncateLeft(vis, ansi.StringWidth(vis)-w+1, "")
		}
		return th.Render(theme.Prompt, view.Fit(vis, w)), tea.NewCursor(min(ansi.StringWidth(vis), w-1), y)
	case m.confirm != nil:
		return th.Render(theme.Warning, view.Fit(m.confirm.Question+" [y/N]", w)), nil
	}
	right := th.Render(theme.LineNumber, m.env.WorktreeName())
	switch {
	case m.env.Head.Branch != "":
		right += " " + th.Render(theme.RefHead, "["+m.env.Head.Branch+"]")
	case m.env.Head.Hash != "":
		right += " " + th.Render(theme.RefHead, "[detached "+view.Short(m.env.Head.Hash, 7)+"]")
	}
	left := m.status
	role := theme.StatusBar
	if m.statusErr {
		role = theme.Error
	}
	room := w - ansi.StringWidth(right) - 1
	if room < 1 {
		return th.Render(role, view.Fit(left, w)), nil
	}
	return th.Render(role, view.Fit(left, room)) + " " + th.Render(theme.RefHead, right), nil
}
