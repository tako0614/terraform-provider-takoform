//go:build linux || darwin || freebsd || openbsd || netbsd

package intentv2

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const storeLockWait = 5 * time.Second

type secureDirectory struct {
	fd     int
	path   string
	device uint64
	inode  uint64
}

func createSecureDirectory(path string) (*secureDirectory, error) {
	if err := validateCanonicalPath(path); err != nil {
		return nil, err
	}
	parentPath, leaf := filepath.Dir(path), filepath.Base(path)
	parent, err := openDirectoryPath(parentPath)
	if err != nil {
		return nil, err
	}
	defer parent.close()
	if err := unix.Mkdirat(parent.fd, leaf, 0700); err != nil {
		return nil, err
	}
	if err := unix.Fsync(parent.fd); err != nil {
		return nil, err
	}
	dir, err := openSecureDirectory(path)
	if err != nil {
		return nil, err
	}
	return dir, nil
}

func openSecureDirectory(path string) (*secureDirectory, error) {
	if err := validateCanonicalPath(path); err != nil {
		return nil, err
	}
	fd, err := openDirectoryFD(path)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || !privateDirectoryStat(&stat) {
		_ = unix.Close(fd)
		return nil, ErrStoreUnavailable
	}
	return &secureDirectory{fd: fd, path: path, device: uint64(stat.Dev), inode: stat.Ino}, nil
}

func validateCanonicalPath(path string) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrStoreUnavailable
	}
	for _, part := range strings.Split(path, string(os.PathSeparator)) {
		if part == ".." {
			return ErrStoreUnavailable
		}
	}
	return nil
}

func openDirectoryPath(path string) (*secureDirectory, error) {
	if err := validateCanonicalPath(path); err != nil {
		return nil, err
	}
	fd, err := openDirectoryFD(path)
	if err != nil {
		return nil, err
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil || stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		_ = unix.Close(fd)
		return nil, ErrStoreUnavailable
	}
	return &secureDirectory{fd: fd, path: path, device: uint64(stat.Dev), inode: stat.Ino}, nil
}

func openDirectoryFD(path string) (int, error) {
	fd, err := unix.Open(string(os.PathSeparator), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return -1, err
	}
	if path == string(os.PathSeparator) {
		return fd, nil
	}
	parts := strings.Split(strings.TrimPrefix(path, string(os.PathSeparator)), string(os.PathSeparator))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			_ = unix.Close(fd)
			return -1, ErrStoreUnavailable
		}
		child, openErr := unix.Openat(fd, part, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if openErr != nil {
			_ = unix.Close(fd)
			return -1, openErr
		}
		_ = unix.Close(fd)
		fd = child
	}
	return fd, nil
}

func privateDirectoryStat(stat *unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFDIR && stat.Uid == uint32(os.Getuid()) && stat.Mode&0077 == 0
}

func privateFileStat(stat *unix.Stat_t) bool {
	return stat.Mode&unix.S_IFMT == unix.S_IFREG && stat.Uid == uint32(os.Getuid()) && stat.Mode&0077 == 0 && stat.Nlink == 1
}

func (d *secureDirectory) close() error {
	if d == nil || d.fd < 0 {
		return nil
	}
	err := unix.Close(d.fd)
	d.fd = -1
	return err
}

func (d *secureDirectory) verifyPathIdentity() error {
	current, err := openSecureDirectory(d.path)
	if err != nil {
		return err
	}
	defer current.close()
	if current.device != d.device || current.inode != d.inode {
		return ErrStoreUnavailable
	}
	return nil
}

func (d *secureDirectory) validateStoreEntries() error {
	fd, err := unix.Openat(d.fd, ".", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(fd), d.path)
	entries, err := directory.ReadDir(-1)
	closeErr := directory.Close()
	if err != nil || closeErr != nil || len(entries) != 3 {
		return ErrStoreUnavailable
	}
	want := map[string]bool{"lock": true, "store.json": true, "state.json": true}
	for _, entry := range entries {
		if !want[entry.Name()] {
			return ErrStoreUnavailable
		}
		delete(want, entry.Name())
		var stat unix.Stat_t
		if err := unix.Fstatat(d.fd, entry.Name(), &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil || !privateFileStat(&stat) {
			return ErrStoreUnavailable
		}
	}
	if len(want) != 0 {
		return ErrStoreUnavailable
	}
	return nil
}

func (d *secureDirectory) initializeFiles(header, snapshot []byte) error {
	for name, contents := range map[string][]byte{"lock": {}, "store.json": header, "state.json": snapshot} {
		fd, err := unix.Openat(d.fd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
		if err != nil {
			return err
		}
		file := os.NewFile(uintptr(fd), name)
		if len(contents) > 0 {
			if err := writeAll(file, contents); err != nil {
				_ = file.Close()
				return err
			}
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	return unix.Fsync(d.fd)
}

func (d *secureDirectory) readFile(name string, max int) ([]byte, error) {
	if name != "store.json" && name != "state.json" {
		return nil, ErrStoreUnavailable
	}
	fd, err := unix.Openat(d.fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || !privateFileStat(&before) || before.Size < 0 || before.Size > int64(max) {
		return nil, ErrStoreUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(max)+1))
	if err != nil || len(data) > max {
		return nil, ErrStoreUnavailable
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil || !privateFileStat(&after) || before.Dev != after.Dev || before.Ino != after.Ino || before.Size != after.Size || int64(len(data)) != after.Size {
		return nil, ErrStoreUnavailable
	}
	return data, nil
}

func (d *secureDirectory) replaceFile(name string, contents []byte) error {
	if name != "state.json" || len(contents) > maxSnapshotBytes {
		return ErrStoreUnavailable
	}
	var existing unix.Stat_t
	if err := unix.Fstatat(d.fd, name, &existing, unix.AT_SYMLINK_NOFOLLOW); err != nil || !privateFileStat(&existing) {
		return ErrStoreUnavailable
	}
	fd, err := unix.Openat(d.fd, ".state.tmp", unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return ErrStoreUnavailable
	}
	file := os.NewFile(uintptr(fd), ".state.tmp")
	if err := writeAll(file, contents); err != nil {
		_ = file.Close()
		return ErrStoreUnavailable
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return ErrStoreUnavailable
	}
	if err := file.Close(); err != nil {
		return ErrStoreUnavailable
	}
	var temp unix.Stat_t
	if err := unix.Fstatat(d.fd, ".state.tmp", &temp, unix.AT_SYMLINK_NOFOLLOW); err != nil || !privateFileStat(&temp) {
		return ErrStoreUnavailable
	}
	if err := unix.Renameat(d.fd, ".state.tmp", d.fd, name); err != nil {
		return ErrStoreUnavailable
	}
	if err := unix.Fsync(d.fd); err != nil {
		return ErrStoreUnavailable
	}
	return nil
}

func (d *secureDirectory) withLock(ctx context.Context, action func() error) error {
	fd, err := unix.Openat(d.fd, "lock", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return ErrStoreUnavailable
	}
	defer unix.Close(fd)
	var lockStat unix.Stat_t
	if err := unix.Fstat(fd, &lockStat); err != nil || !privateFileStat(&lockStat) {
		return ErrStoreUnavailable
	}
	deadline := time.Now().Add(storeLockWait)
	for {
		if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err == nil {
			break
		} else if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EINTR) {
			return ErrStoreUnavailable
		}
		if err := ctx.Err(); err != nil {
			return ErrStoreLocked
		}
		if time.Now().After(deadline) {
			return ErrStoreLocked
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer unix.Flock(fd, unix.LOCK_UN)
	var currentLock unix.Stat_t
	if err := unix.Fstatat(d.fd, "lock", &currentLock, unix.AT_SYMLINK_NOFOLLOW); err != nil || !privateFileStat(&currentLock) || currentLock.Dev != lockStat.Dev || currentLock.Ino != lockStat.Ino {
		return ErrStoreUnavailable
	}
	if err := d.verifyPathIdentity(); err != nil {
		return ErrStoreUnavailable
	}
	return action()
}

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		n, err := file.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
