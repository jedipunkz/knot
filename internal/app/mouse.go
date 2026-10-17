package app

import (
	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/view"
)

// Mouse handling: hit testing over the rendered panels, wheel scrolling
// and click selection.

func (m *Model) panelAt(x, y int) (panel, bool) {
	for _, p := range m.panels {
		if x >= p.x && x < p.x+p.w && y >= p.y && y < p.y+p.h {
			return p, true
		}
	}
	return panel{}, false
}

func (m *Model) focusIdx(idx int) {
	if m.split() {
		m.focusParent = idx == len(m.stack)-2
	}
}

func (m *Model) wheel(ms tea.Mouse) tea.Cmd {
	p, ok := m.panelAt(ms.X, ms.Y)
	if !ok || m.prompt != nil || m.confirm != nil {
		return nil
	}
	v := m.stack[p.idx].v
	a := keymap.ScrollLineDown
	if ms.Button == tea.MouseWheelUp {
		a = keymap.ScrollLineUp
	}
	for i := 0; i < 3; i++ {
		view.Navigate(v, a)
	}
	return nil
}

func (m *Model) click(ms tea.Mouse) tea.Cmd {
	if ms.Button != tea.MouseLeft || m.prompt != nil || m.confirm != nil {
		return nil
	}
	p, ok := m.panelAt(ms.X, ms.Y)
	if !ok {
		return nil
	}
	m.focusIdx(p.idx)
	v := m.stack[p.idx].v
	b := v.B()
	line := b.Offset + ms.Y - p.y
	if line >= v.Len() {
		return nil
	}
	if line == b.Cursor {
		return v.Enter(m.env)
	}
	b.SetCursor(line, v.Len())
	if w, ok := v.(view.CursorWatcher); ok {
		return w.CursorMoved(m.env)
	}
	return nil
}
