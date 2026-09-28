//go:build linux || darwin || freebsd || openbsd || netbsd

package provider

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const maxBearerTokenFileBytes = 8192

var errBearerTokenFile = errors.New("takoform: bearer token file unavailable or insecure")

// readBearerTokenFile opens the current inode on each request. The runner may
// atomically rename a new 0600 regular file over the same path at any time;
// the old or new complete token is read, never a partially written value.
// No path, file content, or OS error is returned to diagnostics.
func readBearerTokenFile(path string) (string, error) {
	fd, err := openBearerTokenNoSymlinks(path)
	if err != nil {
		return "", errBearerTokenFile
	}
	file := os.NewFile(uintptr(fd), "bearer-token")
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		stat.Uid != uint32(os.Getuid()) || stat.Mode&0077 != 0 || stat.Size < 1 || stat.Size > maxBearerTokenFileBytes {
		return "", errBearerTokenFile
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBearerTokenFileBytes+1))
	if err != nil || len(raw) < 1 || len(raw) > maxBearerTokenFileBytes || !validBearerTokenBytes(raw) {
		clearBearerTokenBytes(raw)
		return "", errBearerTokenFile
	}
	token := string(raw)
	clearBearerTokenBytes(raw)
	return token, nil
}

// Walk under directory descriptors so O_NOFOLLOW covers every path component,
// not just the final token file. A concurrent rename cannot switch an already
// opened parent to a different path while the walk continues.
func openBearerTokenNoSymlinks(path string) (int, error) {
	if path == "" {
		return -1, errBearerTokenFile
	}
	// Clean is lexical: link/.. can refer to a different directory after the
	// kernel resolves link. Reject parent traversal in the raw spelling before
	// Clean can erase a symlink component and silently select another token.
	for _, part := range strings.Split(path, string(os.PathSeparator)) {
		if part == ".." {
			return -1, errBearerTokenFile
		}
	}
	clean := filepath.Clean(path)
	base := "."
	if filepath.IsAbs(clean) {
		base = string(os.PathSeparator)
		clean = strings.TrimPrefix(clean, base)
	}
	parts := strings.Split(clean, string(os.PathSeparator))
	parent, err := unix.Open(base, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	defer func() { _ = unix.Close(parent) }()
	for i, part := range parts {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
		if i < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		child, openErr := unix.Openat(parent, part, flags, 0)
		if openErr != nil {
			return -1, openErr
		}
		if i == len(parts)-1 {
			return child, nil
		}
		_ = unix.Close(parent)
		parent = child
	}
	return -1, errBearerTokenFile
}

// RFC 6750 b64token, with any '=' padding only at the end. Whitespace,
// controls, non-ASCII bytes, and JSON envelopes are intentionally invalid.
func validBearerTokenBytes(raw []byte) bool {
	padding := false
	for _, char := range raw {
		if char == '=' {
			padding = true
			continue
		}
		if padding || !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '.' || char == '_' ||
			char == '~' || char == '+' || char == '/') {
			return false
		}
	}
	return raw[0] != '='
}

func clearBearerTokenBytes(raw []byte) {
	for i := range raw {
		raw[i] = 0
	}
}
