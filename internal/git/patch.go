package git

import (
	"fmt"
	"strconv"
	"strings"
)

// Hunk is one @@ section of a file diff.
type Hunk struct {
	Start    int // index of the @@ line in FileDiff.Lines
	End      int // index after the last line of the hunk
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Section  string // text after the closing @@
}

// FileDiff is a parsed diff of a single file.
type FileDiff struct {
	Lines  []string
	Header []string // lines before the first hunk (diff --git, index, ---, +++)
	Hunks  []Hunk
}

// ParseFileDiff parses the output of git diff for a single file.
func ParseFileDiff(text string) *FileDiff {
	fd := &FileDiff{}
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return fd
	}
	fd.Lines = strings.Split(text, "\n")
	for i, l := range fd.Lines {
		if strings.HasPrefix(l, "@@ ") {
			if n := len(fd.Hunks); n > 0 {
				fd.Hunks[n-1].End = i
			}
			h, ok := parseHunkHeader(l)
			if !ok {
				continue
			}
			h.Start = i
			fd.Hunks = append(fd.Hunks, h)
		} else if len(fd.Hunks) == 0 {
			fd.Header = append(fd.Header, l)
		}
	}
	if n := len(fd.Hunks); n > 0 {
		fd.Hunks[n-1].End = len(fd.Lines)
	}
	return fd
}

func parseHunkHeader(l string) (Hunk, bool) {
	// @@ -a,b +c,d @@ section
	rest := strings.TrimPrefix(l, "@@ ")
	ranges, section, ok := strings.Cut(rest, " @@")
	if !ok {
		return Hunk{}, false
	}
	p := strings.Fields(ranges)
	if len(p) != 2 {
		return Hunk{}, false
	}
	var h Hunk
	h.OldStart, h.OldCount = parseRange(strings.TrimPrefix(p[0], "-"))
	h.NewStart, h.NewCount = parseRange(strings.TrimPrefix(p[1], "+"))
	h.Section = section
	return h, true
}

func parseRange(s string) (int, int) {
	a, b, ok := strings.Cut(s, ",")
	start, _ := strconv.Atoi(a)
	if !ok {
		return start, 1
	}
	count, _ := strconv.Atoi(b)
	return start, count
}

// HunkAt returns the index of the hunk containing line, or -1.
func (fd *FileDiff) HunkAt(line int) int {
	for i, h := range fd.Hunks {
		if line >= h.Start && line < h.End {
			return i
		}
	}
	return -1
}

func (fd *FileDiff) header() string {
	return strings.Join(fd.Header, "\n") + "\n"
}

// HunkPatch returns a patch containing only hunk i.
func (fd *FileDiff) HunkPatch(i int) string {
	h := fd.Hunks[i]
	var b strings.Builder
	b.WriteString(fd.header())
	for _, l := range fd.Lines[h.Start:h.End] {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

// LinePatch returns a patch that applies only the change on line (an index
// into Lines). When reverse is true the patch is meant for git apply -R
// (unstaging): unselected additions become context and unselected removals
// are dropped. Otherwise unselected additions are dropped and unselected
// removals become context.
func (fd *FileDiff) LinePatch(line int, reverse bool) (string, error) {
	hi := fd.HunkAt(line)
	if hi < 0 {
		return "", fmt.Errorf("line is not inside a hunk")
	}
	sel := fd.Lines[line]
	if !strings.HasPrefix(sel, "+") && !strings.HasPrefix(sel, "-") {
		return "", fmt.Errorf("line is not an added or removed line")
	}
	h := fd.Hunks[hi]
	var body []string
	oldCount, newCount := 0, 0
	kept := false // whether the previous source line was kept
	keep := func(l string, inOld, inNew bool) {
		body = append(body, l)
		if inOld {
			oldCount++
		}
		if inNew {
			newCount++
		}
		kept = true
	}
	for i := h.Start + 1; i < h.End; i++ {
		l := fd.Lines[i]
		switch {
		case strings.HasPrefix(l, "\\"):
			// "\ No newline at end of file" belongs to the previous line.
			if kept {
				body = append(body, l)
			}
		case i == line:
			keep(l, l[0] == '-', l[0] == '+')
		case strings.HasPrefix(l, "+"):
			if reverse {
				keep(" "+l[1:], true, true)
			} else {
				kept = false
			}
		case strings.HasPrefix(l, "-"):
			if reverse {
				kept = false
			} else {
				keep(" "+l[1:], true, true)
			}
		default:
			keep(l, true, true)
		}
	}
	newStart := h.OldStart
	if oldCount == 0 && newCount > 0 {
		newStart = h.OldStart + 1
	}
	var b strings.Builder
	b.WriteString(fd.header())
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@%s\n", h.OldStart, oldCount, newStart, newCount, h.Section)
	for _, l := range body {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String(), nil
}
