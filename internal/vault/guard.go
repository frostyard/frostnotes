package vault

import (
	"context"
	"errors"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/frostyard/frostnotes/api"
)

// An agent's scope can limit its writes to folders (ADR-0028). The engine
// judges the paths a write names. Where the vault decides the path under
// its lock (the notes a move's link updates rewrite, the daily note), it
// asks the guard the engine put in the context, and refuses the whole
// call when the guard refuses one.

type guardKey struct{}

// WithWriteGuard returns ctx carrying guard, which the vault asks of each
// path it chooses to write.
func WithWriteGuard(ctx context.Context, guard func(path string) error) context.Context {
	return context.WithValue(ctx, guardKey{}, guard)
}

// checkGuard asks ctx's guard, if any, about p.
func checkGuard(ctx context.Context, p string) error {
	if guard, _ := ctx.Value(guardKey{}).(func(string) error); guard != nil {
		return guard(p)
	}
	return nil
}

// checkRewrites asks ctx's guard about every note plans would rewrite, at
// its path after the moves.
func checkRewrites(ctx context.Context, plans []rewritePlan, moves map[string]string) error {
	for _, plan := range plans {
		p := plan.source
		if to, ok := moves[p]; ok {
			p = to
		}
		if err := checkGuard(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// CheckSpelling refuses a path that reaches a file or folder on disk by a
// name its folder does not list, up to case and Unicode normalization
// (api.SameName). A file system can answer to such a name: an 8.3 short
// name on a share, a trailing dot it trims, a letter it folds otherwise.
// An agent's scope judges paths by name, so it must not meet an alias of
// a folder it may not write (ADR-0028).
func (v *Vault) CheckSpelling(p string) error {
	r, err := v.open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	err = checkSpelling(p, func(p string) error {
		_, err := r.Lstat(p)
		return err
	}, func(dir string) ([]string, error) {
		f, err := r.Open(dir)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		return f.Readdirnames(-1)
	})
	return apiError(p, err)
}

// checkSpelling is CheckSpelling over a file system's lstat and its
// folders' listings.
func checkSpelling(p string, lstat func(string) error, list func(dir string) ([]string, error)) error {
	dir := "."
	for seg := range strings.SplitSeq(p, "/") {
		at := path.Join(dir, seg)
		if err := lstat(at); errors.Is(err, fs.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		names, err := list(dir)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(names, func(n string) bool { return api.SameName(n, seg) }) {
			return api.InvalidParams("%s reaches %s by a name its folder does not list: spell it as the folder does", p, at)
		}
		dir = at
	}
	return nil
}
