package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Scope is what an agent's token allows (Lodge-Scope; ADR-0026, ADR-0028):
// one vault, read whole, and the writes its parts allow.
//
//	vault=<id>[;write[=<prefix>[,<prefix>…]]][;except=<prefix>[,<prefix>…]][;daily]
//
// A prefix names a folder by its vault-relative path, ending in "/" and
// percent-encoded as a URL path segment is. A path is judged segment by
// segment, after NFC normalization and case folding, so that on a file
// system that ignores case no spelling escapes Except.
type Scope struct {
	Vault int64
	// Write is a bare ";write": the whole vault, but Except.
	Write bool
	// WriteTo limits writes to paths in these folders ("write=").
	WriteTo []string
	// Except takes paths in these folders out of every write: Write,
	// WriteTo and Daily alike. It always wins.
	Except []string
	// Daily allows appending to today's daily note, and nothing more.
	Daily bool
}

// The parts of a scope after the vault, in the order they must appear.
const (
	partWrite = iota + 1
	partExcept
	partDaily
)

// ParseScope parses a scope. Its parts appear at most once each and in
// order, so a scope reads one way; anything else is an error.
func ParseScope(s string) (Scope, error) {
	rest, ok := strings.CutPrefix(s, "vault=")
	if !ok {
		return Scope{}, fmt.Errorf("scope %q does not start with vault=", s)
	}
	parts := strings.Split(rest, ";")
	id := parts[0]
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 || strconv.FormatInt(n, 10) != id {
		return Scope{}, fmt.Errorf("scope %q: the vault is a positive decimal ID", s)
	}
	sc := Scope{Vault: n}
	last := 0
	for _, part := range parts[1:] {
		name, list, hasList := strings.Cut(part, "=")
		var rank int
		switch {
		case name == "write" && !hasList:
			rank, sc.Write = partWrite, true
		case name == "write":
			rank = partWrite
			if sc.WriteTo, err = parsePrefixes(list); err != nil {
				return Scope{}, fmt.Errorf("scope %q: write=: %w", s, err)
			}
		case name == "except" && hasList:
			rank = partExcept
			if sc.Except, err = parsePrefixes(list); err != nil {
				return Scope{}, fmt.Errorf("scope %q: except=: %w", s, err)
			}
		case name == "daily" && !hasList:
			rank, sc.Daily = partDaily, true
		default:
			return Scope{}, fmt.Errorf("scope %q: %q is not write, write=, except= or daily", s, part)
		}
		if rank <= last {
			return Scope{}, fmt.Errorf("scope %q: write, except and daily appear at most once each, in that order", s)
		}
		last = rank
	}
	if len(sc.Except) > 0 && !sc.CanWrite() && !sc.Daily {
		return Scope{}, fmt.Errorf("scope %q: except= takes from a write right, and there is none", s)
	}
	return sc, nil
}

// parsePrefixes parses a comma-separated list of percent-encoded folder
// prefixes, each ending in "/", into their decoded paths. Each is written
// in its one canonical form: its names in NFC, encoded as encodePrefixes
// encodes them.
func parsePrefixes(list string) ([]string, error) {
	if list == "" {
		return nil, fmt.Errorf("no folder: the whole vault is a bare ;write")
	}
	var out []string
	for raw := range strings.SplitSeq(list, ",") {
		if !strings.HasSuffix(raw, "/") || raw == "/" {
			return nil, fmt.Errorf("%q is not a folder ending in /", raw)
		}
		var segs []string
		for enc := range strings.SplitSeq(strings.TrimSuffix(raw, "/"), "/") {
			seg, err := url.PathUnescape(enc)
			if err != nil {
				return nil, fmt.Errorf("%q is not percent-encoded: %w", raw, err)
			}
			if err := checkSegment(seg); err != nil {
				return nil, fmt.Errorf("%q: %w", raw, err)
			}
			segs = append(segs, seg)
		}
		prefix := strings.Join(segs, "/") + "/"
		if canon := encodePrefixes([]string{prefix}); canon != raw {
			return nil, fmt.Errorf("%q is written %q", raw, canon)
		}
		out = append(out, prefix)
	}
	return out, nil
}

// checkSegment refuses what the vault's path guard refuses in a folder's
// name: nothing, ".", "..", a hidden name, a slash, a NUL or bytes that are
// not UTF-8; and a name not in NFC, which has another spelling.
func checkSegment(seg string) error {
	switch {
	case seg == "" || seg == "." || seg == "..":
		return fmt.Errorf("the segment %q names no folder", seg)
	case strings.HasPrefix(seg, "."):
		return fmt.Errorf("the segment %q is hidden", seg)
	case strings.ContainsAny(seg, "/\x00") || !utf8.ValidString(seg):
		return fmt.Errorf("the segment %q is not a folder's name", seg)
	case !norm.NFC.IsNormalString(seg):
		return fmt.Errorf("the segment %q is not in Unicode NFC", seg)
	}
	return nil
}

// String is the scope's canonical Lodge-Scope form.
func (s Scope) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "vault=%d", s.Vault)
	switch {
	case s.Write:
		b.WriteString(";write")
	case len(s.WriteTo) > 0:
		b.WriteString(";write=" + encodePrefixes(s.WriteTo))
	}
	if len(s.Except) > 0 {
		b.WriteString(";except=" + encodePrefixes(s.Except))
	}
	if s.Daily {
		b.WriteString(";daily")
	}
	return b.String()
}

func encodePrefixes(prefixes []string) string {
	out := make([]string, len(prefixes))
	for i, p := range prefixes {
		segs := strings.Split(strings.TrimSuffix(p, "/"), "/")
		for j, seg := range segs {
			segs[j] = url.PathEscape(seg)
		}
		out[i] = strings.Join(segs, "/") + "/"
	}
	return strings.Join(out, ",")
}

// CanWrite reports whether the scope may write beyond today's daily note:
// a bare write or a write= folder.
func (s Scope) CanWrite() bool { return s.Write || len(s.WriteTo) > 0 }

// Writable reports whether the scope may write the file at p: in a write=
// folder, or anywhere under a bare write, and in no except= folder.
func (s Scope) Writable(p string) bool {
	return !inAny(p, s.Except, true) && (s.Write || inAny(p, s.WriteTo, false))
}

// MayMakeFolder reports whether the scope may create folder p: as
// Writable, but a write= folder itself counts as within it.
func (s Scope) MayMakeFolder(p string) bool {
	p = strings.TrimSuffix(p, "/")
	return !inAny(p, s.Except, true) && (s.Write || inAny(p, s.WriteTo, true))
}

// WritableFolder reports whether the scope may move or delete folder p
// whole: p is in a write= folder, or anywhere under a bare write, and no
// except= folder is at, above or under p.
func (s Scope) WritableFolder(p string) bool {
	p = strings.TrimSuffix(p, "/")
	if !s.Writable(p) {
		return false
	}
	for _, e := range s.Except {
		if within(strings.TrimSuffix(e, "/"), p+"/", false) {
			return false
		}
	}
	return true
}

// Excepted reports whether p is in an except= folder, or is one.
func (s Scope) Excepted(p string) bool { return inAny(p, s.Except, true) }

// Describe says what the scope may write, for a refusal's message and
// the MCP tools' descriptions.
func (s Scope) Describe() string {
	except := ""
	if len(s.Except) > 0 {
		except = " except " + strings.Join(s.Except, ", ")
	}
	var what []string
	switch {
	case s.Write:
		what = append(what, "write anywhere in the vault"+except)
	case len(s.WriteTo) > 0:
		what = append(what, "write under "+strings.Join(s.WriteTo, ", ")+except)
	}
	if s.Daily {
		d := "append to today's daily note"
		if len(what) == 0 && len(s.Except) > 0 {
			d += " outside " + strings.Join(s.Except, ", ")
		}
		what = append(what, d)
	}
	if len(what) == 0 {
		return "this token writes nothing"
	}
	return "this token may " + strings.Join(what, ", and ")
}

// inAny reports whether p lies in one of the folder prefixes, or, with
// orEqual, is one of them.
func inAny(p string, prefixes []string, orEqual bool) bool {
	for _, prefix := range prefixes {
		if within(p, prefix, orEqual) {
			return true
		}
	}
	return false
}

// within reports whether path p lies in folder prefix (which ends in "/"),
// segment by segment after NFC normalization and case folding; with
// orEqual, p may also be the folder itself.
func within(p, prefix string, orEqual bool) bool {
	ps := strings.Split(strings.TrimSuffix(p, "/"), "/")
	fs := strings.Split(strings.TrimSuffix(prefix, "/"), "/")
	if len(ps) < len(fs) || len(ps) == len(fs) && !orEqual {
		return false
	}
	for i, f := range fs {
		if !SameName(ps[i], f) {
			return false
		}
	}
	return true
}

// SameName compares two names in a path as a scope does: after NFC
// normalization and case folding, as a file system that ignores case and
// normalizes Unicode would.
func SameName(a, b string) bool {
	return strings.EqualFold(norm.NFC.String(a), norm.NFC.String(b))
}

// boundScope returns ctx's agent scope, or nil when no scope binds ctx
// (a local or a session caller). An agent without a scope is bound to
// one that allows nothing.
func boundScope(ctx context.Context) *Scope {
	sc, bound := scoped(ctx)
	switch {
	case !bound:
		return nil
	case sc == nil:
		return &Scope{}
	}
	return sc
}

// outside is a refused write's answer: invalidParams, saying what the
// token may write, so an agent can correct itself (ADR-0028).
func outside(sc *Scope, what string) error {
	return InvalidParams("%s is outside this token's writes: %s", what, sc.Describe())
}

// CheckWrite refuses, for an agent, a write of any file at paths that its
// scope does not allow (ADR-0028). Other callers write anywhere.
func CheckWrite(ctx context.Context, paths ...string) error {
	sc := boundScope(ctx)
	if sc == nil {
		return nil
	}
	for _, p := range paths {
		if !sc.Writable(p) {
			return outside(sc, p)
		}
	}
	return nil
}

// CheckMakeFolder refuses, for an agent, creating folder p where its scope
// does not allow it.
func CheckMakeFolder(ctx context.Context, p string) error {
	if sc := boundScope(ctx); sc != nil && !sc.MayMakeFolder(p) {
		return outside(sc, p)
	}
	return nil
}

// CheckWriteFolder refuses, for an agent, moving or deleting folders its
// scope does not allow whole.
func CheckWriteFolder(ctx context.Context, folders ...string) error {
	sc := boundScope(ctx)
	if sc == nil {
		return nil
	}
	for _, f := range folders {
		if !sc.WritableFolder(f) {
			return outside(sc, "the folder "+strings.TrimSuffix(f, "/"))
		}
	}
	return nil
}

// CheckDaily refuses, for an agent, writing the daily note at p unless its
// scope may write p, or allows daily and today is true; except= wins.
func CheckDaily(ctx context.Context, p string, today bool) error {
	sc := boundScope(ctx)
	if sc == nil || sc.Writable(p) || sc.Daily && today && !sc.Excepted(p) {
		return nil
	}
	if sc.Daily && !today && !sc.Excepted(p) {
		return InvalidParams("this token appends to today's daily note only")
	}
	return outside(sc, p)
}

// Scoped reports whether a scope binds ctx's calls: an agent's.
func Scoped(ctx context.Context) bool {
	_, bound := scoped(ctx)
	return bound
}
