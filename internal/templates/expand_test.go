package templates

// CONTRACT TEST for task card T-0013 (docs/tasks). Do not edit.

import (
	"testing"
	"time"
)

// edt is a fixed zone, so the test does not depend on the machine's.
var edt = time.FixedZone("EDT", -4*60*60)

func TestExpand(t *testing.T) {
	friday := Values{Title: "Plan", Now: time.Date(2026, 10, 16, 9, 5, 3, 0, edt)}
	for _, tc := range []struct {
		name string
		v    Values
		tmpl string
		want string
	}{
		{"title", friday, "# {{title}}\n", "# Plan\n"},
		{"default formats", friday, "{{date}} {{time}}", "2026-10-16 09:05"},
		{"settings' formats", Values{Now: friday.Now, DateFormat: "DD.MM.YYYY", TimeFormat: "h:mm A"}, "{{date}} {{time}}", "16.10.2026 9:05 AM"},
		{"date format", friday, "{{date:dddd, MMMM Do}}", "Friday, October 16th"},
		{"a format may hold a colon", friday, "{{time:HH:mm:ss}}", "09:05:03"},
		{"names ignore case and spaces", friday, "{{ Title }} {{DATE}} {{ time : HH }}", "Plan 2026-10-16 09"},
		{"several on a line", friday, "[[{{date}}]] by {{title}} at {{time}} on {{date:ddd}}", "[[2026-10-16]] by Plan at 09:05 on Fri"},
		{"yesterday", friday, "[[{{date-1d:YYYY-MM-DD}}]] [[{{date+1d}}]]", "[[2026-10-15]] [[2026-10-17]]"},
		{"weeks and years", friday, "{{date+1w}} {{date-2y:YYYY}}", "2026-10-23 2024"},
		{"hours, minutes and seconds", friday, "{{time+90m}} {{time-1h:HH}} {{time+57s:HH:mm:ss}}", "10:35 08 09:06:00"},
		{"a month keeps the day or the month's last", Values{Now: time.Date(2026, 1, 31, 12, 0, 0, 0, edt)},
			"{{date+1M}} {{date+2M}} {{date-1M}}", "2026-02-28 2026-03-31 2025-12-31"},
		{"a year from a leap day", Values{Now: time.Date(2024, 2, 29, 12, 0, 0, 0, edt)}, "{{date+1y}}", "2025-02-28"},
		{"its own location", Values{Now: time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC)}, "{{date}} {{time:Z}}", "2026-10-17 +00:00"},
		{"unknown variables stay", friday, "{{foo}} {{title:x}} {{title+1d}} {date} {{ }} {{date+1x}} {{date", "{{foo}} {{title:x}} {{title+1d}} {date} {{ }} {{date+1x}} {{date"},
		{"not across lines", friday, "{{date:\nYYYY}}", "{{date:\nYYYY}}"},
		{"bytes kept", friday, "\xef\xbb\xbf---\r\ntitle: {{title}}\r\n---\r\nbody \xff", "\xef\xbb\xbf---\r\ntitle: Plan\r\n---\r\nbody \xff"},
		{"no variables", friday, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := string(Expand([]byte(tc.tmpl), tc.v)); got != tc.want {
				t.Fatalf("Expand(%q) = %q, want %q", tc.tmpl, got, tc.want)
			}
		})
	}
}
