package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCurrentProviderV3ReferenceSurfacesReturnDefensiveForms(t *testing.T) {
	first, err := CurrentProviderV3ReferenceSurfaces()
	if err != nil {
		t.Fatal(err)
	}
	position := -1
	for index := range first {
		if len(first[index].Form.Fields) != 0 {
			position = index
			break
		}
	}
	if position == -1 {
		t.Fatal("reference projection has no mutable Form witness")
	}
	wantKind := first[position].Form.Kind
	wantField := first[position].Form.Fields[0].HCL
	first[position].Form.Kind = "Mutated"
	first[position].Form.Fields[0].HCL = "mutated"

	second, err := CurrentProviderV3ReferenceSurfaces()
	if err != nil {
		t.Fatal(err)
	}
	if second[position].Form.Kind != wantKind || second[position].Form.Fields[0].HCL != wantField {
		t.Fatalf("reference projection exposed cached Provider model: got %s/%s, want %s/%s",
			second[position].Form.Kind, second[position].Form.Fields[0].HCL, wantKind, wantField)
	}
}

func TestProviderV3ReleaseLedgerMatchesExactProviderProjection(t *testing.T) {
	t.Parallel()

	projection, err := CurrentProviderV3ReleaseIdentityProjection()
	if err != nil {
		t.Fatalf("project provider v3 release identities: %v", err)
	}
	if len(projection.Families) != 8 || len(projection.Forms) != 31 {
		t.Fatalf("provider v3 projection = %d families/%d Forms, want 8/31", len(projection.Families), len(projection.Forms))
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "release", "provider-form-identities.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ledger struct {
		Format   string                                `json:"format"`
		Releases []ProviderV3ReleaseIdentityProjection `json:"releases"`
	}
	if err := json.Unmarshal(raw, &ledger); err != nil {
		t.Fatal(err)
	}
	if ledger.Format != "takoform.provider-form-identities@v1" {
		t.Fatalf("provider identity ledger format = %q", ledger.Format)
	}
	var recorded *ProviderV3ReleaseIdentityProjection
	for index := range ledger.Releases {
		if ledger.Releases[index].ProviderVersion == "3.0.0" {
			recorded = &ledger.Releases[index]
			break
		}
	}
	if recorded == nil {
		t.Fatal("provider identity ledger has no 3.0.0 release projection")
	}
	if !reflect.DeepEqual(*recorded, projection) {
		t.Fatalf("provider v3 release ledger does not match the exact implementation projection\nrecorded: %#v\nprojected: %#v", *recorded, projection)
	}
	withdrawnV1Alpha2Types := map[string]struct{}{
		"takoform_edge_worker":         {},
		"takoform_relational_database": {},
		"takoform_object_bucket":       {},
		"takoform_key_value_store":     {},
		"takoform_queue":               {},
		"takoform_schedule":            {},
		"takoform_container_service":   {},
		"takoform_stateful_entity":     {},
		"takoform_vector_index":        {},
	}
	for _, form := range projection.Forms {
		if form.FormRef.Kind == "ObjectBucket" || form.ResourceType == "takoform_edge_object_bucket" {
			t.Fatalf("withdrawn ObjectBucket leaked into provider v3 release projection: %#v", form)
		}
		if _, withdrawn := withdrawnV1Alpha2Types[form.ResourceType]; withdrawn {
			t.Fatalf("provider v3 reoccupies withdrawn v1alpha2 Terraform type %q with %s/%s", form.ResourceType, form.FormRef.APIVersion, form.FormRef.Kind)
		}
	}
}
