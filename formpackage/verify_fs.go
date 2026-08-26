package formpackage

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// VerifyFS verifies one complete Form Package rooted in an immutable fs.FS.
// It exists for build-time embedded package closures: callers do not need a
// repository-relative path, and successful verification still issues the same
// non-forgeable VerifiedPackage capability as VerifyDirectory.
//
// The fs.FS must remain immutable for the duration of this call. VerifyFS
// inventories the closure before and after bounded reads, rejects links,
// special or executable files and escaping paths, and stages only those copied
// bytes into a private temporary directory. VerifyDirectory then performs the
// one complete package/index/schema/fixture/content verification. The staging
// directory is removed before a capability is returned; a verification or
// cleanup failure returns a zero report and therefore no partial capability.
func VerifyFS(source fs.FS, root string) (VerificationReport, error) {
	if source == nil {
		return VerificationReport{}, fmt.Errorf("Form Package fs root: nil filesystem")
	}
	if root == "" || !fs.ValidPath(root) {
		return VerificationReport{}, fmt.Errorf("Form Package fs root %q is not a valid relative fs path", root)
	}
	packageFS, err := fs.Sub(source, root)
	if err != nil {
		return VerificationReport{}, fmt.Errorf("Form Package fs root %q: %w", root, err)
	}

	first, err := inventoryImmutableFS(packageFS)
	if err != nil {
		return VerificationReport{}, err
	}
	staging, err := os.MkdirTemp("", "takoform-package-verify-")
	if err != nil {
		return VerificationReport{}, fmt.Errorf("create private Form Package staging directory: %w", err)
	}
	removed := false
	defer func() {
		if !removed {
			_ = os.RemoveAll(staging)
		}
	}()

	paths := make([]string, 0, len(first))
	for relative := range first {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		limit := int64(maxPayloadBytes)
		if relative == PackageIndexFilename {
			limit = maxIndexBytes
		}
		raw, err := readImmutableFSFile(packageFS, relative, limit, first[relative])
		if err != nil {
			return VerificationReport{}, fmt.Errorf("read embedded package entry %q: %w", relative, err)
		}
		destination := filepath.Join(staging, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return VerificationReport{}, fmt.Errorf("create embedded package staging parent for %q: %w", relative, err)
		}
		handle, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return VerificationReport{}, fmt.Errorf("create embedded package staging file %q: %w", relative, err)
		}
		written, writeErr := handle.Write(raw)
		closeErr := handle.Close()
		if writeErr != nil {
			return VerificationReport{}, fmt.Errorf("stage embedded package entry %q: %w", relative, writeErr)
		}
		if written != len(raw) {
			return VerificationReport{}, fmt.Errorf("stage embedded package entry %q: wrote %d of %d bytes", relative, written, len(raw))
		}
		if closeErr != nil {
			return VerificationReport{}, fmt.Errorf("close embedded package staging file %q: %w", relative, closeErr)
		}
	}

	second, err := inventoryImmutableFS(packageFS)
	if err != nil {
		return VerificationReport{}, err
	}
	if err := compareImmutableFSInventories(first, second); err != nil {
		return VerificationReport{}, err
	}

	report, verifyErr := VerifyDirectory(staging)
	removeErr := os.RemoveAll(staging)
	removed = removeErr == nil
	if verifyErr != nil {
		return VerificationReport{}, verifyErr
	}
	if removeErr != nil {
		return VerificationReport{}, fmt.Errorf("remove private Form Package staging directory: %w", removeErr)
	}
	return report, nil
}

type immutableFSFile struct {
	mode    fs.FileMode
	size    int64
	modTime time.Time
}

func inventoryImmutableFS(source fs.FS) (map[string]immutableFSFile, error) {
	files := make(map[string]immutableFSFile)
	var total int64
	err := fs.WalkDir(source, ".", func(relative string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if relative == "." {
			return nil
		}
		if !fs.ValidPath(relative) {
			return fmt.Errorf("embedded package entry %q is not a valid relative fs path", relative)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("embedded package entry %q is a symlink", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("embedded package entry %q is a symlink", relative)
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("embedded package entry %q is not a regular file", relative)
		}
		if info.Mode().Perm()&0o111 != 0 {
			return fmt.Errorf("embedded package entry %q is executable", relative)
		}
		if err := validatePackagePath(relative); err != nil {
			return fmt.Errorf("embedded package entry %q: %w", relative, err)
		}
		if _, forbidden := executableExtensions[strings.ToLower(filepath.Ext(relative))]; forbidden {
			return fmt.Errorf("embedded package entry %q has executable-code extension", relative)
		}
		if _, duplicate := files[relative]; duplicate {
			return fmt.Errorf("embedded package entry %q appears more than once", relative)
		}
		if len(files) >= maxPackageFiles+1 {
			return fmt.Errorf("embedded package has more than %d files including %s", maxPackageFiles+1, PackageIndexFilename)
		}
		maximum := int64(maxPayloadBytes)
		if relative == PackageIndexFilename {
			maximum = maxIndexBytes
		}
		if info.Size() < 0 || info.Size() > maximum {
			return fmt.Errorf("embedded package entry %q size is outside 0..%d bytes", relative, maximum)
		}
		if total > int64(maxPackageBytes+maxIndexBytes)-info.Size() {
			return fmt.Errorf("embedded package closure exceeds %d bytes", maxPackageBytes+maxIndexBytes)
		}
		total += info.Size()
		files[relative] = immutableFSFile{mode: info.Mode(), size: info.Size(), modTime: info.ModTime()}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inventory embedded Form Package: %w", err)
	}
	if _, ok := files[PackageIndexFilename]; !ok {
		return nil, fmt.Errorf("embedded Form Package has no root %s", PackageIndexFilename)
	}
	return files, nil
}

func readImmutableFSFile(source fs.FS, relative string, maximum int64, inventoried immutableFSFile) ([]byte, error) {
	handle, err := source.Open(relative)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	before, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !sameImmutableFSFile(inventoried, before) || !before.Mode().IsRegular() || before.Mode().Perm()&0o111 != 0 {
		return nil, fmt.Errorf("file type, mode, size, or modification time changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(handle, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("file exceeds %d bytes while reading", maximum)
	}
	if int64(len(raw)) != inventoried.size {
		return nil, fmt.Errorf("file size changed while reading: read %d, inventoried %d", len(raw), inventoried.size)
	}
	after, err := handle.Stat()
	if err != nil {
		return nil, err
	}
	if !sameImmutableFSFile(inventoried, after) {
		return nil, fmt.Errorf("file type, mode, size, or modification time changed while reading")
	}
	return raw, nil
}

func sameImmutableFSFile(expected immutableFSFile, actual fs.FileInfo) bool {
	return expected.mode == actual.Mode() && expected.size == actual.Size() && expected.modTime.Equal(actual.ModTime())
}

func compareImmutableFSInventories(first, second map[string]immutableFSFile) error {
	if len(first) != len(second) {
		return fmt.Errorf("embedded Form Package closure changed during verification: %d files became %d", len(first), len(second))
	}
	for relative, expected := range first {
		actual, ok := second[relative]
		if !ok {
			return fmt.Errorf("embedded Form Package entry %q disappeared during verification", relative)
		}
		if actual != expected {
			return fmt.Errorf("embedded Form Package entry %q changed during verification", relative)
		}
	}
	return nil
}
