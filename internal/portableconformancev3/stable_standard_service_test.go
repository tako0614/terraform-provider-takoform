package portableconformancev3

import "testing"

func TestStableStandardServiceSlotsFailClosedBeforeMutation(t *testing.T) {
	probe := ExternalServiceProbe{
		Property:          "externalServices",
		ServiceAPIVersion: "standards.takoform.com/v1",
		Protocols:         []string{"com.amazonaws.s3"},
	}

	supported := map[string]any{"externalServices": []any{map[string]any{
		"name": "ASSETS",
		"service": map[string]any{
			"apiVersion": "standards.takoform.com/v1",
			"protocol":   "com.amazonaws.s3",
		},
	}}}
	if hostErr := validateStableStandardServiceSlots(probe, supported); hostErr != nil {
		t.Fatalf("supported S3 slot: %s: %s", hostErr.Code, hostErr.Message)
	}

	requiredUnknown := map[string]any{"externalServices": []any{map[string]any{
		"name": "FUTURE",
		"service": map[string]any{
			"apiVersion": "standards.takoform.com/v1",
			"protocol":   "com.example.future-store",
		},
	}}}
	if hostErr := validateStableStandardServiceSlots(probe, requiredUnknown); hostErr == nil || hostErr.Code != "unsupported_capability" {
		t.Fatalf("required unknown slot = %#v, want unsupported_capability", hostErr)
	}

	optionalUnknown := map[string]any{"externalServices": []any{map[string]any{
		"name":     "FUTURE",
		"required": false,
		"service": map[string]any{
			"apiVersion": "standards.takoform.com/v1",
			"protocol":   "com.example.future-store",
		},
	}}}
	if hostErr := validateStableStandardServiceSlots(probe, optionalUnknown); hostErr != nil {
		t.Fatalf("optional unsupported slot must project nothing without blocking: %s: %s", hostErr.Code, hostErr.Message)
	}
}

func TestStableStandardServiceSlotsRejectNonOpaqueIdentity(t *testing.T) {
	probe := ExternalServiceProbe{
		Property:          "externalServices",
		ServiceAPIVersion: "standards.takoform.com/v1",
		Protocols:         []string{"com.amazonaws.s3"},
	}
	spec := map[string]any{"externalServices": []any{map[string]any{
		"name": "BROKEN",
		"service": map[string]any{
			"apiVersion": "standards.takoform.com/v1",
			"protocol":   "s3-compatible",
		},
	}}}
	if hostErr := validateStableStandardServiceSlots(probe, spec); hostErr == nil || hostErr.Code != "invalid_argument" {
		t.Fatalf("invalid opaque identity = %#v, want invalid_argument", hostErr)
	}
}
