package secrets

import (
	"fmt"
	"io/fs"
)

// checkMode refuses a secrets file that group or other may read.
func checkMode(path string, fi fs.FileInfo) error {
	if fi.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("secrets: %s has mode 0%o, want 0600", path, fi.Mode().Perm())
	}
	return nil
}
