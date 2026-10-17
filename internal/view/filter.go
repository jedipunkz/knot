// Shared machinery of the filterable list views (main, worktree): token
// matching over the rows' filterable fields. Every token must match at
// least one field; the matched rune positions are unioned across tokens
// (last match wins) so a row highlights everything the tokens matched.
package view

import "strings"

// filterField is one filterable text field of a row. Most fields match a
// token fuzzily; prefix fields (commit IDs) only match the case-insensitive
// token as a prefix of their text. A field can also be matchable without
// showing highlight positions (the raw author name of a main row): the
// view keeps only the positions of the fields it renders.
type filterField struct {
	text   string
	prefix bool
}

// matchFields matches every token against the fields. Each token must match
// at least one field: fuzzy fields match through fuzzyMatch, prefix fields
// when the token is a case-insensitive prefix of text. It returns the union
// of the matched rune positions per field (one slice per field, positions
// referring to the field's text), or nil when the row does not match.
func matchFields(tokens []string, fields []filterField) [][]int {
	var hl [][]int
	for _, tok := range tokens {
		perTok := make([][]int, len(fields))
		matched := false
		for i, f := range fields {
			if f.prefix {
				if strings.HasPrefix(strings.ToLower(f.text), strings.ToLower(tok)) {
					matched = true
					// The leading token runes are highlighted.
					perTok[i] = prefixPositions(len(tok))
				}
				continue
			}
			if pos, ok := fuzzyMatch(tok, f.text); ok {
				matched = true
				perTok[i] = pos
			}
		}
		if !matched {
			return nil
		}
		if hl == nil {
			hl = make([][]int, len(fields))
		}
		for i := range fields {
			hl[i] = unionPos(hl[i], perTok[i])
		}
	}
	return hl
}

// prefixPositions lists the runes 0..n-1, the match positions of a prefix
// token of n runes.
func prefixPositions(n int) []int {
	pos := make([]int, n)
	for i := range pos {
		pos[i] = i
	}
	return pos
}

// filterRowsOf returns the rows of in matching every token, keeping the
// original order. The tokens come from the active query; with none the
// rows are returned unchanged. fieldsOf returns the filterable fields of a
// row, or nil when the row is hidden while a filter is active (pseudo
// rows); hlOf copies the row and attaches the match positions to it.
func filterRowsOf[R any](rows []R, tokens []string, fieldsOf func(R) []filterField, hlOf func(R, [][]int) R) []R {
	if len(tokens) == 0 {
		return rows
	}
	out := make([]R, 0, len(rows))
	for _, r := range rows {
		f := fieldsOf(r)
		if f == nil {
			continue
		}
		if m := matchFields(tokens, f); m != nil {
			out = append(out, hlOf(r, m))
		}
	}
	return out
}
