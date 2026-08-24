package formpackage

import (
	"strings"
	"testing"
)

func TestStandardServiceRefAcceptsUnknownNamespacedProtocol(t *testing.T) {
	t.Parallel()
	ref := StandardServiceRef{APIVersion: StandardServiceAPIVersion, Protocol: "dev.example.quantum-cache"}
	if err := ValidateStandardServiceRef(ref); err != nil {
		t.Fatalf("unknown grammar-valid protocol was rejected: %v", err)
	}
	for _, protocol := range []string{"s3-compatible", "Com.Amazonaws.S3", "com..s3"} {
		if err := ValidateStandardServiceRef(StandardServiceRef{APIVersion: StandardServiceAPIVersion, Protocol: protocol}); err == nil {
			t.Errorf("invalid protocol %q was accepted", protocol)
		}
	}
}

func TestStandardServiceSupportMatchesExactOpaqueIdentity(t *testing.T) {
	t.Parallel()
	ref := StandardServiceRef{APIVersion: StandardServiceAPIVersion, Protocol: "com.amazonaws.s3"}
	profile := func(protocol string, satisfiable bool) map[string]any {
		return map[string]any{
			"apiVersion":  StandardServiceSupportAPIVersion,
			"kind":        "StandardServiceSupport",
			"serviceRef":  map[string]any{"apiVersion": StandardServiceAPIVersion, "protocol": protocol},
			"satisfiable": satisfiable,
		}
	}
	if satisfiable, err := ValidateStandardServiceSupport(ref, profile(ref.Protocol, true)); err != nil || !satisfiable {
		t.Fatalf("exact satisfiable profile = %v, %v", satisfiable, err)
	}
	if satisfiable, err := ValidateStandardServiceSupport(ref, profile(ref.Protocol, false)); err != nil || satisfiable {
		t.Fatalf("exact unsatisfied profile = %v, %v", satisfiable, err)
	}
	if _, err := ValidateStandardServiceSupport(ref, profile("org.postgresql.wire", true)); err == nil ||
		!strings.Contains(err.Error(), "different exact") {
		t.Fatalf("substituted profile error = %v", err)
	}
}
