//go:build !linux

package vault

import "os"

// renameNoReplace renames from to to under r, failing with an error
// matching fs.ErrExist when to exists. Only Linux can refuse atomically;
// elsewhere it checks first.
func renameNoReplace(r *os.Root, _ string, from, to string) error { return checkedRename(r, from, to) }
