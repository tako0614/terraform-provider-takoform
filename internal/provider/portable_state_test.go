package provider

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
)

func TestPortableStateProjectionValidatesEveryCurrentForm(t *testing.T) {
	t.Parallel()

	for _, kind := range formcatalog.Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()

			resource := canonicalPortableResource(kind, 7)
			projection, err := validatePortableStateProjection(
				kind,
				kind.FixtureName(),
				kind.CanonicalDesired(),
				&resource,
			)
			if err != nil {
				t.Fatal(err)
			}
			wantID := kind.Kind + "/" + kind.FixtureName()
			if projection.ID != wantID {
				t.Errorf("id = %q, want %q", projection.ID, wantID)
			}
			if projection.DriftStatus != "current" || projection.Portability != "portable" {
				t.Errorf("portable projection = %#v", projection)
			}
			if projection.Outputs["id"] != wantID ||
				projection.Outputs["kind"] != kind.Kind ||
				projection.Outputs["name"] != kind.FixtureName() ||
				projection.Outputs["generation"] != "7" ||
				projection.Outputs["portability"] != "portable" {
				t.Errorf("validated output projection lost portable identity: %#v", projection.Outputs)
			}
		})
	}
}

func TestPortableStateProjectionDerivesDriftOnlyFromValidatedObservedDocument(t *testing.T) {
	t.Parallel()

	kind, _ := formcatalog.ByKind("ObjectBucket")
	resource := canonicalPortableResource(kind, 3)
	resource.Status.Observed["driftedFields"] = []any{"/storageClass"}
	projection, err := validatePortableStateProjection(
		kind,
		kind.FixtureName(),
		kind.CanonicalDesired(),
		&resource,
	)
	if err != nil {
		t.Fatal(err)
	}
	if projection.DriftStatus != "drifted" {
		t.Fatalf("drift_status = %q, want drifted", projection.DriftStatus)
	}
}

func TestPortableStateProjectionRejectsHostAuthorityAndIdentitySubstitution(t *testing.T) {
	t.Parallel()

	kind, _ := formcatalog.ByKind("ObjectBucket")
	tests := []struct {
		name   string
		mutate func(*client.Resource)
		want   string
	}{
		{
			name: "missing status",
			mutate: func(resource *client.Resource) {
				resource.Status = nil
			},
			want: "no portable status",
		},
		{
			name: "different metadata name",
			mutate: func(resource *client.Resource) {
				resource.Metadata.Name = "other"
			},
			want: "metadata.name",
		},
		{
			name: "different spec name",
			mutate: func(resource *client.Resource) {
				resource.Spec["name"] = "other"
			},
			want: "returned spec.name",
		},
		{
			name: "host target in desired",
			mutate: func(resource *client.Resource) {
				resource.Spec["selectedTarget"] = "private-target"
			},
			want: "returned spec: desired document violates the portable data-only policy",
		},
		{
			name: "credential in desired",
			mutate: func(resource *client.Resource) {
				resource.Spec["credential"] = "secret"
			},
			want: "returned spec: desired document violates the portable data-only policy",
		},
		{
			name: "valid desired substitution",
			mutate: func(resource *client.Resource) {
				resource.Spec["storageClass"] = "archive"
			},
			want: "not canonically exact",
		},
		{
			name: "missing observed",
			mutate: func(resource *client.Resource) {
				resource.Status.Observed = nil
			},
			want: "observed document is missing",
		},
		{
			name: "host target in observed",
			mutate: func(resource *client.Resource) {
				resource.Status.Observed["selectedTarget"] = "private-target"
			},
			want: "observed document violates the portable data-only policy",
		},
		{
			name: "credential in output",
			mutate: func(resource *client.Resource) {
				resource.Status.Output["credential"] = "secret"
			},
			want: "output document violates the portable data-only policy",
		},
		{
			name: "different observed identity",
			mutate: func(resource *client.Resource) {
				resource.Status.Observed["id"] = "ObjectBucket/other"
			},
			want: "observed.id",
		},
		{
			name: "different output identity",
			mutate: func(resource *client.Resource) {
				resource.Status.Output["id"] = "ObjectBucket/other"
				resource.Status.Output["name"] = "other"
			},
			want: "output.id",
		},
		{
			name: "different observed generation",
			mutate: func(resource *client.Resource) {
				resource.Status.Observed["generation"] = 8
			},
			want: "observed.generation",
		},
		{
			name: "different output generation",
			mutate: func(resource *client.Resource) {
				resource.Status.Output["generation"] = 8
			},
			want: "output.generation",
		},
		{
			name: "overflowing resource version",
			mutate: func(resource *client.Resource) {
				resource.Metadata.ResourceVersion = "9223372036854775808"
			},
			want: "resourceVersion",
		},
		{
			name: "noncanonical resource version",
			mutate: func(resource *client.Resource) {
				resource.Metadata.ResourceVersion = "07"
			},
			want: "resourceVersion",
		},
		{
			name: "overflowing observed generation",
			mutate: func(resource *client.Resource) {
				resource.Status.Observed["generation"] = json.Number("9223372036854775808")
			},
			want: "observed document",
		},
		{
			name: "different portability",
			mutate: func(resource *client.Resource) {
				resource.Status.Output["portability"] = "host-specific"
			},
			want: "differs from output.portability",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := canonicalPortableResource(kind, 7)
			test.mutate(&resource)
			if _, err := validatePortableStateProjection(
				kind,
				kind.FixtureName(),
				kind.CanonicalDesired(),
				&resource,
			); err == nil {
				t.Fatal("invalid host status entered the provider projection")
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %q, want %q", err, test.want)
			}
		})
	}
}

func TestPortableStateProjectionRejectsInvalidCurrentDesiredState(t *testing.T) {
	t.Parallel()

	kind, _ := formcatalog.ByKind("ObjectBucket")
	currentSpec := kind.CanonicalDesired()
	currentSpec["selectedTarget"] = "private-target"
	resource := canonicalPortableResource(kind, 7)
	resource.Spec["selectedTarget"] = "private-target"

	if _, err := validatePortableStateProjection(
		kind,
		kind.FixtureName(),
		currentSpec,
		&resource,
	); err == nil {
		t.Fatal("invalid current desired state entered the provider projection")
	} else if !strings.Contains(err.Error(), "expected spec: desired document violates the portable data-only policy") {
		t.Fatalf("error = %q, want invalid expected current desired state", err)
	}
}

func TestProviderOwnedResourceIDUsesOnlyPortableKindAndName(t *testing.T) {
	t.Parallel()

	if got := resourceIDForKind("ObjectBucket", "assets"); got != "ObjectBucket/assets" {
		t.Fatalf("resource id = %q", got)
	}
}

func TestPortableStateProjectionPreservesMaximumGenerationExactly(t *testing.T) {
	t.Parallel()

	kind, _ := formcatalog.ByKind("ObjectBucket")
	resource := canonicalPortableResource(kind, formcatalog.MaxPortableGeneration)
	resource.Status.Observed["generation"] = json.Number("9223372036854775807")
	resource.Status.Output["generation"] = json.Number("9223372036854775807")

	projection, err := validatePortableStateProjection(
		kind,
		kind.FixtureName(),
		kind.CanonicalDesired(),
		&resource,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := projection.Outputs["generation"]; got != "9223372036854775807" {
		t.Fatalf("projected generation = %q", got)
	}
}

func TestPortableGenerationRejectsOverflowFractionsAndRoundedFloats(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "one int", value: 1, want: 1, ok: true},
		{name: "max int64", value: int64(9223372036854775807), want: 9223372036854775807, ok: true},
		{name: "max JSON number", value: json.Number("9223372036854775807"), want: 9223372036854775807, ok: true},
		{name: "zero", value: json.Number("0")},
		{name: "negative", value: json.Number("-1")},
		{name: "fraction", value: json.Number("1.5")},
		{name: "overflow", value: json.Number("9223372036854775808")},
		{name: "rounded float", value: float64(9007199254740992)},
		{name: "unsigned host value", value: uint64(1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, ok := portableGeneration(test.value)
			if ok != test.ok || got != test.want {
				t.Fatalf("portableGeneration(%T(%v)) = (%d,%v), want (%d,%v)", test.value, test.value, got, ok, test.want, test.ok)
			}
		})
	}
}

func canonicalPortableResource(kind formcatalog.Kind, generation int64) client.Resource {
	observed := kind.CanonicalObserved()
	output := kind.CanonicalOutput()
	observed["generation"] = generation
	output["generation"] = generation
	return client.Resource{
		APIVersion: client.APIVersion,
		Kind:       kind.Kind,
		Metadata: client.Metadata{
			Name:            kind.FixtureName(),
			Space:           "prod",
			ResourceVersion: strconv.FormatInt(generation, 10),
		},
		Spec:   kind.CanonicalDesired(),
		Status: &client.Status{Observed: observed, Output: output},
	}
}
