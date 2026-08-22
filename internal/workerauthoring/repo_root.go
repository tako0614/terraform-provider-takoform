package workerauthoring

import (
	"errors"
	"os"
	"path/filepath"
)

// RepoRoot walks up from start to the repository that owns this module. The
// sentinel is the current family candidate set: the previous sentinel was the
// central-epoch package set, which was withdrawn with its epoch.
func RepoRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			candidateSet := filepath.Join(
				current, "forms", "candidates", "edge", "v1beta1", "candidate-set.json",
			)
			if _, err := os.Stat(candidateSet); err == nil {
				return current, nil
			}
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("could not find Takoform repository root")
		}
		current = parent
	}
}
