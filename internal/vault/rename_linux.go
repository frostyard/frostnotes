package vault

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// renameNoReplace renames from to to under r, the vault at root, failing
// with an error matching fs.ErrExist when to exists: atomically with
// renameat2's RENAME_NOREPLACE where the file system has it, else after a
// check. Both paths passed the guard, under the vault's lock.
func renameNoReplace(r *os.Root, root, from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, filepath.Join(root, filepath.FromSlash(from)),
		unix.AT_FDCWD, filepath.Join(root, filepath.FromSlash(to)), unix.RENAME_NOREPLACE)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, unix.EEXIST):
		return fmt.Errorf("%s: %w", to, fs.ErrExist)
	case errors.Is(err, unix.EINVAL), errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EOPNOTSUPP):
		return checkedRename(r, from, to) // SMB, among others, has no RENAME_NOREPLACE
	}
	return err
}
