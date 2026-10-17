// Benchmarks for the filter hot path: one op is one filter pass over the
// main view's rows, mirroring the calls filterMain makes per token (subject,
// author, rendered row and refs).
package view

import (
	"fmt"
	"testing"

	"github.com/jedipunkz/knot/internal/git"
	"github.com/jedipunkz/knot/internal/graph"
)

// benchRows builds n deterministic commit-like rows. The moduli spread the
// components so repeated substrings do not skew the alignment scoring.
func benchRows(n int) []string {
	kinds := []string{"feat", "fix", "refactor", "docs", "test", "chore"}
	areas := []string{"worktree", "theme", "config", "graph", "keymap", "status", "diff", "log", "blame", "filter"}
	verbs := []string{"handle", "support", "drop", "stream", "render", "cache", "validate", "highlight", "scroll", "stage"}
	objects := []string{"prunable worktrees", "staged hunks", "colored diffstat", "linked worktrees", "locked branches",
		"untracked files", "relative dates", "search wrapping", "merge lanes", "blame chunks"}
	rows := make([]string, n)
	for i := range rows {
		rows[i] = fmt.Sprintf("%s(%s): %s %s",
			kinds[i%len(kinds)], areas[(i/3)%len(areas)],
			verbs[(i/7)%len(verbs)], objects[(i/11)%len(objects)])
	}
	return rows
}

// benchFilterPass mirrors one filter pass from main.go: four fuzzyMatch calls
// per row against subject, author, rendered row and refs.
func benchFilterPass(rows []string, token string) {
	for _, row := range rows {
		fuzzyMatch(token, row)
		fuzzyMatch(token, "W. Körner")
		fuzzyMatch(token, "abc1234 "+row+"")
		fuzzyMatch(token, "(HEAD -> main, origin/main) tag-1.2")
	}
}

func BenchmarkFilterFuzzyMatch(b *testing.B) {
	rows := benchRows(500)
	for _, token := range []string{"fix", "worktree", "feat(theme)", "staged hunks"} {
		b.Run(token, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchFilterPass(rows, token)
			}
		})
	}
}

// benchFilterEnv is the filter benchmark's environment with options and
// decorations initialized.
func benchFilterEnv() *Env {
	env := benchIngestEnv()
	env.Opts = NewOptions(env.Cfg)
	return env
}

// BenchmarkMainFilterPass measures the main view's real per-keystroke
// filter pass: one SetFilter call over 500 pre-loaded rows. The per-row
// filter text (expanded subject, formatted author, refs text) is counted
// whether it is rebuilt per keystroke or cached at batch arrival.
func BenchmarkMainFilterPass(b *testing.B) {
	env := benchFilterEnv()
	v := NewMain(nil, nil)
	v.builder = graph.New(graph.Unicode)
	commits := make([]git.Commit, 500)
	for i, s := range benchRows(500) {
		commits[i] = git.Commit{
			Hash:        fmt.Sprintf("%08x%032x", i, i),
			AuthorName:  "Jane Q Doe",
			AuthorEmail: "jane@example.com",
			Subject:     s,
		}
	}
	// Load like a batch arrival does; the filterable text of the rows is
	// built here (cached from this point on) rather than in the timed
	// filter passes.
	_, gen := v.startLoad()
	v.HandleMsg(env, commitBatchMsg{id: 1, gen: gen, commits: commits, done: true})
	for _, token := range []string{"fix", "worktree", "feat", "staged hunks"} {
		b.Run(token, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				// Reset to the full row set: SetFilter narrows only
				// when the query extends the previous one.
				v.SetFilter(env, "")
				v.SetFilter(env, token)
			}
			if n := v.Len(); n == 0 {
				b.Fatalf("filter token %q matched nothing", token)
			}
		})
	}
	// Clear the filter so the next token passes over all rows.
	v.SetFilter(env, "")
}
