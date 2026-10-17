package synctest

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Reduce shrinks a failing plan by dropping steps while it still fails
// (ADR-0020): halves, then quarters, down to single steps, stepwise, at
// most budget runs. It returns the smallest failing plan it found and its
// failure.
func Reduce(t testing.TB, o Options, plan []Step, budget int) ([]Step, error) {
	t.Helper()
	o.Concurrent = false
	failure := Run(t, o, plan)
	if failure == nil {
		return plan, nil
	}
	runs := 1
	for chunk := len(plan) / 2; chunk >= 1 && runs < budget; chunk /= 2 {
		for start := 0; start < len(plan) && runs < budget; {
			try := append(append([]Step(nil), plan[:start]...), plan[min(start+chunk, len(plan)):]...)
			runs++
			if err := Run(t, o, try); err != nil {
				plan, failure = try, err
				continue // the same start now holds the next steps
			}
			start += chunk
		}
	}
	return plan, failure
}

// PlanText renders a plan as testdata/regressions lines read it back.
func PlanText(plan []Step) string {
	out := ""
	for _, s := range plan {
		out += s.Line() + "\n"
	}
	return out
}

// Line is a step as one line of a regression file.
func (s Step) Line() string {
	return opNames[s.Op] + "|" + itoa(s.Device) + "|" + utoa(s.A) + "|" + utoa(s.B)
}

func itoa(n int) string    { return strconv.Itoa(n) }
func utoa(n uint64) string { return strconv.FormatUint(n, 10) }

// ParsePlan reads a plan PlanText wrote; lines starting with # are
// comments.
func ParsePlan(text string) ([]Step, error) {
	var plan []Step
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "|")
		if len(f) != 4 {
			return nil, fmt.Errorf("line %d: %q is not op|device|a|b", n+1, line)
		}
		op := slices.Index(opNames[:], f[0])
		d, err1 := strconv.Atoi(f[1])
		a, err2 := strconv.ParseUint(f[2], 10, 64)
		b, err3 := strconv.ParseUint(f[3], 10, 64)
		if op < 0 || err1 != nil || err2 != nil || err3 != nil {
			return nil, fmt.Errorf("line %d: %q is not op|device|a|b", n+1, line)
		}
		plan = append(plan, Step{Op: Op(op), Device: d, A: a, B: b})
	}
	return plan, nil
}
