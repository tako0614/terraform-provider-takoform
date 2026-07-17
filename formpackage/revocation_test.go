package formpackage

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateRevocationStatement(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	raw := []byte(fmt.Sprintf(`{
  "apiVersion":"trust.forms.takoform.com/v1alpha1",
  "kind":"FormPackageRevocation",
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

func TestValidateRevocationStatementFailsClosed(t *testing.T) {
	t.Parallel()
	digest := "sha256:" + strings.Repeat("a", 64)
	base := fmt.Sprintf(`{"apiVersion":"trust.forms.takoform.com/v1alpha1","kind":"FormPackageRevocation","statementVersion":"1.0.0","packageDigest":%q,"formRef":{"apiVersion":"forms.takoform.com/v1alpha1","kind":"ObjectBucket","definitionVersion":"1.0.0","schemaDigest":%q},"reasonCode":"signature-invalid","summary":"invalid","issuedAt":"2026-07-17T00:00:00Z","effects":{"blockNewCreateOrUpdate":true,"blockActivation":true,"retainBytesForObserveAndDelete":true}}`, digest, digest)
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
