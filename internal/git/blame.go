package git

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// BlameCommit holds the commit information of blamed lines.
type BlameCommit struct {
	Hash         string
	Author       string
	AuthorMail   string
	AuthorTime   time.Time
	Summary      string
	Boundary     bool
	Previous     string // previous commit, empty if none
	PreviousFile string
}

// BlameLine is one line of a blamed file.
type BlameLine struct {
	Commit   *BlameCommit
	OrigLine int
	Line     int
	Filename string // file name in Commit
	Text     string
}

// Blame runs git blame --porcelain on path at rev (working tree when empty).
func Blame(ctx context.Context, r Runner, rev, path string) ([]BlameLine, error) {
	args := []string{"blame", "--porcelain"}
	if rev != "" {
		args = append(args, rev)
	}
	args = append(args, "--", path)
	out, err := r.Output(ctx, args...)
	if err != nil {
		return nil, err
	}
	return ParseBlame(string(out)), nil
}

// ParseBlame parses git blame --porcelain output.
func ParseBlame(out string) []BlameLine {
	commits := map[string]*BlameCommit{}
	files := map[string]string{}
	var lines []BlameLine
	var cur *BlameLine
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "\t") {
			if cur != nil {
				cur.Text = l[1:]
				if cur.Filename == "" {
					cur.Filename = files[cur.Commit.Hash]
				}
				lines = append(lines, *cur)
				cur = nil
			}
			continue
		}
		if cur == nil {
			p := strings.Fields(l)
			if len(p) < 3 || len(p[0]) < 40 {
				continue
			}
			c, ok := commits[p[0]]
			if !ok {
				c = &BlameCommit{Hash: p[0]}
				commits[p[0]] = c
			}
			orig, _ := strconv.Atoi(p[1])
			final, _ := strconv.Atoi(p[2])
			cur = &BlameLine{Commit: c, OrigLine: orig, Line: final}
			continue
		}
		key, val, _ := strings.Cut(l, " ")
		c := cur.Commit
		switch key {
		case "author":
			c.Author = val
		case "author-mail":
			c.AuthorMail = strings.Trim(val, "<>")
		case "author-time":
			c.AuthorTime = parseUnix(val)
		case "summary":
			c.Summary = val
		case "boundary":
			c.Boundary = true
		case "previous":
			c.Previous, c.PreviousFile, _ = strings.Cut(val, " ")
		case "filename":
			cur.Filename = val
			files[c.Hash] = val
		}
	}
	return lines
}
