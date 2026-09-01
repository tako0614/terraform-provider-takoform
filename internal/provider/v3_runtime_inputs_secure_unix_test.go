//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSecureReadV3RuntimeInputsFileRejectsFilesystemTricks(t *testing.T) {
	validBody := v3RuntimeInputsDocument(v3RuntimeInputsTestOrigin, v3RuntimeInputsTestNonce, v3RuntimeInputsTestSecret)

	tests := map[string]func(*testing.T) string{
		"relative path": func(t *testing.T) string {
			return "worker-runtime-inputs.json"
		},
		"non-clean path": func(t *testing.T) string {
			path := writeV3RuntimeInputsTestFile(t, validBody)
			return filepath.Dir(path) + "/missing/../" + filepath.Base(path)
		},
		"wrong mode": func(t *testing.T) string {
			path := writeV3RuntimeInputsTestFile(t, validBody)
			if err := os.Chmod(path, 0o640); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"special mode bits": func(t *testing.T) string {
			path := writeV3RuntimeInputsTestFile(t, validBody)
			if err := os.Chmod(path, 0o600|os.ModeSetuid); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"final symlink": func(t *testing.T) string {
			target := writeV3RuntimeInputsTestFile(t, validBody)
			link := filepath.Join(t.TempDir(), "runtime-input-link.json")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			return link
		},
		"symlink path component": func(t *testing.T) string {
			root := t.TempDir()
			realDirectory := filepath.Join(root, "real")
			if err := os.Mkdir(realDirectory, 0o700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(realDirectory, "runtime-inputs.json")
			if err := os.WriteFile(file, []byte(validBody), 0o600); err != nil {
				t.Fatal(err)
			}
			linkedDirectory := filepath.Join(root, "linked")
			if err := os.Symlink(realDirectory, linkedDirectory); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(linkedDirectory, filepath.Base(file))
		},
		"hard link": func(t *testing.T) string {
			target := writeV3RuntimeInputsTestFile(t, validBody)
			link := filepath.Join(filepath.Dir(target), "runtime-input-hardlink.json")
			if err := os.Link(target, link); err != nil {
				t.Fatal(err)
			}
			return link
		},
		"named pipe": func(t *testing.T) string {
			path := filepath.Join(t.TempDir(), "runtime-input-pipe")
			if err := unix.Mkfifo(path, 0o600); err != nil {
				t.Fatal(err)
			}
			return path
		},
		"empty file": func(t *testing.T) string {
			return writeV3RuntimeInputsTestFile(t, "")
		},
		"oversized file": func(t *testing.T) string {
			return writeV3RuntimeInputsTestFile(t, strings.Repeat("x", v3RuntimeInputsMaximumFileBytes+1))
		},
	}

	for name, create := range tests {
		t.Run(name, func(t *testing.T) {
			path := create(t)
			raw, err := secureReadV3RuntimeInputsFile(path)
			clearV3RuntimeInputBytes(raw)
			if err == nil {
				t.Fatal("insecure runtime-input file was accepted")
			}
			if strings.Contains(err.Error(), v3RuntimeInputsTestSecret) {
				t.Fatalf("runtime value leaked through filesystem error: %v", err)
			}
		})
	}
}

func TestSecureReadV3RuntimeInputsFileRejectsWrongOwnerWhenPrivileged(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("changing file ownership requires a privileged test process")
	}
	path := writeV3RuntimeInputsTestFile(t, v3RuntimeInputsDocument(
		v3RuntimeInputsTestOrigin,
		v3RuntimeInputsTestNonce,
		v3RuntimeInputsTestSecret,
	))
	if err := os.Chown(path, 1, -1); err != nil {
		t.Fatal(err)
	}
	raw, err := secureReadV3RuntimeInputsFile(path)
	clearV3RuntimeInputBytes(raw)
	if err == nil || !strings.Contains(err.Error(), "effective uid") {
		t.Fatalf("wrong-owner runtime input error = %v", err)
	}
}
