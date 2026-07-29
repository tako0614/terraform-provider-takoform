//go:build !linux

package main

import (
	"errors"
)

func renameNoReplace(_, _ string) error {
	return errors.New("atomic no-replace output commit requires Linux renameat2(RENAME_NOREPLACE)")
}
