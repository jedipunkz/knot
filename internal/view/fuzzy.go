// Fuzzy matching for the filter bar, plus helpers that split a text into
// styled segments around matched rune positions.
package view

import (
	"strings"
	"unicode/utf8"

	"github.com/jedipunkz/knot/internal/theme"
)

// fuzzyMatch reports whether token matches text as a subsequence and returns
// the rune positions of the match. Matching is smart case: case-insensitive
// unless the token contains an uppercase letter, consistent with the regexp
// search. Among the possible alignments the highest-scoring one is returned,
// where consecutive characters and word starts score higher (fzf-style);
// the score only chooses which characters to highlight, never the row order.
func fuzzyMatch(token, text string) ([]int, bool) {
	if token == "" || text == "" {
		return nil, false
	}
	t := []rune(token)
	s := []rune(text)
	if token == strings.ToLower(token) {
		t = []rune(strings.ToLower(token))
		s = []rune(strings.ToLower(text))
	}
	if len(t) > len(s) {
		return nil, false
	}
	// Candidate positions of each token rune in the text.
	cand := make([][]int, len(t))
	for i, tr := range t {
		for p, sr := range s {
			if sr == tr {
				cand[i] = append(cand[i], p)
			}
		}
		if len(cand[i]) == 0 {
			return nil, false
		}
	}
	// Dynamic programming over the candidates: best[j] is the best score of
	// an alignment ending at cand[i][j]; the sweep over the previous layer's
	// sorted candidates keeps it O(positions) per token rune.
	const negInf = -1 << 30
	best := make([]int, len(cand[0]))
	for j, p := range cand[0] {
		if wordStart(s, p) {
			best[j] += 2
		}
	}
	prevs := make([][]int, len(t))
	for i := 1; i < len(t); i++ {
		next := make([]int, len(cand[i]))
		prevs[i] = make([]int, len(cand[i]))
		cur, ptr, lastScore, lastIdx := cand[i-1], 0, negInf, -1
		for j, p := range cand[i] {
			for ptr < len(cur) && cur[ptr] < p {
				if best[ptr] > lastScore {
					lastScore, lastIdx = best[ptr], ptr
				}
				ptr++
			}
			if lastIdx < 0 {
				next[j], prevs[i][j] = negInf, -1
				continue
			}
			score := lastScore
			if p == cur[lastIdx]+1 {
				score += 2 // consecutive characters
			}
			if wordStart(s, p) {
				score += 2
			}
			next[j], prevs[i][j] = score, lastIdx
		}
		best = next
	}
	last := len(t) - 1
	bi := -1
	for j, score := range best {
		if score == negInf {
			continue
		}
		if bi < 0 || score > best[bi] {
			bi = j
		}
	}
	if bi < 0 {
		return nil, false
	}
	pos := make([]int, len(t))
	j := bi
	for i := last; i > 0; i-- {
		pos[i] = cand[i][j]
		j = prevs[i][j]
	}
	pos[0] = cand[0][j]
	return pos, true
}

func wordStart(s []rune, p int) bool {
	if p == 0 {
		return true
	}
	prev := s[p-1]
	switch {
	case prev >= 'a' && prev <= 'z', prev >= 'A' && prev <= 'Z', prev >= '0' && prev <= '9':
		return false
	}
	return true
}

// splitMatched renders text in role, splitting it into segments at the
// highlighted rune positions. off is the rune offset of text within the
// string the positions refer to; it is advanced by the rune count of text.
func splitMatched(th *theme.Theme, role theme.Role, text string, hl []int, off *int) []Seg {
	if len(hl) == 0 {
		*off += utf8.RuneCountInString(text)
		return []Seg{{role, text}}
	}
	rs := []rune(text)
	var segs []Seg
	run, i := 0, 0
	for k := 0; k < len(rs); k++ {
		if i < len(hl) && hl[i] == *off+k {
			if k > run {
				segs = append(segs, Seg{role, string(rs[run:k])})
			}
			segs = append(segs, Seg{theme.SearchMatch, string(rs[k : k+1])})
			i++
			run = k + 1
		}
	}
	if run < len(rs) {
		segs = append(segs, Seg{role, string(rs[run:])})
	}
	*off += len(rs)
	return segs
}

// unionPos merges two sorted position lists into one sorted list.
func unionPos(a, b []int) []int {
	if len(b) == 0 {
		return a
	}
	out := make([]int, 0, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] < b[j]:
			out = append(out, a[i])
			i++
		case a[i] > b[j]:
			out = append(out, b[j])
			j++
		default:
			out = append(out, a[i])
			i++
			j++
		}
	}
	out = append(out, a[i:]...)
	out = append(out, b[j:]...)
	return out
}
