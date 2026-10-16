// Package moment formats dates with moment.js tokens, as
// Obsidian's daily notes and templates write them
// (docs/design/vault.md#daily-notes).
package moment

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Format renders t, in its own location, with a moment.js format string.
func Format(layout string, t time.Time) string {
	var out strings.Builder
	out.Grow(len(layout))
	for len(layout) > 0 {
		if layout[0] == '[' {
			end := strings.IndexAny(layout[1:], "[]")
			if end >= 0 && layout[end+1] == ']' {
				out.WriteString(layout[1 : end+1])
				layout = layout[end+2:]
				continue
			}
		}
		// Every supported token is at most four bytes long. Trying longer
		// prefixes first also handles adjacent tokens without separators.
		n := min(4, len(layout))
		for ; n > 0; n-- {
			if value, ok := formatToken(layout[:n], t); ok {
				out.WriteString(value)
				break
			}
		}
		if n == 0 {
			out.WriteByte(layout[0])
			n = 1
		}
		layout = layout[n:]
	}
	return out.String()
}

func formatToken(token string, t time.Time) (string, bool) {
	switch token {
	case "MMM":
		return t.Month().String()[:3], true
	case "MMMM":
		return t.Month().String(), true
	case "dd", "ddd":
		return t.Weekday().String()[:len(token)], true
	case "dddd":
		return t.Weekday().String(), true
	case "A", "a":
		value := "AM"
		if t.Hour() >= 12 {
			value = "PM"
		}
		if token == "a" {
			value = strings.ToLower(value)
		}
		return value, true
	case "X":
		return strconv.FormatInt(t.Unix(), 10), true
	case "x":
		return strconv.FormatInt(t.UnixMilli(), 10), true
	case "Z", "ZZ":
		return formatOffset(token, t), true
	}

	value, ok := dateNumber(token, t)
	if !ok {
		value, ok = clockNumber(token, t)
	}
	if !ok {
		return "", false
	}
	return formatNumber(token, value), true
}

func dateNumber(token string, t time.Time) (int, bool) {
	switch token {
	case "YYYY":
		return t.Year(), true
	case "YY":
		return t.Year() % 100, true
	case "Q", "Qo":
		return (int(t.Month())-1)/3 + 1, true
	case "M", "Mo", "MM":
		return int(t.Month()), true
	case "D", "Do", "DD":
		return t.Day(), true
	case "DDD", "DDDo", "DDDD":
		return t.YearDay(), true
	case "d", "do", "e":
		return int(t.Weekday()), true
	case "E":
		return (int(t.Weekday())+6)%7 + 1, true
	case "w", "wo", "ww", "gggg":
		// English locale weeks belong to the year of their Saturday.
		// AddDate preserves calendar days across offset changes.
		saturday := t.AddDate(0, 0, int(time.Saturday-t.Weekday()))
		if token == "gggg" {
			return saturday.Year(), true
		}
		return (saturday.YearDay()-1)/7 + 1, true
	case "W", "Wo", "WW", "GGGG":
		year, week := t.ISOWeek()
		if token == "GGGG" {
			return year, true
		}
		return week, true
	default:
		return 0, false
	}
}

func clockNumber(token string, t time.Time) (int, bool) {
	switch token {
	case "H", "HH":
		return t.Hour(), true
	case "h", "hh":
		hour := t.Hour() % 12
		if hour == 0 {
			hour = 12
		}
		return hour, true
	case "k", "kk":
		hour := t.Hour()
		if hour == 0 {
			hour = 24
		}
		return hour, true
	case "m", "mm":
		return t.Minute(), true
	case "s", "ss":
		return t.Second(), true
	default:
		return 0, false
	}
}

func formatNumber(token string, value int) string {
	if strings.HasSuffix(token, "o") {
		return ordinal(value)
	}
	width := 0
	switch token {
	case "YYYY", "gggg", "GGGG":
		width = 4
	case "DDDD":
		width = 3
	case "YY", "MM", "DD", "ww", "WW", "HH", "hh", "kk", "mm", "ss":
		width = 2
	}
	return fmt.Sprintf("%0*d", width, value)
}

func ordinal(value int) string {
	suffix := "th"
	if value%100 < 11 || value%100 > 13 {
		switch value % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return strconv.Itoa(value) + suffix
}

func formatOffset(token string, t time.Time) string {
	_, offset := t.Zone()
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	separator := ""
	if token == "Z" {
		separator = ":"
	}
	return fmt.Sprintf("%s%02d%s%02d", sign, offset/3600, separator, offset/60%60)
}
