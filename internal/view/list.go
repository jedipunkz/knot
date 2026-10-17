package view

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jedipunkz/knot/internal/keymap"
	"github.com/jedipunkz/knot/internal/theme"
)

// Seg is a styled piece of a row.
type Seg struct {
	Role theme.Role
	Text string
}

// View is one tig-style screen.
type View interface {
	B() *Base
	Name() string
	Title(env *Env) string
	Len() int
	Row(env *Env, i int) []Seg
	// Load starts (re)loading the content.
	Load(env *Env) tea.Cmd
	// HandleMsg receives asynchronous results addressed to this view.
	HandleMsg(env *Env, msg tea.Msg) tea.Cmd
	// Enter opens the selected line.
	Enter(env *Env) tea.Cmd
	// Do handles view-specific actions.
	Do(env *Env, a keymap.Action) (tea.Cmd, bool)
	Selection(env *Env) Selection
}

// CursorWatcher is implemented by views that react to cursor movement.
type CursorWatcher interface {
	CursorMoved(env *Env) tea.Cmd
}

// Filterable is implemented by list views whose rows can be narrowed with a
// fuzzy filter bar (currently the main view). The app focuses the bar from
// such a view and routes line editing to it.
type Filterable interface {
	View
	// SetFilter applies a fuzzy query to the rows; an empty query clears it.
	SetFilter(env *Env, q string)
	// FilterStatus returns the active query, the number of matching rows,
	// the number of loaded rows and whether loading is in progress.
	FilterStatus() (query string, matches, loaded int, loading bool)
}

// Closer is implemented by views that need to act when closed.
type Closer interface {
	Closed(env *Env) tea.Cmd
}

// Base holds the cursor and loading state shared by all views.
type Base struct {
	ID      int
	Cursor  int
	Offset  int
	Col     int
	Height  int
	Loading bool
	Err     error

	search    *regexp.Regexp
	searchRaw string
	cancel    context.CancelFunc
}

// B returns the base itself (embedding gives every view this method).
func (b *Base) B() *Base { return b }

// startLoad cancels a running load and returns a fresh context.
func (b *Base) startLoad() context.Context {
	b.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	b.cancel = cancel
	b.Loading = true
	b.Err = nil
	return ctx
}

// Stop cancels background loading.
func (b *Base) Stop() {
	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.Loading = false
}

func (b *Base) finish(err error) {
	b.Loading = false
	if err != nil && err != context.Canceled {
		b.Err = err
	}
}

// Messages carrying asynchronous results.

type loadedMsg struct {
	id   int
	data any
	err  error
}

func (m loadedMsg) ViewID() int { return m.id }

// ViewMsg is implemented by messages addressed to a single view.
type ViewMsg interface{ ViewID() int }

func (b *Base) load(fn func(ctx context.Context) (any, error)) tea.Cmd {
	ctx := b.startLoad()
	id := b.ID
	return func() tea.Msg {
		data, err := fn(ctx)
		return loadedMsg{id: id, data: data, err: err}
	}
}

// Clamp keeps cursor and offset inside the content.
func (b *Base) Clamp(n int) {
	if b.Cursor >= n {
		b.Cursor = n - 1
	}
	if b.Cursor < 0 {
		b.Cursor = 0
	}
	h := max(b.Height, 1)
	if b.Offset > b.Cursor {
		b.Offset = b.Cursor
	}
	if b.Cursor >= b.Offset+h {
		b.Offset = b.Cursor - h + 1
	}
	if b.Offset > n-h {
		b.Offset = max(n-h, 0)
	}
	if b.Offset < 0 {
		b.Offset = 0
	}
}

// SetCursor moves the cursor to i.
func (b *Base) SetCursor(i, n int) {
	b.Cursor = i
	b.Clamp(n)
}

// Navigate handles generic cursor and scroll actions.
func Navigate(v View, a keymap.Action) bool {
	b := v.B()
	n := v.Len()
	h := max(b.Height, 1)
	switch a {
	case keymap.MoveUp, keymap.Previous:
		b.Cursor--
	case keymap.MoveDown, keymap.Next:
		b.Cursor++
	case keymap.MovePageUp:
		b.Cursor -= h
		b.Offset -= h
	case keymap.MovePageDown:
		b.Cursor += h
		b.Offset += h
	case keymap.MoveHalfPageUp:
		b.Cursor -= h / 2
		b.Offset -= h / 2
	case keymap.MoveHalfPageDown:
		b.Cursor += h / 2
		b.Offset += h / 2
	case keymap.MoveFirstLine:
		b.Cursor = 0
	case keymap.MoveLastLine:
		b.Cursor = n - 1
	case keymap.ScrollLineUp:
		if b.Offset > 0 {
			b.Offset--
			if b.Cursor >= b.Offset+h {
				b.Cursor = b.Offset + h - 1
			}
		}
		b.Clamp(n)
		return true
	case keymap.ScrollLineDown:
		if b.Offset+h < n {
			b.Offset++
			if b.Cursor < b.Offset {
				b.Cursor = b.Offset
			}
		}
		b.Clamp(n)
		return true
	case keymap.ScrollLeft:
		b.Col = max(b.Col-4, 0)
		return true
	case keymap.ScrollRight:
		b.Col += 4
		return true
	case keymap.ScrollFirstCol:
		b.Col = 0
		return true
	default:
		return false
	}
	if b.Offset < 0 {
		b.Offset = 0
	}
	b.Clamp(n)
	return true
}

// Plain returns the text of row i without styles.
func Plain(env *Env, v View, i int) string {
	var sb strings.Builder
	for _, s := range v.Row(env, i) {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// SetSearch compiles a search pattern (smart case).
func (b *Base) SetSearch(pattern string) error {
	if pattern == "" {
		b.search, b.searchRaw = nil, ""
		return nil
	}
	expr := pattern
	if strings.ToLower(pattern) == pattern {
		expr = "(?i)" + pattern
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return err
	}
	b.search, b.searchRaw = re, pattern
	return nil
}

// Find moves to the next (dir=1) or previous (dir=-1) match, starting after
// the cursor and wrapping around.
func Find(env *Env, v View, dir int) error {
	b := v.B()
	if b.search == nil {
		return fmt.Errorf("no previous search")
	}
	n := v.Len()
	for k := 1; k <= n; k++ {
		i := ((b.Cursor+dir*k)%n + n) % n
		if b.search.MatchString(Plain(env, v, i)) {
			b.SetCursor(i, n)
			return nil
		}
	}
	return fmt.Errorf("no match for %q", b.searchRaw)
}

// Render draws rows of the view in a box of width×height.
func Render(env *Env, v View, width, height int) string {
	b := v.B()
	b.Height = height
	n := v.Len()
	b.Clamp(n)
	th := env.Theme
	numWidth := len(fmt.Sprint(n))
	var out strings.Builder
	for row := 0; row < height; row++ {
		i := b.Offset + row
		if row > 0 {
			out.WriteByte('\n')
		}
		if i >= n {
			if i == 0 {
				out.WriteString(emptyText(th, b, width))
			}
			continue
		}
		segs := v.Row(env, i)
		prefix := ""
		if env.Opts.LineNumbers {
			prefix = fmt.Sprintf("%*d ", numWidth, i+1)
		}
		if i == b.Cursor {
			plain := prefix
			for _, s := range segs {
				plain += s.Text
			}
			plain = cutPlain(plain, b.Col, width)
			pad := width - ansi.StringWidth(plain)
			out.WriteString(th.Render(theme.Cursor, plain+strings.Repeat(" ", max(pad, 0))))
			continue
		}
		var line strings.Builder
		if prefix != "" {
			line.WriteString(th.Render(theme.LineNumber, prefix))
		}
		matched := b.search != nil && b.search.MatchString(joinSegs(segs))
		for _, s := range segs {
			if matched {
				line.WriteString(highlight(th, s, b.search))
			} else {
				line.WriteString(th.Render(s.Role, s.Text))
			}
		}
		out.WriteString(ansi.Cut(line.String(), b.Col, b.Col+width))
	}
	return out.String()
}

func emptyText(th *theme.Theme, b *Base, width int) string {
	switch {
	case b.Err != nil:
		return ansi.Truncate(th.Render(theme.Error, b.Err.Error()), width, "…")
	case b.Loading:
		return th.Render(theme.LineNumber, "Loading…")
	default:
		return th.Render(theme.LineNumber, "(empty)")
	}
}

func joinSegs(segs []Seg) string {
	var sb strings.Builder
	for _, s := range segs {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

func highlight(th *theme.Theme, s Seg, re *regexp.Regexp) string {
	locs := re.FindAllStringIndex(s.Text, -1)
	if len(locs) == 0 {
		return th.Render(s.Role, s.Text)
	}
	var sb strings.Builder
	last := 0
	for _, l := range locs {
		if l[0] == l[1] {
			continue
		}
		sb.WriteString(th.Render(s.Role, s.Text[last:l[0]]))
		sb.WriteString(th.Render(theme.SearchMatch, s.Text[l[0]:l[1]]))
		last = l[1]
	}
	sb.WriteString(th.Render(s.Role, s.Text[last:]))
	return sb.String()
}

func cutPlain(s string, left, width int) string {
	return ansi.Cut(s, left, left+width)
}

// Position returns "line x of y" information for the title bar.
func Position(v View) string {
	b := v.B()
	n := v.Len()
	if n == 0 {
		return ""
	}
	pct := 100
	if n > 0 {
		pct = (min(b.Offset+max(b.Height, 1), n)) * 100 / n
	}
	return fmt.Sprintf("line %d of %d  %3d%%", b.Cursor+1, n, pct)
}
