// Benchmarks for graph construction over the main view's log: one op builds
// the rows for a 1000-commit history with a realistic merge topology.
package graph

import (
	"fmt"
	"math/rand/v2"
	"testing"
)

type benchCommit struct {
	hash    string
	parents []string
}

// benchHistory builds a deterministic history of n commits: a spine of
// first-parent commits with occasional side branches that merge back in.
// The hashes are fixed-width strings, as git log reports them.
func benchHistory(n int) []benchCommit {
	rng := rand.New(rand.NewPCG(42, 1))
	commits := make([]benchCommit, 0, n)
	hash := func(i int) string { return fmt.Sprintf("%08x", i) }
	for i := range n {
		var parents []string
		switch {
		case i == 0:
			parents = nil
		case rng.IntN(10) == 0: // merge: two parents, one being a side branch tip
			side := hash(i - 1 - rng.IntN(min(5, i)))
			parents = []string{hash(i - 1), side}
		default:
			parents = []string{hash(i - 1)}
		}
		commits = append(commits, benchCommit{hash(i), parents})
	}
	return commits
}

func BenchmarkBuilderAdd1000(b *testing.B) {
	commits := benchHistory(1000)
	b.ReportAllocs()
	for b.Loop() {
		builder := New(Unicode)
		for _, c := range commits {
			builder.Add(c.hash, c.parents)
		}
	}
}
