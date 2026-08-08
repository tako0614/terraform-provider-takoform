package portableconformancev3

// artifact_media_test.go proves the host half of the reconciled module
// media-type set, behaviorally rather than by comparing a variable: what this
// validator ADMITS is the lane's bundle set, what it refuses is everything
// else, and an auxiliary module is admitted into a bundle while being refused
// as its `mainModule` (spec/decisions/0012 and 0019).

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// moduleEntry renders one manifest module entry addressing the given bytes.
func moduleEntry(name, mediaType, body string) map[string]any {
	return map[string]any{
		"name":      name,
		"mediaType": mediaType,
		"size":      json.Number(strconv.Itoa(len(body))),
		"digest":    formpackage.DigestBytes([]byte(body)),
	}
}

func decodeManifest(t *testing.T, document map[string]any) artifactManifest {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var manifest artifactManifest
	if err := formpackage.DecodeStrictIJSON(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

// TestManifestAdmitsExactlyTheBundleMediaTypeSet drives every admitted media
// type through the validator and one that is not admitted, so a host list that
// drifted from the lane's set fails here rather than at deploy.
func TestManifestAdmitsExactlyTheBundleMediaTypeSet(t *testing.T) {
	const code = "export default { async fetch() { return new Response(\"ok\"); } };\n"
	for _, mediaType := range currentformmodel.BundleModuleMediaTypes() {
		name := "index.js"
		modules := []any{moduleEntry(name, "application/javascript+module", code)}
		if !currentformmodel.ModuleMediaTypeLoadable(mediaType) {
			// An auxiliary module rides beside loadable code and never replaces
			// it, so it is added rather than substituted. The ".map" naming rule
			// binds it to the module it describes.
			modules = append(modules, moduleEntry(name+".map", mediaType, "{}\n"))
		} else {
			modules = []any{moduleEntry(name, mediaType, code)}
		}
		manifest := decodeManifest(t, map[string]any{
			"apiVersion": artifactAPIVersion,
			"kind":       "WorkerBundle",
			"mainModule": name,
			"modules":    modules,
		})
		if hostErr := validateArtifactManifest(manifest); hostErr != nil {
			t.Fatalf("the bundle media type %s was refused: %s", mediaType, hostErr.Message)
		}
	}

	// application/json is admitted by no surface of this lane.
	refused := decodeManifest(t, map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "config.json",
		"modules":    []any{moduleEntry("config.json", "application/json", "{}\n")},
	})
	hostErr := validateArtifactManifest(refused)
	if hostErr == nil || hostErr.Code != "artifact_invalid" {
		t.Fatal("an application/json module must be refused: this ABI version loads none")
	}
}

// TestAuxiliaryModuleIsCarriedButNeverTheMainModule is the host statement of
// the rule the published manifest schema cannot express: `modules` admits a
// source map beside executable code and cannot tell the two apart, so only the
// host stops a bundle whose first module is evidence rather than code.
func TestAuxiliaryModuleIsCarriedButNeverTheMainModule(t *testing.T) {
	const code = "export default { async fetch() { return new Response(\"ok\"); } };\n"
	const sourceMap = "{\"version\":3,\"sources\":[\"index.js\"],\"mappings\":\"\"}\n"
	modules := []any{
		moduleEntry("index.js", "application/javascript+module", code),
		moduleEntry("index.js.map", "application/source-map+json", sourceMap),
	}
	carried := decodeManifest(t, map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "index.js",
		"modules":    modules,
	})
	if hostErr := validateArtifactManifest(carried); hostErr != nil {
		t.Fatalf("a bundle carrying a source map beside its code was refused: %s", hostErr.Message)
	}
	loaded := decodeManifest(t, map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       "WorkerBundle",
		"mainModule": "index.js.map",
		"modules":    modules,
	})
	hostErr := validateArtifactManifest(loaded)
	if hostErr == nil || hostErr.Code != "artifact_invalid" ||
		!strings.Contains(hostErr.Message, "loadable") {
		t.Fatal("a source map named as mainModule must be refused: the runtime never imports one")
	}
}

// TestSourceMapIsTheOnlyAuxiliaryMediaType pins the assumption the ".map"
// naming rule rests on. A second auxiliary class would need its own binding
// rule, and this fails rather than letting the source-map rule silently govern
// something it was never written for.
func TestSourceMapIsTheOnlyAuxiliaryMediaType(t *testing.T) {
	auxiliary := currentformmodel.AuxiliaryModuleMediaTypes()
	if len(auxiliary) != 1 || auxiliary[0] != sourceMapMediaType {
		t.Fatalf(
			"auxiliary media types are %v; the source-map naming rule below assumes exactly [%s]",
			auxiliary, sourceMapMediaType,
		)
	}
}
