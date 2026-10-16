// Package templates expands the variables of Obsidian's
// templates and daily-note templates (docs/design/vault.md#daily-notes).
package templates

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/frostyard/frostnotes/internal/moment"
)

// Values supplies the title, clock and default formats for expansion.
type Values struct {
	Title      string    // {{title}}
	Now        time.Time // {{date}} and {{time}}, formatted in its own location
	DateFormat string    // the moment.js format of {{date}}; "" is "YYYY-MM-DD"
	TimeFormat string    // the moment.js format of {{time}}; "" is "HH:mm"
}

var variable = regexp.MustCompile(`\{\{[ \t]*((?i:title|date|time))([+-][0-9]{1,6}[yMwdhms])?[ \t]*(:[^\r\n]*?)?[ \t]*\}\}`)

// Expand returns tmpl with its variables replaced; every other byte is
// copied as it is.
func Expand(tmpl []byte, v Values) []byte {
	return variable.ReplaceAllFunc(tmpl, func(match []byte) []byte {
		parts := variable.FindSubmatch(match)
		name := strings.ToLower(string(parts[1]))
		offset, format := string(parts[2]), string(parts[3])
		if name == "title" {
			if offset != "" || format != "" {
				return match
			}
			return []byte(v.Title)
		}

		layout := v.DateFormat
		if layout == "" {
			layout = "YYYY-MM-DD"
		}
		if name == "time" {
			layout = v.TimeFormat
			if layout == "" {
				layout = "HH:mm"
			}
		}
		if format != "" {
			layout = strings.Trim(format[1:], " \t")
		}
		return []byte(moment.Format(layout, applyOffset(v.Now, offset)))
	})
}

func applyOffset(t time.Time, offset string) time.Time {
	if offset == "" {
		return t
	}
	// The regexp guarantees a signed integer with at most six digits.
	n, _ := strconv.Atoi(offset[:len(offset)-1])
	switch offset[len(offset)-1] {
	case 'y':
		return addMonths(t, 12*n)
	case 'M':
		return addMonths(t, n)
	case 'w':
		return t.AddDate(0, 0, 7*n)
	case 'd':
		return t.AddDate(0, 0, n)
	case 'h':
		return t.Add(time.Duration(n) * time.Hour)
	case 'm':
		return t.Add(time.Duration(n) * time.Minute)
	case 's':
		return t.Add(time.Duration(n) * time.Second)
	default:
		return t
	}
}

func addMonths(t time.Time, months int) time.Time {
	// Normalize from day one so a short target month cannot overflow.
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, 0, 0, 0, 0, t.Location())
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(t.Day(), last),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
}
