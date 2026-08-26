package providerdiagnostics

import "testing"

func TestAutomationContractCodesAreExact(t *testing.T) {
	if ImmutableRevisionSameName != "takoform.provider/immutable-revision-same-name" {
		t.Fatalf("immutable-revision code = %q", ImmutableRevisionSameName)
	}
	if HostDoesNotSupportValue != "takoform.provider/host-does-not-support-value" {
		t.Fatalf("host-support code = %q", HostDoesNotSupportValue)
	}
}
