package view

import (
	"reflect"
	"testing"

	"github.com/jedipunkz/knot/internal/theme"
)

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		token, text string
		want        []int
		ok          bool
	}{
		{"", "text", nil, false},
		{"abc", "", nil, false},
		{"abcd", "abc", nil, false},
		{"fix", "fix(worktree): handle prunable", []int{0, 1, 2}, true},
		{"hw", "hello world", []int{0, 6}, true},
		{"hlo", "hello", []int{0, 2, 4}, true},
		{"hello", "hello", []int{0, 1, 2, 3, 4}, true},
		{"hello", "hell", nil, false},
		{"zz", "hello", nil, false},
		// Consecutive characters win over scattered ones.
		{"ab", "abab", []int{0, 1}, true},
		// Word starts win over mid-word positions.
		{"fb", "foo-bar", []int{0, 4}, true},
		// Smart case: a lowercase token is case-insensitive.
		{"foo", "FOObar", []int{0, 1, 2}, true},
		{"foo", "fóobar", nil, false},
		// Smart case: an uppercase token is case-sensitive.
		{"FOO", "foobar", nil, false},
		{"FOO", "FOObar", []int{0, 1, 2}, true},
		{"Foo", "foobar", nil, false},
		{"Foo", "aFoo", []int{1, 2, 3}, true},
	}
	for _, c := range cases {
		pos, ok := fuzzyMatch(c.token, c.text)
		if ok != c.ok {
			t.Errorf("fuzzyMatch(%q, %q) ok = %v, want %v (pos %v)", c.token, c.text, ok, c.ok, pos)
			continue
		}
		if !ok {
			continue
		}
		if !reflect.DeepEqual(pos, c.want) {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", c.token, c.text, pos, c.want)
			continue
		}
		// The positions must form an ordered subsequence of the text.
		rs := []rune(c.text)
		for _, p := range pos {
			if p < 0 || p >= len(rs) {
				t.Errorf("fuzzyMatch(%q, %q): position %d out of range", c.token, c.text, p)
			}
		}
	}
}

func TestSplitMatched(t *testing.T) {
	th := testEnv(t).Theme
	hl := []int{1, 3}
	text := "abcd"
	// Positions 1 and 3 of "abcd", with the text at offset 0.
	segs := splitMatched(th, theme.Default, text, hl, new(int))
	var got string
	var hlText string
	for _, s := range segs {
		got += s.Text
		if s.Role == theme.SearchMatch {
			hlText += s.Text
		}
	}
	if got != text {
		t.Errorf("joined = %q, want %q", got, text)
	}
	if hlText != "bd" {
		t.Errorf("highlighted = %q, want %q", hlText, "bd")
	}
	// Offsets: the positions refer to the parent string.
	off := 2
	segs = splitMatched(th, theme.Default, text, []int{2, 5}, &off)
	hlText = ""
	for _, s := range segs {
		if s.Role == theme.SearchMatch {
			hlText += s.Text
		}
	}
	if hlText != "ad" {
		t.Errorf("offset highlighted = %q, want %q", hlText, "ad")
	}
	if off != 6 {
		t.Errorf("offset = %d, want 6", off)
	}
}
