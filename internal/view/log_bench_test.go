// Benchmarks for the text-view ingestion path that feeds the log view:
// one op streams a synthetic git log --stat shaped output through
// handleChunk with the same batch size and per-line pipeline
// (classification, tab expansion, diffstat coloring, word highlight
// pairing) the streamed load uses.
package view

import (
	"fmt"
	"testing"

	"github.com/jedipunkz/knot/internal/config"
	"github.com/jedipunkz/knot/internal/theme"
)

// benchLogOutput builds a deterministic git log --stat shaped stream of
// n lines over ~n/10 commits.
func benchLogOutput(n int) []string {
	lines := make([]string, 0, n)
	commit := []string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"}
	msg := []string{"parser: handle section headers", "log: drop the cached offset", "view: bound row allocation",
		"theme: dim the header labels", "git: stream output before splitting"}
	files := []string{"worktree.go", "list.go", "base.go", "files.go", "main.go"}
	for len(lines) < n {
		lines = append(lines,
			fmt.Sprintf("commit %s%s%s", commit[len(lines)%3], commit[(len(lines)/3)%3], commit[(len(lines)/7)%3]))
		lines = append(lines, "Merge: c1c1c1c1 c2c2c2c2", "Refs: <HEAD> main, origin/main")
		lines = append(lines, "Author:     Jane Q Doe <jane@example.com>", "AuthorDate: Fri Feb 28 10:11:12 2026 +0900")
		lines = append(lines, "Commit:     John Smith <john@example.com>", "CommitDate: Fri Feb 28 11:12:13 2026 +0900")
		for _, m := range msg[(len(lines)/5)%len(msg):] {
			lines = append(lines, "    "+m)
			break
		}
		rows := (len(lines) % 9) + 1
		for i := 0; i < rows && len(lines) < n; i++ {
			lines = append(lines, fmt.Sprintf(" %s | %d +-++--+-++--+-", files[(len(lines))%len(files)], i))
		}
		lines = append(lines, fmt.Sprintf(" %d files changed, %d insertions(+), %d deletions(-)", rows%5+1, rows+3, rows))
	}
	return lines[:n]
}

func benchIngestEnv() *Env {
	cfg := config.Default()
	cfg.Diff.Highlight = "word"
	th, err := theme.Load("", "default")
	if err != nil {
		panic(err)
	}
	return &Env{Cfg: cfg, Theme: th}
}

func BenchmarkLogIngest1kLines(b *testing.B) {
	env := benchIngestEnv()
	lines := benchLogOutput(1000)
	b.ReportAllocs()
	for b.Loop() {
		var tv textView
		_, gen := tv.startLoad()
		for start := 0; start < len(lines); start += streamBatch {
			end := min(start+streamBatch, len(lines))
			tv.handleChunk(env, textChunkMsg{id: 1, gen: gen, lines: lines[start:end]}, classifyDiff)
		}
		tv.handleChunk(env, textChunkMsg{id: 1, gen: gen, done: true}, classifyDiff)
		if tv.Len() != len(lines) {
			b.Fatalf("ingested %d of %d lines", tv.Len(), len(lines))
		}
	}
}

func BenchmarkLogIngest10kLines(b *testing.B) {
	env := benchIngestEnv()
	lines := benchLogOutput(10_000)
	b.ReportAllocs()
	for b.Loop() {
		var tv textView
		_, gen := tv.startLoad()
		for start := 0; start < len(lines); start += streamBatch {
			end := min(start+streamBatch, len(lines))
			tv.handleChunk(env, textChunkMsg{id: 1, gen: gen, lines: lines[start:end]}, classifyDiff)
		}
		tv.handleChunk(env, textChunkMsg{id: 1, gen: gen, done: true}, classifyDiff)
		if tv.Len() != len(lines) {
			b.Fatalf("ingested %d of %d lines", tv.Len(), len(lines))
		}
	}
}
