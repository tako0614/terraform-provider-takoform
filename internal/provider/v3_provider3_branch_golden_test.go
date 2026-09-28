package provider

// This file is the branch (rather than merely schema/codec) lock for the
// Provider 3 surface.  The all-31 codec golden proves the happy path; this
// golden records the smallest representative of every field codec, the
// provider-only artifact paths, update fences, replacement-only timeouts, and
// the accepted/error recovery envelopes.  Every vector below is derived by
// calling the production codec/resource/client path.  The fixture is a
// release artifact and is intentionally read-only here.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const v3Provider3BranchGoldenPath = "testdata/v3-provider3-branch-golden.json"

const (
	v3Provider3SourceTag    = "v3.0.0"
	v3Provider3SourceCommit = "a225cfa7c84aa551981cc8ad56c9a281fa6e051a"
)

type v3Provider3BranchGolden struct {
	Format       string                          `json:"format"`
	SourceTag    string                          `json:"sourceTag"`
	SourceCommit string                          `json:"sourceCommit"`
	FieldCodecs  []v3Provider3FieldBranchGolden  `json:"fieldCodecs"`
	Artifacts    []v3Provider3ArtifactBranch     `json:"artifacts"`
	Lifecycle    []v3Provider3LifecycleBranch    `json:"lifecycle"`
	Host         []v3Provider3HostEnvelopeBranch `json:"host"`
	Uncovered    []string                        `json:"uncovered"`
}

type v3Provider3FieldBranchGolden struct {
	Semantic        string                              `json:"semantic"`
	FormRef         v3FormRef                           `json:"formRef"`
	Field           string                              `json:"field"`
	Wire            string                              `json:"wire"`
	Required        bool                                `json:"required"`
	DeclaredDefault bool                                `json:"declaredDefault"`
	AbsenceSemantic bool                                `json:"absenceIsSemantic"`
	Branches        map[string]v3Provider3BranchOutcome `json:"branches"`
}

type v3Provider3BranchOutcome struct {
	Outcome          string `json:"outcome"`
	Projection       string `json:"projection"`
	InputDigest      string `json:"inputDigest,omitempty"`
	ResultDigest     string `json:"resultDigest"`
	DiagnosticDigest string `json:"diagnosticDigest,omitempty"`
}

type v3Provider3ArtifactBranch struct {
	FormKind            string            `json:"formKind"`
	Events              []string          `json:"events"`
	ManifestDigest      string            `json:"manifestDigest"`
	ManifestBodyDigest  string            `json:"manifestBodyDigest"`
	ApplyMethod         string            `json:"applyMethod"`
	ApplyPath           string            `json:"applyPath"`
	ApplyHeaders        map[string]string `json:"applyHeaders"`
	ApplyBodyDigest     string            `json:"applyBodyDigest"`
	ApplySpecDigest     string            `json:"applySpecDigest"`
	ReadbackStateDigest string            `json:"readbackStateDigest"`
}

type v3Provider3LifecycleBranch struct {
	Name              string            `json:"name"`
	FormKind          string            `json:"formKind"`
	Outcome           string            `json:"outcome"`
	Projection        string            `json:"projection"`
	RequestMethod     string            `json:"requestMethod,omitempty"`
	RequestPath       string            `json:"requestPath,omitempty"`
	RequestHeaders    map[string]string `json:"requestHeaders,omitempty"`
	RequestBodyDigest string            `json:"requestBodyDigest,omitempty"`
	RequestSpecDigest string            `json:"requestSpecDigest,omitempty"`
	StateDigest       string            `json:"stateDigest"`
	DiagnosticDigest  string            `json:"diagnosticDigest,omitempty"`
	HostEventDigest   string            `json:"hostEventDigest"`
}

type v3Provider3HostEnvelopeBranch struct {
	Name             string            `json:"name"`
	FormKind         string            `json:"formKind"`
	Outcome          string            `json:"outcome"`
	EnvelopeDigests  map[string]string `json:"envelopeDigests"`
	StateDigest      string            `json:"stateDigest"`
	DiagnosticDigest string            `json:"diagnosticDigest"`
}

// v3Provider3BranchDependencies is deliberately expressed only in terms of
// Provider-3 baseline types.  The immutable release has no Snapshot assembly
// type, while the forward provider can pass the projection-backed assembly's
// current Forms, exact registry, and codec table through the same seam.
type v3Provider3BranchDependencies struct {
	CurrentForms []model.Form
	Registry     interface {
		DefaultCreate(v3GroupKind) (v3FormRef, error)
		Lookup(v3ExactFormKey) (v3FormRef, bool)
		SupportedRefs() []v3FormRef
		SupportedRefsFor(v3GroupKind) []v3FormRef
		SupportedRefsForKind(string) []v3FormRef
	}
	Codecs *v3CodecTable
}

// TestV3Provider3BranchGoldenLocksBehavior exercises production conversion,
// resource, artifact, and Host-client paths before comparing their immutable
// vectors.  It deliberately has no fixture writer; changing these vectors is
// a release-evidence decision rather than a normal test refresh.
func TestV3Provider3BranchGoldenLocksBehavior(t *testing.T) {
	want := readV3Provider3BranchGolden(t)
	got := deriveV3Provider3BranchGolden(t)
	if !reflect.DeepEqual(got, want) {
		wantRaw, _ := json.MarshalIndent(want, "", "  ")
		gotRaw, _ := json.MarshalIndent(got, "", "  ")
		t.Fatalf("Provider 3 branch golden drifted:\nwant:\n%s\ngot:\n%s", wantRaw, gotRaw)
	}
}

func deriveV3Provider3BranchGolden(t *testing.T) v3Provider3BranchGolden {
	return deriveV3Provider3BranchGoldenWith(t, v3Provider3BranchDependencies{
		CurrentForms: mustProviderV3SnapshotAssembly().currentForms,
		Registry:     mustProviderV3SnapshotAssembly().registry,
		Codecs:       v3Codecs(),
	})
}

func deriveV3Provider3BranchGoldenWith(t *testing.T, dependencies v3Provider3BranchDependencies) v3Provider3BranchGolden {
	t.Helper()
	if len(dependencies.CurrentForms) == 0 || dependencies.Registry == nil || dependencies.Codecs == nil {
		t.Fatalf("Provider 3 branch dependencies are incomplete: forms=%d registry=%T codecs=%t", len(dependencies.CurrentForms), dependencies.Registry, dependencies.Codecs != nil)
	}
	return v3Provider3BranchGolden{
		Format:       "takoform.provider3-branch-golden@v1",
		SourceTag:    v3Provider3SourceTag,
		SourceCommit: v3Provider3SourceCommit,
		FieldCodecs:  deriveV3Provider3FieldBranches(t, dependencies),
		Artifacts:    deriveV3Provider3ArtifactBranches(t, dependencies),
		Lifecycle:    deriveV3Provider3LifecycleBranches(t, dependencies),
		Host:         deriveV3Provider3HostEnvelopeBranches(t, dependencies),
		Uncovered:    []string{},
	}
}

func deriveV3Provider3FieldBranches(t *testing.T, dependencies v3Provider3BranchDependencies) []v3Provider3FieldBranchGolden {
	t.Helper()
	type candidate struct {
		form      model.Form
		field     model.Field
		fieldPath string
		ref       v3FormRef
	}
	byKind := map[model.FieldKind]candidate{}
	registry := dependencies.Registry
	// The provider has dedicated artifact authoring for these revisions.  A
	// generic top-level field of the same kind is selected instead so this lock
	// characterizes the actual codec, not the manifest adapter (which has its
	// own vectors below).
	for _, form := range dependencies.CurrentForms {
		if _, artifactBacked := v3ProviderArtifactForForm(t, form); artifactBacked {
			continue
		}
		ref, err := registry.DefaultCreate(v3GroupKind{
			APIVersion: form.Family.APIVersion(), Kind: form.Kind,
		})
		if err != nil {
			t.Fatalf("default ref for %s: %v", form.Kind, err)
		}
		var visit func([]model.Field, string)
		visit = func(fields []model.Field, prefix string) {
			for _, field := range fields {
				fieldPath := field.HCL
				if prefix != "" {
					fieldPath = prefix + "." + field.HCL
				}
				if _, exists := byKind[field.Kind]; !exists {
					byKind[field.Kind] = candidate{form: form, field: field, fieldPath: fieldPath, ref: ref}
				}
				visit(field.Fields, fieldPath)
				for _, variant := range field.Variants {
					visit(variant.Fields, fieldPath+"["+variant.Tag+"]")
				}
			}
		}
		visit(form.Fields, "")
	}
	wantKinds := []model.FieldKind{
		model.KindString, model.KindStringEnum, model.KindInteger, model.KindBoolean,
		model.KindStringList, model.KindStringSet, model.KindStringMap, model.KindStringSetMap,
		model.KindJSONMap, model.KindResourceRef, model.KindExternalServiceList,
		model.KindBindingList, model.KindObjectList, model.KindObject, model.KindTaggedObject,
	}
	result := make([]v3Provider3FieldBranchGolden, 0, len(wantKinds))
	for _, kind := range wantKinds {
		candidate, ok := byKind[kind]
		if !ok {
			t.Fatalf("Provider 3 has no top-level representative for field kind %q", kind)
		}
		result = append(result, deriveV3Provider3FieldBranch(t, dependencies, candidate.form, candidate.field, candidate.fieldPath, candidate.ref))
	}
	return result
}

func deriveV3Provider3FieldBranch(t *testing.T, dependencies v3Provider3BranchDependencies, form model.Form, field model.Field, fieldPath string, ref v3FormRef) v3Provider3FieldBranchGolden {
	t.Helper()
	field = v3Provider3PopulateNestedExample(form, field, fieldPath)
	branchForm := form
	if strings.Contains(fieldPath, ".") || strings.Contains(fieldPath, "[") {
		// Nested object members have the same production codec functions but no
		// top-level Terraform attribute.  Build a one-field synthetic declaration
		// for the branch harness only; its exact source ref remains the enclosing
		// FormRef recorded in the golden.
		branchForm.Fields = []model.Field{field}
		branchForm.Kind = form.Kind + "BranchMember"
	}
	resource := &v3FormResource{form: branchForm, resourceType: "branch_golden", codecs: dependencies.Codecs}
	var codecDiags diag.Diagnostics
	var codec v3FormCodec
	var ok bool
	if branchForm.Kind == form.Kind {
		codec, ok = resource.v3DefaultCodec(&codecDiags)
	} else {
		desired, err := v3DesiredSchemaForCodec(branchForm)
		if err != nil {
			t.Fatalf("synthetic codec for %s.%s: %v", form.Kind, fieldPath, err)
		}
		codec = v3FormCodec{Ref: ref, Form: branchForm, DesiredSchema: desired}
		ok = true
	}
	if !ok || codecDiags.HasError() {
		t.Fatalf("codec for %s.%s: %v", form.Kind, fieldPath, codecDiags)
	}
	baseline := v3Provider3BranchBaselineValues(t, branchForm)
	branches := map[string]v3Provider3BranchOutcome{}
	branches["absent"] = deriveV3Provider3NullBranch(t, resource, codec, baseline, field, false)
	branches["explicit-null"] = deriveV3Provider3NullBranch(t, resource, codec, baseline, field, true)
	branches["unknown"] = deriveV3Provider3UnknownBranch(t, resource, codec, baseline, field)
	branches["invalid"] = deriveV3Provider3InvalidBranch(t, dependencies, codec, baseline, field)
	branches["alternate"] = deriveV3Provider3AlternateBranch(t, form, field)
	if field.Default != nil {
		branches["default"] = deriveV3Provider3DefaultBranch(t, resource, codec, baseline, field)
	} else {
		// Required/no-default and semantic-absence fields have no default
		// branch.  The metadata is still locked so an accidental default cannot
		// silently change omission behavior.
		branches["default"] = v3Provider3BranchOutcome{
			Outcome: "not-declared", Projection: "none",
			ResultDigest: v3CanonicalDigest(t, map[string]any{
				"required": field.Required, "absenceIsSemantic": field.AbsenceIsSemantic,
			}),
		}
	}
	return v3Provider3FieldBranchGolden{
		Semantic:        string(field.Kind),
		FormRef:         ref,
		Field:           fieldPath,
		Wire:            field.Wire,
		Required:        field.Required,
		DeclaredDefault: field.Default != nil,
		AbsenceSemantic: field.AbsenceIsSemantic,
		Branches:        branches,
	}
}

// Nested members inherit their normative example from the enclosing object
// fixture.  The catalog intentionally keeps examples on the object as one
// portable value, so a recursive codec vector must project the member before
// constructing its one-field harness.
func v3Provider3PopulateNestedExample(form model.Form, field model.Field, targetPath string) model.Field {
	if field.Example != nil || !strings.ContainsAny(targetPath, ".[") {
		return field
	}
	values := map[string]any{}
	for _, top := range form.Fields {
		raw := top.Example
		if raw == nil {
			raw = top.Default
		}
		if raw != nil {
			values[top.Wire] = raw
		}
	}
	if raw, ok := v3Provider3FindNestedRaw(form.Fields, values, targetPath, ""); ok {
		field.Example = raw
	}
	return field
}

func v3Provider3FindNestedRaw(fields []model.Field, values map[string]any, targetPath, prefix string) (any, bool) {
	for _, candidate := range fields {
		candidatePath := candidate.HCL
		if prefix != "" {
			candidatePath = prefix + "." + candidate.HCL
		}
		raw, present := values[candidate.Wire]
		if candidatePath == targetPath {
			return raw, present
		}
		if !present {
			continue
		}
		switch candidate.Kind {
		case model.KindObject:
			if child, ok := v3Provider3RawMap(raw); ok {
				if found, exists := v3Provider3FindNestedRaw(candidate.Fields, child, targetPath, candidatePath); exists {
					return found, true
				}
			}
		case model.KindObjectList:
			items, ok := raw.([]any)
			if !ok || len(items) == 0 {
				continue
			}
			if child, ok := v3Provider3RawMap(items[0]); ok {
				if found, exists := v3Provider3FindNestedRaw(candidate.Fields, child, targetPath, candidatePath); exists {
					return found, true
				}
			}
		case model.KindTaggedObject:
			object, ok := v3Provider3RawMap(raw)
			if !ok {
				continue
			}
			tag, _ := object[candidate.Discriminator].(string)
			for _, variant := range candidate.Variants {
				if variant.Tag != tag {
					continue
				}
				if found, exists := v3Provider3FindNestedRaw(variant.Fields, object, targetPath, candidatePath+"["+variant.Tag+"]"); exists {
					return found, true
				}
			}
		}
	}
	return nil, false
}

func v3Provider3RawMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out, true
	default:
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, false
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil || out == nil {
			return nil, false
		}
		return out, true
	}
}

func v3Provider3BranchBaselineValues(t *testing.T, form model.Form) map[string]attr.Value {
	t.Helper()
	ctx := context.Background()
	values := make(map[string]attr.Value, len(form.Fields))
	var diags diag.Diagnostics
	for _, field := range form.Fields {
		raw := field.Example
		if raw == nil {
			raw = field.Default
		}
		if raw == nil {
			values[v3AttributeName(field)] = v3NullFieldValue(field)
			continue
		}
		value := v3FieldValueFromSpec(ctx, form.Family.APIVersion(), field, decodedWire(t, raw), &diags)
		if value == nil || value.IsNull() && field.Required {
			t.Fatalf("baseline value for required %s.%s is null (diagnostics=%v)", form.Kind, field.Wire, diags)
		}
		values[v3AttributeName(field)] = value
	}
	if diags.HasError() {
		t.Fatalf("decode baseline %s: %v", form.Kind, diags)
	}
	return values
}

func deriveV3Provider3NullBranch(t *testing.T, resource *v3FormResource, codec v3FormCodec, baseline map[string]attr.Value, field model.Field, explicit bool) v3Provider3BranchOutcome {
	t.Helper()
	values := cloneV3Provider3AttrValues(baseline)
	values[v3AttributeName(field)] = v3NullFieldValue(field)
	branchValues := v3Values{Name: types.StringValue("branch-golden"), Space: types.StringValue("prod"), Fields: values}
	spec, diags := resource.v3SpecFromValues(context.Background(), codec, branchValues)
	return v3Provider3BranchOutcomeFromDiags(t, spec, diags, explicit, field)
}

func deriveV3Provider3DefaultBranch(t *testing.T, resource *v3FormResource, codec v3FormCodec, baseline map[string]attr.Value, field model.Field) v3Provider3BranchOutcome {
	t.Helper()
	values := cloneV3Provider3AttrValues(baseline)
	var diags diag.Diagnostics
	values[v3AttributeName(field)] = v3FieldValueFromSpec(context.Background(), resource.form.Family.APIVersion(), field, decodedWire(t, field.Default), &diags)
	if diags.HasError() {
		t.Fatalf("decode default %s.%s: %v", resource.form.Kind, field.Wire, diags)
	}
	branchValues := v3Values{Name: types.StringValue("branch-golden"), Space: types.StringValue("prod"), Fields: values}
	spec, specDiags := resource.v3SpecFromValues(context.Background(), codec, branchValues)
	return v3Provider3BranchOutcomeFromDiags(t, spec, specDiags, false, field)
}

func deriveV3Provider3UnknownBranch(t *testing.T, resource *v3FormResource, codec v3FormCodec, baseline map[string]attr.Value, field model.Field) v3Provider3BranchOutcome {
	t.Helper()
	values := cloneV3Provider3AttrValues(baseline)
	values[v3AttributeName(field)] = v3UnknownFieldValue(field)
	branchValues := v3Values{Name: types.StringValue("branch-golden"), Space: types.StringValue("prod"), Fields: values}
	spec, diags := resource.v3SpecFromValues(context.Background(), codec, branchValues)
	return v3Provider3BranchOutcomeFromDiags(t, spec, diags, false, field)
}

func deriveV3Provider3InvalidBranch(t *testing.T, dependencies v3Provider3BranchDependencies, codec v3FormCodec, baseline map[string]attr.Value, field model.Field) v3Provider3BranchOutcome {
	t.Helper()
	validValues := cloneV3Provider3AttrValues(baseline)
	resourceSpec := v3Values{Name: types.StringValue("branch-golden"), Space: types.StringValue("prod"), Fields: validValues}
	resource := &v3FormResource{form: codec.Form, resourceType: "branch_golden", codecs: dependencies.Codecs}
	valid, diags := resource.v3SpecFromValues(context.Background(), codec, resourceSpec)
	if diags.HasError() {
		t.Fatalf("baseline spec %s: %v", codec.Form.Kind, diags)
	}
	invalid := cloneV3Provider3Map(valid)
	invalid[field.Wire] = v3Provider3InvalidRaw(field)
	err := v3ValidateHostSpec(codec, invalid)
	if err == nil {
		t.Fatalf("invalid %s.%s was accepted by exact Host schema: %#v", codec.Form.Kind, field.Wire, invalid[field.Wire])
	}
	return v3Provider3BranchOutcome{
		Outcome: "error", Projection: "diagnostic",
		InputDigest:      v3CanonicalDigest(t, invalid[field.Wire]),
		ResultDigest:     v3Provider3ValidationErrorDigest(t, err),
		DiagnosticDigest: v3Provider3ValidationErrorDigest(t, err),
	}
}

// JSON-schema validators may walk object properties through a Go map and
// report independent failures in process-dependent order.  The public branch
// contract is the complete validation set, not that incidental traversal
// order, so retain every line while sorting only the independent detail lines.
func v3Provider3ValidationErrorDigest(t *testing.T, err error) string {
	t.Helper()
	lines := strings.Split(err.Error(), "\n")
	if len(lines) > 1 {
		sort.Strings(lines[1:])
	}
	return v3CanonicalDigest(t, map[string]any{"error": strings.Join(lines, "\n")})
}

func deriveV3Provider3AlternateBranch(t *testing.T, form model.Form, field model.Field) v3Provider3BranchOutcome {
	t.Helper()
	raw := v3Provider3AlternateRaw(field)
	if raw == nil {
		t.Fatalf("no alternate vector for %s.%s (%s)", form.Kind, field.Wire, field.Kind)
	}
	var decodeDiags diag.Diagnostics
	value := v3FieldValueFromSpec(context.Background(), form.Family.APIVersion(), field, decodedWire(t, raw), &decodeDiags)
	if decodeDiags.HasError() {
		t.Fatalf("decode alternate %s.%s: %v (raw=%#v)", form.Kind, field.Wire, decodeDiags, raw)
	}
	wire, encodeDiags := v3FieldToWire(context.Background(), form.Family.APIVersion(), field, v3AttributeName(field), value)
	if encodeDiags.HasError() {
		t.Fatalf("encode alternate %s.%s: %v (raw=%#v)", form.Kind, field.Wire, encodeDiags, raw)
	}
	return v3Provider3BranchOutcome{
		Outcome: "accepted", Projection: "wire",
		InputDigest: v3CanonicalDigest(t, raw), ResultDigest: v3CanonicalDigest(t, wire),
	}
}

func v3Provider3BranchOutcomeFromDiags(t *testing.T, result any, diags diag.Diagnostics, explicit bool, field model.Field) v3Provider3BranchOutcome {
	t.Helper()
	if diags.HasError() {
		return v3Provider3BranchOutcome{
			Outcome: "error", Projection: "diagnostic",
			ResultDigest: v3Provider3DiagnosticDigest(t, diags), DiagnosticDigest: v3Provider3DiagnosticDigest(t, diags),
		}
	}
	return v3Provider3BranchOutcome{
		Outcome: "accepted", Projection: "spec", ResultDigest: v3CanonicalDigest(t, result),
		InputDigest: v3CanonicalDigest(t, map[string]any{"explicit": explicit, "field": field.Wire}),
	}
}

func cloneV3Provider3AttrValues(input map[string]attr.Value) map[string]attr.Value {
	result := make(map[string]attr.Value, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func v3UnknownFieldValue(field model.Field) attr.Value {
	switch field.Kind {
	case model.KindBoolean:
		return types.BoolUnknown()
	case model.KindInteger:
		return types.Int64Unknown()
	case model.KindString, model.KindStringEnum, model.KindJSONMap, model.KindResourceRef:
		return types.StringUnknown()
	case model.KindStringList, model.KindResourceRefList:
		return types.ListUnknown(types.StringType)
	case model.KindStringSet:
		return types.SetUnknown(types.StringType)
	case model.KindStringMap:
		return types.MapUnknown(types.StringType)
	case model.KindStringSetMap:
		return types.MapUnknown(types.SetType{ElemType: types.StringType})
	case model.KindExternalServiceList:
		return types.ListUnknown(v3ExternalServiceObjectType())
	case model.KindBindingList:
		return types.ListUnknown(v3BindingObjectType())
	case model.KindObjectList:
		return types.ListUnknown(v3ObjectListType(field))
	case model.KindObject:
		return types.ObjectUnknown(v3ObjectType(field).AttrTypes)
	case model.KindTaggedObject:
		return types.ObjectUnknown(v3TaggedObjectType(field).AttrTypes)
	default:
		panic(fmt.Sprintf("unsupported unknown FieldKind %q", field.Kind))
	}
}

func v3Provider3InvalidRaw(field model.Field) any {
	if field.CounterExample != nil {
		return field.CounterExample
	}
	switch field.Kind {
	case model.KindString:
		return "not a valid value"
	case model.KindStringEnum:
		return "not-a-declared-enum"
	case model.KindInteger:
		return "not-an-integer"
	case model.KindBoolean:
		return "not-a-boolean"
	case model.KindStringList:
		return []any{int64(1)}
	case model.KindStringSet:
		return []any{int64(1)}
	case model.KindStringMap:
		return map[string]any{"bad key": int64(1)}
	case model.KindStringSetMap:
		return map[string]any{"bad key": []any{int64(1)}}
	case model.KindJSONMap:
		return map[string]any{"1 invalid key": "value"}
	case model.KindResourceRef:
		return map[string]any{"apiVersion": "other.example.com/v1", "kind": "Wrong", "name": "bad"}
	case model.KindExternalServiceList:
		return []any{map[string]any{"name": "bad", "service": map[string]any{"apiVersion": model.StandardServiceAPIVersion, "protocol": "not a protocol"}}}
	case model.KindBindingList:
		return []any{map[string]any{"name": "bad", "resource": map[string]any{"apiVersion": "other.example.com/v1", "kind": "Wrong", "name": "bad"}}}
	case model.KindObjectList:
		if items, ok := field.Example.([]any); ok && len(items) > 0 {
			copyItems := append([]any{}, items...)
			first := cloneV3Provider3Map(copyItems[0])
			if _, present := first["weight"]; present {
				first["weight"] = int64(0)
			}
			copyItems[0] = first
			return copyItems
		}
		return []any{}
	case model.KindObject:
		return map[string]any{"runWorkerFirst": true, "notFoundHandling": "backend_default", "bundle": map[string]any{"apiVersion": "edge.forms.takoform.com", "kind": "StaticAssetBundle", "name": "static-asset-bundle"}}
	case model.KindTaggedObject:
		return map[string]any{"type": "unknownVariant"}
	default:
		return nil
	}
}

func v3Provider3AlternateRaw(field model.Field) any {
	if field.AltExample != nil {
		return field.AltExample
	}
	raw := field.Example
	if raw == nil {
		raw = field.Default
	}
	switch field.Kind {
	case model.KindString:
		if text, ok := raw.(string); ok {
			return text + "-alt"
		}
	case model.KindStringEnum:
		if len(field.Enum) > 1 {
			return field.Enum[1]
		}
	case model.KindInteger:
		if field.Min != nil && (raw == nil || fmt.Sprint(raw) != fmt.Sprint(*field.Min)) {
			return *field.Min
		}
		if field.Max != nil && (raw == nil || fmt.Sprint(raw) != fmt.Sprint(*field.Max)) {
			return *field.Max
		}
		if value, ok := raw.(int); ok {
			return value + 1
		}
		if value, ok := raw.(int64); ok {
			return value + 1
		}
	case model.KindBoolean:
		if value, ok := raw.(bool); ok {
			return !value
		}
	case model.KindStringList:
		items, _ := raw.([]any)
		if len(items) == 0 || field.MaxItems != 1 {
			return append(append([]any{}, items...), itemsOrAltString(items)...)
		}
		return []any{}
	case model.KindStringSet:
		if len(field.Enum) > 1 {
			return []any{field.Enum[1]}
		}
		return []any{"ALT_VALUE"}
	case model.KindStringMap:
		out := cloneV3Provider3Map(raw)
		if len(out) < field.MaxProperties {
			out["ALT"] = "alternate"
		} else {
			for key := range out {
				out[key] = "alternate"
				break
			}
		}
		return out
	case model.KindStringSetMap:
		out := cloneV3Provider3Map(raw)
		if len(out) == 0 {
			out["eventType"] = []any{"order.deleted"}
		} else {
			for key, value := range out {
				items, _ := value.([]any)
				if len(items) < field.MaxItems {
					items = append(items, "order.deleted")
				} else if len(items) > 0 {
					items[0] = "order.deleted"
				}
				out[key] = items
				break
			}
		}
		return out
	case model.KindJSONMap:
		return map[string]any{"ALT": "alternate"}
	case model.KindResourceRef:
		out := cloneV3Provider3Map(raw)
		out["name"] = "alternate-name"
		return out
	case model.KindExternalServiceList:
		items, _ := raw.([]any)
		if len(items) == 0 {
			return []any{map[string]any{"name": "ALT_DB", "service": map[string]any{"apiVersion": model.StandardServiceAPIVersion, "protocol": "org.postgresql.wire"}}}
		}
		out := cloneV3Provider3Map(items)
		_ = out
		copyItems := append([]any{}, items...)
		entry := cloneV3Provider3Map(copyItems[0])
		entry["name"] = "ALT_DB"
		copyItems[0] = entry
		return copyItems
	case model.KindBindingList:
		items, _ := raw.([]any)
		if len(items) == 0 {
			return []any{map[string]any{"name": "ALT", "resource": map[string]any{"apiVersion": "edge.forms.takoform.com", "kind": "EdgeKVNamespace", "name": "alternate"}}}
		}
		copyItems := append([]any{}, items...)
		entry := cloneV3Provider3Map(copyItems[0])
		entry["name"] = "ALT"
		if target, ok := entry["resource"].(map[string]any); ok {
			target["name"] = "alternate"
		}
		copyItems[0] = entry
		return copyItems
	case model.KindObjectList:
		items, _ := raw.([]any)
		if len(items) == 0 {
			return []any{}
		}
		first := cloneV3Provider3Map(items[0])
		if _, ok := first["weight"]; ok {
			first["weight"] = int64(5000)
		}
		second := cloneV3Provider3Map(first)
		if _, ok := second["weight"]; ok {
			second["weight"] = int64(5000)
		}
		for key, value := range second {
			if reference, ok := value.(map[string]any); ok && reference["name"] != nil {
				reference["name"] = "alternate-name"
				second[key] = reference
				break
			}
		}
		return []any{first, second}
	case model.KindObject:
		out := cloneV3Provider3Map(raw)
		if value, ok := out["runWorkerFirst"].(bool); ok {
			out["runWorkerFirst"] = !value
			return out
		}
		for key, value := range out {
			if text, ok := value.(string); ok {
				out[key] = text + "-alt"
				break
			}
		}
		return out
	case model.KindTaggedObject:
		if len(field.Variants) > 1 {
			variant := field.Variants[1]
			out := map[string]any{field.Discriminator: variant.Tag}
			for _, member := range variant.Fields {
				out[member.Wire] = v3Provider3FieldExampleOrDefault(member)
			}
			return out
		}
	}
	return nil
}

func itemsOrAltString(items []any) []any {
	if len(items) == 0 {
		return []any{"/app/alternate"}
	}
	text, _ := items[0].(string)
	return []any{text}
}

func v3Provider3FieldExampleOrDefault(field model.Field) any {
	if field.Example != nil {
		return field.Example
	}
	if field.Default != nil {
		return field.Default
	}
	return v3Provider3AlternateRaw(field)
}

func cloneV3Provider3Map(value any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func deriveV3Provider3ArtifactBranches(t *testing.T, dependencies v3Provider3BranchDependencies) []v3Provider3ArtifactBranch {
	t.Helper()
	tests := []struct {
		kind, manifestKind, pathName, mediaType string
		content                                 []byte
	}{
		{kind: "WorkerBundle", manifestKind: "WorkerBundle", pathName: "worker.mjs", mediaType: "application/javascript+module", content: []byte("export default { fetch() { return new Response(\"branch\") } }\n")},
		{kind: "StaticAssetBundle", manifestKind: "StaticAssetBundle", pathName: "index.html", mediaType: "text/html", content: []byte("<!doctype html><title>branch</title>\n")},
		{kind: "SQLiteMigrationSet", manifestKind: "MigrationBundle", pathName: "0001_create_branch.sql", mediaType: "application/sql", content: []byte("CREATE TABLE branch (id TEXT PRIMARY KEY);\n")},
	}
	result := make([]v3Provider3ArtifactBranch, 0, len(tests))
	for _, test := range tests {
		t.Run("artifact/"+test.kind, func(t *testing.T) {
			host := newV3FakeHost(t)
			data, capture := newV3BranchRecordingProviderData(t, host)
			resource := v3Provider3BranchResource(t, test.kind, data, dependencies)
			ctx := context.Background()
			schemaResponse := v3SchemaOf(t, resource)
			contentFile := v3BundleFile(t, t.TempDir(), "payload", test.content)
			planValues := map[string]attr.Value{"name": types.StringValue(strings.ToLower(test.kind))}
			var wantManifest, wantBlob string
			if test.kind == "WorkerBundle" {
				wantManifest = v3ExpectedManifestDigest(t, test.pathName, contentFile, test.content)
				wantBlob = digestBlob(test.content)
				planValues["main_module"] = types.StringValue(test.pathName)
				planValues["modules"] = v3BundleModulesValue(test.pathName, contentFile)
			} else {
				wantManifest, wantBlob = edgeAppManifestDigest(t, test.manifestKind, test.pathName, test.mediaType, test.content)
				planValues["files"] = edgeAppArtifactFilesValue(test.pathName, test.mediaType, contentFile)
			}
			response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
			resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, planValues)}, &response)
			if response.Diagnostics.HasError() {
				t.Fatalf("create: %v", response.Diagnostics)
			}
			if !bytes.Equal(host.blobs[wantBlob], test.content) {
				t.Fatalf("uploaded blob mismatch")
			}
			apply := lastV3BranchRequest(t, capture, http.MethodPut, "/resources/")
			if len(host.applySpecs) != 1 || !reflect.DeepEqual(host.applySpecs[0], map[string]any{"manifestDigest": wantManifest}) {
				t.Fatalf("apply spec = %#v, want manifest digest %s", host.applySpecs, wantManifest)
			}
			stateSnapshot := v3ArtifactStateSnapshot(t, ctx, response.State, test.kind)
			result = append(result, v3Provider3ArtifactBranch{
				FormKind: test.kind, Events: append([]string{}, host.events...),
				ManifestDigest:     wantManifest,
				ManifestBodyDigest: digestJSONOrBytes(host.manifestRaw),
				ApplyMethod:        apply.Method, ApplyPath: apply.Path, ApplyHeaders: apply.Headers,
				ApplyBodyDigest: apply.BodyDigest, ApplySpecDigest: v3CanonicalDigest(t, host.applySpecs[0]),
				ReadbackStateDigest: v3CanonicalDigest(t, stateSnapshot),
			})
		})
	}
	return result
}

func deriveV3Provider3LifecycleBranches(t *testing.T, dependencies v3Provider3BranchDependencies) []v3Provider3LifecycleBranch {
	t.Helper()
	return []v3Provider3LifecycleBranch{
		deriveV3Provider3UpdateBranch(t, dependencies),
		deriveV3Provider3NoUpdateTimeoutBranch(t, dependencies),
	}
}

func deriveV3Provider3UpdateBranch(t *testing.T, dependencies v3Provider3BranchDependencies) v3Provider3LifecycleBranch {
	t.Helper()
	host := newV3FakeHost(t)
	data, capture := newV3BranchRecordingProviderData(t, host)
	resource := v3Provider3BranchResource(t, "WorkerCronTrigger", data, dependencies)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	create := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("worker-cron-trigger"), "worker": types.StringValue("module-worker"), "cron": types.StringValue("0 3 * * *"),
	})}, &create)
	if create.Diagnostics.HasError() {
		t.Fatalf("create update fixture: %v", create.Diagnostics)
	}
	update := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("worker-cron-trigger"), "space": types.StringValue("prod"), "worker": types.StringValue("module-worker"), "cron": types.StringValue("15 0 * * *"),
	}), State: create.State}, &update)
	if update.Diagnostics.HasError() {
		t.Fatalf("update fixture: %v", update.Diagnostics)
	}
	request := lastV3BranchRequest(t, capture, http.MethodPut, "/resources/edge.forms.takoform.com/WorkerCronTrigger/")
	body := decodeRequestBody(t, request.Body)
	state := map[string]any{
		"uid":        v3StateString(t, ctx, update.State, "uid").ValueString(),
		"generation": v3StateString(t, ctx, update.State, "generation").ValueString(),
		"revision":   v3StateString(t, ctx, update.State, "revision").ValueString(),
		"cron":       v3StateString(t, ctx, update.State, "cron").ValueString(),
	}
	return v3Provider3LifecycleBranch{
		Name: "in-place-update", FormKind: "WorkerCronTrigger", Outcome: "accepted", Projection: "host-update",
		RequestMethod: request.Method, RequestPath: request.Path, RequestHeaders: request.Headers,
		RequestBodyDigest: request.BodyDigest, RequestSpecDigest: v3CanonicalDigest(t, body["spec"]),
		StateDigest: v3CanonicalDigest(t, state), HostEventDigest: v3CanonicalDigest(t, host.events),
	}
}

func deriveV3Provider3NoUpdateTimeoutBranch(t *testing.T, dependencies v3Provider3BranchDependencies) v3Provider3LifecycleBranch {
	t.Helper()
	host := newV3FakeHost(t)
	data, _ := newV3BranchRecordingProviderData(t, host)
	resource := v3Provider3BranchResource(t, "WorkerCustomDomain", data, dependencies)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	create := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("worker-domain"), "worker": types.StringValue("module-worker"), "hostname": types.StringValue("app.example.invalid"),
	})}, &create)
	if create.Diagnostics.HasError() {
		t.Fatalf("create no-update fixture: %v", create.Diagnostics)
	}
	eventsAfterCreate := append([]string{}, host.events...)
	timeoutPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: create.State.Raw}
	if diags := timeoutPlan.SetAttribute(ctx, provider3BranchPathRoot("create_timeout"), types.StringValue("25m")); diags.HasError() {
		t.Fatalf("set timeout-only plan: %v", diags)
	}
	timeoutUpdate := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: timeoutPlan, State: create.State}, &timeoutUpdate)
	if timeoutUpdate.Diagnostics.HasError() || len(host.events) != len(eventsAfterCreate) {
		t.Fatalf("timeout-only no-update path failed or reached host: diagnostics=%v events=%v", timeoutUpdate.Diagnostics, host.events[len(eventsAfterCreate):])
	}
	illegalPlan := tfsdk.Plan{Schema: schemaResponse.Schema, Raw: create.State.Raw}
	if diags := illegalPlan.SetAttribute(ctx, provider3BranchPathRoot("hostname"), types.StringValue("other.example.invalid")); diags.HasError() {
		t.Fatalf("set illegal no-update plan: %v", diags)
	}
	illegal := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Update(ctx, frameworkresource.UpdateRequest{Plan: illegalPlan, State: create.State}, &illegal)
	if !illegal.Diagnostics.HasError() || len(host.events) != len(eventsAfterCreate) {
		t.Fatalf("illegal desired no-update update did not fail closed: diagnostics=%v events=%v", illegal.Diagnostics, host.events[len(eventsAfterCreate):])
	}
	return v3Provider3LifecycleBranch{
		Name: "no-update-timeout-only", FormKind: "WorkerCustomDomain", Outcome: "accepted-timeout-and-rejected-desired", Projection: "provider-only",
		StateDigest: v3CanonicalDigest(t, map[string]any{
			"timeoutState":      map[string]any{"create_timeout": v3StateString(t, ctx, timeoutUpdate.State, "create_timeout").ValueString(), "uid": v3StateString(t, ctx, timeoutUpdate.State, "uid").ValueString()},
			"illegalDiagnostic": v3Provider3DiagnosticDigest(t, illegal.Diagnostics),
		}),
		DiagnosticDigest: v3Provider3DiagnosticDigest(t, illegal.Diagnostics), HostEventDigest: v3CanonicalDigest(t, host.events),
	}
}

func deriveV3Provider3HostEnvelopeBranches(t *testing.T, dependencies v3Provider3BranchDependencies) []v3Provider3HostEnvelopeBranch {
	t.Helper()
	return []v3Provider3HostEnvelopeBranch{
		deriveV3Provider3Host202Branch(t, dependencies),
		deriveV3Provider3HostErrorBranch(t, dependencies),
		deriveV3Provider3HostRecoveryBranch(t, dependencies),
	}
}

func deriveV3Provider3Host202Branch(t *testing.T, dependencies v3Provider3BranchDependencies) v3Provider3HostEnvelopeBranch {
	t.Helper()
	host := newV3FakeHost(t)
	host.apply202 = true
	data, capture := newV3BranchRecordingProviderData(t, host)
	resource := v3Provider3BranchResource(t, "ModuleWorker", data, dependencies)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{"name": types.StringValue("module-worker")})}, &response)
	if response.Diagnostics.HasError() {
		t.Fatalf("202 create: %v", response.Diagnostics)
	}
	envelopes := v3Provider3EnvelopeDigests(t, capture, "/resources/", "/operations/op_apply1")
	return v3Provider3HostEnvelopeBranch{
		Name: "apply-202-terminal-success", FormKind: "ModuleWorker", Outcome: "accepted", EnvelopeDigests: envelopes,
		StateDigest:      v3CanonicalDigest(t, map[string]any{"uid": v3StateString(t, ctx, response.State, "uid").ValueString(), "generation": v3StateString(t, ctx, response.State, "generation").ValueString(), "pending": v3StateString(t, ctx, response.State, "pending_operation_id").IsNull()}),
		DiagnosticDigest: v3Provider3DiagnosticDigest(t, response.Diagnostics),
	}
}

func deriveV3Provider3HostErrorBranch(t *testing.T, dependencies v3Provider3BranchDependencies) v3Provider3HostEnvelopeBranch {
	t.Helper()
	host := newV3FakeHost(t)
	host.storeResource("ModuleWorker", "module-worker", "prod", "edge.forms.takoform.com", "uid-existing", map[string]any{})
	data, capture := newV3BranchRecordingProviderData(t, host)
	resource := v3Provider3BranchResource(t, "ModuleWorker", data, dependencies)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{"name": types.StringValue("module-worker")})}, &response)
	if !response.Diagnostics.HasError() || !v3RawIsNull(response.State.Raw) {
		t.Fatalf("precondition error did not fail without state: diagnostics=%v state=%s", response.Diagnostics, response.State.Raw.String())
	}
	envelopes := v3Provider3EnvelopeDigests(t, capture, "/resources/", "")
	return v3Provider3HostEnvelopeBranch{
		Name: "host-error-before-acceptance", FormKind: "ModuleWorker", Outcome: "error", EnvelopeDigests: envelopes,
		StateDigest: v3CanonicalDigest(t, map[string]any{"stateNull": v3RawIsNull(response.State.Raw)}), DiagnosticDigest: v3Provider3DiagnosticDigest(t, response.Diagnostics),
	}
}

func deriveV3Provider3HostRecoveryBranch(t *testing.T, dependencies v3Provider3BranchDependencies) v3Provider3HostEnvelopeBranch {
	t.Helper()
	host := newV3FakeHost(t)
	host.apply202Pending = true
	data, capture := newV3BranchRecordingProviderData(t, host)
	resource := v3Provider3BranchResource(t, "ModuleWorker", data, dependencies)
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)
	response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)}}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{"name": types.StringValue("module-worker"), "create_timeout": types.StringValue("1s")})}, &response)
	if !response.Diagnostics.HasError() || v3RawIsNull(response.State.Raw) {
		t.Fatalf("pending accepted create did not produce recoverable state: diagnostics=%v", response.Diagnostics)
	}
	// Create records the recovery marker and returns before a pending operation
	// settles.  A subsequent production Read is the recovery branch that
	// captures the operation envelope (and proves the marker survives a still
	// pending result).
	read := frameworkresource.ReadResponse{State: response.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: response.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatalf("pending recovery read: %v", read.Diagnostics)
	}
	envelopes := v3Provider3EnvelopeDigests(t, capture, "/resources/", "/operations/op_apply_pending")
	return v3Provider3HostEnvelopeBranch{
		Name: "apply-202-pending-recovery", FormKind: "ModuleWorker", Outcome: "accepted-recovery", EnvelopeDigests: envelopes,
		StateDigest:      v3CanonicalDigest(t, map[string]any{"uid": v3StateString(t, ctx, read.State, "uid").ValueString(), "pending": v3StateString(t, ctx, read.State, "pending_operation_id").ValueString(), "form": v3StateString(t, ctx, read.State, "form_schema_digest").ValueString()}),
		DiagnosticDigest: v3Provider3DiagnosticDigest(t, response.Diagnostics),
	}
}

func v3Provider3BranchResource(t *testing.T, kind string, data *providerData, dependencies v3Provider3BranchDependencies) *v3FormResource {
	t.Helper()
	for _, form := range dependencies.CurrentForms {
		if form.Family.APIVersion() == "edge.forms.takoform.com" && form.Kind == kind {
			return v3Provider3CurrentResourceHarness(t, form, "", data, dependencies.Codecs)
		}
	}
	t.Fatalf("Provider 3 branch dependencies have no edge Form %s", kind)
	return nil
}

func v3Provider3EnvelopeDigests(t *testing.T, capture *v3BranchRecordingTransport, initialPrefix, operationPrefix string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, request := range capture.requests {
		key := request.Method + " " + request.Path
		if strings.Contains(key, initialPrefix) && request.Status != 0 {
			result["initial"] = request.ResponseDigest
		}
		if operationPrefix != "" && strings.Contains(key, operationPrefix) && request.Status != 0 {
			result["operation"] = request.ResponseDigest
		}
	}
	if result["initial"] == "" {
		t.Fatalf("no initial Host envelope matching %q in %v", initialPrefix, capture.requests)
	}
	if operationPrefix != "" && result["operation"] == "" {
		t.Fatalf("no operation Host envelope matching %q in %v", operationPrefix, capture.requests)
	}
	return result
}

type v3BranchRecordedRequest struct {
	Method         string
	Path           string
	Headers        map[string]string
	BodyDigest     string
	Body           []byte
	Status         int
	ResponseDigest string
}

type v3BranchRecordingTransport struct {
	base     http.RoundTripper
	requests []v3BranchRecordedRequest
}

func (transport *v3BranchRecordingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var rawBody []byte
	var err error
	if request.Body != nil {
		rawBody, err = io.ReadAll(request.Body)
	}
	if err != nil {
		return nil, err
	}
	request.Body = io.NopCloser(bytes.NewReader(rawBody))
	base := transport.base
	if base == nil {
		base = http.DefaultTransport
	}
	response, err := base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	rawResponse, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	response.Body = io.NopCloser(bytes.NewReader(rawResponse))
	transport.requests = append(transport.requests, v3BranchRecordedRequest{
		Method: request.Method, Path: request.URL.EscapedPath(), Headers: v3BranchApplyHeaders(request.Header),
		BodyDigest: digestJSONOrBytes(rawBody), Body: append([]byte{}, rawBody...), Status: response.StatusCode,
		ResponseDigest: digestJSONOrBytes(rawResponse),
	})
	return response, nil
}

func newV3BranchRecordingProviderData(t *testing.T, host *v3FakeHost) (*providerData, *v3BranchRecordingTransport) {
	t.Helper()
	baseClient := host.server.Client()
	transport := &v3BranchRecordingTransport{base: baseClient.Transport}
	httpClient := &http.Client{Transport: transport, CheckRedirect: baseClient.CheckRedirect}
	client := clientv3.NewWithOptions(host.server.URL, "test-token", httpClient, clientv3.Options{})
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("v1beta1 discovery: %v", err)
	}
	return &providerData{clientV3: client, defaultSpace: "prod"}, transport
}

func v3BranchApplyHeaders(headers http.Header) map[string]string {
	result := map[string]string{}
	for _, name := range []string{"Content-Type", "Idempotency-Key", "If-None-Match", v3ExpectedGenerationHeader} {
		if value := headers.Get(name); value != "" {
			result[name] = value
		}
	}
	return result
}

func lastV3BranchRequest(t *testing.T, capture *v3BranchRecordingTransport, method, pathPrefix string) v3BranchRecordedRequest {
	t.Helper()
	for index := len(capture.requests) - 1; index >= 0; index-- {
		request := capture.requests[index]
		if request.Method == method && strings.Contains(request.Path, pathPrefix) {
			return request
		}
	}
	t.Fatalf("no recorded %s %s request; captured=%v", method, pathPrefix, capture.requests)
	return v3BranchRecordedRequest{}
}

func decodeRequestBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	return body
}

func digestJSONOrBytes(raw []byte) string {
	if canonical, err := formpackage.Canonicalize(raw); err == nil {
		return formpackage.DigestBytes(canonical)
	}
	return formpackage.DigestBytes(raw)
}

func digestBlob(raw []byte) string {
	return formpackage.DigestBytes(raw)
}

func v3Provider3DiagnosticDigest(t *testing.T, diagnostics diag.Diagnostics) string {
	t.Helper()
	entries := make([]map[string]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics.Errors() {
		entries = append(entries, v3Provider3DiagnosticEntry(diagnostic, "error"))
	}
	for _, diagnostic := range diagnostics.Warnings() {
		entries = append(entries, v3Provider3DiagnosticEntry(diagnostic, "warning"))
	}
	sort.Slice(entries, func(left, right int) bool {
		leftKey := entries[left]["severity"] + "\x00" + entries[left]["summary"] + "\x00" + entries[left]["detail"] + "\x00" + entries[left]["path"]
		rightKey := entries[right]["severity"] + "\x00" + entries[right]["summary"] + "\x00" + entries[right]["detail"] + "\x00" + entries[right]["path"]
		return leftKey < rightKey
	})
	return v3CanonicalDigest(t, entries)
}

func v3Provider3DiagnosticEntry(diagnostic diag.Diagnostic, severity string) map[string]string {
	entry := map[string]string{"severity": severity, "summary": diagnostic.Summary(), "detail": diagnostic.Detail()}
	if withPath, ok := diagnostic.(diag.DiagnosticWithPath); ok {
		entry["path"] = withPath.Path().String()
	}
	return entry
}

func v3ArtifactStateSnapshot(t *testing.T, ctx context.Context, state tfsdk.State, kind string) map[string]any {
	t.Helper()
	snapshot := map[string]any{"manifest_digest": v3StateString(t, ctx, state, "manifest_digest").ValueString()}
	if kind == "WorkerBundle" {
		snapshot["main_module"] = v3StateString(t, ctx, state, "main_module").ValueString()
		var modules types.List
		if diags := state.GetAttribute(ctx, provider3BranchPathRoot("modules"), &modules); diags.HasError() {
			t.Fatalf("modules state: %v", diags)
		}
		snapshot["modules"] = v3ArtifactListSnapshot(t, modules, true)
		return snapshot
	}
	var files types.List
	if diags := state.GetAttribute(ctx, provider3BranchPathRoot("files"), &files); diags.HasError() {
		t.Fatalf("files state: %v", diags)
	}
	snapshot["files"] = v3ArtifactListSnapshot(t, files, false)
	return snapshot
}

func v3ArtifactListSnapshot(t *testing.T, list types.List, worker bool) []map[string]any {
	t.Helper()
	result := make([]map[string]any, 0, len(list.Elements()))
	for _, element := range list.Elements() {
		object := element.(types.Object).Attributes()
		item := map[string]any{
			"content_file": "<local-file>",
			"size":         object["size"].(types.Int64).ValueInt64(),
			"digest":       object["digest"].(types.String).ValueString(),
		}
		if worker {
			item["name"] = object["name"].(types.String).ValueString()
			item["content_type"] = object["content_type"].(types.String).ValueString()
		} else {
			item["path"] = object["path"].(types.String).ValueString()
			item["media_type"] = object["media_type"].(types.String).ValueString()
		}
		result = append(result, item)
	}
	return result
}

func provider3BranchPathRoot(name string) path.Path {
	return path.Root(name)
}

func v3RawIsNull(value interface{ IsNull() bool }) bool {
	return value.IsNull()
}

func readV3Provider3BranchGolden(t *testing.T) v3Provider3BranchGolden {
	t.Helper()
	raw, err := os.ReadFile(v3Provider3BranchGoldenPath)
	if err != nil {
		t.Fatalf("read %s: %v", v3Provider3BranchGoldenPath, err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", v3Provider3BranchGoldenPath, err)
	}
	if !bytes.Equal(raw, canonical) {
		t.Fatalf("%s is not RFC 8785 canonical JSON", v3Provider3BranchGoldenPath)
	}
	var golden v3Provider3BranchGolden
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("decode %s: %v", v3Provider3BranchGoldenPath, err)
	}
	if golden.Format != "takoform.provider3-branch-golden@v1" || golden.SourceTag != v3Provider3SourceTag || golden.SourceCommit != v3Provider3SourceCommit {
		t.Fatalf("branch golden immutable source fence drifted: %#v", golden)
	}
	if len(golden.FieldCodecs) != 15 || len(golden.Artifacts) != 3 || len(golden.Lifecycle) != 2 || len(golden.Host) != 3 {
		t.Fatalf("branch golden inventory counts field=%d artifact=%d lifecycle=%d host=%d", len(golden.FieldCodecs), len(golden.Artifacts), len(golden.Lifecycle), len(golden.Host))
	}
	return golden
}
