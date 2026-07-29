//go:build linux

package main

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

func renameNoReplace(source, target string) error {
	sourceParent, err := unix.Open(
		filepath.Dir(source),
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return err
	}
	defer unix.Close(sourceParent)
	targetParent, err := unix.Open(
		filepath.Dir(target),
		unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW,
		0,
	)
	if err != nil {
		return err
	}
	defer unix.Close(targetParent)
	return unix.Renameat2(
		sourceParent,
		filepath.Base(source),
		targetParent,
		filepath.Base(target),
		unix.RENAME_NOREPLACE,
	)
}
