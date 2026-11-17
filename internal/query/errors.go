package query

import "fmt"

// Error is a base that notesd refuses: YAML that does not parse, a key or
// a statement outside the subset, or a filter that cannot be compiled
// exactly. The engine reports it as invalidParams.
type Error struct {
	Location string // such as views[0].filters.and[1]; empty for the document
	Expr     string // the statement, when the error is in one
	Msg      string
}

func (e *Error) Error() string {
	switch {
	case e.Location == "":
		return e.Msg
	case e.Expr == "":
		return e.Location + ": " + e.Msg
	}
	return fmt.Sprintf("%s: %q: %s", e.Location, e.Expr, e.Msg)
}

// errf is an error in a statement, before its location is known.
func errf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// at gives err a location, and the statement it is in.
func (e *Error) at(location, expr string) *Error {
	if e.Location == "" {
		e.Location = location
	}
	if e.Expr == "" {
		e.Expr = expr
	}
	return e
}
