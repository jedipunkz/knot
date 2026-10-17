package graph

import "testing"

func TestLinearHistory(t *testing.T) {
	b := New(ASCII)
	rows := []string{
		String(b.Add("c3", []string{"c2"})),
		String(b.Add("c2", []string{"c1"})),
		String(b.Add("c1", nil)),
	}
	for i, r := range rows {
		if r != "*" {
			t.Errorf("row %d = %q", i, r)
		}
	}
}

func TestMergeHistory(t *testing.T) {
	// m merges a (first parent) and b; both come from base.
	b := New(ASCII)
	got := []string{
		String(b.Add("m", []string{"a", "b"})),
		String(b.Add("b", []string{"base"})),
		String(b.Add("a", []string{"base"})),
		String(b.Add("base", nil)),
	}
	want := []string{
		"M-\\",
		"| *",
		"* |",
		"*-/",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %q, want %q", i, got[i], want[i])
		}
	}
}
