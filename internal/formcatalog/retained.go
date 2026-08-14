package formcatalog

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// The retained bytes are copies of the immutable definitions under
// forms/releases. They are embedded beside the provider codec because Go
// embed patterns cannot reach outside this package. Tests compare each copy
// byte-for-byte with the published release artifact, while the initializer
// also checks its RFC 8785 FormRef digest.
//
//go:embed retained-definitions/edge-worker-3.0.0.json
var edgeWorker3DefinitionJSON []byte

//go:embed retained-definitions/relational-database-2.0.0.json
var relationalDatabase2DefinitionJSON []byte

type retainedFormDefinition struct {
	raw   []byte
	value formpackage.FormDefinition
}

// retainedCodec declares one historical wire shape that this maintenance
// provider can still decode. Historical shapes are closed: fields introduced
// by a successor are explicitly removed instead of being accepted because the
// Kind token happens to match.
type retainedCodec struct {
	kind          string
	version       string
	removedFields map[string]struct{}
	definition    *retainedFormDefinition
}

var retainedCodecs = []retainedCodec{
	{
		kind: "RelationalDatabase", version: "2.0.0",
		removedFields: fieldSet("schema_url", "schema_sha256", "schema_format"),
		definition: mustRetainedFormDefinition(
			relationalDatabase2DefinitionJSON,
			"RelationalDatabase",
			"2.0.0",
			"sha256:3898f8ee507bcebd9e03e80fbc1931b67b477299b1ebe2ff395facb7acf018de",
		),
	},
	{
		kind: "EdgeWorker", version: "3.0.0",
		removedFields: fieldSet("assets_path", "assets_not_found_handling"),
		definition: mustRetainedFormDefinition(
			edgeWorker3DefinitionJSON,
			"EdgeWorker",
			"3.0.0",
			"sha256:c7fb07db10c937fd6ab119b192552ac239cbcad45dcc12bccd7993decffd2781",
		),
	},
}

func mustRetainedFormDefinition(raw []byte, kind, version, wantDigest string) *retainedFormDefinition {
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		panic(fmt.Errorf("takoform: digest retained %s@%s Form Definition: %w", kind, version, err))
	}
	if digest != wantDigest {
		panic(fmt.Errorf(
			"takoform: retained %s@%s Form Definition digest = %s, want %s",
			kind,
			version,
			digest,
			wantDigest,
		))
	}
	var definition formpackage.FormDefinition
	if err := json.Unmarshal(raw, &definition); err != nil {
		panic(fmt.Errorf("takoform: decode retained %s@%s Form Definition: %w", kind, version, err))
	}
	if definition.APIVersion != formpackage.FormAPIVersion || definition.Kind != kind ||
		definition.DefinitionVersion != version || definition.DesiredSchema == nil ||
		definition.ObservedSchema == nil || definition.OutputSchema == nil {
		panic(fmt.Errorf("takoform: retained %s@%s Form Definition is incomplete", kind, version))
	}
	return &retainedFormDefinition{raw: append([]byte(nil), raw...), value: definition}
}

func fieldSet(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		out[name] = struct{}{}
	}
	return out
}

// ByKindVersion returns only an exact current or explicitly retained codec.
// It deliberately performs no SemVer inference.
func ByKindVersion(kind, definitionVersion string) (Kind, bool) {
	current, ok := ByKind(kind)
	if !ok {
		return Kind{}, false
	}
	if current.Version() == definitionVersion {
		return current, true
	}
	for _, retained := range retainedCodecs {
		if retained.kind != kind || retained.version != definitionVersion {
			continue
		}
		codec := current
		codec.DefinitionVersion = retained.version
		codec.exactDefinition = retained.definition
		codec.Fields = make([]Field, 0, len(current.Fields)-len(retained.removedFields))
		for _, field := range current.Fields {
			if _, removed := retained.removedFields[field.HCL]; !removed {
				codec.Fields = append(codec.Fields, field)
			}
		}
		return codec, true
	}
	return Kind{}, false
}

func retainedDefinitionBytes(kind, definitionVersion string) ([]byte, bool) {
	for _, retained := range retainedCodecs {
		if retained.kind == kind && retained.version == definitionVersion {
			return append([]byte(nil), retained.definition.raw...), true
		}
	}
	return nil, false
}
