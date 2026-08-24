package provider

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// TestV3W0TypedFieldsSurvivePlanWireReadState is the provider-side W0
// acceptance path. One synthetic Form (not a published family definition)
// exercises every new representation through the real resource lifecycle:
// typed schema -> plan -> exact wire -> host echo -> read -> typed state.
// It also pins portable defaults and semantic absence as distinct states.
func TestV3W0TypedFieldsSurvivePlanWireReadState(t *testing.T) {
	host := newV3FakeHost(t)
	form := v3W0SyntheticForm()
	ref := currentformregistry.V3Ref{
		APIVersion: form.Family.APIVersion(), Kind: form.Kind, DefinitionVersion: form.DefinitionVersion,
		SchemaDigest: "sha256:" + strings.Repeat("a", 64), PackageDigest: "sha256:" + strings.Repeat("b", 64),
	}
	codecs, err := v3Codecs().withCodec(ref, form, true)
	if err != nil {
		t.Fatal(err)
	}
	resource := &v3FormResource{
		form: form, resourceType: "takoform_delivery_policy",
		data: newV3TestProviderData(t, host), codecs: codecs,
	}
	ctx := context.Background()
	schemaResponse := v3SchemaOf(t, resource)

	if _, ok := schemaResponse.Schema.Attributes["arguments"].(schema.ListAttribute); !ok {
		t.Fatalf("arguments schema = %T, want typed ListAttribute", schemaResponse.Schema.Attributes["arguments"])
	}
	if got, ok := schemaResponse.Schema.Attributes["labels"].(schema.MapAttribute); !ok || !got.ElementType.Equal(types.StringType) {
		t.Fatalf("labels schema = %#v, want map(string)", schemaResponse.Schema.Attributes["labels"])
	}
	if got, ok := schemaResponse.Schema.Attributes["filters"].(schema.MapAttribute); !ok ||
		!got.ElementType.Equal(types.SetType{ElemType: types.StringType}) {
		t.Fatalf("filters schema = %#v, want map(set(string))", schemaResponse.Schema.Attributes["filters"])
	}
	if _, ok := schemaResponse.Schema.Attributes["destination"].(schema.SingleNestedAttribute); !ok {
		t.Fatalf("destination schema = %T, want typed SingleNestedAttribute", schemaResponse.Schema.Attributes["destination"])
	}

	profileField := form.Fields[5]
	destinationField := form.Fields[6]
	profileType := v3ObjectType(profileField)
	innerType := v3ObjectType(profileField.Fields[1])
	profile := types.ObjectValueMust(profileType.AttrTypes, map[string]attr.Value{
		"enabled": types.BoolValue(true),
		"nested": types.ObjectValueMust(innerType.AttrTypes, map[string]attr.Value{
			"entries": types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("repeat"), types.StringValue("repeat"),
			}),
			"metadata": types.MapValueMust(types.StringType, map[string]attr.Value{
				"Team": types.StringValue("core"),
			}),
		}),
	})
	destinationValues := v3NullTaggedObjectMembers(destinationField)
	destinationValues["type"] = types.StringValue("queue")
	destinationValues["queue"] = types.StringValue("jobs")
	destinationValues["mode"] = types.StringNull() // selected-branch default materializes on the wire
	destinationValues["settings"] = types.ObjectValueMust(
		v3ObjectType(destinationField.Variants[0].Fields[2]).AttrTypes,
		map[string]attr.Value{
			"arguments": types.ListValueMust(types.StringType, []attr.Value{
				types.StringValue("--tag"), types.StringValue("x"), types.StringValue("--tag"),
			}),
		},
	)
	destination := types.ObjectValueMust(v3TaggedObjectType(destinationField).AttrTypes, destinationValues)

	defaultArguments, ok := v3AttributeDefaultValue(t, schemaResponse.Schema.Attributes["default_arguments"])
	if !ok {
		t.Fatal("default_arguments has no framework default")
	}
	defaultLabels, ok := v3AttributeDefaultValue(t, schemaResponse.Schema.Attributes["default_labels"])
	if !ok {
		t.Fatal("default_labels has no framework default")
	}
	plan := v3PlanWith(t, ctx, schemaResponse, map[string]attr.Value{
		"name": types.StringValue("delivery-policy"), "space": types.StringValue("prod"),
		"arguments": types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("--tag"), types.StringValue("x"), types.StringValue("--tag"),
		}),
		"labels": types.MapValueMust(types.StringType, map[string]attr.Value{
			"Team": types.StringValue("core"), "Zone": types.StringValue("west"),
		}),
		"filters": types.MapValueMust(types.SetType{ElemType: types.StringType}, map[string]attr.Value{
			"color": types.SetValueMust(types.StringType, []attr.Value{
				types.StringValue("red"), types.StringValue("blue"),
			}),
		}),
		"watchers": types.ListValueMust(types.StringType, []attr.Value{
			types.StringValue("watch-a"), types.StringValue("watch-b"),
		}),
		"unused":  types.SetValueMust(types.StringType, []attr.Value{types.StringValue("legacy")}),
		"profile": profile, "destination": destination,
		"default_arguments": defaultArguments, "default_labels": defaultLabels,
		"optional_policy": types.ObjectNull(v3ObjectType(form.Fields[9]).AttrTypes),
	})
	createResponse := frameworkresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &createResponse)
	if createResponse.Diagnostics.HasError() {
		t.Fatalf("create: %v", createResponse.Diagnostics)
	}
	if len(host.applySpecs) != 1 {
		t.Fatalf("host apply count = %d, want 1", len(host.applySpecs))
	}
	sent := host.applySpecs[0]
	if got := sent["arguments"]; !reflect.DeepEqual(got, []any{"--tag", "x", "--tag"}) {
		t.Fatalf("ordered arguments = %#v, want duplicates preserved", got)
	}
	if got := sent["filters"].(map[string]any)["color"]; !reflect.DeepEqual(got, []any{"blue", "red"}) {
		t.Fatalf("deterministic set-map value = %#v, want sorted set", got)
	}
	if got := sent["watchers"]; !reflect.DeepEqual(got, []any{
		map[string]any{"apiVersion": "identity.forms.example", "kind": "Watcher", "name": "watch-a"},
		map[string]any{"apiVersion": "identity.forms.example", "kind": "Watcher", "name": "watch-b"},
	}) {
		t.Fatalf("explicit ResourceTarget list = %#v", got)
	}
	wantDestination := map[string]any{
		"type": "queue", "queue": map[string]any{
			"apiVersion": "queue.forms.example", "kind": "Queue", "name": "jobs",
		},
		"mode":     "push",
		"settings": map[string]any{"arguments": []any{"--tag", "x", "--tag"}},
	}
	if got := sent["destination"]; !reflect.DeepEqual(got, wantDestination) {
		t.Fatalf("tagged recursive destination = %#v, want %#v", got, wantDestination)
	}
	if _, present := sent["optionalPolicy"]; present {
		t.Fatalf("semantic absence was materialized: %#v", sent["optionalPolicy"])
	}
	if got := sent["defaultArguments"]; !reflect.DeepEqual(got, []any{"same", "same"}) {
		t.Fatalf("ordered-list default = %#v", got)
	}
	if got := sent["defaultLabels"]; !reflect.DeepEqual(got, map[string]any{}) {
		t.Fatalf("empty typed-map default = %#v", got)
	}

	readResponse := frameworkresource.ReadResponse{State: createResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: createResponse.State}, &readResponse)
	if readResponse.Diagnostics.HasError() {
		t.Fatalf("read: %v", readResponse.Diagnostics)
	}
	var readArguments types.List
	if diags := readResponse.State.GetAttribute(ctx, pathRoot("arguments"), &readArguments); diags.HasError() {
		t.Fatal(diags)
	}
	var gotArguments []string
	if diags := readArguments.ElementsAs(ctx, &gotArguments, false); diags.HasError() {
		t.Fatal(diags)
	}
	if got, want := gotArguments, []string{"--tag", "x", "--tag"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("read ordered arguments = %#v, want %#v", got, want)
	}
	var readDestination types.Object
	if diags := readResponse.State.GetAttribute(ctx, pathRoot("destination"), &readDestination); diags.HasError() {
		t.Fatal(diags)
	}
	if got := readDestination.Attributes()["mode"].(types.String).ValueString(); got != "push" {
		t.Fatalf("selected-branch default in read state = %q, want push", got)
	}
	var absent types.Object
	if diags := readResponse.State.GetAttribute(ctx, pathRoot("optional_policy"), &absent); diags.HasError() {
		t.Fatal(diags)
	}
	if !absent.IsNull() {
		t.Fatalf("semantic absence in read state = %v, want null", absent)
	}

	// Exact-schema validation happens before provider decode. It distinguishes
	// semantic absence from malformed Host data at every recursive depth,
	// including defaults selected inside a tagged branch.
	for name, mutate := range map[string]func(map[string]any){
		"missing required top-level": func(spec map[string]any) { delete(spec, "labels") },
		"explicit null required":     func(spec map[string]any) { spec["labels"] = nil },
		"missing top-level default":  func(spec map[string]any) { delete(spec, "defaultArguments") },
		"missing nested required": func(spec map[string]any) {
			delete(spec["profile"].(map[string]any)["nested"].(map[string]any), "entries")
		},
		"missing selected-branch default": func(spec map[string]any) {
			delete(spec["destination"].(map[string]any), "mode")
		},
	} {
		t.Run(name, func(t *testing.T) {
			invalid := v3CloneJSONMap(t, sent)
			mutate(invalid)
			if err := v3ValidateHostSpec(v3FormCodec{Ref: ref, Form: form, DesiredSchema: codecs.codecs[ref.ExactKey()].DesiredSchema}, invalid); err == nil {
				t.Fatal("malformed Host spec passed exact-schema validation")
			}
		})
	}
	if err := v3ValidateHostSpec(
		v3FormCodec{Ref: ref, Form: form, DesiredSchema: codecs.codecs[ref.ExactKey()].DesiredSchema},
		v3CloneJSONMap(t, sent),
	); err != nil {
		t.Fatalf("semantic absence was rejected: %v", err)
	}

	// A real Read of malformed nested data returns an error and leaves the
	// caller's state byte-for-byte untouched; it never writes a partial
	// identity/status before discovering the bad field.
	host.mu.Lock()
	host.resources[host.resourceKey(form.Kind, "delivery-policy")].spec = v3CloneJSONMap(t, sent)
	delete(host.resources[host.resourceKey(form.Kind, "delivery-policy")].spec["profile"].(map[string]any)["nested"].(map[string]any), "entries")
	host.mu.Unlock()
	invalidRead := frameworkresource.ReadResponse{State: readResponse.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: readResponse.State}, &invalidRead)
	if !invalidRead.Diagnostics.HasError() {
		t.Fatal("Read accepted a Host spec with a missing nested required field")
	}
	if !invalidRead.State.Raw.Equal(readResponse.State.Raw) {
		t.Fatal("Read wrote partial state before rejecting malformed Host spec")
	}

	// Import intentionally records only the exact identity. The mandatory
	// refresh that follows must apply the same schema-before-decode rule: a
	// malformed existing Host resource cannot be adopted as partial state.
	importID, err := json.Marshal(map[string]string{
		"space":             "prod",
		"apiVersion":        ref.APIVersion,
		"kind":              ref.Kind,
		"definitionVersion": ref.DefinitionVersion,
		"schemaDigest":      ref.SchemaDigest,
		"name":              "delivery-policy",
	})
	if err != nil {
		t.Fatal(err)
	}
	imported := frameworkresource.ImportStateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema, Raw: v3EmptyRaw(t, ctx, schemaResponse)},
	}
	resource.ImportState(ctx, frameworkresource.ImportStateRequest{ID: string(importID)}, &imported)
	if imported.Diagnostics.HasError() {
		t.Fatalf("import exact identity: %v", imported.Diagnostics)
	}
	importIdentityState := imported.State.Raw.Copy()
	importRead := frameworkresource.ReadResponse{State: imported.State}
	resource.Read(ctx, frameworkresource.ReadRequest{State: imported.State}, &importRead)
	if !importRead.Diagnostics.HasError() {
		t.Fatal("Read after import accepted a Host spec with a missing nested required field")
	}
	if !importRead.State.Raw.Equal(importIdentityState) {
		t.Fatal("Read after import wrote partial state before rejecting malformed Host spec")
	}
}

func pathRoot(name string) path.Path { return path.Root(name) }

func v3CloneJSONMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var cloned map[string]any
	if err := json.Unmarshal(raw, &cloned); err != nil {
		t.Fatal(err)
	}
	return cloned
}

func v3W0SyntheticForm() model.Form {
	stringList := func(hcl, wire string, required bool) model.Field {
		return model.Field{
			HCL: hcl, Wire: wire, Kind: model.KindStringList, Required: required,
			Doc: "Ordered arguments; duplicates remain meaningful.", ItemPattern: `^[A-Za-z0-9-]+$`, MaxLength: 32, MaxItems: 12,
		}
	}
	stringMap := func(hcl, wire string, required bool) model.Field {
		return model.Field{
			HCL: hcl, Wire: wire, Kind: model.KindStringMap, Required: required,
			Doc: "Bounded typed labels.", ItemPattern: `^[a-z]+$`, MaxLength: 16, MinProperties: 0, MaxProperties: 8,
		}
	}
	ref := func(hcl, wire, group, kind string) model.Field {
		return model.Field{
			HCL: hcl, Wire: wire, Kind: model.KindResourceRef, Required: true, Doc: "Exact target resource.",
			ResourceTarget: &model.ResourceTarget{Group: group, Kind: kind, Contract: model.TargetContract{ExactForm: true}},
		}
	}
	profile := model.Field{
		HCL: "profile", Wire: "profile", Kind: model.KindObject, Required: true, Doc: "Recursive typed profile.",
		Fields: []model.Field{
			{HCL: "enabled", Wire: "enabled", Kind: model.KindBoolean, Required: true, Doc: "Whether enabled."},
			{HCL: "nested", Wire: "nested", Kind: model.KindObject, Required: true, Doc: "Nested configuration.", Fields: []model.Field{
				stringList("entries", "entries", true), stringMap("metadata", "metadata", true),
			}},
		},
	}
	destination := model.Field{
		HCL: "destination", Wire: "destination", Kind: model.KindTaggedObject, Required: true,
		Doc: "Closed destination variant.", Discriminator: "type",
		Variants: []model.TaggedObjectVariant{
			{Tag: "queue", Fields: []model.Field{
				ref("queue", "queue", "queue.forms.example", "Queue"),
				{HCL: "mode", Wire: "mode", Kind: model.KindStringEnum, Doc: "Delivery mode.", Enum: []string{"push", "pull"}, Default: "push"},
				{HCL: "settings", Wire: "settings", Kind: model.KindObject, Required: true, Doc: "Queue settings.", Fields: []model.Field{
					stringList("arguments", "arguments", true),
				}},
			}},
			{Tag: "topic", Fields: []model.Field{ref("topic", "topic", "topic.forms.example", "Topic")}},
		},
	}
	return model.Form{
		Family: model.Family{Group: "delivery.forms.example"}, Kind: "DeliveryPolicy",
		Role: model.RoleIdentity, RequiresHostAPI: "forms.takoform.com/v1", DefinitionVersion: "0.1.0",
		Title: "Delivery Policy", Description: "Synthetic provider W0 coverage only.",
		Fields: []model.Field{
			stringList("arguments", "arguments", true),
			stringMap("labels", "labels", true),
			{HCL: "filters", Wire: "filters", Kind: model.KindStringSetMap, Required: true, Doc: "Bounded deterministic set map.", ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxItems: 8, MaxProperties: 8},
			{HCL: "watchers", Wire: "watchers", Kind: model.KindResourceRefList, Required: true, Doc: "Exact watcher targets.", MaxItems: 8, ResourceTarget: &model.ResourceTarget{Group: "identity.forms.example", Kind: "Watcher", Contract: model.TargetContract{ExactForm: true}}},
			{HCL: "unused", Wire: "unused", Kind: model.KindStringSet, Required: true, Doc: "Legacy set coverage.", ItemPattern: `^[a-z]+$`, MaxItems: 8},
			profile,
			destination,
			{HCL: "default_arguments", Wire: "defaultArguments", Kind: model.KindStringList, Doc: "Portable ordered default.", ItemPattern: `^[a-z-]+$`, MaxLength: 16, MaxItems: 8, Default: []any{"same", "same"}},
			{HCL: "default_labels", Wire: "defaultLabels", Kind: model.KindStringMap, Doc: "Portable map default.", ItemPattern: `^[a-z]+$`, MaxLength: 16, MaxProperties: 8, Default: map[string]any{}},
			{HCL: "optional_policy", Wire: "optionalPolicy", Kind: model.KindObject, Doc: "When absent, no policy override is requested.", AbsenceIsSemantic: true, Fields: []model.Field{
				{HCL: "enabled", Wire: "enabled", Kind: model.KindBoolean, Required: true, Doc: "Whether enabled."},
			}},
		},
	}
}
