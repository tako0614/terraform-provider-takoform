package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

const (
	v3Provider3TofuSchemaPath   = "testdata/v3-provider3-tofu-schema.json"
	v3Provider3TofuSchemaDigest = "sha256:0dc07fc814386e51f66745cf1c482438c050a5c14c197c1778988cab46184da6"
)

// TestV3Provider3ProtocolSchemaMatchesPublishedBinary compares the complete
// protocol-visible provider/resource schema with `tofu providers schema
// -json` captured from the immutable Registry-installed v3.0.0 Linux AMD64
// binary. Unlike the smaller structural golden, this includes descriptions,
// deprecations, nested blocks, sensitivity and every protocol shape exposed to
// OpenTofu/Terraform tooling.
func TestV3Provider3ProtocolSchemaMatchesPublishedBinary(t *testing.T) {
	published := readV3Provider3TofuSchema(t)
	server := providerserver.NewProtocol6(New("3.0.0")())()
	response, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Diagnostics) != 0 {
		t.Fatalf("GetProviderSchema diagnostics = %#v", response.Diagnostics)
	}
	v3AssertProvider3ProtocolOptionalSurfaces(t, response)
	historical := v3Provider3HistoricalProtocolSchemaDocument(t, response)
	if !bytes.Equal(historical, published) {
		t.Fatalf("current protocol schema differs from immutable Provider 3.0.0 (`%s`); current digest %s",
			v3Provider3TofuSchemaDigest, formpackage.DigestBytes(historical))
	}
}

// TestV3Provider31ProtocolSchemaOwnsApplyIdempotencyKey keeps the additive
// provider-only WorkerVersion surface in a current lane. The historical
// comparison above deliberately projects that one known additive member out
// through v3Provider3HistoricalProtocolSchemaDocument; this test proves the
// current source still exposes it exactly where intended.
func TestV3Provider31ProtocolSchemaOwnsApplyIdempotencyKey(t *testing.T) {
	server := providerserver.NewProtocol6(New("3.1.0")())()
	response, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Diagnostics) != 0 {
		t.Fatalf("GetProviderSchema diagnostics = %#v", response.Diagnostics)
	}
	worker, ok := response.ResourceSchemas["takoform_worker_version"]
	if !ok || worker == nil || worker.Block == nil {
		t.Fatal("current Provider 3.1 schema has no WorkerVersion resource")
	}
	var key *tfprotov6.SchemaAttribute
	for _, attribute := range worker.Block.Attributes {
		if attribute != nil && attribute.Name == "apply_idempotency_key" {
			key = attribute
			break
		}
	}
	if key == nil {
		t.Fatal("current Provider 3.1 WorkerVersion schema has no apply_idempotency_key")
	}
	if !key.Optional || key.Required || !key.Computed {
		t.Fatalf("current apply_idempotency_key flags = optional=%t required=%t computed=%t", key.Optional, key.Required, key.Computed)
	}
	for name, schema := range response.ResourceSchemas {
		if name == "takoform_worker_version" || schema == nil || schema.Block == nil {
			continue
		}
		for _, attribute := range schema.Block.Attributes {
			if attribute != nil && attribute.Name == "apply_idempotency_key" {
				t.Fatalf("current Provider 3.1 resource %q unexpectedly exposes apply_idempotency_key", name)
			}
		}
	}
}

// v3AssertProvider3ProtocolOptionalSurfaces locks the portions of the
// GetProviderSchema response which are not represented by OpenTofu's schema
// JSON. Provider 3.0.0 intentionally publishes no provider_meta, data
// sources, functions, or ephemeral resources. The framework's fixed protocol
// capabilities are also part of the baseline and are checked exactly. Keep
// these assertions separate from the byte comparison above so that a future
// non-empty surface cannot be silently omitted by the projection.
func v3AssertProvider3ProtocolOptionalSurfaces(t *testing.T, response *tfprotov6.GetProviderSchemaResponse) {
	t.Helper()
	if response.ProviderMeta != nil {
		t.Fatalf("Provider 3.0.0 unexpectedly publishes ProviderMeta schema: %#v", response.ProviderMeta)
	}
	if len(response.DataSourceSchemas) != 0 {
		t.Fatalf("Provider 3.0.0 unexpectedly publishes data source schemas: %v", v3Provider3ProtocolMapKeys(response.DataSourceSchemas))
	}
	if len(response.Functions) != 0 {
		t.Fatalf("Provider 3.0.0 unexpectedly publishes functions: %v", v3Provider3ProtocolMapKeys(response.Functions))
	}
	if len(response.EphemeralResourceSchemas) != 0 {
		t.Fatalf("Provider 3.0.0 unexpectedly publishes ephemeral resource schemas: %v", v3Provider3ProtocolMapKeys(response.EphemeralResourceSchemas))
	}
	wantCapabilities := &tfprotov6.ServerCapabilities{
		GetProviderSchemaOptional: true,
		MoveResourceState:         true,
		PlanDestroy:               true,
	}
	if !reflect.DeepEqual(response.ServerCapabilities, wantCapabilities) {
		t.Fatalf("Provider 3.0.0 server capabilities = %#v, want %#v", response.ServerCapabilities, wantCapabilities)
	}
}

func v3Provider3ProtocolMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func readV3Provider3TofuSchema(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(v3Provider3TofuSchemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := formpackage.DigestBytes(raw); got != v3Provider3TofuSchemaDigest {
		t.Fatalf("published Provider 3 schema digest = %s, want %s", got, v3Provider3TofuSchemaDigest)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func v3Provider3TofuSchemaDocument(t *testing.T, response *tfprotov6.GetProviderSchemaResponse) []byte {
	t.Helper()
	resources := make(map[string]any, len(response.ResourceSchemas))
	for name, schema := range response.ResourceSchemas {
		resources[name] = v3Provider3ProtocolSchema(t, schema)
	}
	providerSchema := map[string]any{
		"provider":         v3Provider3ProtocolSchema(t, response.Provider),
		"resource_schemas": resources,
	}
	document := map[string]any{
		"format_version": "1.0",
		"provider_schemas": map[string]any{
			"registry.terraform.io/tako0614/takoform": providerSchema,
		},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

// v3Provider3HistoricalProtocolSchemaDocument is an explicit projection of
// the current protocol response to the immutable v3.0.0 public surface. The
// Provider 3.0.0 binary predates this one provider-only WorkerVersion member;
// no Form/Host shape is removed here, and any unexpected additive member still
// remains visible to the byte comparison. The current lane is asserted by
// TestV3Provider31ProtocolSchemaOwnsApplyIdempotencyKey.
func v3Provider3HistoricalProtocolSchemaDocument(t *testing.T, response *tfprotov6.GetProviderSchemaResponse) []byte {
	t.Helper()
	resources := make(map[string]any, len(response.ResourceSchemas))
	for name, schema := range response.ResourceSchemas {
		resources[name] = v3Provider3ProtocolSchema(t, schema)
	}
	worker, ok := resources["takoform_worker_version"].(map[string]any)
	if !ok {
		t.Fatal("historical protocol projection has no WorkerVersion schema")
	}
	block, ok := worker["block"].(map[string]any)
	if !ok {
		t.Fatal("historical protocol projection has no WorkerVersion block")
	}
	attributes, ok := block["attributes"].(map[string]any)
	if !ok {
		t.Fatal("historical protocol projection has no WorkerVersion attributes")
	}
	if _, ok := attributes["apply_idempotency_key"]; !ok {
		t.Fatal("current WorkerVersion schema has no apply_idempotency_key to project from v3.0.0 history")
	}
	delete(attributes, "apply_idempotency_key")
	return v3Provider3ProtocolSchemaDocument(t, response.Provider, resources)
}

func v3Provider3ProtocolSchemaDocument(t *testing.T, provider *tfprotov6.Schema, resources map[string]any) []byte {
	t.Helper()
	providerSchema := map[string]any{
		"provider":         v3Provider3ProtocolSchema(t, provider),
		"resource_schemas": resources,
	}
	document := map[string]any{
		"format_version": "1.0",
		"provider_schemas": map[string]any{
			"registry.terraform.io/tako0614/takoform": providerSchema,
		},
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}

func v3Provider3ProtocolSchema(t *testing.T, schema *tfprotov6.Schema) map[string]any {
	t.Helper()
	if schema == nil {
		t.Fatal("protocol schema is nil")
	}
	return map[string]any{
		"version": schema.Version,
		"block":   v3Provider3ProtocolBlock(t, schema.Block),
	}
}

func v3Provider3ProtocolBlock(t *testing.T, block *tfprotov6.SchemaBlock) map[string]any {
	t.Helper()
	if block == nil {
		t.Fatal("protocol schema block is nil")
	}
	result := map[string]any{}
	if len(block.Attributes) != 0 {
		attributes := make(map[string]any, len(block.Attributes))
		for _, attribute := range block.Attributes {
			if attribute == nil {
				t.Fatal("protocol schema contains a nil attribute")
			}
			attributes[attribute.Name] = v3Provider3ProtocolAttribute(t, attribute)
		}
		result["attributes"] = attributes
	}
	if len(block.BlockTypes) != 0 {
		blocks := make(map[string]any, len(block.BlockTypes))
		for _, nested := range block.BlockTypes {
			if nested == nil {
				t.Fatal("protocol schema contains a nil nested block")
			}
			entry := map[string]any{
				"nesting_mode": v3Provider3NestedBlockMode(t, nested.Nesting),
				"block":        v3Provider3ProtocolBlock(t, nested.Block),
			}
			if nested.MinItems != 0 {
				entry["min_items"] = nested.MinItems
			}
			if nested.MaxItems != 0 {
				entry["max_items"] = nested.MaxItems
			}
			blocks[nested.TypeName] = entry
		}
		result["block_types"] = blocks
	}
	if block.Description != "" {
		result["description"] = block.Description
		result["description_kind"] = v3Provider3StringKind(t, block.DescriptionKind)
	}
	if block.Deprecated {
		result["deprecated"] = true
	}
	return result
}

func v3Provider3ProtocolAttribute(t *testing.T, attribute *tfprotov6.SchemaAttribute) map[string]any {
	t.Helper()
	result := map[string]any{}
	if attribute.NestedType != nil {
		nested := map[string]any{
			"nesting_mode": v3Provider3NestedObjectMode(t, attribute.NestedType.Nesting),
		}
		members := make(map[string]any, len(attribute.NestedType.Attributes))
		for _, member := range attribute.NestedType.Attributes {
			if member == nil {
				t.Fatal("protocol nested attribute contains a nil member")
			}
			members[member.Name] = v3Provider3ProtocolAttribute(t, member)
		}
		nested["attributes"] = members
		result["nested_type"] = nested
	} else {
		if attribute.Type == nil {
			t.Fatalf("protocol attribute %s has neither type nor nested type", attribute.Name)
		}
		result["type"] = v3Provider3ProtocolType(t, attribute.Type)
	}
	if attribute.Description != "" {
		result["description"] = attribute.Description
		result["description_kind"] = v3Provider3StringKind(t, attribute.DescriptionKind)
	}
	if attribute.Required {
		result["required"] = true
	}
	if attribute.Optional {
		result["optional"] = true
	}
	if attribute.Computed {
		result["computed"] = true
	}
	if attribute.Sensitive {
		result["sensitive"] = true
	}
	if attribute.Deprecated {
		result["deprecated"] = true
	}
	return result
}

func v3Provider3ProtocolType(t *testing.T, value any) any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func v3Provider3StringKind(t *testing.T, kind tfprotov6.StringKind) string {
	t.Helper()
	switch kind {
	case tfprotov6.StringKindPlain:
		return "plain"
	case tfprotov6.StringKindMarkdown:
		return "markdown"
	default:
		t.Fatalf("unknown protocol description kind %d", kind)
		return ""
	}
}

func v3Provider3NestedObjectMode(t *testing.T, mode tfprotov6.SchemaObjectNestingMode) string {
	t.Helper()
	switch mode {
	case tfprotov6.SchemaObjectNestingModeSingle:
		return "single"
	case tfprotov6.SchemaObjectNestingModeList:
		return "list"
	case tfprotov6.SchemaObjectNestingModeSet:
		return "set"
	case tfprotov6.SchemaObjectNestingModeMap:
		return "map"
	default:
		t.Fatalf("unknown protocol nested object mode %d", mode)
		return ""
	}
}

func v3Provider3NestedBlockMode(t *testing.T, mode tfprotov6.SchemaNestedBlockNestingMode) string {
	t.Helper()
	switch mode {
	case tfprotov6.SchemaNestedBlockNestingModeSingle:
		return "single"
	case tfprotov6.SchemaNestedBlockNestingModeList:
		return "list"
	case tfprotov6.SchemaNestedBlockNestingModeSet:
		return "set"
	case tfprotov6.SchemaNestedBlockNestingModeMap:
		return "map"
	case tfprotov6.SchemaNestedBlockNestingModeGroup:
		return "group"
	default:
		t.Fatalf("unknown protocol nested block mode %d", mode)
		return ""
	}
}
