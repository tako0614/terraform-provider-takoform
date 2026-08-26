package main

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestSelfTestCommandEmitsPassedNonPublicationReport(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{
		"self-test",
		"--contract", filepath.Join("..", "..", "conformance", "takoform-v1", "family-host", "edge", "portable-host"),
	}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"format": "takoform.portable-host-runner-report@v3"`,
		`"classification": "deterministic-reference-host-self-test"`,
		`"publicationReady": false`,
		`"status": "passed"`,
		`"prepare-binds-exact-spec"`,
		`"replay-record-retires-with-its-incarnation"`,
	} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("self-test output omitted %s:\n%s", want, output)
		}
	}
}

func TestRunCommandRequiresEndpoint(t *testing.T) {
	err := run([]string{
		"run",
		"--contract", filepath.Join("..", "..", "conformance", "takoform-v1", "family-host", "edge", "portable-host"),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run accepted no endpoint")
	}
}
