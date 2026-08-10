package currentformmodel

import (
	"regexp"
	"strings"
)

// artifact_media.go is the ONE statement of what a module inside a Worker
// Bundle may be, for the whole v1beta1 channel.
//
// Four surfaces describe the same bytes: the runtime ABI contract
// (`worker.runtime`, which says what the module graph can import), the
// published artifact-manifest schema (which says what may appear in a
// manifest), the host's manifest validator, and the provider's authoring
// allowlist. Before this file each spelled its own list, and two of them
// disagreed: the manifest admitted `application/source-map+json`, which the
// runtime could not load, and the runtime claimed to load `application/json`,
// which the manifest did not admit. A bundle could therefore be accepted by
// one surface and unusable by the other, and nothing discovered it until
// deploy.
//
// The set is split by what the module graph does with it rather than by where
// it may appear:
//
//   - LOADABLE — a module the graph may import, and the only kind `mainModule`
//     may name;
//   - AUXILIARY — a module a bundle may carry that the graph never imports and
//     `mainModule` may never name. Today that is exactly source-map evidence
//     about another module.
//
// The union is what a manifest admits, which is what the published schema's
// enum already says. `application/json` is deliberately absent from both: the
// published enum never admitted it, so a runtime claiming to load it promised
// a module no conforming bundle can carry (spec/decisions/0012 and 0019).
//
// It lives in the authoring model because that is the one package the
// contract author (internal/edgeformcatalog), the provider, and the reference
// host all already depend on, and because the lane's other artifact grammars —
// PatternRelativePath and PatternCanonicalSHA256 — are declared here for the
// same reason.

// loadableModuleMediaTypes is the closed set of module media types a
// conforming runtime instantiates and the module graph may import. Each entry
// states what the importing JavaScript receives; the exact prose lives on the
// runtime contract's loadModule operation, which reads this list.
var loadableModuleMediaTypes = []string{
	"application/javascript+module",
	"application/octet-stream",
	"application/wasm",
	"text/plain",
}

// auxiliaryModuleMediaTypes is the closed set of module media types a bundle
// may carry that are never imported and never `mainModule`. A source map is
// evidence ABOUT another module, not code the graph links.
var auxiliaryModuleMediaTypes = []string{
	"application/source-map+json",
}

// normalizedMediaTypePattern is the published artifact-manifest v1alpha1
// grammar for a concrete media type: the type and subtype are lowercase and
// parameters (which would make one manifest entry mean more than one
// representation) are absent. Static assets deliberately use this extensible
// grammar rather than a closed allowlist; WorkerBundle modules keep their
// separate closed set above and MigrationBundle is exactly application/sql.
var normalizedMediaTypePattern = regexp.MustCompile("^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$")

// ValidNormalizedMediaType reports whether value is one concrete normalized
// RFC media type without parameters.  It intentionally does not know which
// artifact kind is using the type: the WorkerBundle and MigrationBundle
// policies are applied by their respective manifest validators.
func ValidNormalizedMediaType(value string) bool {
	return value != "" && value == strings.ToLower(value) && normalizedMediaTypePattern.MatchString(value)
}

// LoadableModuleMediaTypes returns the importable module media types, sorted.
func LoadableModuleMediaTypes() []string {
	return append([]string(nil), loadableModuleMediaTypes...)
}

// AuxiliaryModuleMediaTypes returns the carry-only module media types, sorted.
func AuxiliaryModuleMediaTypes() []string {
	return append([]string(nil), auxiliaryModuleMediaTypes...)
}

// BundleModuleMediaTypes returns every media type a Worker Bundle manifest
// admits: the loadable set followed by the auxiliary set. It is the value the
// published artifact-manifest schema's `modules[].mediaType` enum states, and
// a drift gate proves the two stay equal.
func BundleModuleMediaTypes() []string {
	out := make([]string, 0, len(loadableModuleMediaTypes)+len(auxiliaryModuleMediaTypes))
	out = append(out, loadableModuleMediaTypes...)
	out = append(out, auxiliaryModuleMediaTypes...)
	return out
}

// ModuleMediaTypeLoadable reports whether one media type may be imported by
// the module graph, and therefore whether it may be a bundle's `mainModule`.
func ModuleMediaTypeLoadable(mediaType string) bool {
	for _, candidate := range loadableModuleMediaTypes {
		if candidate == mediaType {
			return true
		}
	}
	return false
}

// ModuleMediaTypeAuxiliary reports whether one media type may sit in a bundle
// without ever being imported.
func ModuleMediaTypeAuxiliary(mediaType string) bool {
	for _, candidate := range auxiliaryModuleMediaTypes {
		if candidate == mediaType {
			return true
		}
	}
	return false
}

// ModuleMediaTypeAdmitted reports whether one media type may appear in a
// Worker Bundle manifest at all.
func ModuleMediaTypeAdmitted(mediaType string) bool {
	return ModuleMediaTypeLoadable(mediaType) || ModuleMediaTypeAuxiliary(mediaType)
}
