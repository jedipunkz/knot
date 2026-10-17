// Package graph computes a one-row-per-commit revision graph from commit
// parents, in the style of tig.
package graph

// Cell is one character of a graph row. Lane selects the color.
type Cell struct {
	Ch   rune
	Lane int
}

// Symbols used for drawing.
type Symbols struct {
	Commit, Merge, Vertical, Horizontal, Cross rune
	EndRight, EndLeft, StartRight, StartLeft   rune
	JoinRight, JoinLeft                        rune
}

// Unicode and ASCII symbol sets.
var (
	Unicode = Symbols{'●', '◎', '│', '─', '┼', '╯', '╰', '╮', '╭', '┤', '├'}
	ASCII   = Symbols{'*', 'M', '|', '-', '|', '/', '\\', '\\', '/', '|', '|'}
)

// Builder holds the lane state between commits. Commits must be added in the
// order git log prints them.
type Builder struct {
	lanes []string
	Sym   Symbols
}

// New returns a builder with the given symbols.
func New(sym Symbols) *Builder {
	return &Builder{Sym: sym}
}

type link struct {
	lane int
	ch   rune
}

// Add computes the row for one commit.
func (b *Builder) Add(hash string, parents []string) []Cell {
	col := -1
	var links []link
	for i, h := range b.lanes {
		if h != hash {
			continue
		}
		if col < 0 {
			col = i
		} else {
			links = append(links, link{i, 0})
			b.lanes[i] = ""
		}
	}
	if col < 0 {
		col = b.free(-1)
	}
	for i := range links {
		if links[i].lane > col {
			links[i].ch = b.Sym.EndRight
		} else {
			links[i].ch = b.Sym.EndLeft
		}
	}
	if len(parents) > 0 {
		b.lanes[col] = parents[0]
	} else {
		b.lanes[col] = ""
	}
	for _, p := range parents[min(1, len(parents)):] {
		existing := -1
		for i, h := range b.lanes {
			if h == p && i != col {
				existing = i
				break
			}
		}
		if existing >= 0 {
			ch := b.Sym.JoinRight
			if existing < col {
				ch = b.Sym.JoinLeft
			}
			links = append(links, link{existing, ch})
			continue
		}
		lane := b.free(col)
		b.lanes[lane] = p
		ch := b.Sym.StartRight
		if lane < col {
			ch = b.Sym.StartLeft
		}
		links = append(links, link{lane, ch})
	}

	width := len(b.lanes)
	for _, l := range links {
		width = max(width, l.lane+1)
	}
	row := make([]Cell, width*2)
	for i := range row {
		row[i] = Cell{' ', i / 2}
	}
	for i, h := range b.lanes {
		if h != "" {
			row[i*2] = Cell{b.Sym.Vertical, i}
		}
	}
	for _, l := range links {
		lo, hi := min(col, l.lane), max(col, l.lane)
		for x := lo*2 + 1; x < hi*2; x++ {
			if x%2 == 0 && row[x].Ch == b.Sym.Vertical {
				row[x] = Cell{b.Sym.Cross, x / 2}
			} else if row[x].Ch == ' ' {
				row[x] = Cell{b.Sym.Horizontal, l.lane}
			}
		}
		row[l.lane*2] = Cell{l.ch, l.lane}
	}
	sym := b.Sym.Commit
	if len(parents) > 1 {
		sym = b.Sym.Merge
	}
	row[col*2] = Cell{sym, col}

	for len(b.lanes) > 0 && b.lanes[len(b.lanes)-1] == "" {
		b.lanes = b.lanes[:len(b.lanes)-1]
	}
	for len(row) > 0 && row[len(row)-1].Ch == ' ' {
		row = row[:len(row)-1]
	}
	return row
}

// free returns the first empty lane after the given index, appending one
// if none is available.
func (b *Builder) free(after int) int {
	for i := after + 1; i < len(b.lanes); i++ {
		if b.lanes[i] == "" {
			return i
		}
	}
	b.lanes = append(b.lanes, "")
	return len(b.lanes) - 1
}

// String renders a row without colors.
func String(row []Cell) string {
	rs := make([]rune, len(row))
	for i, c := range row {
		rs[i] = c.Ch
	}
	return string(rs)
}
