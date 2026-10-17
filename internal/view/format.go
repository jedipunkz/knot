package view

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// now is replaced in tests.
var now = time.Now

// FormatDate formats t according to the date mode.
func FormatDate(t time.Time, mode, custom string) string {
	if t.IsZero() {
		return ""
	}
	switch mode {
	case "off":
		return ""
	case "relative":
		return relative(t, false)
	case "relative-compact":
		return relative(t, true)
	case "custom":
		return t.Local().Format(strftime(custom))
	default:
		return t.Local().Format("2006-01-02 15:04 -0700")
	}
}

// DateWidth is the column width for a date mode.
func DateWidth(mode, custom string) int {
	switch mode {
	case "off":
		return 0
	case "relative":
		return 14
	case "relative-compact":
		return 4
	case "custom":
		return utf8.RuneCountInString(time.Date(2006, 12, 28, 23, 59, 59, 0, time.Local).Format(strftime(custom)))
	default:
		return 22
	}
}

func relative(t time.Time, compact bool) string {
	d := now().Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	units := []struct {
		d       time.Duration
		long    string
		compact string
	}{
		{365 * 24 * time.Hour, "year", "Y"},
		{30 * 24 * time.Hour, "month", "M"},
		{7 * 24 * time.Hour, "week", "w"},
		{24 * time.Hour, "day", "D"},
		{time.Hour, "hour", "h"},
		{time.Minute, "minute", "m"},
		{time.Second, "second", "s"},
	}
	for _, u := range units {
		if d >= u.d {
			n := int(d / u.d)
			if compact {
				return fmt.Sprintf("%d%s", n, u.compact)
			}
			s := fmt.Sprintf("%d %s", n, u.long)
			if n != 1 {
				s += "s"
			}
			if future {
				return "in " + s
			}
			return s + " ago"
		}
	}
	if compact {
		return "now"
	}
	return "just now"
}

// strftime converts the common strftime directives to a Go layout.
func strftime(f string) string {
	repl := strings.NewReplacer(
		"%Y", "2006", "%y", "06", "%m", "01", "%d", "02", "%e", "_2", "%H", "15", "%I", "03",
		"%M", "04", "%S", "05", "%p", "PM", "%z", "-0700", "%Z", "MST", "%b", "Jan", "%B", "January",
		"%a", "Mon", "%A", "Monday", "%%", "%",
	)
	return repl.Replace(f)
}

// FormatAuthor formats an author according to the author mode.
func FormatAuthor(name, email, mode string) string {
	switch mode {
	case "off":
		return ""
	case "abbreviated":
		var sb strings.Builder
		for _, w := range strings.Fields(name) {
			r, _ := utf8.DecodeRuneInString(w)
			sb.WriteRune(r)
		}
		return sb.String()
	case "email":
		return email
	case "email-user":
		user, _, _ := strings.Cut(email, "@")
		return user
	default:
		return name
	}
}

// Fit pads or truncates s to exactly width cells.
func Fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w > width {
		return ansi.Truncate(s, width-1, "") + "…"
	}
	return s + strings.Repeat(" ", width-w)
}

// FitTail is like Fit but keeps the end of s (for paths).
func FitTail(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(s)
	if w > width {
		return "…" + ansi.TruncateLeft(s, w-width+1, "")
	}
	return s + strings.Repeat(" ", width-w)
}

// ExpandTabs replaces tabs with spaces up to the next tab stop and removes
// control characters that would corrupt the screen.
func ExpandTabs(s string, size int) string {
	if !strings.ContainsAny(s, "\t\r\x1b") {
		return s
	}
	var sb strings.Builder
	col := 0
	for _, r := range s {
		switch {
		case r == '\t':
			n := size - col%size
			sb.WriteString(strings.Repeat(" ", n))
			col += n
		case r == '\r' || r == 0x1b:
			// dropped
		default:
			sb.WriteRune(r)
			col += max(ansi.StringWidth(string(r)), 0)
		}
	}
	return sb.String()
}

// Short abbreviates a hash.
func Short(hash string, n int) string {
	if len(hash) > n {
		return hash[:n]
	}
	return hash
}
