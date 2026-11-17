package store

import (
	"database/sql/driver"
	"fmt"
	"time"

	"modernc.org/sqlite"
)

// timeFunc is the name of the SQL function that reads a date as Unix
// milliseconds, for queries over frontmatter dates (docs/specs/bases.md#sql).
const timeFunc = "frostnotes_time"

// init registers frostnotes_time(text, zone) with the SQLite driver, which
// gives it to every connection opened after: it must run before the first
// sql.Open in this package. The function is deterministic and keeps no
// state. It returns the Unix milliseconds of an ISO 8601 date (see
// ParseISOTime), or NULL when the value is not one. A zone of "" is
// time.Local; any other is an IANA name.
func init() {
	err := sqlite.RegisterDeterministicScalarFunction(timeFunc, 2,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			text, ok := args[0].(string)
			if !ok {
				return nil, nil
			}
			zone, _ := args[1].(string)
			loc := time.Local
			if zone != "" {
				var err error
				if loc, err = time.LoadLocation(zone); err != nil {
					return nil, fmt.Errorf("%s: %w", timeFunc, err)
				}
			}
			if ms, ok := ParseISOTime(text, loc); ok {
				return ms, nil
			}
			return nil, nil
		})
	if err != nil {
		panic("store: register " + timeFunc + ": " + err.Error())
	}
}

// ParseISOTime reads s as the dates a query compares (docs/specs/bases.md#values-and-types):
// YYYY-MM-DD, optionally followed by 'T' or a space and HH:MM, HH:MM:SS or
// HH:MM:SS.f (1 to 9 digits, cut to milliseconds), and then, only after a
// time, by 'Z' or +HH:MM or -HH:MM. Without a zone the time is in loc; a
// date alone is its midnight. It returns Unix milliseconds.
func ParseISOTime(s string, loc *time.Location) (int64, bool) {
	p := isoParser{s: s}
	year, ok := p.digits(4)
	if !ok || !p.lit('-') {
		return 0, false
	}
	month, ok := p.digits(2)
	if !ok || !p.lit('-') {
		return 0, false
	}
	day, ok := p.digits(2)
	if !ok {
		return 0, false
	}
	var hour, minute, second, nanos int
	zone := loc
	if !p.done() {
		if !p.lit('T') && !p.lit(' ') {
			return 0, false
		}
		if hour, ok = p.digits(2); !ok || !p.lit(':') {
			return 0, false
		}
		if minute, ok = p.digits(2); !ok {
			return 0, false
		}
		if p.lit(':') {
			if second, ok = p.digits(2); !ok {
				return 0, false
			}
			if p.lit('.') {
				if nanos, ok = p.fraction(); !ok {
					return 0, false
				}
			}
		}
		switch {
		case p.done():
		case p.lit('Z'):
			zone = time.UTC
		default:
			sign := 1
			if p.lit('-') {
				sign = -1
			} else if !p.lit('+') {
				return 0, false
			}
			zh, ok := p.digits(2)
			if !ok || !p.lit(':') {
				return 0, false
			}
			zm, ok := p.digits(2)
			if !ok || zh > 23 || zm > 59 {
				return 0, false
			}
			zone = time.FixedZone("", sign*(zh*3600+zm*60))
		}
		if !p.done() {
			return 0, false
		}
	}
	if month < 1 || month > 12 || day < 1 || hour > 23 || minute > 59 || second > 59 {
		return 0, false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, nanos, zone)
	if t.Day() != day || int(t.Month()) != month {
		return 0, false // a day the month does not have
	}
	return t.UnixMilli(), true
}

// isoParser reads fixed-width digits from a string.
type isoParser struct {
	s   string
	pos int
}

func (p *isoParser) done() bool { return p.pos == len(p.s) }

func (p *isoParser) lit(c byte) bool {
	if p.pos < len(p.s) && p.s[p.pos] == c {
		p.pos++
		return true
	}
	return false
}

func (p *isoParser) digits(n int) (int, bool) {
	if p.pos+n > len(p.s) {
		return 0, false
	}
	v := 0
	for _, c := range []byte(p.s[p.pos : p.pos+n]) {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	p.pos += n
	return v, true
}

// fraction reads 1 to 9 digits after a '.', as nanoseconds cut to a
// millisecond.
func (p *isoParser) fraction() (int, bool) {
	start := p.pos
	for p.pos < len(p.s) && p.s[p.pos] >= '0' && p.s[p.pos] <= '9' {
		p.pos++
	}
	n := p.pos - start
	if n < 1 || n > 9 {
		return 0, false
	}
	v := 0
	for i := range 9 {
		v *= 10
		if i < n {
			v += int(p.s[start+i] - '0')
		}
	}
	return v / 1_000_000 * 1_000_000, true
}
