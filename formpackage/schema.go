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
	formRefSchemaID         = "https://forms.takoform.com/schemas/v1alpha1/form-ref.schema.json"
	formDefinitionSchemaID  = "https://forms.takoform.com/schemas/v1alpha1/form-definition.schema.json"
	packageIndexSchemaID    = "https://forms.takoform.com/schemas/v1alpha1/package-index.schema.json"
	portableMapKeyPattern   = `^[A-Za-z][A-Za-z0-9._-]{0,63}$`
	portableMapPolicyKey    = "x-takoform-fieldPolicy"
	portableMapPolicyValue  = "portable-data-only-v1"
	maxSchemaProofDepth     = 64
	maxSchemaProofOps       = 4096
	maxSchemaValidationWork = 16384
	maxConformanceFixtures  = 32
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

type compiledDefinitionSchemas struct {
	desired  *jsonschema.Schema
	observed *jsonschema.Schema
}

func validateDefinitionWithSchemas(raw []byte) (FormDefinition, any, compiledDefinitionSchemas, error) {
	schemas, err := loadSchemas()
	if err != nil {
		return FormDefinition{}, nil, compiledDefinitionSchemas{}, err
	}
	var definition FormDefinition
	var value any
	if err := validateDocumentWithValue(raw, schemas.definition, &definition, &value); err != nil {
		return FormDefinition{}, nil, compiledDefinitionSchemas{}, fmt.Errorf("Form Definition: %w", err)
	}
	if err := rejectForbiddenContent(value, "$"); err != nil {
		return FormDefinition{}, nil, compiledDefinitionSchemas{}, fmt.Errorf("Form Definition content policy: %w", err)
	}
	desired, err := compileInlineSchema(definition.DesiredSchema, "desiredSchema")
	if err != nil {
		return FormDefinition{}, nil, compiledDefinitionSchemas{}, err
	}
	observed, err := compileInlineSchema(definition.ObservedSchema, "observedSchema")
	if err != nil {
		return FormDefinition{}, nil, compiledDefinitionSchemas{}, err
	}
	for index, descriptor := range definition.Interfaces {
		if descriptor.DocumentSchema != nil {
			if _, err := compileInlineSchema(descriptor.DocumentSchema, fmt.Sprintf("interfaces[%d].documentSchema", index)); err != nil {
				return FormDefinition{}, nil, compiledDefinitionSchemas{}, err
			}
		}
	}
	if err := validateDefinitionSemantics(definition); err != nil {
		return FormDefinition{}, nil, compiledDefinitionSchemas{}, err
	}
	return definition, value, compiledDefinitionSchemas{desired: desired, observed: observed}, nil
}

func validateDefinition(raw []byte) (FormDefinition, any, error) {
	definition, value, _, err := validateDefinitionWithSchemas(raw)
	return definition, value, err
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

type schemaProofState uint8

const (
	proofVisiting schemaProofState = iota + 1
	proofDone
)

type schemaProofResult struct {
	mode     objectAdmission
	maxDepth int
}

type schemaProofMemo struct {
	state  schemaProofState
	result schemaProofResult
}

type portableSchemaValidator struct {
	root       any
	memo       map[string]schemaProofMemo
	operations int
}

// validatePortableSchemaStructure proves at every schema node that object
// values are either impossible or constrained by an explicit closed object or
// the exact reviewed typed-map escape. JSON Schema's permissive empty/implicit
// schemas otherwise accept arbitrary objects, so uncertainty fails closed.
func validatePortableSchemaStructure(value any, location string) error {
	validator := portableSchemaValidator{
		root: value,
		memo: make(map[string]schemaProofMemo),
	}
	if _, err := validator.validate(value, location, "#", 0); err != nil {
		return err
	}
	estimator := schemaValidationWorkEstimator{
		root: value,
		memo: make(map[string]schemaValidationWorkMemo),
	}
	work, err := estimator.estimate(value, "#")
	if err != nil {
		return fmt.Errorf("%s runtime validation work: %w", location, err)
	}
	if work > maxSchemaValidationWork {
		return fmt.Errorf("%s worst-case fixture validation work exceeds %d schema evaluations", location, maxSchemaValidationWork)
	}
	return nil
}

func (validator *portableSchemaValidator) validate(value any, location, pointer string, depth int) (schemaProofResult, error) {
	if depth > maxSchemaProofDepth {
		return schemaProofResult{}, fmt.Errorf("%s portable schema closure proof exceeds depth limit %d", location, maxSchemaProofDepth)
	}
	if memo, present := validator.memo[pointer]; present {
		if memo.state == proofVisiting {
			return schemaProofResult{}, fmt.Errorf("%s cyclic schema references are not accepted by the portable closure proof", location)
		}
		if depth+memo.result.maxDepth > maxSchemaProofDepth {
			return schemaProofResult{}, fmt.Errorf("%s portable schema closure proof exceeds depth limit %d", location, maxSchemaProofDepth)
		}
		return memo.result, nil
	}
	if err := validator.consumeOperation(location, "schema node"); err != nil {
		return schemaProofResult{}, err
	}
	validator.memo[pointer] = schemaProofMemo{state: proofVisiting}
	result, err := validator.validateUncached(value, location, pointer, depth)
	if err != nil {
		return schemaProofResult{}, err
	}
	if depth+result.maxDepth > maxSchemaProofDepth {
		return schemaProofResult{}, fmt.Errorf("%s portable schema closure proof exceeds depth limit %d", location, maxSchemaProofDepth)
	}
	validator.memo[pointer] = schemaProofMemo{state: proofDone, result: result}
	return result, nil
}

func (validator *portableSchemaValidator) consumeOperation(location, operation string) error {
	validator.operations++
	if validator.operations > maxSchemaProofOps {
		return fmt.Errorf("%s portable schema closure proof exceeds combined node/ref operation budget %d while proving %s", location, maxSchemaProofOps, operation)
	}
	return nil
}

func (validator *portableSchemaValidator) validateUncached(value any, location, pointer string, depth int) (schemaProofResult, error) {
	if boolean, ok := value.(bool); ok {
		if boolean {
			return schemaProofResult{}, fmt.Errorf("%s boolean true schema can admit arbitrary object values", location)
		}
		return schemaProofResult{mode: objectExcluded}, nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return schemaProofResult{}, fmt.Errorf("%s must be a JSON Schema object or boolean", location)
	}
	if _, present := schema["patternProperties"]; present {
		return schemaProofResult{}, fmt.Errorf("%s patternProperties is forbidden; use the reviewed typed-map escape", location)
	}
	if _, present := schema["dependencies"]; present {
		return schemaProofResult{}, fmt.Errorf("%s legacy dependencies is forbidden; use dependentRequired or dependentSchemas", location)
	}
	for _, keyword := range []string{"contentEncoding", "contentMediaType", "contentSchema"} {
		if _, present := schema[keyword]; present {
			return schemaProofResult{}, fmt.Errorf("%s.%s is forbidden because portable Forms do not decode or transform embedded content", location, keyword)
		}
	}
	if dialect, present := schema["$schema"]; present && dialect != "https://json-schema.org/draft/2020-12/schema" {
		return schemaProofResult{}, fmt.Errorf("%s.$schema must remain Draft 2020-12", location)
	}
	for _, keyword := range []string{"$id", "$anchor", "$dynamicAnchor", "$recursiveAnchor", "$recursiveRef", "$vocabulary"} {
		if _, present := schema[keyword]; present {
			return schemaProofResult{}, fmt.Errorf("%s.%s is forbidden because alternate or recursive resolution scopes cannot be proven closed", location, keyword)
		}
	}

	properties, hasProperties, err := schemaObjectKeyword(schema, "properties", location)
	if err != nil {
		return schemaProofResult{}, err
	}
	if hasProperties {
		for name := range properties {
			if isForbiddenFieldName(name) {
				return schemaProofResult{}, fmt.Errorf("forbidden field %q at %s.properties", name, location)
			}
		}
	}
	if err := validateSchemaFieldNameArray(schema["required"], location+".required"); err != nil {
		return schemaProofResult{}, err
	}
	if err := validateDependentRequiredNames(schema["dependentRequired"], location+".dependentRequired"); err != nil {
		return schemaProofResult{}, err
	}

	hasObjectType := schemaTypeIncludes(schema["type"], "object")
	if schemaTypeIncludes(schema["type"], "array") {
		if _, present := schema["items"]; !present {
			return schemaProofResult{}, fmt.Errorf("%s array schema must declare items so nested object admission is proven closed", location)
		}
	}
	mode := objectOpen
	if hasObjectType {
		if err := validateExplicitObjectClosure(schema, hasProperties, location); err != nil {
			return schemaProofResult{}, err
		}
		mode = objectClosed
	} else if _, typePresent := schema["type"]; typePresent {
		mode = objectExcluded
	} else if hasObjectKeywords(schema) {
		return schemaProofResult{}, fmt.Errorf("%s uses object keywords without explicit type=object and closed additionalProperties", location)
	}
	maxDepth := 0
	recordChild := func(result schemaProofResult) {
		if result.maxDepth+1 > maxDepth {
			maxDepth = result.maxDepth + 1
		}
	}

	for _, keyword := range []string{"$defs", "definitions", "properties", "dependentSchemas"} {
		children, present, err := schemaObjectKeyword(schema, keyword, location)
		if err != nil {
			return schemaProofResult{}, err
		}
		if !present {
			continue
		}
		for name, child := range children {
			childResult, err := validator.validate(child, location+"."+keyword+"."+name, appendSchemaPointer(pointer, keyword, name), depth+1)
			if err != nil {
				return schemaProofResult{}, err
			}
			recordChild(childResult)
		}
	}
	for _, keyword := range []string{"additionalProperties", "items", "contains", "unevaluatedItems", "unevaluatedProperties", "propertyNames", "not", "if", "then", "else"} {
		child, present := schema[keyword]
		if !present || (keyword == "additionalProperties" && child == false) {
			continue
		}
		childResult, err := validator.validate(child, location+"."+keyword, appendSchemaPointer(pointer, keyword), depth+1)
		if err != nil {
			return schemaProofResult{}, err
		}
		recordChild(childResult)
	}

	compoundModes := map[string][]objectAdmission{}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		children, present := schema[keyword]
		if !present {
			continue
		}
		array, ok := children.([]any)
		if !ok || len(array) == 0 {
			return schemaProofResult{}, fmt.Errorf("%s.%s must be a non-empty array of schemas", location, keyword)
		}
		modes := make([]objectAdmission, 0, len(array))
		for index, child := range array {
			childResult, err := validator.validate(child, fmt.Sprintf("%s.%s[%d]", location, keyword, index), appendSchemaPointer(pointer, keyword, strconv.Itoa(index)), depth+1)
			if err != nil {
				return schemaProofResult{}, err
			}
			recordChild(childResult)
			modes = append(modes, childResult.mode)
		}
		compoundModes[keyword] = modes
	}

	if constant, present := schema["const"]; present {
		mode = intersectObjectAdmission(mode, admissionForLiteral(constant))
	}
	if rawEnum, present := schema["enum"]; present {
		values, ok := rawEnum.([]any)
		if !ok || len(values) == 0 {
			return schemaProofResult{}, fmt.Errorf("%s.enum must be a non-empty array", location)
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
			return schemaProofResult{}, fmt.Errorf("%s.$ref must be a string", location)
		}
		if err := validator.consumeOperation(location+".$ref", "local reference"); err != nil {
			return schemaProofResult{}, err
		}
		target, targetPointer, err := resolveLocalSchemaReference(validator.root, reference)
		if err != nil {
			return schemaProofResult{}, fmt.Errorf("%s.$ref: %w", location, err)
		}
		targetResult, err := validator.validate(target, location+".$ref("+reference+")", targetPointer, depth+1)
		if err != nil {
			return schemaProofResult{}, err
		}
		recordChild(targetResult)
		mode = intersectObjectAdmission(mode, targetResult.mode)
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
		return schemaProofResult{}, fmt.Errorf("%s can admit arbitrary object values; declare a non-object type, a closed object, or the reviewed typed-map escape", location)
	}
	return schemaProofResult{mode: mode, maxDepth: maxDepth}, nil
}

func appendSchemaPointer(pointer string, tokens ...string) string {
	var result strings.Builder
	result.WriteString(pointer)
	for _, token := range tokens {
		result.WriteByte('/')
		result.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(token))
	}
	return result.String()
}

type schemaValidationWorkMemo struct {
	state schemaProofState
	work  uint64
}

type schemaValidationWorkEstimator struct {
	root any
	memo map[string]schemaValidationWorkMemo
}

// estimate computes a conservative upper bound for one validation pass. It
// intentionally expands the cost of every local $ref occurrence even though
// target analysis is memoized: the JSON Schema evaluator may revisit a shared
// target for every edge in an allOf/anyOf/oneOf DAG. Definitions themselves do
// not execute unless referenced.
func (estimator *schemaValidationWorkEstimator) estimate(value any, pointer string) (uint64, error) {
	if memo, present := estimator.memo[pointer]; present {
		if memo.state == proofVisiting {
			return 0, fmt.Errorf("cyclic schema reference at %s", pointer)
		}
		return memo.work, nil
	}
	estimator.memo[pointer] = schemaValidationWorkMemo{state: proofVisiting}
	if _, ok := value.(bool); ok {
		estimator.memo[pointer] = schemaValidationWorkMemo{state: proofDone, work: 1}
		return 1, nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("schema node %s is not an object or boolean", pointer)
	}

	work := uint64(1)
	addChild := func(child any, childPointer string) error {
		childWork, err := estimator.estimate(child, childPointer)
		if err != nil {
			return err
		}
		work = saturatingSchemaWorkAdd(work, childWork)
		return nil
	}

	for _, keyword := range []string{"properties", "dependentSchemas"} {
		children, present, err := schemaObjectKeyword(schema, keyword, pointer)
		if err != nil {
			return 0, err
		}
		if !present {
			continue
		}
		for name, child := range children {
			if err := addChild(child, appendSchemaPointer(pointer, keyword, name)); err != nil {
				return 0, err
			}
			if work > maxSchemaValidationWork {
				break
			}
		}
	}
	for _, keyword := range []string{"additionalProperties", "items", "contains", "unevaluatedItems", "unevaluatedProperties", "propertyNames", "not", "if", "then", "else"} {
		child, present := schema[keyword]
		if !present {
			continue
		}
		if err := addChild(child, appendSchemaPointer(pointer, keyword)); err != nil {
			return 0, err
		}
		if work > maxSchemaValidationWork {
			break
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		children, present := schema[keyword]
		if !present {
			continue
		}
		array, ok := children.([]any)
		if !ok {
			return 0, fmt.Errorf("schema node %s/%s is not an array", pointer, keyword)
		}
		for index, child := range array {
			if err := addChild(child, appendSchemaPointer(pointer, keyword, strconv.Itoa(index))); err != nil {
				return 0, err
			}
			if work > maxSchemaValidationWork {
				break
			}
		}
	}
	if reference, present := schema["$ref"]; present {
		text, ok := reference.(string)
		if !ok {
			return 0, fmt.Errorf("schema node %s/$ref is not a string", pointer)
		}
		target, targetPointer, err := resolveLocalSchemaReference(estimator.root, text)
		if err != nil {
			return 0, err
		}
		if err := addChild(target, targetPointer); err != nil {
			return 0, err
		}
	}

	estimator.memo[pointer] = schemaValidationWorkMemo{state: proofDone, work: work}
	return work, nil
}

func saturatingSchemaWorkAdd(left, right uint64) uint64 {
	limit := uint64(maxSchemaValidationWork) + 1
	if left >= limit || right >= limit || left > limit-right {
		return limit
	}
	return left + right
}

type schemaInstanceValidationWorkKey struct {
	schemaPointer   string
	instancePointer string
	instanceRole    string
}

type schemaInstanceValidationWorkEstimator struct {
	root any
	memo map[schemaInstanceValidationWorkKey]schemaValidationWorkMemo
}

// estimate charges schema work against the concrete fixture instance. Schema
// proof and structural work alone cannot bound repeatable keywords: items,
// contains, additionalProperties, and propertyNames can evaluate the same
// shared-reference DAG once per array element or object property. Canonical
// schema/instance pointers plus the value/property-name role are memoized so
// analysis stays linear, while each repeated edge still adds the cached child
// work to the saturating total that guards the real validator call.
func (estimator *schemaInstanceValidationWorkEstimator) estimate(
	schemaValue any,
	schemaPointer string,
	instanceValue any,
	instancePointer string,
	instanceRole string,
) (uint64, error) {
	key := schemaInstanceValidationWorkKey{
		schemaPointer:   schemaPointer,
		instancePointer: instancePointer,
		instanceRole:    instanceRole,
	}
	if memo, present := estimator.memo[key]; present {
		if memo.state == proofVisiting {
			return 0, fmt.Errorf("cyclic schema reference at %s for instance %s", schemaPointer, instancePointer)
		}
		return memo.work, nil
	}
	estimator.memo[key] = schemaValidationWorkMemo{state: proofVisiting}
	if _, ok := schemaValue.(bool); ok {
		estimator.memo[key] = schemaValidationWorkMemo{state: proofDone, work: 1}
		return 1, nil
	}
	schema, ok := schemaValue.(map[string]any)
	if !ok {
		return 0, fmt.Errorf("schema node %s is not an object or boolean", schemaPointer)
	}

	work := uint64(1)
	addChildWithRole := func(childSchema any, childSchemaPointer string, childInstance any, childInstancePointer, childInstanceRole string) error {
		if work > maxSchemaValidationWork {
			return nil
		}
		childWork, err := estimator.estimate(childSchema, childSchemaPointer, childInstance, childInstancePointer, childInstanceRole)
		if err != nil {
			return err
		}
		work = saturatingSchemaWorkAdd(work, childWork)
		return nil
	}
	addChild := func(childSchema any, childSchemaPointer string, childInstance any, childInstancePointer string) error {
		return addChildWithRole(childSchema, childSchemaPointer, childInstance, childInstancePointer, instanceRole)
	}

	object, isObject := instanceValue.(map[string]any)
	properties, hasProperties, err := schemaObjectKeyword(schema, "properties", schemaPointer)
	if err != nil {
		return 0, err
	}
	if isObject && hasProperties {
		for name, childSchema := range properties {
			childInstance, present := object[name]
			if !present {
				continue
			}
			if err := addChild(
				childSchema,
				appendSchemaPointer(schemaPointer, "properties", name),
				childInstance,
				appendSchemaPointer(instancePointer, name),
			); err != nil {
				return 0, err
			}
		}
	}
	dependentSchemas, hasDependentSchemas, err := schemaObjectKeyword(schema, "dependentSchemas", schemaPointer)
	if err != nil {
		return 0, err
	}
	if isObject && hasDependentSchemas {
		for name, childSchema := range dependentSchemas {
			if _, present := object[name]; !present {
				continue
			}
			if err := addChild(
				childSchema,
				appendSchemaPointer(schemaPointer, "dependentSchemas", name),
				instanceValue,
				instancePointer,
			); err != nil {
				return 0, err
			}
		}
	}

	if isObject {
		if childSchema, present := schema["additionalProperties"]; present {
			for name, childInstance := range object {
				if _, declared := properties[name]; declared {
					continue
				}
				if err := addChild(
					childSchema,
					appendSchemaPointer(schemaPointer, "additionalProperties"),
					childInstance,
					appendSchemaPointer(instancePointer, name),
				); err != nil {
					return 0, err
				}
			}
		}
		if childSchema, present := schema["propertyNames"]; present {
			for name := range object {
				if err := addChildWithRole(
					childSchema,
					appendSchemaPointer(schemaPointer, "propertyNames"),
					name,
					appendSchemaPointer(instancePointer, "@propertyName", name),
					"property-name",
				); err != nil {
					return 0, err
				}
			}
		}
		// Evaluation annotations are deliberately not reimplemented here. Applying
		// unevaluatedProperties to every property is a safe upper bound.
		if childSchema, present := schema["unevaluatedProperties"]; present {
			for name, childInstance := range object {
				if err := addChild(
					childSchema,
					appendSchemaPointer(schemaPointer, "unevaluatedProperties"),
					childInstance,
					appendSchemaPointer(instancePointer, name),
				); err != nil {
					return 0, err
				}
			}
		}
	}

	array, isArray := instanceValue.([]any)
	prefixItems, _ := schema["prefixItems"].([]any)
	if isArray {
		for index, childSchema := range prefixItems {
			if index >= len(array) {
				break
			}
			if err := addChild(
				childSchema,
				appendSchemaPointer(schemaPointer, "prefixItems", strconv.Itoa(index)),
				array[index],
				appendSchemaPointer(instancePointer, strconv.Itoa(index)),
			); err != nil {
				return 0, err
			}
		}
		if childSchema, present := schema["items"]; present {
			for index := len(prefixItems); index < len(array); index++ {
				if err := addChild(
					childSchema,
					appendSchemaPointer(schemaPointer, "items"),
					array[index],
					appendSchemaPointer(instancePointer, strconv.Itoa(index)),
				); err != nil {
					return 0, err
				}
			}
		}
		if childSchema, present := schema["contains"]; present {
			for index, childInstance := range array {
				if err := addChild(
					childSchema,
					appendSchemaPointer(schemaPointer, "contains"),
					childInstance,
					appendSchemaPointer(instancePointer, strconv.Itoa(index)),
				); err != nil {
					return 0, err
				}
			}
		}
		// Applying unevaluatedItems to every item safely overestimates evaluation
		// without duplicating the validator's annotation machinery.
		if childSchema, present := schema["unevaluatedItems"]; present {
			for index, childInstance := range array {
				if err := addChild(
					childSchema,
					appendSchemaPointer(schemaPointer, "unevaluatedItems"),
					childInstance,
					appendSchemaPointer(instancePointer, strconv.Itoa(index)),
				); err != nil {
					return 0, err
				}
			}
		}
	}

	for _, keyword := range []string{"not", "if", "then", "else"} {
		childSchema, present := schema[keyword]
		if !present {
			continue
		}
		if err := addChild(
			childSchema,
			appendSchemaPointer(schemaPointer, keyword),
			instanceValue,
			instancePointer,
		); err != nil {
			return 0, err
		}
	}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		children, present := schema[keyword]
		if !present {
			continue
		}
		array, ok := children.([]any)
		if !ok {
			return 0, fmt.Errorf("schema node %s/%s is not an array", schemaPointer, keyword)
		}
		for index, childSchema := range array {
			if err := addChild(
				childSchema,
				appendSchemaPointer(schemaPointer, keyword, strconv.Itoa(index)),
				instanceValue,
				instancePointer,
			); err != nil {
				return 0, err
			}
		}
	}
	if reference, present := schema["$ref"]; present {
		text, ok := reference.(string)
		if !ok {
			return 0, fmt.Errorf("schema node %s/$ref is not a string", schemaPointer)
		}
		target, targetPointer, err := resolveLocalSchemaReference(estimator.root, text)
		if err != nil {
			return 0, err
		}
		if err := addChild(target, targetPointer, instanceValue, instancePointer); err != nil {
			return 0, err
		}
	}

	estimator.memo[key] = schemaValidationWorkMemo{state: proofDone, work: work}
	return work, nil
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

func resolveLocalSchemaReference(root any, reference string) (any, string, error) {
	if reference == "#" {
		return root, "#", nil
	}
	if !strings.HasPrefix(reference, "#/") {
		return nil, "", fmt.Errorf("only root or JSON Pointer fragments are supported")
	}
	pointer, err := url.PathUnescape(reference[1:])
	if err != nil {
		return nil, "", fmt.Errorf("decode fragment: %w", err)
	}
	current := root
	canonicalPointer := "#"
	for _, rawToken := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		token, err := decodeJSONPointerToken(rawToken)
		if err != nil {
			return nil, "", err
		}
		canonicalPointer = appendSchemaPointer(canonicalPointer, token)
		switch typed := current.(type) {
		case map[string]any:
			child, ok := typed[token]
			if !ok {
				return nil, "", fmt.Errorf("fragment token %q does not exist", token)
			}
			current = child
		case []any:
			if token == "-" || (len(token) > 1 && token[0] == '0') {
				return nil, "", fmt.Errorf("fragment array token %q is invalid", token)
			}
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, "", fmt.Errorf("fragment array token %q is out of range", token)
			}
			current = typed[index]
		default:
			return nil, "", fmt.Errorf("fragment traverses non-container at %q", token)
		}
	}
	return current, canonicalPointer, nil
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
	if len(definition.ConformanceFixtures) > maxConformanceFixtures {
		return fmt.Errorf("Form Definition has %d conformance fixtures; maximum is %d", len(definition.ConformanceFixtures), maxConformanceFixtures)
	}
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
