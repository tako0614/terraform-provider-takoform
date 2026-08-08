package spec

// artifact_media_types_test.go is the drift gate over the module media-type
// set of a Worker Bundle.
//
// Four surfaces describe the same bytes: the published artifact-manifest
// schema (what a manifest may carry), the `worker.runtime` Interface contract
// (what the module graph may import), the host's manifest validator, and the
// provider's authoring allowlist. Two of them used to disagree — the manifest
// admitted a source map the runtime could not load, and the runtime claimed an
// `application/json` module the manifest never admitted — so a bundle could be
// accepted by one and unusable by the other, discovered at deploy.
//
// The last two now DERIVE their set from internal/currentformmodel and hold no
// list of their own, so the only statements that can still drift are the two
// documents, and this file compares both against that source. The behavioral
// halves are proved where a host and a client can actually be wrong: the host's
// validator in internal/portableconformancev3, the authoring surface in
// internal/provider, and the required conformance check
// `bundle-main-module-is-loadable`.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
)

// unsupportedModuleMediaType is the one media type the first ABI version
// deliberately does not support. It is named here so a future widening has to
// touch this gate rather than happening by accident.
const unsupportedModuleMediaType = "application/json"

func sortedCopy(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

// TestModuleMediaTypeSetsAgree holds every statement of the set to one source.
func TestModuleMediaTypeSetsAgree(t *testing.T) {
	loadable := currentformmodel.LoadableModuleMediaTypes()
	auxiliary := currentformmodel.AuxiliaryModuleMediaTypes()
	bundle := currentformmodel.BundleModuleMediaTypes()

	if len(loadable) == 0 || len(auxiliary) == 0 {
		t.Fatal("the lane must state both a loadable and an auxiliary module class")
	}
	// The two classes are what makes `mainModule` and the import rule
	// decidable, so a media type in both would make them undecidable.
	for _, mediaType := range loadable {
		if currentformmodel.ModuleMediaTypeAuxiliary(mediaType) {
			t.Fatalf("%s is both loadable and auxiliary", mediaType)
		}
	}
	if want := append(sortedCopy(loadable), sortedCopy(auxiliary)...); !reflect.DeepEqual(sortedCopy(bundle), sortedCopy(want)) {
		t.Fatalf("bundle set %v is not the union of %v and %v", bundle, loadable, auxiliary)
	}
	for _, mediaType := range bundle {
		if mediaType == unsupportedModuleMediaType {
			t.Fatalf("%s is not supported in this ABI version", unsupportedModuleMediaType)
		}
	}

	// 1. The published artifact manifest schema. Its bytes are immutable
	// (decision 0014), so this asserts the code narrowed onto the published
	// enum rather than away from it.
	published := publishedManifestModuleMediaTypes(t)
	if !reflect.DeepEqual(published, sortedCopy(bundle)) {
		t.Fatalf(
			"the published manifest enum is %v; the lane admits %v. The published bytes are immutable, "+
				"so the code must narrow onto them, never past them",
			published, sortedCopy(bundle),
		)
	}

	// 2. The runtime ABI contract, read back out of the authored definition
	// exactly the way a host reads it.
	contractLoadable, err := edgeformcatalog.RuntimeLoadableMediaTypes()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sortedCopy(contractLoadable), sortedCopy(loadable)) {
		t.Fatalf("worker.runtime loads %v; the lane's loadable set is %v", contractLoadable, loadable)
	}
	contractAuxiliary, err := edgeformcatalog.RuntimeAuxiliaryMediaTypes()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sortedCopy(contractAuxiliary), sortedCopy(auxiliary)) {
		t.Fatalf("worker.runtime carries %v; the lane's auxiliary set is %v", contractAuxiliary, auxiliary)
	}

	// 3. The SHIPPED Interface candidate, which is the document a host
	// actually installs. The authored contract and the distributed bytes are
	// two artifacts, and only the second is what a host reads.
	shippedLoadable, shippedAuxiliary := shippedRuntimeMediaTypes(t)
	if !reflect.DeepEqual(shippedLoadable, sortedCopy(loadable)) {
		t.Fatalf("the shipped worker.runtime candidate loads %v; the lane's loadable set is %v", shippedLoadable, loadable)
	}
	if !reflect.DeepEqual(shippedAuxiliary, sortedCopy(auxiliary)) {
		t.Fatalf("the shipped worker.runtime candidate carries %v; the lane's auxiliary set is %v", shippedAuxiliary, auxiliary)
	}
}

func publishedManifestModuleMediaTypes(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("schemas", "artifact-manifest-v1alpha1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Modules struct {
				Items struct {
					Properties struct {
						MediaType struct {
							Enum []string `json:"enum"`
						} `json:"mediaType"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"modules"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	enum := schema.Properties.Modules.Items.Properties.MediaType.Enum
	if len(enum) == 0 {
		t.Fatal("the published manifest schema states no module media-type enum")
	}
	return sortedCopy(enum)
}

func shippedRuntimeMediaTypes(t *testing.T) (loadable, auxiliary []string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(
		"..", "interfaces", "candidates", "v1alpha1", "worker.runtime", "definition.json",
	))
	if err != nil {
		t.Fatal(err)
	}
	var definition struct {
		Operations []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]struct {
					Items struct {
						Properties struct {
							MediaType struct {
								Enum []string `json:"enum"`
							} `json:"mediaType"`
						} `json:"properties"`
					} `json:"items"`
				} `json:"properties"`
			} `json:"inputSchema"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(raw, &definition); err != nil {
		t.Fatal(err)
	}
	for _, operation := range definition.Operations {
		if operation.Name != edgeformcatalog.RuntimeHandlerOperation {
			continue
		}
		properties := operation.InputSchema.Properties
		loadable = sortedCopy(properties[edgeformcatalog.RuntimeLoadableProperty].Items.Properties.MediaType.Enum)
		auxiliary = sortedCopy(properties[edgeformcatalog.RuntimeAuxiliaryProperty].Items.Properties.MediaType.Enum)
		if len(loadable) == 0 || len(auxiliary) == 0 {
			t.Fatal("the shipped worker.runtime candidate states no loadable or auxiliary media-type set")
		}
		return loadable, auxiliary
	}
	t.Fatalf("the shipped worker.runtime candidate declares no %s operation", edgeformcatalog.RuntimeHandlerOperation)
	return nil, nil
}
