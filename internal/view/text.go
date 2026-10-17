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
	hl    wordHLState // pending -/+ run pairing for the word highlight
	// streamGen is the load generation of the content currently loaded
	// by streamed ingestion; 0 = none. The first chunk of a load
	// replaces whatever content the view still held.
	streamGen int
}

func (t *textView) Len() int { return len(t.lines) }

func (t *textView) Row(env *Env, i int) []Seg {
	if t.rows[i] == nil {
		t.rows[i] = []Seg{{t.roles[i], t.lines[i]}}
	}
	return t.rows[i]
}

// finishWordHL applies the -/+ run pair the content ends on, which the
// one-shot scan paired when it went past the end of the buffer. Rows may
// already be cached from rendering; pair simply replaces them.
func (t *textView) finishWordHL() {
	if t.hl.mid >= 0 {
		t.hl.pair(t, len(t.lines))
	}
}

// setLines replaces the content with lines (one-shot ingestion).
func (t *textView) setLines(env *Env, lines []string, classify func(string) theme.Role) {
	t.startStream(env)
	t.append(env, lines, classify)
	t.finishWordHL()
}

// startStream (re)initializes the buffers for a new content generation.
func (t *textView) startStream(env *Env) {
	t.tab = env.Cfg.UI.TabSize
	t.hl.reset()
	t.lines = nil
	t.roles = nil
	t.rows = nil
}

// append ingests raw lines into the (initialized) buffers: tabs are expanded,
// each line is classified and the whole-buffer passes of the old setLines
// (stat coloring, word highlight) run per line as they arrive. A trailing
// -/+ run pair the buffer ends on is left pending for finishWordHL.
func (t *textView) append(env *Env, lines []string, classify func(string) theme.Role) {
	mode := env.Cfg.Diff.Highlight
	for _, l := range lines {
		t.lines = append(t.lines, ExpandTabs(l, t.tab))
		r := classify(l)
		if mode == "off" && (r == theme.DiffAdd || r == theme.DiffDel) {
			r = theme.Default
		}
		t.roles = append(t.roles, r)
		t.rows = append(t.rows, nil)
		i := len(t.roles) - 1
		if mode == "word" {
			t.hl.step(t, i)
		}
		t.styleStatLine(i)
	}
}

var statLine = regexp.MustCompile(`^ (.*\S)\s+\|\s+(\d+ ?)?([+-]*)$`)

// styleStatLine colors +/- in one diffstat line, caching its segments so
// Row() does not have to rebuild them.
func (t *textView) styleStatLine(i int) {
	if t.roles[i] != theme.Default {
		return
	}
	l := t.lines[i]
	m := statLine.FindStringSubmatchIndex(l)
	if m == nil || m[6] < 0 {
		return
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

// wordHLState pairs the DiffDel run starting at delStart with a following
// equal-length DiffAdd run starting at mid and highlights the changed parts
// of each -/+ line. It advances one line at a time, so the pairing carries
// across batch boundaries during streamed ingestion.
type wordHLState struct {
	delStart int
	mid      int // -1 = no run pending
}

func (s *wordHLState) reset() { s.delStart, s.mid = -1, -1 }

// step processes line i (already ingested into t).
func (s *wordHLState) step(t *textView, i int) {
	switch t.roles[i] {
	case theme.DiffDel:
		if s.mid >= 0 {
			// The add run ended as this line appeared.
			s.pair(t, i)
			s.reset()
		}
		if s.delStart < 0 {
			s.delStart = i
		}
	case theme.DiffAdd:
		if s.delStart >= 0 && s.mid < 0 {
			s.mid = i
		}
	default:
		// A non -/+ line ends the pending runs (an add-only tail,
		// without a matching del run, stays unhighlighted).
		if s.mid >= 0 {
			s.pair(t, i)
		}
		s.reset()
	}
}

// pair highlights the equal-length run pair [delStart, mid) + [mid, end).
func (s *wordHLState) pair(t *textView, end int) {
	n := s.mid - s.delStart
	if end-s.mid != n {
		return
	}
	for k := 0; k < n; k++ {
		a, b := s.delStart+k, s.mid+k
		pre, suf := commonAffixes(t.lines[a][1:], t.lines[b][1:])
		t.rows[a] = splitHL(t.lines[a], pre+1, len(t.lines[a])-suf, theme.DiffDel, theme.DiffDelHL)
		t.rows[b] = splitHL(t.lines[b], pre+1, len(t.lines[b])-suf, theme.DiffAdd, theme.DiffAddHL)
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

// textChunkMsg is one batch of lines streamed from a git command and the
// command that receives the next batch.
type textChunkMsg struct {
	id    int
	gen   int // load generation captured at load start
	lines []string
	next  tea.Cmd
	done  bool
	err   error
}

func (m textChunkMsg) ViewID() int { return m.id }

// streamBatch is the number of lines appended per message while a git
// command's output is streamed into the view.
const streamBatch = 2000

// streamText streams the output of a read-only git command into the text
// view: every fixed number of lines the view renders what it has received
// so far, instead of waiting for (and buffering) the whole output.
func (t *textView) streamText(env *Env, batch int, classify func(string) theme.Role, args ...string) tea.Cmd {
	ctx, gen := t.startLoad()
	r := env.Git()
	id := t.ID
	ch := make(chan textChunkMsg, 4)
	send := func(chunk []string) error {
		select {
		case ch <- textChunkMsg{id: id, gen: gen, lines: chunk}:
			return nil
		case <-ctx.Done():
			return context.Canceled
		}
	}
	go func() {
		defer close(ch)
		var lines []string
		err := r.StreamLines(ctx, func(line string) error {
			lines = append(lines, line)
			if len(lines) < batch {
				return nil
			}
			chunk := lines
			lines = nil
			return send(chunk)
		}, args...)
		if err == nil && len(lines) > 0 {
			err = send(lines)
		}
		select {
		case ch <- textChunkMsg{id: id, gen: gen, done: true, err: err}:
		case <-ctx.Done():
		}
	}()
	var wait tea.Cmd
	wait = func() tea.Msg {
		m, ok := <-ch
		if !ok {
			return textChunkMsg{id: id, gen: gen, done: true, err: context.Canceled}
		}
		if !m.done {
			m.next = wait
		}
		return m
	}
	return wait
}

// handleChunk applies one streamed batch, starting a replacement stream
// with the load's first chunk and finishing the load on the done message.
func (t *textView) handleChunk(env *Env, msg tea.Msg, classify func(string) theme.Role) (tea.Cmd, bool) {
	m, ok := msg.(textChunkMsg)
	if !ok {
		return nil, false
	}
	if t.stale(m.gen) {
		// A superseded load's result: keep the current content and
		// drop the batch along with the rest of its wait chain.
		return nil, true
	}
	if t.streamGen != m.gen {
		// First chunk of the load: replace the old content.
		t.startStream(env)
		t.streamGen = m.gen
	}
	if m.err != nil {
		t.finish(m.err)
		return nil, true
	}
	t.append(env, m.lines, classify)
	if m.done {
		t.finishWordHL()
		t.finish(nil)
		return nil, true
	}
	return m.next, true
}

func (t *textView) handleText(env *Env, msg tea.Msg, classify func(string) theme.Role) bool {
	m, ok := msg.(loadedMsg)
	if !ok {
		return false
	}
	if t.stale(m.gen) {
		// A superseded load's result: keep the current content.
		return true
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
