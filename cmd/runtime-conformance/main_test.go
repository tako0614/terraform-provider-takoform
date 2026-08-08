package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const corpus = "../../conformance/runtime-abi-v1"

func TestVerifyReportsTheCorpusIdentity(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"verify", "--contract", corpus}, &stdout); err != nil {
		t.Fatalf("verify: %v", err)
	}
	var document struct {
		ContractDigest string   `json:"contractDigest"`
		Checks         []string `json:"checks"`
		Interface      struct {
			Name string `json:"name"`
		} `json:"interface"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.HasPrefix(document.ContractDigest, "sha256:") ||
		document.Interface.Name != "worker.runtime" || len(document.Checks) == 0 {
		t.Fatalf("verify output is incomplete: %s", stdout.String())
	}
}

func TestSelfTestPrintsAReportThatCannotBeMistakenForARuntimeRun(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"self-test", "--contract", corpus}, &stdout); err != nil {
		t.Fatalf("self-test: %v", err)
	}
	output := stdout.String()
	for _, want := range []string{
		`"classification": "in-process-fake-runtime-self-test"`,
		`"publicationReady": false`,
		"proves nothing about any runtime",
		"V3-008",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("self-test report is missing %q", want)
		}
	}
}

func TestTheCommandRefusesArgumentsThatWouldConfuseTheSubject(t *testing.T) {
	for name, args := range map[string][]string{
		"no command":           {},
		"unknown command":      {"measure", "--contract", corpus},
		"self-test endpoint":   {"self-test", "--contract", corpus, "--endpoint", "https://worker.example"},
		"verify endpoint":      {"verify", "--contract", corpus, "--endpoint", "https://worker.example"},
		"run without endpoint": {"run", "--contract", corpus},
		"positional argument":  {"self-test", "--contract", corpus, "extra"},
		"missing corpus":       {"self-test", "--contract", "conformance/does-not-exist"},
		"missing token in env": {"run", "--contract", corpus, "--endpoint", "https://worker.example", "--token-env", "TAKOFORM_ABSENT_TOKEN"},
	} {
		var stdout bytes.Buffer
		if err := run(args, &stdout); err == nil {
			t.Fatalf("%s: expected a refusal", name)
		}
	}
}
