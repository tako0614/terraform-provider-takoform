package formpackage

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	formRefSchemaID        = "https://forms.takoform.com/schemas/v1alpha1/form-ref.schema.json"
	formDefinitionSchemaID = "https://forms.takoform.com/schemas/v1alpha1/form-definition.schema.json"
	packageIndexSchemaID   = "https://forms.takoform.com/schemas/v1alpha1/package-index.schema.json"
	portableMapKeyPattern  = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
	portableMapPolicyKey   = "x-takoform-fieldPolicy"
	portableMapPolicyValue = "portable-data-only-v1"
)

//go:embed schemas/*.schema.json
var schemaFiles embed.FS

type closedSchemaLoader struct{}

func (closedSchemaLoader) Load(resourceURL string) (any, error) {
	return nil, fmt.Errorf("schema resource %q is outside the embedded Form Package schema closure", resourceURL)
}

type compiledSchemas struct {
	formRef    *jsonschema.Schema
	definition *jsonschema.Schema
	index      *jsonschema.Schema
}

var (
	schemasOnce  sync.Once
	schemasValue compiledSchemas
	schemasErr   error
)

func loadSchemas() (compiledSchemas, error) {
	schemasOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.AssertFormat()
		compiler.UseLoader(closedSchemaLoader{})
		files := []string{"form-ref.schema.json", "form-definition.schema.json", "package-index.schema.json"}
		entries, err := schemaFiles.ReadDir("schemas")
		if err != nil {
			schemasErr = fmt.Errorf("read embedded schema closure: %w", err)
			return
		}
		if len(entries) != len(files) {
			schemasErr = fmt.Errorf("embedded schema closure has %d entries, want %d", len(entries), len(files))
			return
		}
		wantFiles := make(map[string]struct{}, len(files))
		for _, file := range files {
			wantFiles[file] = struct{}{}
		}
		for _, entry := range entries {
			if entry.IsDir() {
				schemasErr = fmt.Errorf("embedded schema closure contains directory %q", entry.Name())
				return
			}
			if _, ok := wantFiles[entry.Name()]; !ok {
				schemasErr = fmt.Errorf("embedded schema closure contains unexpected file %q", entry.Name())
				return
			}
		}
		for _, file := range files {
			raw, err := schemaFiles.ReadFile("schemas/" + file)
			if err != nil {
				schemasErr = fmt.Errorf("read embedded schema %s: %w", file, err)
				return
			}
			if _, err := Canonicalize(raw); err != nil {
				schemasErr = fmt.Errorf("embedded schema %s is not I-JSON: %w", file, err)
				return
			}
			value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				schemasErr = fmt.Errorf("decode embedded schema %s: %w", file, err)
				return
			}
			object, ok := value.(map[string]any)
			if !ok || object["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
				schemasErr = fmt.Errorf("embedded schema %s must declare Draft 2020-12", file)
				return
			}
			id, ok := object["$id"].(string)
			if !ok || id == "" {
				schemasErr = fmt.Errorf("embedded schema %s has no $id", file)
				return
			}
			if err := compiler.AddResource(id, value); err != nil {
				schemasErr = fmt.Errorf("register embedded schema %s: %w", file, err)
				return
			}
		}
		schemasValue.formRef, schemasErr = compiler.Compile(formRefSchemaID)
		if schemasErr != nil {
			schemasErr = fmt.Errorf("compile FormRef schema: %w", schemasErr)
			return
		}
		schemasValue.definition, schemasErr = compiler.Compile(formDefinitionSchemaID)
		if schemasErr != nil {
			schemasErr = fmt.Errorf("compile Form Definition schema: %w", schemasErr)
			return
		}
		schemasValue.index, schemasErr = compiler.Compile(packageIndexSchemaID)
		if schemasErr != nil {
			schemasErr = fmt.Errorf("compile package-index schema: %w", schemasErr)
		}
	})
	return schemasValue, schemasErr
}

func validateFormRef(raw []byte) (FormRef, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return FormRef{}, err
	}
	var ref FormRef
	if err := validateDocument(raw, schemas.formRef, &ref); err != nil {
		return FormRef{}, fmt.Errorf("FormRef: %w", err)
	}
	return ref, nil
}

// ValidateFormRef validates the exact four-field Draft 2020-12 FormRef and
// returns its typed value.
func ValidateFormRef(raw []byte) (FormRef, error) {
	return validateFormRef(raw)
}

func validateDefinition(raw []byte) (FormDefinition, any, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return FormDefinition{}, nil, err
	}
	var definition FormDefinition
	var value any
	if err := validateDocumentWithValue(raw, schemas.definition, &definition, &value); err != nil {
		return FormDefinition{}, nil, fmt.Errorf("Form Definition: %w", err)
	}
	if err := rejectForbiddenContent(value, "$"); err != nil {
		return FormDefinition{}, nil, fmt.Errorf("Form Definition content policy: %w", err)
	}
	if _, err := compileInlineSchema(definition.DesiredSchema, "desiredSchema"); err != nil {
		return FormDefinition{}, nil, err
	}
	if _, err := compileInlineSchema(definition.ObservedSchema, "observedSchema"); err != nil {
		return FormDefinition{}, nil, err
	}
	for index, descriptor := range definition.Interfaces {
		if descriptor.DocumentSchema != nil {
			if _, err := compileInlineSchema(descriptor.DocumentSchema, fmt.Sprintf("interfaces[%d].documentSchema", index)); err != nil {
				return FormDefinition{}, nil, err
			}
		}
	}
	if err := validateDefinitionSemantics(definition); err != nil {
		return FormDefinition{}, nil, err
	}
	return definition, value, nil
}

// ValidateDefinition validates the Draft 2020-12 Form Definition, its inline
// schemas, and the fail-closed data-only content policy.
func ValidateDefinition(raw []byte) (FormDefinition, error) {
	definition, _, err := validateDefinition(raw)
	return definition, err
}

func validateIndex(raw []byte) (PackageIndex, any, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return PackageIndex{}, nil, err
	}
	var index PackageIndex
	var value any
	if err := validateDocumentWithValue(raw, schemas.index, &index, &value); err != nil {
		return PackageIndex{}, nil, fmt.Errorf("package index: %w", err)
	}
	return index, value, nil
}

// ValidatePackageIndex validates the exact Draft 2020-12 package-index
// document. Filesystem closure and payload bytes are verified separately by
// VerifyDirectory.
func ValidatePackageIndex(raw []byte) (PackageIndex, error) {
	index, _, err := validateIndex(raw)
	return index, err
}

func validateDocument(raw []byte, schema *jsonschema.Schema, destination any) error {
	var value any
	return validateDocumentWithValue(raw, schema, destination, &value)
}

func validateDocumentWithValue(raw []byte, schema *jsonschema.Schema, destination, valueDestination any) error {
	if _, err := Canonicalize(raw); err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("does not satisfy Draft 2020-12 schema: %w", err)
	}
	if err := json.Unmarshal(raw, destination); err != nil {
		return err
	}
	if pointer, ok := valueDestination.(*any); ok {
		*pointer = value
	}
	return nil
}

func compileInlineSchema(value map[string]any, field string) (*jsonschema.Schema, error) {
	if err := verifyFragmentOnlyReferences(value, field); err != nil {
		return nil, err
	}
	if err := validatePortableSchemaStructure(value, field); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(closedSchemaLoader{})
	id := "https://forms.takoform.com/inline/" + url.PathEscape(field) + ".schema.json"
	if err := compiler.AddResource(id, value); err != nil {
		return nil, fmt.Errorf("%s register Draft 2020-12 schema: %w", field, err)
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		return nil, fmt.Errorf("%s compile Draft 2020-12 schema: %w", field, err)
	}
	return compiled, nil
}

type objectAdmission uint8

const (
	objectOpen objectAdmission = iota
	objectClosed
	objectExcluded
)

type portableSchemaValidator struct {
	root any
}

// validatePortableSchemaStructure proves at every schema node that object
// values are either impossible or constrained by an explicit closed object or
// the exact reviewed typed-map escape. JSON Schema's permissive empty/implicit
// schemas otherwise accept arbitrary objects, so uncertainty fails closed.
func validatePortableSchemaStructure(value any, location string) error {
	validator := portableSchemaValidator{root: value}
	_, err := validator.validate(value, location, nil)
	return err
}

func (validator portableSchemaValidator) validate(value any, location string, references []string) (objectAdmission, error) {
	if boolean, ok := value.(bool); ok {
		if boolean {
			return objectOpen, fmt.Errorf("%s boolean true schema can admit arbitrary object values", location)
		}
		return objectExcluded, nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return objectOpen, fmt.Errorf("%s must be a JSON Schema object or boolean", location)
	}
	if _, present := schema["patternProperties"]; present {
		return objectOpen, fmt.Errorf("%s patternProperties is forbidden; use the reviewed typed-map escape", location)
	}
	if dialect, present := schema["$schema"]; present && dialect != "https://json-schema.org/draft/2020-12/schema" {
		return objectOpen, fmt.Errorf("%s.$schema must remain Draft 2020-12", location)
	}
	for _, keyword := range []string{"$id", "$anchor", "$dynamicAnchor", "$recursiveAnchor", "$recursiveRef", "$vocabulary"} {
		if _, present := schema[keyword]; present {
			return objectOpen, fmt.Errorf("%s.%s is forbidden because alternate or recursive resolution scopes cannot be proven closed", location, keyword)
		}
	}

	properties, hasProperties, err := schemaObjectKeyword(schema, "properties", location)
	if err != nil {
		return objectOpen, err
	}
	if hasProperties {
		for name := range properties {
			if isForbiddenFieldName(name) {
				return objectOpen, fmt.Errorf("forbidden field %q at %s.properties", name, location)
			}
		}
	}
	if err := validateSchemaFieldNameArray(schema["required"], location+".required"); err != nil {
		return objectOpen, err
	}
	if err := validateDependentRequiredNames(schema["dependentRequired"], location+".dependentRequired"); err != nil {
		return objectOpen, err
	}

	hasObjectType := schemaTypeIncludes(schema["type"], "object")
	if schemaTypeIncludes(schema["type"], "array") {
		if _, present := schema["items"]; !present {
			return objectOpen, fmt.Errorf("%s array schema must declare items so nested object admission is proven closed", location)
		}
	}
	mode := objectOpen
	if hasObjectType {
		if err := validateExplicitObjectClosure(schema, hasProperties, location); err != nil {
			return objectOpen, err
		}
		mode = objectClosed
	} else if _, typePresent := schema["type"]; typePresent {
		mode = objectExcluded
	} else if hasObjectKeywords(schema) {
		return objectOpen, fmt.Errorf("%s uses object keywords without explicit type=object and closed additionalProperties", location)
	}

	for _, keyword := range []string{"$defs", "definitions", "properties", "dependentSchemas"} {
		children, present, err := schemaObjectKeyword(schema, keyword, location)
		if err != nil {
			return objectOpen, err
		}
		if !present {
			continue
		}
		for name, child := range children {
			if _, err := validator.validate(child, location+"."+keyword+"."+name, references); err != nil {
				return objectOpen, err
			}
		}
	}
	for _, keyword := range []string{"additionalProperties", "items", "contains", "contentSchema", "unevaluatedItems", "unevaluatedProperties", "propertyNames", "not", "if", "then", "else"} {
		child, present := schema[keyword]
		if !present || (keyword == "additionalProperties" && child == false) {
			continue
		}
		if _, err := validator.validate(child, location+"."+keyword, references); err != nil {
			return objectOpen, err
		}
	}

	compoundModes := map[string][]objectAdmission{}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		children, present := schema[keyword]
		if !present {
			continue
		}
		array, ok := children.([]any)
		if !ok || len(array) == 0 {
			return objectOpen, fmt.Errorf("%s.%s must be a non-empty array of schemas", location, keyword)
		}
		modes := make([]objectAdmission, 0, len(array))
		for index, child := range array {
			childMode, err := validator.validate(child, fmt.Sprintf("%s.%s[%d]", location, keyword, index), references)
			if err != nil {
				return objectOpen, err
			}
			modes = append(modes, childMode)
		}
		compoundModes[keyword] = modes
	}

	if constant, present := schema["const"]; present {
		mode = intersectObjectAdmission(mode, admissionForLiteral(constant))
	}
	if rawEnum, present := schema["enum"]; present {
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 {
			return objectOpen, fmt.Errorf("%s.enum must be a non-empty array", location)
		}
		enumMode := objectExcluded
		for _, candidate := range values {
			enumMode = unionObjectAdmission(enumMode, admissionForLiteral(candidate))
		}
		mode = intersectObjectAdmission(mode, enumMode)
	}
	if referenceValue, present := schema["$ref"]; present {
		reference, ok := referenceValue.(string)
		if !ok {
			return objectOpen, fmt.Errorf("%s.$ref must be a string", location)
		}
		target, err := resolveLocalSchemaReference(validator.root, reference)
		if err != nil {
			return objectOpen, fmt.Errorf("%s.$ref: %w", location, err)
		}
		for _, active := range references {
			if active == reference {
				return objectOpen, fmt.Errorf("%s.$ref cyclic schema references are not accepted by the portable closure proof", location)
			}
		}
		targetMode, err := validator.validate(target, location+".$ref("+reference+")", append(references, reference))
		if err != nil {
			return objectOpen, err
		}
		mode = intersectObjectAdmission(mode, targetMode)
	}
	if modes, present := compoundModes["allOf"]; present {
		allMode := objectOpen
		for _, candidate := range modes {
			allMode = intersectObjectAdmission(allMode, candidate)
		}
		mode = intersectObjectAdmission(mode, allMode)
	}
	for _, keyword := range []string{"anyOf", "oneOf"} {
		if modes, present := compoundModes[keyword]; present {
			unionMode := objectExcluded
			for _, candidate := range modes {
				unionMode = unionObjectAdmission(unionMode, candidate)
			}
			mode = intersectObjectAdmission(mode, unionMode)
		}
	}
	if mode == objectOpen {
		return objectOpen, fmt.Errorf("%s can admit arbitrary object values; declare a non-object type, a closed object, or the reviewed typed-map escape", location)
	}
	return mode, nil
}

func validateExplicitObjectClosure(schema map[string]any, hasProperties bool, location string) error {
	additional, present := schema["additionalProperties"]
	switch typed := additional.(type) {
	case bool:
		if !present || typed {
			return fmt.Errorf("%s object schema must set additionalProperties to false or use the reviewed typed-map escape", location)
		}
	case map[string]any:
		if hasProperties || schema["required"] != nil || schema["dependentRequired"] != nil || schema["dependentSchemas"] != nil || schema["unevaluatedProperties"] != nil {
			return fmt.Errorf("%s typed map must be a pure map without fixed or dependent properties", location)
		}
		if err := validatePortableMapPropertyNames(schema["propertyNames"], location+".propertyNames"); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%s object schema must set additionalProperties to false or a typed schema", location)
	}
	return nil
}

func schemaObjectKeyword(schema map[string]any, keyword, location string) (map[string]any, bool, error) {
	value, present := schema[keyword]
	if !present {
		return nil, false, nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, true, fmt.Errorf("%s.%s must be an object", location, keyword)
	}
	return object, true, nil
}

func hasObjectKeywords(schema map[string]any) bool {
	for _, keyword := range []string{"properties", "required", "additionalProperties", "unevaluatedProperties", "propertyNames", "dependentRequired", "dependentSchemas", "minProperties", "maxProperties"} {
		if _, present := schema[keyword]; present {
			return true
		}
	}
	return false
}

func schemaTypeIncludes(value any, wanted string) bool {
	switch typed := value.(type) {
	case string:
		return typed == wanted
	case []any:
		for _, candidate := range typed {
			if candidate == wanted {
				return true
			}
		}
	}
	return false
}

func validatePortableMapPropertyNames(value any, location string) error {
	propertyNames, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must declare the reviewed portable map-key policy", location)
	}
	if len(propertyNames) != 3 || propertyNames["type"] != "string" || propertyNames["pattern"] != portableMapKeyPattern || propertyNames[portableMapPolicyKey] != portableMapPolicyValue {
		return fmt.Errorf("%s must be exactly type=string, pattern=%q, and %s=%q", location, portableMapKeyPattern, portableMapPolicyKey, portableMapPolicyValue)
	}
	return nil
}

func admissionForLiteral(value any) objectAdmission {
	if _, object := value.(map[string]any); object {
		return objectOpen
	}
	return objectExcluded
}

func intersectObjectAdmission(left, right objectAdmission) objectAdmission {
	if left == objectExcluded || right == objectExcluded {
		return objectExcluded
	}
	if left == objectClosed || right == objectClosed {
		return objectClosed
	}
	return objectOpen
}

func unionObjectAdmission(left, right objectAdmission) objectAdmission {
	if left == objectOpen || right == objectOpen {
		return objectOpen
	}
	if left == objectClosed || right == objectClosed {
		return objectClosed
	}
	return objectExcluded
}

func resolveLocalSchemaReference(root any, reference string) (any, error) {
	if reference == "#" {
		return root, nil
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, fmt.Errorf("only root or JSON Pointer fragments are supported")
	}
	pointer, err := url.PathUnescape(reference[1:])
	if err != nil {
		return nil, fmt.Errorf("decode fragment: %w", err)
	}
	current := root
	for _, rawToken := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		token, err := decodeJSONPointerToken(rawToken)
		if err != nil {
			return nil, err
		}
		switch typed := current.(type) {
		case map[string]any:
			child, ok := typed[token]
			if !ok {
				return nil, fmt.Errorf("fragment token %q does not exist", token)
			}
			current = child
		case []any:
			if token == "-" || (len(token) > 1 && token[0] == '0') {
				return nil, fmt.Errorf("fragment array token %q is invalid", token)
			}
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("fragment array token %q is out of range", token)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("fragment traverses non-container at %q", token)
		}
	}
	return current, nil
}

func decodeJSONPointerToken(value string) (string, error) {
	var decoded strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '~' {
			decoded.WriteByte(value[index])
			continue
		}
		if index+1 >= len(value) || (value[index+1] != '0' && value[index+1] != '1') {
			return "", fmt.Errorf("fragment contains invalid JSON Pointer escape")
		}
		index++
		if value[index] == '0' {
			decoded.WriteByte('~')
		} else {
			decoded.WriteByte('/')
		}
	}
	return decoded.String(), nil
}

func validateSchemaFieldNameArray(value any, location string) error {
	if value == nil {
		return nil
	}
	values, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array", location)
	}
	for _, value := range values {
		name, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s entries must be strings", location)
		}
		if isForbiddenFieldName(name) {
			return fmt.Errorf("forbidden field %q at %s", name, location)
		}
	}
	return nil
}

func validateDependentRequiredNames(value any, location string) error {
	if value == nil {
		return nil
	}
	dependencies, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be an object", location)
	}
	for name, required := range dependencies {
		if isForbiddenFieldName(name) {
			return fmt.Errorf("forbidden field %q at %s", name, location)
		}
		if err := validateSchemaFieldNameArray(required, location+"."+name); err != nil {
			return err
		}
	}
	return nil
}

func validateDefinitionSemantics(definition FormDefinition) error {
	interfaces := map[string]struct{}{}
	for _, descriptor := range definition.Interfaces {
		key := descriptor.Name + "@" + descriptor.Version
		if _, duplicate := interfaces[key]; duplicate {
			return fmt.Errorf("Form Definition has duplicate Interface %q", key)
		}
		interfaces[key] = struct{}{}
	}
	fixtures := map[string]struct{}{}
	for _, fixture := range definition.ConformanceFixtures {
		if _, duplicate := fixtures[fixture.Name]; duplicate {
			return fmt.Errorf("Form Definition has duplicate conformance fixture name %q", fixture.Name)
		}
		fixtures[fixture.Name] = struct{}{}
		if err := validatePackagePath(fixture.DesiredPath); err != nil {
			return fmt.Errorf("conformance fixture %q desiredPath: %w", fixture.Name, err)
		}
		if fixture.ObservedPath != "" {
			if err := validatePackagePath(fixture.ObservedPath); err != nil {
				return fmt.Errorf("conformance fixture %q observedPath: %w", fixture.Name, err)
			}
		}
	}
	return nil
}

func verifyFragmentOnlyReferences(value any, location string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childLocation := location + "." + key
			if key == "$dynamicRef" {
				return fmt.Errorf("%s is forbidden because dynamic resolution cannot be proven closed", childLocation)
			}
			if key == "$ref" {
				reference, ok := child.(string)
				if !ok || (reference != "#" && !strings.HasPrefix(reference, "#/")) {
					return fmt.Errorf("%s must be a document-local fragment using the root or a JSON Pointer; network, package-path, anchor, and dynamic references are forbidden", childLocation)
				}
			}
			if err := verifyFragmentOnlyReferences(child, childLocation); err != nil {
				return err
			}
		}
	case []any:
		for index, child := range typed {
			if err := verifyFragmentOnlyReferences(child, fmt.Sprintf("%s[%d]", location, index)); err != nil {
				return err
			}
		}
	}
	return nil
}
