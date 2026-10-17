package view

import (
	"strings"
	"testing"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/graph"
	"github.com/jedipunkz/knot/internal/theme"
)

func overflowView(t *testing.T, env *Env, subject string) *MainView {
	t.Helper()
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	feedCommits(env, v, []git.Commit{
		{Hash: "111111aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AuthorName: "Alice", Subject: subject},
	})
	return v
}

func hasOverflowSeg(segs []Seg) *Seg {
	for i, s := range segs {
		if s.Role == theme.Overflow {
			return &segs[i]
		}
	}
	return nil
}

func TestMainViewTitleOverflowOffByDefault(t *testing.T) {
	env := testEnv(t)
	if env.Cfg.UI.TitleOverflow != 0 {
		t.Fatalf("default ui.title-overflow = %d, want 0", env.Cfg.UI.TitleOverflow)
	}
	if env.Opts.TitleOverflow {
		t.Fatal("title overflow highlight active by default, want off")
	}
	v := overflowView(t, env, strings.Repeat("a", 60))
	if got := hasOverflowSeg(v.Row(env, 0)); got != nil {
		t.Fatalf("row has an Overflow segment %q with the default config", got.Text)
	}
}

func TestMainViewTitleOverflowOptIn(t *testing.T) {
	env := testEnv(t)
	env.Cfg.UI.TitleOverflow = 10
	env.Opts = NewOptions(env.Cfg)
	if !env.Opts.TitleOverflow {
		t.Fatal("positive title-overflow did not activate the highlight")
	}
	const head, tail = "abcdefghij", "klmnopqrst"
	v := overflowView(t, env, head+tail)
	got := hasOverflowSeg(v.Row(env, 0))
	if got == nil || got.Text != tail {
		t.Fatalf("overflow segment = %+v, want tail %q", got, tail)
	}

	// Toggling the highlight off ($) hides it again.
	env.Opts.TitleOverflow = false
	if got := hasOverflowSeg(v.Row(env, 0)); got != nil {
		t.Fatalf("row has an Overflow segment %q while toggled off", got.Text)
	}
}
