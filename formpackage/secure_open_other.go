//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package formpackage

import (
	"os"
	"path/filepath"
)

func secureOpenPackageRoot(root string) (*os.File, error) {
	return os.Open(root)
}

// Non-Unix callers must provide an immutable staging tree for the complete
// verification. The common path and metadata checks remain defense in depth.
func secureOpenRelative(_ *os.File, root, relative string) (*os.File, error) {
	if err := validatePackagePath(relative); err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(root, filepath.FromSlash(relative)))
}
