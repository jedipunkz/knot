// Benchmarks for the filter hot path: one op is one filter pass over the
// main view's rows, mirroring the calls filterMain makes per token (subject,
// author, rendered row and refs).
package view

import (
	"fmt"
	"testing"
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
