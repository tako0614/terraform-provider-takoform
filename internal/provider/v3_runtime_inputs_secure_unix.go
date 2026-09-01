//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package provider

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func secureReadV3RuntimeInputsFile(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("path must be a clean absolute path")
	}
	components := strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator))
	if len(components) == 0 {
		return nil, errors.New("path must name a file")
	}
	for _, component := range components {
		if component == "" || component == "." || component == ".." {
			return nil, errors.New("path contains an invalid component")
		}
	}

	current, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open filesystem root: %w", err)
	}
	defer func() {
		if current >= 0 {
			_ = unix.Close(current)
		}
	}()
	for _, component := range components[:len(components)-1] {
		next, openErr := unix.Openat(current, component, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			return nil, fmt.Errorf("open path component without following symlinks: %w", openErr)
		}
		_ = unix.Close(current)
		current = next
	}

	descriptor, err := unix.Openat(
		current,
		components[len(components)-1],
		unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("open file without following symlinks: %w", err)
	}
	handle := os.NewFile(uintptr(descriptor), path)
	if handle == nil {
		_ = unix.Close(descriptor)
		return nil, errors.New("open file returned no handle")
	}
	defer handle.Close()

	beforeInfo, err := handle.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	var before unix.Stat_t
	if err := unix.Fstat(descriptor, &before); err != nil {
		return nil, fmt.Errorf("stat file descriptor: %w", err)
	}
	if !beforeInfo.Mode().IsRegular() {
		return nil, errors.New("file must be regular")
	}
	if beforeInfo.Mode().Perm() != 0o600 || beforeInfo.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, errors.New("file mode must be exactly 0600")
	}
	if before.Uid != uint32(os.Geteuid()) {
		return nil, errors.New("file must be owned by the provider process effective uid")
	}
	if before.Nlink != 1 {
		return nil, errors.New("file must have exactly one hard link")
	}
	if beforeInfo.Size() < 1 || beforeInfo.Size() > v3RuntimeInputsMaximumFileBytes {
		return nil, fmt.Errorf("file size must be in 1..%d bytes", v3RuntimeInputsMaximumFileBytes)
	}

	first, err := io.ReadAll(io.LimitReader(handle, v3RuntimeInputsMaximumFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if len(first) != int(beforeInfo.Size()) {
		clearV3RuntimeInputBytes(first)
		return nil, errors.New("file changed while it was being read")
	}
	if _, err := handle.Seek(0, io.SeekStart); err != nil {
		clearV3RuntimeInputBytes(first)
		return nil, fmt.Errorf("rewind file: %w", err)
	}
	second, err := io.ReadAll(io.LimitReader(handle, v3RuntimeInputsMaximumFileBytes+1))
	if err != nil {
		clearV3RuntimeInputBytes(first)
		return nil, fmt.Errorf("re-read file: %w", err)
	}
	defer clearV3RuntimeInputBytes(second)
	afterInfo, err := handle.Stat()
	if err != nil {
		clearV3RuntimeInputBytes(first)
		return nil, fmt.Errorf("re-stat file: %w", err)
	}
	var after unix.Stat_t
	if err := unix.Fstat(descriptor, &after); err != nil {
		clearV3RuntimeInputBytes(first)
		return nil, fmt.Errorf("re-stat file descriptor: %w", err)
	}
	if !os.SameFile(beforeInfo, afterInfo) || beforeInfo.Mode() != afterInfo.Mode() ||
		beforeInfo.Size() != afterInfo.Size() || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) ||
		before.Uid != after.Uid || before.Nlink != after.Nlink || !bytes.Equal(first, second) {
		clearV3RuntimeInputBytes(first)
		return nil, errors.New("file changed while it was being read")
	}
	return first, nil
}
