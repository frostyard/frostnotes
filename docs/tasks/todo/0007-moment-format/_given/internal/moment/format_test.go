package moment

// CONTRACT TEST for task card T-0007 (docs/tasks). Do not edit.

import (
	"testing"
	"time"
)

// edt is a fixed zone, so the test does not depend on the machine's.
var edt = time.FixedZone("EDT", -4*60*60)

func at(year int, month time.Month, day, hour, minute, second int) time.Time {
	return time.Date(year, month, day, hour, minute, second, 0, edt)
}

func TestFormat(t *testing.T) {
	friday := at(2026, 10, 16, 9, 5, 3)
	for _, tc := range []struct {
		t            time.Time
		layout, want string
	}{
		{friday, "YYYY-MM-DD", "2026-10-16"},
		{friday, "dddd, MMMM Do YYYY", "Friday, October 16th 2026"},
		{friday, "ddd MMM D YY, dd", "Fri Oct 16 26, Fr"},
		{friday, "YYYY/MM/YYYY-MM-DD", "2026/10/2026-10-16"},
		{friday, "HH:mm:ss A a h hh k kk H m s", "09:05:03 AM am 9 09 9 09 9 5 3"},
		{friday, "Q Qo M Mo DDD DDDD DDDo d do e E", "4 4th 10 10th 289 289 289th 5 5th 5 5"},
		{friday, "gggg-[W]ww w wo GGGG-[W]WW W Wo", "2026-W42 42 42nd 2026-W42 42 42nd"},
		{friday, "X x Z ZZ", "1792155903 1792155903000 -04:00 -0400"},
		{friday, "[Daily]/YYYY-MM-DD [at] HH:mm", "Daily/2026-10-16 at 09:05"},
		{friday, "YYYY[", "2026["},
		{friday, "", ""},
		{friday, "Daily", "16amily"},
		{friday, "YYYY-MM-DD 📅 ☕ — №", "2026-10-16 📅 ☕ — №"},
		{at(2026, 10, 16, 0, 30, 0), "h:mm A, k", "12:30 AM, 24"},
		{at(2026, 10, 16, 12, 30, 0), "h:mm A, k", "12:30 PM, 12"},
		{at(2026, 10, 16, 23, 59, 59), "hh:mm a", "11:59 pm"},
		{at(987, 6, 1, 0, 0, 0), "YYYY YY", "0987 87"},
		{at(2026, 2, 3, 0, 0, 0), "DDD DDDD", "34 034"},
		{time.Date(2026, 10, 16, 9, 5, 3, 0, time.FixedZone("IST", 5*60*60+30*60)), "Z ZZ", "+05:30 +0530"},
		{time.Date(2026, 10, 16, 9, 5, 3, 0, time.UTC), "Z", "+00:00"},
	} {
		if got := Format(tc.layout, tc.t); got != tc.want {
			t.Errorf("Format(%q, %v) = %q, want %q", tc.layout, tc.t, got, tc.want)
		}
	}
}

func TestOrdinals(t *testing.T) {
	want := map[int]string{
		1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 10: "10th", 11: "11th", 12: "12th", 13: "13th",
		20: "20th", 21: "21st", 22: "22nd", 23: "23rd", 24: "24th", 30: "30th", 31: "31st",
	}
	for day, w := range want {
		if got := Format("Do", at(2026, 1, day, 0, 0, 0)); got != w {
			t.Errorf("Do of January %d = %q, want %q", day, got, w)
		}
	}
	// Day of the year 111, 112 and 113 take "th"; 101 and 122 do not.
	for doy, w := range map[int]string{101: "101st", 111: "111th", 112: "112th", 113: "113th", 122: "122nd"} {
		if got := Format("DDDo", time.Date(2026, 1, doy, 0, 0, 0, 0, edt)); got != w {
			t.Errorf("DDDo of day %d = %q, want %q", doy, got, w)
		}
	}
}

// Weeks at the turn of the year: locale weeks (w, gggg) start on Sunday
// and week 1 holds January 1st, as moment.js's default English locale
// counts them; ISO weeks (W, GGGG) start on Monday.
func TestWeeksAtTheTurnOfTheYear(t *testing.T) {
	for _, tc := range []struct {
		date string
		want string
	}{
		{"2026-12-26", "2026-52 2026-52"},
		{"2026-12-27", "2027-01 2026-52"},
		{"2026-12-31", "2027-01 2026-53"},
		{"2027-01-01", "2027-01 2026-53"},
		{"2027-01-03", "2027-02 2026-53"},
		{"2021-01-01", "2021-01 2020-53"},
		{"2024-12-30", "2025-01 2025-01"},
		{"2025-12-31", "2026-01 2026-01"},
		{"2026-01-01", "2026-01 2026-01"},
	} {
		d, err := time.ParseInLocation("2006-01-02", tc.date, edt)
		if err != nil {
			t.Fatal(err)
		}
		if got := Format("gggg-ww GGGG-WW", d); got != tc.want {
			t.Errorf("weeks of %s = %q, want %q", tc.date, got, tc.want)
		}
	}
}
