package query

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// kind is the kind of a constant.
type kind int

const (
	kNum kind = iota
	kStr
	kBool
	kDate
	kList
)

func (k kind) String() string {
	return [...]string{"a number", "a string", "a boolean", "a date", "a list"}[k]
}

// constant is a value known before the query runs: a literal, a date that
// date(), today() or now() gives, or a property of the this file.
type constant struct {
	k      kind
	num    float64
	numArg any // a number's SQL argument: an int64 when the literal is a whole number, else its float64
	str    string
	b      bool
	ms     int64    // a date, as Unix milliseconds
	list   []string // a list of text items
}

// durationUnits are the units of a duration string
// (docs/specs/bases.md#values-and-types).
var durationUnits = map[string]byte{
	"y": 'y', "year": 'y', "years": 'y',
	"M": 'M', "month": 'M', "months": 'M',
	"w": 'w', "week": 'w', "weeks": 'w',
	"d": 'd', "day": 'd', "days": 'd',
	"h": 'h', "hour": 'h', "hours": 'h',
	"m": 'm', "minute": 'm', "minutes": 'm',
	"s": 's', "second": 's', "seconds": 's',
}

// addDuration adds the duration text ("1 week", "3d") to t, subtracting
// it when negate is set.
func addDuration(t time.Time, text string, negate bool) (time.Time, error) {
	s := strings.TrimSpace(text)
	i := digitsLen(s, 0)
	if i == 0 || i > 6 {
		return t, fmt.Errorf("duration %q: want a whole number of 1 to 6 digits and a unit, such as \"1 week\"", text)
	}
	n, _ := strconv.Atoi(s[:i])
	unit, ok := durationUnits[strings.TrimSpace(s[i:])]
	if !ok {
		return t, fmt.Errorf("duration %q: unknown unit %q", text, strings.TrimSpace(s[i:]))
	}
	if negate {
		n = -n
	}
	switch unit {
	case 'y':
		return t.AddDate(n, 0, 0), nil
	case 'M':
		return t.AddDate(0, n, 0), nil
	case 'w':
		return t.AddDate(0, 0, 7*n), nil
	case 'd':
		return t.AddDate(0, 0, n), nil
	case 'h':
		return t.Add(time.Duration(n) * time.Hour), nil
	case 'm':
		return t.Add(time.Duration(n) * time.Minute), nil
	}
	return t.Add(time.Duration(n) * time.Second), nil
}
