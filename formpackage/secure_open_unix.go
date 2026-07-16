//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package formpackage

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func secureOpenPackageRoot(root string) (*os.File, error) {
	descriptor, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), root), nil
}

// secureOpenRelative resolves every component beneath the held package root.
// It never re-enters the root through a pathname and never follows a symlink.
func secureOpenRelative(rootHandle *os.File, _ string, relative string) (*os.File, error) {
	if err := validatePackagePath(relative); err != nil {
		return nil, err
	}
	current, err := unix.Dup(int(rootHandle.Fd()))
	if err != nil {
		return nil, fmt.Errorf("duplicate package root descriptor: %w", err)
	}
	unix.CloseOnExec(current)
	defer func() {
		if current >= 0 {
			_ = unix.Close(current)
		}
	}()

	components := strings.Split(relative, "/")
	for _, component := range components[:len(components)-1] {
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return nil, fmt.Errorf("open package directory component %q: %w", component, openErr)
		}
		_ = unix.Close(current)
		current = next
	}

	name := components[len(components)-1]
	descriptor, err := unix.Openat(current, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, fmt.Errorf("open package file %q: %w", relative, err)
	}
	return os.NewFile(uintptr(descriptor), relative), nil
}
