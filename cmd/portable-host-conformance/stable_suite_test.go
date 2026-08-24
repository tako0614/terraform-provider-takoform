package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestStableV1SuiteEmitsTheManifestOwnedPublicationReport(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	err := run([]string{
		"suite",
		"--manifest", filepath.Join("..", "..", "conformance", "takoform-v1", "manifest.json"),
	}, &stdout)
	if err != nil {
		t.Fatal(err)
	}

	var report struct {
		Format      string `json:"format"`
		Status      string `json:"status"`
		HostAPILane string `json:"hostApiLane"`
		Families    []struct {
			Group          string           `json:"group"`
			RunnerFormRefs []map[string]any `json:"runnerFormRefs"`
		} `json:"families"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode suite report: %v\n%s", err, stdout.String())
	}
	if report.Format != "takoform.reference-host-suite-report@v1" ||
		report.Status != "passed" ||
		report.HostAPILane != "forms.takoform.com/v1" {
		t.Fatalf("stable suite report identity = %#v", report)
	}
	if len(report.Families) != 8 {
		t.Fatalf("stable suite reported %d families, want all 8", len(report.Families))
	}
	for _, family := range report.Families {
		if len(family.RunnerFormRefs) == 0 {
			t.Fatalf("stable suite family %s reported no exact FormRefs", family.Group)
		}
	}
}
