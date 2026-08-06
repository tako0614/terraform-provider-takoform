package formpackage

// namespaced_group_test.go pins the Go Form-group guard against the normative
// grammar it mirrors. spec/schemas/form-ref-v1alpha3.schema.json excludes
// exactly two enum values; the Go guard is deliberately a strict superset and
// additionally refuses every version of the bare forms.takoform.com group and
// the reserved packages./trust. envelope namespaces. Anything accepted here
// therefore also satisfies the normative schema.

import "testing"

func TestNamespacedFormGroupIsAStrictSupersetOfTheSchemaExclusions(t *testing.T) {
	t.Parallel()
	for apiVersion, want := range map[string]bool{
		// The two enum values the normative schema itself excludes.
		"forms.takoform.com/v1alpha1": false,
		"forms.takoform.com/v1alpha2": false,
		// Every OTHER version of the bare group: that domain names retained Form
		// epochs and Host API wire identities, never a Form family.
		"forms.takoform.com/v1alpha3": false,
		"forms.takoform.com/v1alpha4": false,
		"forms.takoform.com/v1":       false,
		"forms.takoform.com/v2beta1":  false,
		// The reserved envelope namespaces: package indexes and trust statements
		// live there, so a Form Definition never can.
		"packages.forms.takoform.com/v1alpha1": false,
		"packages.forms.takoform.com/v1alpha3": false,
		"packages.forms.takoform.com/v1alpha4": false,
		"trust.forms.takoform.com/v1alpha1":    false,
		"trust.forms.takoform.com/v1":          false,
		// Official families are subdomains of forms.takoform.com.
		"edge.forms.takoform.com/v1alpha1": true,
		// Third-party families are any other DNS-like group.
		"forms.example.com/v1alpha1": true,
		"forms.example.com/v1":       true,
		// Grammar failures stay grammar failures.
		"forms/v1":                   false,
		"Forms.Example.com/v1alpha":  false,
		"forms.example.com":          false,
		"forms.example.com/version1": false,
	} {
		apiVersion, want := apiVersion, want
		t.Run(apiVersion, func(t *testing.T) {
			t.Parallel()
			if got := NamespacedFormGroup(apiVersion); got != want {
				t.Fatalf("NamespacedFormGroup(%q) = %v, want %v", apiVersion, got, want)
			}
		})
	}
}

// TestReservedNamespacesAreTheEnvelopeIdentities keeps the reserved list bound
// to the constants it protects rather than to a copied string.
func TestReservedNamespacesAreTheEnvelopeIdentities(t *testing.T) {
	t.Parallel()
	for _, envelope := range []string{
		PackageAPIVersion, LegacyContentAddressedPackageAPIVersion,
		CurrentPackageAPIVersion, FamilyPackageAPIVersion, TrustAPIVersion,
	} {
		if NamespacedFormGroup(envelope) {
			t.Fatalf("envelope identity %q was accepted as a Form group", envelope)
		}
	}
}

// TestFamilyLanesRejectTheReservedNamespacesEndToEnd proves the guard actually
// closes the lanes it gates: FormRef validation, Form Definition validation,
// and family publication identity.
func TestFamilyLanesRejectTheReservedNamespacesEndToEnd(t *testing.T) {
	t.Parallel()
	digest := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	for _, group := range []string{
		"forms.takoform.com/v1alpha3",
		"packages.forms.takoform.com/v1alpha1",
		"trust.forms.takoform.com/v1alpha1",
	} {
		group := group
		t.Run(group, func(t *testing.T) {
			t.Parallel()
			ref := map[string]any{
				"apiVersion": group, "kind": "ExampleStore",
				"definitionVersion": "0.1.0", "schemaDigest": digest,
			}
			if _, err := validateFormRef(canonicalMarshal(t, ref)); err == nil {
				t.Errorf("FormRef in reserved group %q was accepted", group)
			}
			definition := makeFamilyDefinition()
			definition["apiVersion"] = group
			if _, err := ValidateDefinition(canonicalMarshal(t, definition)); err == nil {
				t.Errorf("Form Definition in reserved group %q was accepted", group)
			}
			index := PackageIndex{
				APIVersion: FamilyPackageAPIVersion, Kind: PackageKind,
				FormRef:        FormRef{APIVersion: group, Kind: "ExampleStore", DefinitionVersion: "0.1.0", SchemaDigest: digest},
				DefinitionPath: "definition.json",
			}
			if _, err := PublicationLocatorFor(index, digest); err == nil {
				t.Errorf("family publication identity accepted reserved group %q", group)
			}
		})
	}
}
