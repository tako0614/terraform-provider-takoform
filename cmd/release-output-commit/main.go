package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const protocol = "takoform.release-output-commit@v1"

type result struct {
	Format string `json:"format"`
	Status string `json:"status"`
}

var renameNoReplaceOperation = renameNoReplace

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(arguments []string, output io.Writer) error {
	if len(arguments) == 0 {
		return errors.New("usage: release-output-commit <probe|commit> [exact options]")
	}
	switch arguments[0] {
	case "probe":
		flags := flag.NewFlagSet("probe", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		parent := flags.String("parent", "", "existing same-filesystem probe parent")
		if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
			return errors.New("usage: release-output-commit probe --parent <absolute-directory>")
		}
		if err := probe(*parent); err != nil {
			return err
		}
	case "commit":
		flags := flag.NewFlagSet("commit", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		source := flags.String("source", "", "existing source directory")
		target := flags.String("target", "", "absent destination path")
		if err := flags.Parse(arguments[1:]); err != nil || flags.NArg() != 0 {
			return errors.New("usage: release-output-commit commit --source <absolute-directory> --target <absolute-path>")
		}
		if err := commit(*source, *target); err != nil {
			return err
		}
	default:
		return errors.New("usage: release-output-commit <probe|commit> [exact options]")
	}
	encoder := json.NewEncoder(output)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(result{Format: protocol, Status: "verified"})
}

func probe(parent string) error {
	parent, err := exactDirectory(parent, "probe parent")
	if err != nil {
		return err
	}
	root, err := os.MkdirTemp(parent, ".takoform-renameat2-probe-")
	if err != nil {
		return fmt.Errorf("create atomic no-replace probe: %w", err)
	}
	defer os.RemoveAll(root)

	movable := filepath.Join(root, "movable")
	moved := filepath.Join(root, "moved")
	if err := os.Mkdir(movable, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(movable, "marker"), []byte("move\n"), 0o600); err != nil {
		return err
	}
	if err := renameNoReplaceOperation(movable, moved); err != nil {
		return fmt.Errorf("atomic no-replace success probe: %w", err)
	}
	if _, err := os.Lstat(movable); !errors.Is(err, fs.ErrNotExist) {
		return errors.New("atomic no-replace success probe retained its source")
	}
	if raw, err := os.ReadFile(filepath.Join(moved, "marker")); err != nil || string(raw) != "move\n" {
		return errors.New("atomic no-replace success probe changed destination bytes")
	}

	blockedSource := filepath.Join(root, "blocked-source")
	existingTarget := filepath.Join(root, "existing-target")
	if err := os.Mkdir(blockedSource, 0o700); err != nil {
		return err
	}
	if err := os.Mkdir(existingTarget, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(blockedSource, "marker"), []byte("source\n"), 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(existingTarget, "marker"), []byte("target\n"), 0o600); err != nil {
		return err
	}
	if err := renameNoReplaceOperation(blockedSource, existingTarget); !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("atomic no-replace collision probe did not return EEXIST: %w", err)
	}
	if raw, err := os.ReadFile(filepath.Join(blockedSource, "marker")); err != nil || string(raw) != "source\n" {
		return errors.New("atomic no-replace collision probe changed its source")
	}
	if raw, err := os.ReadFile(filepath.Join(existingTarget, "marker")); err != nil || string(raw) != "target\n" {
		return errors.New("atomic no-replace collision probe changed its target")
	}
	return nil
}

func commit(source, target string) error {
	source, err := exactDirectory(source, "source")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) || filepath.Clean(target) != target || source == target {
		return errors.New("target must be a distinct clean absolute path")
	}
	if _, err := exactDirectory(filepath.Dir(target), "target parent"); err != nil {
		return err
	}
	if err := renameNoReplaceOperation(source, target); err != nil {
		return fmt.Errorf("atomic create-only output commit: %w", err)
	}
	if _, err := os.Lstat(source); !errors.Is(err, fs.ErrNotExist) {
		return errors.New("atomic output commit retained its source")
	}
	if _, err := exactDirectory(target, "committed target"); err != nil {
		return err
	}
	return nil
}

func exactDirectory(name, label string) (string, error) {
	if !filepath.IsAbs(name) || filepath.Clean(name) != name {
		return "", fmt.Errorf("%s must be a clean absolute path", label)
	}
	info, err := os.Lstat(name)
	if err != nil {
		return "", fmt.Errorf("inspect %s: %w", label, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%s must be a real directory, not a symlink", label)
	}
	real, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", label, err)
	}
	if filepath.Clean(real) != name {
		return "", fmt.Errorf("%s must not traverse symlinks", label)
	}
	return name, nil
}
