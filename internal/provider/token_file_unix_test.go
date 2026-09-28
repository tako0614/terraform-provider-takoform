//go:build linux || darwin || freebsd || openbsd || netbsd

package provider

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestBearerTokenFileSecurityAndGrammar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "current")
	write := func(content string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
	}
	write("first.token-1", 0600)
	if got, err := readBearerTokenFile(path); err != nil || got != "first.token-1" {
		t.Fatalf("secure token read: token=%q err=%v", got, err)
	}
	replacement := filepath.Join(dir, "replacement")
	if err := os.WriteFile(replacement, []byte("second.token-2"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if got, err := readBearerTokenFile(path); err != nil || got != "second.token-2" {
		t.Fatalf("atomic replacement not observed: token=%q err=%v", got, err)
	}

	for _, test := range []struct {
		name    string
		content string
		mode    os.FileMode
	}{
		{"empty", "", 0600},
		{"newline", "token\n", 0600},
		{"control", "token\x00", 0600},
		{"json", `{"token":"secret"}`, 0600},
		{"whitespace", "token value", 0600},
		{"oversize", strings.Repeat("a", maxBearerTokenFileBytes+1), 0600},
		{"group readable", "secret", 0640},
		{"other writable", "secret", 0602},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(test.content, test.mode)
			if token, err := readBearerTokenFile(path); err == nil || token != "" {
				t.Fatalf("insecure/malformed file accepted: token=%q err=%v", token, err)
			}
		})
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(replacement, path); err != nil {
		t.Fatal(err)
	}
	if token, err := readBearerTokenFile(path); err == nil || token != "" {
		t.Fatalf("symlink accepted: token=%q err=%v", token, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if token, err := readBearerTokenFile(path); err == nil || token != "" {
		t.Fatalf("fifo accepted: token=%q err=%v", token, err)
	}
	secureDir := filepath.Join(dir, "secure")
	if err := os.Mkdir(secureDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secureDir, "token"), []byte("hidden"), 0600); err != nil {
		t.Fatal(err)
	}
	linkedDir := filepath.Join(dir, "linked")
	if err := os.Symlink(secureDir, linkedDir); err != nil {
		t.Fatal(err)
	}
	if token, err := readBearerTokenFile(filepath.Join(linkedDir, "token")); err == nil || token != "" {
		t.Fatalf("parent symlink accepted: token=%q err=%v", token, err)
	}
}

func TestBearerTokenFileRejectsParentTraversalBeforeClean(t *testing.T) {
	dir := t.TempDir()
	outer := filepath.Join(dir, "outer")
	target := filepath.Join(dir, "target")
	if err := os.MkdirAll(filepath.Join(target, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(outer, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "token"), []byte("wrong.principal"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "token"), []byte("intended.principal"), 0600); err != nil {
		t.Fatal(err)
	}
	if token, err := readBearerTokenFile(outer + "/./token"); err != nil || token != "wrong.principal" {
		t.Fatalf("ordinary dot path was rejected: token=%q err=%v", token, err)
	}
	if err := os.Symlink(filepath.Join("..", "target", "child"), filepath.Join(outer, "link")); err != nil {
		t.Fatal(err)
	}
	path := outer + "/link/../token"
	// The raw path resolves through the symlink to target/token, but lexical
	// cleaning erases link/.. and selects outer/token instead. Refuse this path
	// before resolving either credential identity.
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "intended.principal" {
		t.Fatalf("fixture did not resolve through the symlink: %v", err)
	}
	if token, err := readBearerTokenFile(path); err == nil || token != "" {
		t.Fatalf("parent traversal silently selected another credential: token=%q err=%v", token, err)
	}
}
