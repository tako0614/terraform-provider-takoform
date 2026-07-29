package providerlifecycle

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// copyProviderBinary preserves the exact bytes of a reviewed release archive
// while keeping Terraform's dev_overrides lookup inside the runner's isolated
// temporary directory.
func copyProviderBinary(sourcePath, destinationPath string) error {
	if !filepath.IsAbs(sourcePath) {
		return fmt.Errorf("provider binary path must be absolute: %q", sourcePath)
	}
	sourceInfo, err := os.Lstat(sourcePath)
	if err != nil {
		return fmt.Errorf("inspect provider binary: %w", err)
	}
	if !sourceInfo.Mode().IsRegular() {
		return fmt.Errorf("provider binary must be a regular file: %q", sourcePath)
	}
	if sourceInfo.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("provider binary must be executable: %q", sourcePath)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open provider binary: %w", err)
	}
	defer source.Close()
	openedInfo, err := source.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened provider binary: %w", err)
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(sourceInfo, openedInfo) {
		return fmt.Errorf("provider binary changed while opening: %q", sourcePath)
	}

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return fmt.Errorf("create isolated provider directory: %w", err)
	}
	destination, err := os.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		return fmt.Errorf("create isolated provider binary: %w", err)
	}
	copied := false
	defer func() {
		_ = destination.Close()
		if !copied {
			_ = os.Remove(destinationPath)
		}
	}()
	if _, err := io.Copy(destination, source); err != nil {
		return fmt.Errorf("copy provider binary: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close isolated provider binary: %w", err)
	}
	copied = true
	return nil
}

func verifyProviderBinaryUnchanged(path, expectedSHA256 string) error {
	actualSHA256, err := fileSHA256(path)
	if err != nil {
		return fmt.Errorf("re-read provider binary after lifecycle: %w", err)
	}
	if actualSHA256 != expectedSHA256 {
		return fmt.Errorf("provider binary bytes changed during lifecycle: got %s, want %s", actualSHA256, expectedSHA256)
	}
	return nil
}
