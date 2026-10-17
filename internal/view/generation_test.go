package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/graph"
)

// The tests below are regressions for issue #12: results of a superseded
// (cancelled) load must be dropped, not applied on top of the current
// load. The generation is bumped by calling startLoad directly, so no git
// command or goroutine is involved.

func TestStaleCommitBatchDropped(t *testing.T) {
	env := testEnv(t)
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	_, gen := v.startLoad()

	// A batch and its wait chain from the superseded load arrive later:
	// neither may be applied nor continued.
	next := func() tea.Msg { return commitBatchMsg{gen: gen - 1} }
	if cmd := v.HandleMsg(env, commitBatchMsg{gen: gen - 1, commits: testCommits(), next: next}); cmd != nil {
		t.Fatalf("stale batch returned a command %v", cmd)
	}
	if v.Len() != 0 {
		t.Fatalf("stale batch applied: len = %d", v.Len())
	}
	v.HandleMsg(env, changesMsg{gen: gen - 1, unstaged: true, staged: true})
	if v.Len() != 0 {
		t.Fatalf("stale changes applied: len = %d", v.Len())
	}

	// The current generation still applies.
	v.HandleMsg(env, commitBatchMsg{gen: gen, commits: testCommits()})
	if v.Len() != len(testCommits()) {
		t.Fatalf("fresh batch not applied: len = %d", v.Len())
	}
}

func TestStaleLoadedTextDropped(t *testing.T) {
	env := testEnv(t)
	var tv textView
	_, gen := tv.startLoad()

	tv.handleText(env, loadedMsg{gen: gen - 1, data: []string{"stale"}}, classifyPlain)
	if tv.Len() != 0 {
		t.Fatalf("stale text applied: len = %d", tv.Len())
	}
	tv.handleText(env, loadedMsg{gen: gen, data: []string{"fresh"}}, classifyPlain)
	if tv.Len() != 1 || tv.lines[0] != "fresh" {
		t.Fatalf("fresh text not applied: lines = %v", tv.lines)
	}
}

func TestStaleStreamedChunkDropped(t *testing.T) {
	env := testEnv(t)
	var tv textView
	_, gen := tv.startLoad()

	// A chunk from the superseded load is dropped along with its wait
	// chain, and the current content is kept.
	tv.handleChunk(env, textChunkMsg{gen: gen - 1, lines: []string{"stale"}, next: func() tea.Msg { return textChunkMsg{} }}, classifyPlain)
	if tv.Len() != 0 {
		t.Fatalf("stale chunk applied: len = %d", tv.Len())
	}

	// So is the done message of the superseded load; the loading flag
	// belongs to the newer load and stays until it finishes.
	tv.handleChunk(env, textChunkMsg{gen: gen - 1, done: true}, classifyPlain)
	if tv.Len() != 0 {
		t.Fatalf("stale done message applied content: len = %d", tv.Len())
	}

	// The current generation still applies, chunk by chunk, until its
	// own done message.
	tv.handleChunk(env, textChunkMsg{gen: gen, lines: []string{"first", "second"}}, classifyPlain)
	if tv.Len() != 2 {
		t.Fatalf("fresh chunk not applied: len = %d", tv.Len())
	}
	tv.handleChunk(env, textChunkMsg{gen: gen, done: true}, classifyPlain)
	if tv.Loading {
		t.Fatal("still loading after the done chunk")
	}
}

func TestStaleWorktreeResultsDropped(t *testing.T) {
	env := testEnv(t)
	env.Cfg.Worktree.ShowDirty = false
	v := NewWorktree()
	_, gen := v.startLoad()

	// A list from the superseded load is ignored.
	if v.HandleMsg(env, worktreeListMsg{gen: gen - 1, rows: []worktreeRow{{dirty: -1}}}); v.Len() != 0 {
		t.Fatalf("stale worktree list applied: len = %d", v.Len())
	}
	rows := []worktreeRow{{wt: git.Worktree{Path: "/x/repo"}, dirty: -1}}
	v.HandleMsg(env, worktreeListMsg{gen: gen, rows: rows})
	if v.Len() != 1 {
		t.Fatalf("fresh worktree list not applied: len = %d", v.Len())
	}

	// A fresh list normally triggers dirtyCheck, which bumps the
	// generation again; a dirty result from before it is ignored.
	_, gen2 := v.startLoad()
	v.HandleMsg(env, dirtyMsg{gen: gen, dirty: map[string]bool{"/x/repo": true}})
	if rows[0].dirty != -1 {
		t.Fatalf("stale dirty result applied: dirty = %d", rows[0].dirty)
	}
	v.HandleMsg(env, dirtyMsg{gen: gen2, dirty: map[string]bool{"/x/repo": true}})
	if rows[0].dirty != 1 {
		t.Fatalf("fresh dirty result not applied: dirty = %d", rows[0].dirty)
	}
}
