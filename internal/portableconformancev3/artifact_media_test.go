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

func TestStaticAssetMediaTypeIsExtensibleButNormalized(t *testing.T) {
	valid := decodeManifest(t, map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       staticAssetBundleKind,
		"files": []any{map[string]any{
			"path": "data.bin", "mediaType": "application/vnd.example+json", "size": 0,
			"digest": formpackage.DigestBytes(nil),
		}},
	})
	if hostErr := validateArtifactManifest(valid); hostErr != nil {
		t.Fatalf("valid extensible static media type refused: %+v", hostErr)
	}
	for _, mediaType := range []string{"Application/vnd.example+json", "application/vnd.example+json; charset=utf-8", "application/"} {
		invalid := valid
		invalid.Files = append([]artifactFile(nil), valid.Files...)
		invalid.Files[0].MediaType = mediaType
		if hostErr := validateArtifactManifest(invalid); hostErr == nil || hostErr.Code != "artifact_invalid" {
			t.Errorf("media type %q = %+v, want artifact_invalid", mediaType, hostErr)
		}
	}
}

func TestArtifactManifestCountAndAggregateCeilings(t *testing.T) {
	zeroDigest := formpackage.DigestBytes(nil)
	modules := make([]artifactModule, maximumWorkerBundleModules)
	for index := range modules {
		name := "module-" + strconv.Itoa(index) + ".js"
		if index == 0 {
			name = "index.js"
		}
		modules[index] = artifactModule{Name: name, MediaType: "application/javascript+module", Size: "0", Digest: zeroDigest}
	}
	worker := artifactManifest{APIVersion: artifactAPIVersion, Kind: workerBundleKind, MainModule: "index.js", Modules: modules}
	if hostErr := validateArtifactManifest(worker); hostErr != nil {
		t.Fatalf("worker at exact module-count ceiling refused: %+v", hostErr)
	}
	worker.Modules = append(worker.Modules, artifactModule{Name: "overrun.js", MediaType: "application/javascript+module", Size: "0", Digest: zeroDigest})
	if hostErr := validateArtifactManifest(worker); hostErr == nil || hostErr.Code != "artifact_invalid" {
		t.Fatalf("worker module-count overrun = %+v, want artifact_invalid", hostErr)
	}

	files := make([]artifactFile, maximumBundleFiles)
	for index := range files {
		files[index] = artifactFile{
			Path:      "asset-" + strconv.Itoa(index) + ".bin",
			MediaType: "application/octet-stream",
			Size:      "0",
			Digest:    zeroDigest,
		}
	}
	staticCount := artifactManifest{APIVersion: artifactAPIVersion, Kind: staticAssetBundleKind, Files: files}
	if hostErr := validateArtifactManifest(staticCount); hostErr != nil {
		t.Fatalf("static bundle at exact file-count ceiling refused: %+v", hostErr)
	}
	staticCount.Files = append(staticCount.Files, artifactFile{
		Path: "file-count-overrun.bin", MediaType: "application/octet-stream", Size: "0", Digest: zeroDigest,
	})
	if hostErr := validateArtifactManifest(staticCount); hostErr == nil || hostErr.Code != "artifact_invalid" {
		t.Fatalf("static file-count overrun = %+v, want artifact_invalid", hostErr)
	}

	static := artifactManifest{APIVersion: artifactAPIVersion, Kind: staticAssetBundleKind, Files: []artifactFile{{
		Path: "large.bin", MediaType: "application/octet-stream", Size: json.Number(strconv.Itoa(maximumBundleBytes)), Digest: zeroDigest,
	}}}
	if hostErr := validateArtifactManifest(static); hostErr != nil {
		t.Fatalf("static bundle at exact aggregate ceiling refused: %+v", hostErr)
	}
	static.Files[0].Size = json.Number(strconv.Itoa(maximumBundleBytes + 1))
	if hostErr := validateArtifactManifest(static); hostErr == nil || hostErr.Code != "artifact_invalid" {
		t.Fatalf("static aggregate overrun = %+v, want artifact_invalid", hostErr)
	}
}
