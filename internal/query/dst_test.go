package query_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestDaysAreCalendarDays: across the clock change of 2026-03-08 in New
// York, "1 day" is the same wall-clock time on the next day, and "24 hours"
// is 24 exact hours, so it is 1 hour off.
func TestDaysAreCalendarDays(t *testing.T) {
	dir := t.TempDir()
	for name, at := range map[string]string{
		"mar07-11.md": "2026-03-07T11:00", "mar07-12.md": "2026-03-07T12:00",
		"mar08-12.md": "2026-03-08T12:00", "mar08-13.md": "2026-03-08T13:00",
		"mar14-12.md": "2026-03-14T12:00", "mar14-13.md": "2026-03-14T13:00",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("---\nat: "+at+"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := indexedEnv(t, dir)
	ny := e.loc
	tests := []struct {
		now  time.Time
		stmt string
		want []string
	}{
		// EST to EDT: 12:00 on the 7th plus a day is 12:00 on the 8th.
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "1 day"`, []string{"mar08-12.md"}},
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "1d"`, []string{"mar08-12.md"}},
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "24 hours"`, []string{"mar08-13.md"}},
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "24h"`, []string{"mar08-13.md"}},
		// And back: 12:00 on the 8th minus a day is 12:00 on the 7th; minus 24 hours it is 11:00.
		{time.Date(2026, 3, 8, 12, 0, 0, 0, ny), `at == now() - "1 day"`, []string{"mar07-12.md"}},
		{time.Date(2026, 3, 8, 12, 0, 0, 0, ny), `at == now() - "24 hours"`, []string{"mar07-11.md"}},
		// A week across the change keeps the wall-clock time; 168 hours does not.
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "1 week"`, []string{"mar14-12.md"}},
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "7 days"`, []string{"mar14-12.md"}},
		{time.Date(2026, 3, 7, 12, 0, 0, 0, ny), `at == now() + "168 hours"`, []string{"mar14-13.md"}},
		// today() is the local midnight, and a day from it is the next midnight.
		{time.Date(2026, 3, 7, 18, 0, 0, 0, ny), `at > today() + "1 day" && at < today() + "2 days"`, []string{"mar08-12.md", "mar08-13.md"}},
	}
	for _, tc := range tests {
		e.now = tc.now
		res := e.mustRun(filterBase(tc.stmt))
		if got := paths(res); !slices.Equal(got, tc.want) {
			t.Errorf("now = %s: %s\n got %q\nwant %q", tc.now.Format(time.RFC3339), tc.stmt, got, tc.want)
		}
	}
}
