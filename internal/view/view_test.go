package view

import (
	"strings"
	"testing"
	"time"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/theme"
)

func testEnv(t *testing.T) *Env {
	t.Helper()
	cfg := config.Default()
	th, err := theme.Load("", "default")
	if err != nil {
		t.Fatal(err)
	}
	return &Env{Cfg: cfg, Theme: th, Opts: NewOptions(cfg)}
}

func TestFormatDate(t *testing.T) {
	ref := time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return ref }
	defer func() { now = time.Now }()
	cases := []struct {
		t    time.Time
		mode string
		want string
	}{
		{ref.Add(-3 * time.Hour), "relative", "3 hours ago"},
		{ref.Add(-1 * time.Minute), "relative", "1 minute ago"},
		{ref.Add(-49 * time.Hour), "relative-compact", "2D"},
		{ref, "off", ""},
		{ref.Add(-400 * 24 * time.Hour), "relative", "1 year ago"},
	}
	for _, c := range cases {
		if got := FormatDate(c.t, c.mode, ""); got != c.want {
			t.Errorf("%s %v = %q, want %q", c.mode, c.t, got, c.want)
		}
	}
	if got := FormatDate(ref, "custom", "%Y/%m/%d"); got != ref.Local().Format("2006/01/02") {
		t.Errorf("custom = %q", got)
	}
}

func TestFormatAuthor(t *testing.T) {
	cases := map[string]string{"full": "Jane Q Doe", "abbreviated": "JQD", "email": "jane@example.com", "email-user": "jane", "off": ""}
	for mode, want := range cases {
		if got := FormatAuthor("Jane Q Doe", "jane@example.com", mode); got != want {
			t.Errorf("%s = %q, want %q", mode, got, want)
		}
	}
}

func TestFitAndTabs(t *testing.T) {
	if got := Fit("abcdef", 4); got != "abc…" {
		t.Errorf("Fit = %q", got)
	}
	if got := Fit("ab", 4); got != "ab  " {
		t.Errorf("Fit pad = %q", got)
	}
	if got := FitTail("/a/b/c/d", 5); got != "…/c/d" {
		t.Errorf("FitTail = %q", got)
	}
	if got := ExpandTabs("a\tb\x1b[31m", 4); got != "a   b[31m" {
		t.Errorf("ExpandTabs = %q", got)
	}
}

func TestLineRef(t *testing.T) {
	lines := []string{
		"diff --git a/x.go b/x.go",
		"--- a/x.go",
		"+++ b/x.go",
		"@@ -10,4 +20,5 @@ func f()",
		" ctx",
		"-old",
		"+new",
		" ctx2",
	}
	file, line := lineRef(lines, 6)
	if file != "x.go" || line != 21 {
		t.Errorf("lineRef = %q %d", file, line)
	}
	if _, line := lineRef(lines, 7); line != 22 {
		t.Errorf("context line = %d", line)
	}
}

func TestWordHighlight(t *testing.T) {
	env := testEnv(t)
	env.Cfg.Diff.Highlight = "word"
	var tv textView
	tv.setLines(env, []string{"-hello world", "+hello there"}, classifyDiff)
	del := tv.Row(env, 0)
	add := tv.Row(env, 1)
	if len(del) != 2 || del[1].Text != "world" || del[1].Role != theme.DiffDelHL {
		t.Errorf("del segs = %+v", del)
	}
	if len(add) != 2 || add[1].Text != "there" || add[1].Role != theme.DiffAddHL {
		t.Errorf("add segs = %+v", add)
	}
}

func TestStatLineColors(t *testing.T) {
	env := testEnv(t)
	var tv textView
	tv.setLines(env, []string{" file.go | 3 ++-"}, classifyDiff)
	segs := tv.Row(env, 0)
	if len(segs) != 3 || segs[1].Text != "++" || segs[2].Text != "-" {
		t.Errorf("stat segs = %+v", segs)
	}
}

func TestNavigateAndFind(t *testing.T) {
	env := testEnv(t)
	p := NewPager("t", []string{"alpha", "beta", "gamma", "delta", "beta two"})
	p.Load(env)
	p.Height = 2
	if err := p.SetSearch("beta"); err != nil {
		t.Fatal(err)
	}
	if err := Find(env, p, 1); err != nil || p.Cursor != 1 {
		t.Fatalf("find = %v cursor %d", err, p.Cursor)
	}
	if err := Find(env, p, 1); err != nil || p.Cursor != 4 {
		t.Fatalf("find next = %v cursor %d", err, p.Cursor)
	}
	if err := Find(env, p, 1); err != nil || p.Cursor != 1 {
		t.Fatalf("wrap = %v cursor %d", err, p.Cursor)
	}
	out := Render(env, p, 20, 2)
	if !strings.Contains(out, "beta") {
		t.Errorf("render = %q", out)
	}
	if p.Offset != 1 {
		t.Errorf("offset = %d", p.Offset)
	}
}
