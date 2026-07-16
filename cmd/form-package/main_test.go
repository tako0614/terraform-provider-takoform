package main

import "testing"

func TestRunRejectsUnknownOrIncompleteCommands(t *testing.T) {
	t.Parallel()
	for _, arguments := range [][]string{nil, {"unknown"}, {"verify"}, {"digest"}, {"canonicalize"}, {"conformance", "a", "b"}} {
		if err := run(arguments); err == nil {
			t.Fatalf("run(%q) unexpectedly succeeded", arguments)
		}
	}
}
