//go:build linux || darwin || freebsd || openbsd || netbsd

package intentv2

import (
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// readPrivateDescriptor reopens the current complete inode on every request.
// The Runner may atomically rename a new regular file over this path, but no
// symlink, multiply linked file, public mode or foreign owner is accepted.
func readPrivateDescriptor(path string, max int) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || max < 1 {
		return nil, ErrStoreUnavailable
	}
	dir, err := openSecureDirectory(filepath.Dir(path))
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	defer dir.close()
	fd, err := unix.Openat(dir.fd, filepath.Base(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrStoreUnavailable
	}
	file := os.NewFile(uintptr(fd), "private-run-files")
	defer file.Close()
	var before unix.Stat_t
	if err := unix.Fstat(fd, &before); err != nil || !privateFileStat(&before) || before.Size < 1 || before.Size > int64(max) {
		return nil, ErrStoreUnavailable
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(max)+1))
	if err != nil || len(data) < 1 || len(data) > max {
		clearDescriptorBytes(data)
		return nil, ErrStoreUnavailable
	}
	var after unix.Stat_t
	if err := unix.Fstat(fd, &after); err != nil || !privateFileStat(&after) || before.Dev != after.Dev ||
		before.Ino != after.Ino || before.Size != after.Size || int64(len(data)) != after.Size || dir.verifyPathIdentity() != nil {
		clearDescriptorBytes(data)
		return nil, ErrStoreUnavailable
	}
	return data, nil
}

func clearDescriptorBytes(data []byte) {
	for i := range data {
		data[i] = 0
	}
}
