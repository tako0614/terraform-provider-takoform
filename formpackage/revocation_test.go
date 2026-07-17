package formpackage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateRevocationStatement(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	raw := []byte(fmt.Sprintf(`{
  "apiVersion":"trust.forms.takoform.com/v1alpha1",
  "kind":"FormPackageRevocation",
  "sequence":1,
  "statementVersion":"1.0.0",
  "packageDigest":%q,
  "formRef":{"apiVersion":"forms.takoform.com/v1alpha1","kind":"ObjectBucket","definitionVersion":"1.0.0","schemaDigest":%q},
  "reasonCode":"signature-invalid",
  "summary":"The retained signature cannot be validated.",
  "advisoryUrl":"https://example.com/advisories/TF-1",
  "issuedAt":"2026-07-17T00:00:00Z",
  "effects":{"blockNewCreateOrUpdate":true,"blockActivation":true,"retainBytesForObserveAndDelete":true}
}`, digest, digest))
	statement, err := ValidateRevocationStatement(raw)
	if err != nil {
		t.Fatal(err)
	}
	if statement.PackageDigest != digest || statement.StatementVersion != "1.0.0" {
		t.Fatalf("unexpected statement: %+v", statement)
	}
}

func TestRevocationCheckpointFixtureAdvancesPinnedHashChain(t *testing.T) {
	t.Parallel()
	fixture := filepath.Join("..", "conformance", "revocation-checkpoint-v1", "positive")
	first, err := os.ReadFile(filepath.Join(fixture, "checkpoint-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(fixture, "checkpoint-2.json"))
	if err != nil {
		t.Fatal(err)
	}
	firstPin, err := AdvanceRevocationCheckpoint(nil, first)
	if err != nil {
		t.Fatal(err)
	}
	secondPin, err := AdvanceRevocationCheckpoint(&firstPin, second)
	if err != nil {
		t.Fatal(err)
	}
	if firstPin.Sequence != 1 || secondPin.Sequence != 2 || firstPin.Digest == secondPin.Digest || !ValidDigest(secondPin.EntriesDigest) {
		t.Fatalf("unexpected checkpoint pins: first=%+v second=%+v", firstPin, secondPin)
	}
	if _, err := AdvanceRevocationCheckpoint(&secondPin, first); err == nil {
		t.Fatal("rollback to an older checkpoint unexpectedly accepted")
	}
	wrongPrevious := strings.Replace(string(second), firstPin.Digest, "sha256:"+strings.Repeat("e", 64), 1)
	if _, err := AdvanceRevocationCheckpoint(&firstPin, []byte(wrongPrevious)); err == nil {
		t.Fatal("checkpoint fork unexpectedly accepted")
	}
	rewrittenPrefix := strings.Replace(string(second), "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "sha256:"+strings.Repeat("f", 64), 1)
	if _, err := AdvanceRevocationCheckpoint(&firstPin, []byte(rewrittenPrefix)); err == nil {
		t.Fatal("checkpoint cumulative-prefix rewrite unexpectedly accepted")
	}
	omitted := strings.Replace(string(second), `"sequence": 1`, `"sequence": 9`, 1)
	if _, err := ValidateRevocationCheckpoint([]byte(omitted)); err == nil {
		t.Fatal("checkpoint omission/reordering unexpectedly accepted")
	}
}

func TestValidateRevocationStatementFailsClosed(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	base := fmt.Sprintf(`{"apiVersion":"trust.forms.takoform.com/v1alpha1","kind":"FormPackageRevocation","sequence":1,"statementVersion":"1.0.0","packageDigest":%q,"formRef":{"apiVersion":"forms.takoform.com/v1alpha1","kind":"ObjectBucket","definitionVersion":"1.0.0","schemaDigest":%q},"reasonCode":"signature-invalid","summary":"invalid","issuedAt":"2026-07-17T00:00:00Z","effects":{"blockNewCreateOrUpdate":true,"blockActivation":true,"retainBytesForObserveAndDelete":true}}`, digest, digest)
	for _, mutation := range []struct {
		name string
		from string
		to   string
	}{
		{name: "allows update", from: `"blockNewCreateOrUpdate":true`, to: `"blockNewCreateOrUpdate":false`},
		{name: "allows activation", from: `"blockActivation":true`, to: `"blockActivation":false`},
		{name: "drops retained bytes", from: `"retainBytesForObserveAndDelete":true`, to: `"retainBytesForObserveAndDelete":false`},
		{name: "deprecation is not revocation", from: `"signature-invalid"`, to: `"deprecated"`},
		{name: "non-https advisory", from: `"issuedAt"`, to: `"advisoryUrl":"http://example.com/a","issuedAt"`},
	} {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()
			_, err := ValidateRevocationStatement([]byte(strings.Replace(base, mutation.from, mutation.to, 1)))
			if err == nil {
				t.Fatal("invalid revocation unexpectedly accepted")
			}
		})
	}
}
