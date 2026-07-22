package main

import (
	"io"
	"strings"
	"testing"
)

func TestBuildRequiresExactFlags(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		nil,
		{"build"},
		{"other"},
		{"build", "--host-reports", "a", "--host-reports", "b", "--provider-reports", "c", "--output-dir", "d", "--admission-version", "1.0.0"},
		{"build", "--host-reports", "a", "--provider-reports", "b", "--output-dir", "c", "--admission-version", "1.0.0", "--extra", "x"},
	} {
		if err := run(args, io.Discard); err == nil || !strings.Contains(err.Error(), "usage") && !strings.Contains(err.Error(), "duplicate") {
			t.Fatalf("args %q: err = %v", args, err)
		}
	}
}
