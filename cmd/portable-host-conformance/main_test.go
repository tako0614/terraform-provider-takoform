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
		"--contract", filepath.Join("..", "..", "conformance", "portable-host-v1"),
	}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"format": "takoform.portable-host-runner-report@v1"`,
		`"classification": "deterministic-reference-host-self-test"`,
		`"publicationReady": false`,
		`"status": "passed"`,
		`"preview-plan-spec-binding"`,
		`"interface-ready-projection"`,
	} {
		if !bytes.Contains(stdout.Bytes(), []byte(want)) {
			t.Fatalf("self-test output omitted %s:\n%s", want, output)
		}
	}
}

func TestRunCommandRequiresEndpoint(t *testing.T) {
	err := run([]string{
		"run",
		"--contract", filepath.Join("..", "..", "conformance", "portable-host-v1"),
	}, &bytes.Buffer{})
	if err == nil {
		t.Fatal("run accepted no endpoint")
	}
}
