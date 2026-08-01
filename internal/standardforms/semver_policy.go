package standardforms

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

var stableFormVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type stableFormVersion struct {
	Raw                 string
	Major, Minor, Patch uint64
}

type formSchemaRelease struct {
	Kind          string
	Version       stableFormVersion
	DesiredSchema any
}

// VerifyFormSemVerHistory checks every retained Form release source without
// rewriting it. Patch releases must preserve the canonical desiredSchema
// exactly. Minor releases pass only when the conservative schema proof can
// demonstrate that every desired document accepted by every earlier release
// in the same major line remains accepted.
func VerifyFormSemVerHistory(root string) error {
	histories, err := loadFormSchemaHistories(root)
	if err != nil {
		return err
	}
	kinds := make([]string, 0, len(histories))
	for kind := range histories {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if err := verifyFormSemVerSequence(histories[kind]); err != nil {
			return err
		}
	}
	return nil
}

// verifyCandidateFormSemVer checks generated candidate bytes before the
// release-source directory is replaced. It intentionally does not grant
// publication or mutate any retained release.
func verifyCandidateFormSemVer(root, definitionPath string) error {
	candidate, err := readFormSchemaRelease(definitionPath, "")
	if err != nil {
		return err
	}
	histories, err := loadFormSchemaHistories(root)
	if err != nil {
		return err
	}
	history := histories[candidate.Kind]
	releases := make([]formSchemaRelease, 0, len(history)+1)
	for _, release := range history {
		if release.Version.Raw == candidate.Version.Raw {
			equivalent, err := canonicalSchemaEqual(release.DesiredSchema, candidate.DesiredSchema)
			if err != nil {
				return fmt.Errorf("%s %s existing desiredSchema: %w", candidate.Kind, candidate.Version.Raw, err)
			}
			if !equivalent {
				return fmt.Errorf(
					"%s %s already exists with a different desiredSchema; choose a new Form SemVer instead of reshaping that release source",
					candidate.Kind, candidate.Version.Raw,
				)
			}
			continue
		}
		releases = append(releases, release)
	}
	releases = append(releases, candidate)
	if err := verifyFormSemVerSequence(releases); err != nil {
		return err
	}
	return verifyCandidateBumpIsMinimal(history, candidate)
}

func loadFormSchemaHistories(root string) (map[string][]formSchemaRelease, error) {
	releasesRoot := filepath.Join(root, "forms", "releases")
	releaseIDs, err := os.ReadDir(releasesRoot)
	if err != nil {
		return nil, fmt.Errorf("read Form release history: %w", err)
	}
	histories := make(map[string][]formSchemaRelease)
	for _, releaseID := range releaseIDs {
		if !releaseID.IsDir() {
			continue
		}
		releaseRoot := filepath.Join(releasesRoot, releaseID.Name())
		versions, err := os.ReadDir(releaseRoot)
		if err != nil {
			return nil, fmt.Errorf("read Form release history %s: %w", releaseID.Name(), err)
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			definitionPath := filepath.Join(releaseRoot, version.Name(), "definition.json")
			release, err := readFormSchemaRelease(definitionPath, version.Name())
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", releaseID.Name(), version.Name(), err)
			}
			if want := releaseIDForKind(release.Kind); releaseID.Name() != want {
				return nil, fmt.Errorf(
					"Form release history %s/%s contains kind %s, whose release id is %s",
					releaseID.Name(), version.Name(), release.Kind, want,
				)
			}
			histories[release.Kind] = append(histories[release.Kind], release)
		}
	}
	return histories, nil
}

func readFormSchemaRelease(path, directoryVersion string) (formSchemaRelease, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return formSchemaRelease{}, fmt.Errorf("read %s: %w", path, err)
	}
	var envelope struct {
		Kind              string          `json:"kind"`
		DefinitionVersion string          `json:"definitionVersion"`
		DesiredSchema     json.RawMessage `json:"desiredSchema"`
	}
	if err := decodeJSONNumber(raw, &envelope); err != nil {
		return formSchemaRelease{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if envelope.Kind == "" || envelope.DefinitionVersion == "" || len(envelope.DesiredSchema) == 0 {
		return formSchemaRelease{}, fmt.Errorf("definition omits kind, definitionVersion, or desiredSchema")
	}
	if directoryVersion != "" && envelope.DefinitionVersion != directoryVersion {
		return formSchemaRelease{}, fmt.Errorf(
			"definitionVersion %q differs from release directory %q",
			envelope.DefinitionVersion, directoryVersion,
		)
	}
	version, err := parseStableFormVersion(envelope.DefinitionVersion)
	if err != nil {
		return formSchemaRelease{}, err
	}
	var schema any
	if err := decodeJSONNumber(envelope.DesiredSchema, &schema); err != nil {
		return formSchemaRelease{}, fmt.Errorf("decode desiredSchema: %w", err)
	}
	if _, object := schema.(map[string]any); !object {
		return formSchemaRelease{}, fmt.Errorf("desiredSchema must be a JSON object")
	}
	return formSchemaRelease{Kind: envelope.Kind, Version: version, DesiredSchema: schema}, nil
}

func decodeJSONNumber(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func parseStableFormVersion(value string) (stableFormVersion, error) {
	match := stableFormVersionPattern.FindStringSubmatch(value)
	if match == nil {
		return stableFormVersion{}, fmt.Errorf(
			"Form release version %q is not stable SemVer; automatic compatibility proof fails closed for explicit review",
			value,
		)
	}
	parts := [3]uint64{}
	for index := range parts {
		parsed, err := strconv.ParseUint(match[index+1], 10, 64)
		if err != nil {
			return stableFormVersion{}, fmt.Errorf("parse Form release version %q: %w", value, err)
		}
		parts[index] = parsed
	}
	return stableFormVersion{Raw: value, Major: parts[0], Minor: parts[1], Patch: parts[2]}, nil
}

// stableFormVersionLess orders two stable Form versions.
func stableFormVersionLess(a, b stableFormVersion) bool {
	if a.Major != b.Major {
		return a.Major < b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor < b.Minor
	}
	return a.Patch < b.Patch
}

func verifyFormSemVerSequence(releases []formSchemaRelease) error {
	ordered := append([]formSchemaRelease(nil), releases...)
	sort.Slice(ordered, func(left, right int) bool {
		a, b := ordered[left].Version, ordered[right].Version
		if a.Major != b.Major {
			return a.Major < b.Major
		}
		if a.Minor != b.Minor {
			return a.Minor < b.Minor
		}
		return a.Patch < b.Patch
	})
	for index, current := range ordered {
		if current.Kind == "" {
			return fmt.Errorf("Form release history contains an empty kind")
		}
		if index > 0 && current.Version == ordered[index-1].Version {
			return fmt.Errorf("%s release history duplicates version %s", current.Kind, current.Version.Raw)
		}
		for earlierIndex := 0; earlierIndex < index; earlierIndex++ {
			earlier := ordered[earlierIndex]
			if earlier.Kind != current.Kind {
				return fmt.Errorf("Form release history mixes %s and %s", earlier.Kind, current.Kind)
			}
			if earlier.Version.Major != current.Version.Major {
				continue
			}
			if earlier.Version.Minor == current.Version.Minor {
				equivalent, err := canonicalSchemaEqual(earlier.DesiredSchema, current.DesiredSchema)
				if err != nil {
					return fmt.Errorf("%s %s patch desiredSchema: %w", current.Kind, current.Version.Raw, err)
				}
				if !equivalent {
					return fmt.Errorf(
						"%s %s is a patch release after %s, but patch desiredSchema must be canonically equivalent",
						current.Kind, current.Version.Raw, earlier.Version.Raw,
					)
				}
				continue
			}
			proof := schemaAcceptanceProof{
				oldRoot: earlier.DesiredSchema,
				newRoot: current.DesiredSchema,
			}
			if err := proof.prove(earlier.DesiredSchema, current.DesiredSchema, "$", 0, map[string]bool{}); err != nil {
				return fmt.Errorf(
					"%s %s cannot prove backwards acceptance of desiredSchema valid under %s: %v; "+
						"this change requires explicit compatibility review or a major release",
					current.Kind, current.Version.Raw, earlier.Version.Raw, err,
				)
			}
		}
	}
	return nil
}

// verifyCandidateBumpIsMinimal rejects a candidate bump larger than its change
// requires.
//
// The rest of this policy only proves a version is not too *low*. Nothing
// stopped one being too high, and the version is hand-written in the catalog,
// so an additive change could quietly take a major and burn a version line
// nobody needed. A Form's major line is what a host pins, so spending one
// costs every consumer a migration for no reason.
//
// This runs on candidates only. Published history is immutable and already
// contains bumps that were larger than they needed to be; re-judging them
// would block every future release to punish a version nobody can change.
func verifyCandidateBumpIsMinimal(history []formSchemaRelease, candidate formSchemaRelease) error {
	var previous *formSchemaRelease
	for index := range history {
		release := history[index]
		if !stableFormVersionLess(release.Version, candidate.Version) {
			continue
		}
		if previous == nil || stableFormVersionLess(previous.Version, release.Version) {
			previous = &history[index]
		}
	}
	if previous == nil || candidate.Version.Major == previous.Version.Major {
		return nil
	}
	// A major line opened for a genuinely narrowing change is correct. One
	// opened for a change that every earlier document still satisfies is not:
	// that is a minor.
	proof := schemaAcceptanceProof{
		oldRoot: previous.DesiredSchema,
		newRoot: candidate.DesiredSchema,
	}
	if err := proof.prove(
		previous.DesiredSchema, candidate.DesiredSchema, "$", 0, map[string]bool{},
	); err != nil {
		return nil
	}
	return fmt.Errorf(
		"%s %s opens a major line after %s, but every desired document valid "+
			"under %s is still accepted, so this change is a minor release",
		candidate.Kind, candidate.Version.Raw, previous.Version.Raw, previous.Version.Raw,
	)
}

type schemaAcceptanceProof struct {
	oldRoot any
	newRoot any
}

func (proof schemaAcceptanceProof) prove(oldSchema, newSchema any, path string, depth int, resolving map[string]bool) error {
	if depth > 128 {
		return fmt.Errorf("%s exceeds compatibility-proof depth", path)
	}
	var err error
	oldSchema, oldRef, err := dereferenceSchema(oldSchema, proof.oldRoot, "old", path)
	if err != nil {
		return err
	}
	newSchema, newRef, err := dereferenceSchema(newSchema, proof.newRoot, "new", path)
	if err != nil {
		return err
	}
	if oldRef != "" || newRef != "" {
		key := oldRef + "\x00" + newRef
		if resolving[key] {
			return fmt.Errorf("%s uses a recursive $ref pair that the automatic proof cannot establish", path)
		}
		next := cloneStringBoolMap(resolving)
		next[key] = true
		resolving = next
	}
	equal, err := canonicalSchemaEqual(oldSchema, newSchema)
	if err != nil {
		return fmt.Errorf("%s canonical comparison: %w", path, err)
	}
	if equal {
		return nil
	}

	if oldBoolean, ok := oldSchema.(bool); ok {
		if !oldBoolean {
			return nil
		}
		if newBoolean, ok := newSchema.(bool); ok && newBoolean {
			return nil
		}
		return fmt.Errorf("%s narrows the previously unconstrained schema", path)
	}
	if newBoolean, ok := newSchema.(bool); ok {
		if newBoolean {
			return nil
		}
		return fmt.Errorf("%s changes a satisfiable schema to false", path)
	}
	oldObject, oldOK := oldSchema.(map[string]any)
	newObject, newOK := newSchema.(map[string]any)
	if !oldOK || !newOK {
		return fmt.Errorf("%s is not a supported JSON Schema object or boolean", path)
	}

	handled := map[string]bool{
		"$schema": true, "$id": true, "$anchor": true, "$comment": true,
		"$defs": true, "definitions": true, "$ref": true,
		"title": true, "description": true, "default": true, "examples": true,
		"deprecated": true, "readOnly": true, "writeOnly": true,
		"type": true, "enum": true, "const": true,
		"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true,
		"minLength": true, "maxLength": true, "pattern": true, "format": true,
		"minItems": true, "maxItems": true, "uniqueItems": true, "items": true,
		"minProperties": true, "maxProperties": true, "required": true,
		"properties": true, "additionalProperties": true, "propertyNames": true,
	}
	if err := proveTypeAcceptance(oldObject, newObject, path); err != nil {
		return err
	}
	if err := proveEnumAcceptance(oldObject, newObject, path); err != nil {
		return err
	}
	if err := proveConstAcceptance(oldObject, newObject, path); err != nil {
		return err
	}
	for _, keyword := range []string{"minimum", "exclusiveMinimum", "minLength", "minItems", "minProperties"} {
		if err := proveLowerBound(oldObject, newObject, keyword, path); err != nil {
			return err
		}
	}
	for _, keyword := range []string{"maximum", "exclusiveMaximum", "maxLength", "maxItems", "maxProperties"} {
		if err := proveUpperBound(oldObject, newObject, keyword, path); err != nil {
			return err
		}
	}
	for _, keyword := range []string{"pattern", "format"} {
		if err := proveExactOrRemoved(oldObject, newObject, keyword, path); err != nil {
			return err
		}
	}
	if err := proveUniqueItems(oldObject, newObject, path); err != nil {
		return err
	}
	if err := proof.proveSchemaKeyword(oldObject, newObject, "items", path, depth, resolving); err != nil {
		return err
	}
	if err := proof.proveObjectAcceptance(oldObject, newObject, path, depth, resolving); err != nil {
		return err
	}
	for keyword := range unionKeys(oldObject, newObject) {
		if handled[keyword] || strings.HasPrefix(keyword, "x-") {
			continue
		}
		return fmt.Errorf(
			"%s contains JSON Schema keyword %q in a changed schema node; its backwards acceptance is not automatically provable",
			path, keyword,
		)
	}
	return nil
}

func (proof schemaAcceptanceProof) proveSchemaKeyword(
	oldObject, newObject map[string]any,
	keyword, path string,
	depth int,
	resolving map[string]bool,
) error {
	oldValue, oldExists := oldObject[keyword]
	newValue, newExists := newObject[keyword]
	if !oldExists {
		oldValue = true
	}
	if !newExists {
		newValue = true
	}
	return proof.prove(oldValue, newValue, path+"/"+keyword, depth+1, resolving)
}

func (proof schemaAcceptanceProof) proveObjectAcceptance(
	oldObject, newObject map[string]any,
	path string,
	depth int,
	resolving map[string]bool,
) error {
	oldRequired, err := stringSetKeyword(oldObject, "required")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	newRequired, err := stringSetKeyword(newObject, "required")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	for name := range newRequired {
		if !oldRequired[name] {
			return fmt.Errorf("%s/required adds required property %q", path, name)
		}
	}
	oldProperties, err := schemaMapKeyword(oldObject, "properties")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	newProperties, err := schemaMapKeyword(newObject, "properties")
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	oldAdditional := any(true)
	if value, ok := oldObject["additionalProperties"]; ok {
		oldAdditional = value
	}
	newAdditional := any(true)
	if value, ok := newObject["additionalProperties"]; ok {
		newAdditional = value
	}
	for name, oldProperty := range oldProperties {
		newProperty, retained := newProperties[name]
		if !retained {
			newProperty = newAdditional
		}
		if err := proof.prove(
			oldProperty, newProperty, path+"/properties/"+escapeJSONPointer(name), depth+1, resolving,
		); err != nil {
			return err
		}
	}
	for name, newProperty := range newProperties {
		if _, existed := oldProperties[name]; existed {
			continue
		}
		if err := proof.prove(
			oldAdditional, newProperty, path+"/properties/"+escapeJSONPointer(name), depth+1, resolving,
		); err != nil {
			return fmt.Errorf(
				"%s/properties/%s adds an optional property but the old object previously allowed arbitrary values under that name: %w",
				path, escapeJSONPointer(name), err,
			)
		}
	}
	if err := proof.prove(oldAdditional, newAdditional, path+"/additionalProperties", depth+1, resolving); err != nil {
		return err
	}
	return proof.proveSchemaKeyword(oldObject, newObject, "propertyNames", path, depth, resolving)
}

func proveTypeAcceptance(oldObject, newObject map[string]any, path string) error {
	oldTypes, oldConstrained, err := schemaTypes(oldObject["type"])
	if err != nil {
		return fmt.Errorf("%s/type: %w", path, err)
	}
	newTypes, newConstrained, err := schemaTypes(newObject["type"])
	if err != nil {
		return fmt.Errorf("%s/type: %w", path, err)
	}
	if !newConstrained {
		return nil
	}
	if !oldConstrained {
		return fmt.Errorf("%s/type narrows an unconstrained type", path)
	}
	for oldType := range oldTypes {
		if newTypes[oldType] || (oldType == "integer" && newTypes["number"]) {
			continue
		}
		return fmt.Errorf("%s/type no longer accepts %q", path, oldType)
	}
	return nil
}

func schemaTypes(value any) (map[string]bool, bool, error) {
	if value == nil {
		return nil, false, nil
	}
	result := map[string]bool{}
	switch typed := value.(type) {
	case string:
		result[typed] = true
	case []any:
		for _, item := range typed {
			name, ok := item.(string)
			if !ok {
				return nil, false, fmt.Errorf("type array contains a non-string")
			}
			result[name] = true
		}
	default:
		return nil, false, fmt.Errorf("type must be a string or string array")
	}
	return result, true, nil
}

func proveEnumAcceptance(oldObject, newObject map[string]any, path string) error {
	oldValue, oldExists := oldObject["enum"]
	newValue, newExists := newObject["enum"]
	if !newExists {
		return nil
	}
	if !oldExists {
		return fmt.Errorf("%s/enum adds a constraint", path)
	}
	oldValues, ok := oldValue.([]any)
	if !ok {
		return fmt.Errorf("%s/enum old value is not an array", path)
	}
	newValues, ok := newValue.([]any)
	if !ok {
		return fmt.Errorf("%s/enum new value is not an array", path)
	}
	accepted := make(map[string]bool, len(newValues))
	for _, value := range newValues {
		key, err := canonicalSchemaKey(value)
		if err != nil {
			return err
		}
		accepted[key] = true
	}
	for _, value := range oldValues {
		key, err := canonicalSchemaKey(value)
		if err != nil {
			return err
		}
		if !accepted[key] {
			return fmt.Errorf("%s/enum removes previously accepted value %s", path, key)
		}
	}
	return nil
}

func proveConstAcceptance(oldObject, newObject map[string]any, path string) error {
	oldValue, oldExists := oldObject["const"]
	newValue, newExists := newObject["const"]
	if !newExists {
		return nil
	}
	if !oldExists {
		return fmt.Errorf("%s/const adds a constraint", path)
	}
	equal, err := canonicalSchemaEqual(oldValue, newValue)
	if err != nil {
		return err
	}
	if !equal {
		return fmt.Errorf("%s/const changes the accepted value", path)
	}
	return nil
}

func proveLowerBound(oldObject, newObject map[string]any, keyword, path string) error {
	oldValue, oldExists := oldObject[keyword]
	newValue, newExists := newObject[keyword]
	if !newExists {
		return nil
	}
	if !oldExists {
		return fmt.Errorf("%s/%s adds a lower bound", path, keyword)
	}
	comparison, err := compareJSONNumbers(newValue, oldValue)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", path, keyword, err)
	}
	if comparison > 0 {
		return fmt.Errorf("%s/%s raises the lower bound", path, keyword)
	}
	return nil
}

func proveUpperBound(oldObject, newObject map[string]any, keyword, path string) error {
	oldValue, oldExists := oldObject[keyword]
	newValue, newExists := newObject[keyword]
	if !newExists {
		return nil
	}
	if !oldExists {
		return fmt.Errorf("%s/%s adds an upper bound", path, keyword)
	}
	comparison, err := compareJSONNumbers(newValue, oldValue)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", path, keyword, err)
	}
	if comparison < 0 {
		return fmt.Errorf("%s/%s lowers the upper bound", path, keyword)
	}
	return nil
}

func proveExactOrRemoved(oldObject, newObject map[string]any, keyword, path string) error {
	oldValue, oldExists := oldObject[keyword]
	newValue, newExists := newObject[keyword]
	if !newExists {
		return nil
	}
	if !oldExists {
		return fmt.Errorf("%s/%s adds a constraint", path, keyword)
	}
	equal, err := canonicalSchemaEqual(oldValue, newValue)
	if err != nil {
		return err
	}
	if !equal {
		return fmt.Errorf("%s/%s changes a constraint whose language inclusion is not automatically provable", path, keyword)
	}
	return nil
}

func proveUniqueItems(oldObject, newObject map[string]any, path string) error {
	oldValue, oldExists := oldObject["uniqueItems"]
	newValue, newExists := newObject["uniqueItems"]
	if !newExists || newValue == false {
		return nil
	}
	if newValue != true {
		return fmt.Errorf("%s/uniqueItems is not boolean", path)
	}
	if !oldExists || oldValue != true {
		return fmt.Errorf("%s/uniqueItems adds a uniqueness constraint", path)
	}
	return nil
}

func stringSetKeyword(object map[string]any, keyword string) (map[string]bool, error) {
	value, exists := object[keyword]
	if !exists {
		return map[string]bool{}, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", keyword)
	}
	result := make(map[string]bool, len(items))
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s contains a non-string", keyword)
		}
		result[name] = true
	}
	return result, nil
}

func schemaMapKeyword(object map[string]any, keyword string) (map[string]any, error) {
	value, exists := object[keyword]
	if !exists {
		return map[string]any{}, nil
	}
	result, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", keyword)
	}
	return result, nil
}

func compareJSONNumbers(left, right any) (int, error) {
	leftRat, err := jsonNumberRat(left)
	if err != nil {
		return 0, err
	}
	rightRat, err := jsonNumberRat(right)
	if err != nil {
		return 0, err
	}
	return leftRat.Cmp(rightRat), nil
}

func jsonNumberRat(value any) (*big.Rat, error) {
	var text string
	switch typed := value.(type) {
	case json.Number:
		text = typed.String()
	case float64:
		text = strconv.FormatFloat(typed, 'g', -1, 64)
	case float32:
		text = strconv.FormatFloat(float64(typed), 'g', -1, 32)
	case int:
		text = strconv.Itoa(typed)
	case int64:
		text = strconv.FormatInt(typed, 10)
	case uint64:
		text = strconv.FormatUint(typed, 10)
	default:
		return nil, fmt.Errorf("value %v is not a JSON number", value)
	}
	result := new(big.Rat)
	if _, ok := result.SetString(text); !ok {
		return nil, fmt.Errorf("invalid JSON number %q", text)
	}
	return result, nil
}

func dereferenceSchema(schema, root any, side, path string) (any, string, error) {
	object, ok := schema.(map[string]any)
	if !ok {
		return schema, "", nil
	}
	value, hasRef := object["$ref"]
	if !hasRef {
		return schema, "", nil
	}
	ref, ok := value.(string)
	if !ok {
		return nil, "", fmt.Errorf("%s %s $ref is not a string", path, side)
	}
	for keyword := range object {
		if keyword == "$ref" || keyword == "$defs" || keyword == "definitions" ||
			isSchemaAnnotation(keyword) || strings.HasPrefix(keyword, "x-") {
			continue
		}
		return nil, "", fmt.Errorf(
			"%s %s $ref has assertion sibling %q, which the automatic proof cannot compose",
			path, side, keyword,
		)
	}
	resolved, err := resolveLocalJSONPointer(root, ref)
	if err != nil {
		return nil, "", fmt.Errorf("%s %s $ref %q: %w", path, side, ref, err)
	}
	return resolved, ref, nil
}

func resolveLocalJSONPointer(root any, ref string) (any, error) {
	if ref == "#" {
		return root, nil
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("only local JSON Pointer references are supported")
	}
	fragment, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	if err != nil {
		return nil, err
	}
	current := root
	for _, encoded := range strings.Split(strings.TrimPrefix(fragment, "/"), "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			next, exists := typed[token]
			if !exists {
				return nil, fmt.Errorf("pointer token %q does not exist", token)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("pointer array index %q is invalid", token)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("pointer token %q traverses a scalar", token)
		}
	}
	return current, nil
}

func isSchemaAnnotation(keyword string) bool {
	switch keyword {
	case "$schema", "$id", "$anchor", "$comment", "title", "description", "default",
		"examples", "deprecated", "readOnly", "writeOnly":
		return true
	default:
		return false
	}
}

func canonicalSchemaEqual(left, right any) (bool, error) {
	leftCanonical, err := canonicalSchemaValue(left)
	if err != nil {
		return false, err
	}
	rightCanonical, err := canonicalSchemaValue(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftCanonical, rightCanonical), nil
}

func canonicalSchemaKey(value any) (string, error) {
	canonical, err := canonicalSchemaValue(value)
	if err != nil {
		return "", err
	}
	return string(canonical), nil
}

func canonicalSchemaValue(value any) ([]byte, error) {
	// formpackage.Canonicalize validates complete JSON documents whose root is
	// an object. Wrapping also lets the proof compare scalar and boolean schema
	// fragments with the same RFC 8785 number and string rules.
	raw, err := json.Marshal(map[string]any{"value": value})
	if err != nil {
		return nil, err
	}
	return formpackage.Canonicalize(raw)
}

func unionKeys(left, right map[string]any) map[string]bool {
	result := make(map[string]bool, len(left)+len(right))
	for key := range left {
		result[key] = true
	}
	for key := range right {
		result[key] = true
	}
	return result
}

func cloneStringBoolMap(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}
