package formcatalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

type compiledRuntimeSchema struct {
	schema *jsonschema.Schema
	err    error
}

var runtimeSchemaCache sync.Map

// ValidateDesired validates one Resource spec against the exact immutable
// desiredSchema compiled into this provider build. The schema is closed, so a
// host cannot add target, credential, backend, or other authority-bearing
// fields and have them enter provider state.
func (k Kind) ValidateDesired(value map[string]any) error {
	return validateRuntimeDocument("desired", k.Kind, k.DesiredSchema(), value)
}

// ValidateObserved validates one host-produced observed document against the
// exact portable lifecycle contract compiled for this provider build.
func (k Kind) ValidateObserved(value map[string]any) error {
	return validateRuntimeDocument("observed", k.Kind, k.ObservedSchema(), value)
}

// ValidateOutput validates one host-produced public output document against
// this exact Form's closed output contract. Arbitrary host metadata, backend
// details, credentials, and undeclared output keys are rejected by that
// contract before provider state can consume them.
func (k Kind) ValidateOutput(value map[string]any) error {
	return validateRuntimeDocument("output", k.Kind, k.OutputSchema(), value)
}

func validateRuntimeDocument(document, kind string, schemaValue, value map[string]any) error {
	if value == nil {
		return fmt.Errorf("%s document is missing", document)
	}
	// A maintenance provider can carry more than one exact closed codec for a
	// Kind. Include the schema's canonical digest in the cache key so a retained
	// codec cannot poison the current codec (or vice versa) merely because both
	// share the same Kind token.
	schemaRaw, err := json.Marshal(schemaValue)
	if err != nil {
		return fmt.Errorf("encode %s schema for cache identity: %w", document, err)
	}
	schemaDigest, err := formpackage.DigestCanonicalJSON(schemaRaw)
	if err != nil {
		return fmt.Errorf("digest %s schema for cache identity: %w", document, err)
	}
	key := document + ":" + kind + ":" + schemaDigest
	cached, ok := runtimeSchemaCache.Load(key)
	if !ok {
		compiled := compiledRuntimeSchema{}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.AssertFormat()
		schemaID := "https://forms.takoform.invalid/runtime/" + document + "/" + kind + ".schema.json"
		normalizedSchema, err := normalizeRuntimeJSON(schemaValue)
		if err != nil {
			compiled.err = fmt.Errorf("normalize %s schema: %w", document, err)
		} else if err := compiler.AddResource(schemaID, normalizedSchema); err != nil {
			compiled.err = fmt.Errorf("register %s schema: %w", document, err)
		} else {
			compiled.schema, compiled.err = compiler.Compile(schemaID)
			if compiled.err != nil {
				compiled.err = fmt.Errorf("compile %s schema: %w", document, compiled.err)
			}
		}
		cached, _ = runtimeSchemaCache.LoadOrStore(key, compiled)
	}
	compiled := cached.(compiledRuntimeSchema)
	if compiled.err != nil {
		return compiled.err
	}
	normalizedValue, err := normalizeRuntimeJSON(value)
	if err != nil {
		return fmt.Errorf("%s document is not valid JSON: %w", document, err)
	}
	if err := formpackage.ValidatePortableData(normalizedValue); err != nil {
		return fmt.Errorf("%s document violates the portable data-only policy: %w", document, err)
	}
	if err := compiled.schema.Validate(normalizedValue); err != nil {
		return fmt.Errorf("%s document does not satisfy the exact %s Form contract: %w", document, kind, err)
	}
	return nil
}

// normalizeRuntimeJSON converts Go-specific collection and number types into
// the exact data model consumed by the JSON Schema validator. The catalog is
// intentionally convenient Go data (for example []string), while Host status
// is a JSON document; validation must operate on the latter representation.
func normalizeRuntimeJSON(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(raw))
}
