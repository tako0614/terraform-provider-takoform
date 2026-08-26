package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestEmbeddedProviderV3SnapshotAssemblyClosesExactArtifacts(t *testing.T) {
	t.Parallel()

	assembly, err := loadEmbeddedProviderV3Assembly()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(assembly.snapshot.Forms()); got != 31 {
		t.Fatalf("embedded Snapshot Forms = %d, want 31", got)
	}
	if got := len(assembly.snapshot.Interfaces()); got != 13 {
		t.Fatalf("embedded Snapshot Interfaces = %d, want 13", got)
	}
	if got := len(assembly.snapshot.Bindings()); got != 6 {
		t.Fatalf("embedded Snapshot Bindings = %d, want 6", got)
	}
	if got := len(assembly.currentForms); got != 31 {
		t.Fatalf("projected current Forms = %d, want 31", got)
	}
	if got := len(assembly.registry.SupportedRefs()); got != 46 {
		t.Fatalf("projected retained+current refs = %d, want 46", got)
	}
	if got := len(assembly.codecs.codecs); got != 45 {
		t.Fatalf("projected readable codecs = %d, want 45", got)
	}
}

func TestEmbeddedProviderV3SnapshotAssemblyIsCachedOnce(t *testing.T) {
	first, err := providerV3SnapshotAssembly()
	if err != nil {
		t.Fatal(err)
	}
	second, err := providerV3SnapshotAssembly()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("embedded Provider 3 assembly was verified and compiled more than once")
	}
}

func TestV3ArtifactBackedResourceFailsClosedWithoutInjectedProjection(t *testing.T) {
	assembly := mustProviderV3SnapshotAssembly()
	var formFound bool
	for _, key := range assembly.projection.currentOrder {
		mapping := assembly.projection.resources[key]
		if mapping.Artifact == nil {
			continue
		}
		formFound = true
		form := assembly.projection.forms[key].Form
		resource := &v3FormResource{form: form, resourceType: mapping.ResourceType, codecs: assembly.codecs}

		var response frameworkresource.SchemaResponse
		resource.Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)
		if !response.Diagnostics.HasError() {
			t.Fatalf("artifact-backed %s constructed without its exact projection rule produced a schema", key)
		}
		errors := response.Diagnostics.Errors()
		if len(errors) == 0 || errors[0].Summary() != "Provider artifact projection is missing" {
			t.Fatalf("artifact-backed %s missing-injection diagnostic = %v", key, response.Diagnostics)
		}
		if _, ok := resource.v3RevisionNameFromSpec(map[string]any{
			"manifestDigest": "sha256:" + strings.Repeat("ab", 32),
		}, v3TestRevisionOwner); ok {
			t.Fatalf("artifact-backed %s derived a revision name without injected metadata", key)
		}
	}
	if !formFound {
		t.Fatal("Provider 3 projection has no artifact-backed current Form")
	}
}

func TestProviderV3ArtifactClosureFailsClosed(t *testing.T) {
	tests := map[string]struct {
		mutate func(*testing.T, fstest.MapFS)
		want   string
	}{
		"missing package payload": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				delete(artifacts, closure.Packages[0].Root+"/definition.json")
			},
			want: "verify embedded Form Package",
		},
		"tampered package payload": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				prefix := closure.Packages[0].Root + "/fixtures/"
				for name, file := range artifacts {
					if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".json") {
						file.Data = append(append([]byte(nil), file.Data...), ' ')
						return
					}
				}
				t.Fatal("selected embedded package has no JSON fixture to tamper")
			},
			want: "verify embedded Form Package",
		},
		"missing projection": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				delete(artifacts, closure.Projection.Path)
			},
			want: "read Provider 3 projection",
		},
		"tampered projection": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				raw := artifacts[closure.Projection.Path].Data
				tampered := bytes.Replace(raw, []byte("takoform_"), []byte("xakoform_"), 1)
				if bytes.Equal(raw, tampered) {
					t.Fatal("projection has no Terraform resource type to tamper")
				}
				artifacts[closure.Projection.Path].Data = tampered
			},
			want: "projection digest",
		},
		"extra unreferenced artifact": {
			mutate: func(_ *testing.T, artifacts fstest.MapFS) {
				artifacts["unreferenced.json"] = &fstest.MapFile{Data: []byte("{}\n"), Mode: 0o444}
			},
			want: "unreferenced artifact file",
		},
		"overlapping package roots": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				closure.Packages[1].Root = closure.Packages[0].Root + "/nested"
				writeV3JSONFile(t, artifacts, "closure.json", closure)
			},
			want: "package roots",
		},
		"projection and Interface path collision": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				closure.Projection.Path = closure.Interfaces[0].Path
				writeV3JSONFile(t, artifacts, "closure.json", closure)
			},
			want: "collides",
		},
		"duplicate Interface path with distinct refs": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				closure.Interfaces[1].Path = closure.Interfaces[0].Path
				writeV3JSONFile(t, artifacts, "closure.json", closure)
			},
			want: "collides",
		},
		"duplicate Binding path with distinct refs": {
			mutate: func(t *testing.T, artifacts fstest.MapFS) {
				closure := readV3Closure(t, artifacts)
				closure.Bindings[1].Path = closure.Bindings[0].Path
				writeV3JSONFile(t, artifacts, "closure.json", closure)
			},
			want: "collides",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			artifacts := embeddedProviderV3MapFS(t)
			test.mutate(t, artifacts)
			if _, err := loadProviderV3Assembly(artifacts, "."); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("load error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestProviderV3ProjectionFailsClosed(t *testing.T) {
	tests := map[string]struct {
		mutate func(*testing.T, *v3ProviderProjection)
		want   string
	}{
		"duplicate exact ref": {
			mutate: func(_ *testing.T, projection *v3ProviderProjection) {
				projection.Forms = append(projection.Forms, projection.Forms[0])
			},
			want: "duplicate exact FormRef",
		},
		"missing exact ref": {
			mutate: func(_ *testing.T, projection *v3ProviderProjection) {
				projection.Forms = projection.Forms[:len(projection.Forms)-1]
			},
			want: "projection Form history",
		},
		"extra exact ref": {
			mutate: func(t *testing.T, projection *v3ProviderProjection) {
				for _, candidate := range projection.Forms {
					if candidate.Generation == v3ProjectionCurrent && len(candidate.Form.Fields) == 0 {
						extra := candidate
						extra.Ref.APIVersion = "wrong.forms.takoform.com"
						extra.Form.Family.Group = extra.Ref.APIVersion
						projection.Forms = append(projection.Forms, extra)
						return
					}
				}
				t.Fatal("projection has no fieldless current Form for an extra-ref mutation")
			},
			want: "projection Form history",
		},
		"same Kind in wrong group": {
			mutate: mutateV3DefaultToRetainedSameKind,
			want:   "is not one exact current Form",
		},
		"duplicate resource mapping": {
			mutate: func(_ *testing.T, projection *v3ProviderProjection) {
				projection.Resources = append(projection.Resources, projection.Resources[0])
			},
			want: "duplicate resource mapping",
		},
		"missing resource mapping": {
			mutate: func(_ *testing.T, projection *v3ProviderProjection) {
				projection.Resources = projection.Resources[:len(projection.Resources)-1]
			},
			want: "exact resource mappings",
		},
		"extra resource mapping": {
			mutate: func(t *testing.T, projection *v3ProviderProjection) {
				for _, candidate := range projection.Forms {
					if candidate.Generation == v3ProjectionUnreadable {
						projection.Resources = append(projection.Resources, v3ProjectedResource{
							Ref: candidate.Ref, ResourceType: "takoform_unreadable_history",
						})
						return
					}
				}
				t.Fatal("projection has no explicitly unreadable retained Form")
			},
			want: "exact resource mappings",
		},
		"missing resource type": {
			mutate: func(_ *testing.T, projection *v3ProviderProjection) {
				projection.Resources[0].ResourceType = ""
			},
			want: "resource type",
		},
		"duplicate registered resource type": {
			mutate: func(t *testing.T, projection *v3ProviderProjection) {
				first := -1
				for index := range projection.Resources {
					if !projection.Resources[index].Register {
						continue
					}
					if first == -1 {
						first = index
						continue
					}
					projection.Resources[index].ResourceType = projection.Resources[first].ResourceType
					return
				}
				t.Fatal("projection has fewer than two registered resources")
			},
			want: "maps both",
		},
		"missing artifact rule": {
			mutate: func(t *testing.T, projection *v3ProviderProjection) {
				for index := range projection.Resources {
					if projection.Resources[index].Artifact != nil {
						projection.Resources[index].Artifact = nil
						return
					}
				}
				t.Fatal("projection has no artifact rule to remove")
			},
			want: "artifact rules",
		},
		"artifact rule attached to wrong Form": {
			mutate: func(t *testing.T, projection *v3ProviderProjection) {
				var artifact *v3ArtifactProjection
				removed := -1
				for index := range projection.Resources {
					if projection.Resources[index].Artifact != nil {
						artifact = cloneV3ArtifactProjection(projection.Resources[index].Artifact)
						projection.Resources[index].Artifact = nil
						removed = index
						break
					}
				}
				for index := range projection.Resources {
					if index != removed && projection.Resources[index].Artifact == nil {
						projection.Resources[index].Artifact = artifact
						return
					}
				}
				t.Fatal("projection has no non-artifact resource")
			},
			want: "not attached to an exact required manifestDigest revision",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			artifacts := embeddedProviderV3MapFS(t)
			projection := readV3Projection(t, artifacts)
			test.mutate(t, &projection)
			writeV3Projection(t, artifacts, projection)
			if _, err := loadProviderV3Assembly(artifacts, "."); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("load error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestProviderV3AssemblyIsInputOrderDeterministic(t *testing.T) {
	originalFS := embeddedProviderV3MapFS(t)
	original, err := loadProviderV3Assembly(originalFS, ".")
	if err != nil {
		t.Fatal(err)
	}

	reorderedFS := embeddedProviderV3MapFS(t)
	closure := readV3Closure(t, reorderedFS)
	reverseV3(closure.Packages)
	reverseV3(closure.Interfaces)
	reverseV3(closure.Bindings)
	projection := readV3Projection(t, reorderedFS)
	reverseV3(projection.Forms)
	reverseV3(projection.Resources)
	reverseV3(projection.DefaultCreates)
	reverseV3(projection.ReadableRefs)
	writeV3Projection(t, reorderedFS, projection)
	// writeV3Projection updates the closure digest, so preserve its update while
	// applying the deliberately reversed closure arrays.
	updated := readV3Closure(t, reorderedFS)
	closure.Projection = updated.Projection
	writeV3JSONFile(t, reorderedFS, "closure.json", closure)

	reordered, err := loadProviderV3Assembly(reorderedFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	if left, right := providerV3AssemblyFingerprint(t, original), providerV3AssemblyFingerprint(t, reordered); !bytes.Equal(left, right) {
		t.Fatalf("assembly changed with artifact input order:\noriginal: %s\nreordered: %s", left, right)
	}
}

func embeddedProviderV3MapFS(t *testing.T) fstest.MapFS {
	t.Helper()
	source, err := fs.Sub(providerV3EmbeddedArtifacts, "artifacts/v3")
	if err != nil {
		t.Fatal(err)
	}
	artifacts := fstest.MapFS{}
	if err := fs.WalkDir(source, ".", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(source, name)
		if err != nil {
			return err
		}
		artifacts[name] = &fstest.MapFile{Data: append([]byte(nil), raw...), Mode: 0o444}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return artifacts
}

func readV3Closure(t *testing.T, artifacts fstest.MapFS) v3ArtifactClosure {
	t.Helper()
	var closure v3ArtifactClosure
	if err := formpackage.DecodeStrictIJSON(artifacts["closure.json"].Data, &closure); err != nil {
		t.Fatal(err)
	}
	return closure
}

func readV3Projection(t *testing.T, artifacts fstest.MapFS) v3ProviderProjection {
	t.Helper()
	closure := readV3Closure(t, artifacts)
	var projection v3ProviderProjection
	if err := formpackage.DecodeStrictIJSON(artifacts[closure.Projection.Path].Data, &projection); err != nil {
		t.Fatal(err)
	}
	return projection
}

func writeV3Projection(t *testing.T, artifacts fstest.MapFS, projection v3ProviderProjection) {
	t.Helper()
	closure := readV3Closure(t, artifacts)
	raw := writeV3JSONFile(t, artifacts, closure.Projection.Path, projection)
	digest, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	closure.Projection.Digest = digest
	writeV3JSONFile(t, artifacts, "closure.json", closure)
}

func writeV3JSONFile(t *testing.T, artifacts fstest.MapFS, name string, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	artifacts[name] = &fstest.MapFile{Data: raw, Mode: 0o444}
	return raw
}

func mutateV3DefaultToRetainedSameKind(t *testing.T, projection *v3ProviderProjection) {
	t.Helper()
	retained := map[string]v3ProjectedForm{}
	for _, form := range projection.Forms {
		if form.Generation != v3ProjectionCurrent {
			retained[form.Ref.Kind] = form
		}
	}
	for index, ref := range projection.DefaultCreates {
		if historical, ok := retained[ref.Kind]; ok && historical.Ref.APIVersion != ref.APIVersion {
			projection.DefaultCreates[index] = historical.Ref
			return
		}
	}
	t.Fatal("projection has no current/retained same-Kind pair in distinct groups")
}

func reverseV3[T any](values []T) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func providerV3AssemblyFingerprint(t *testing.T, assembly *v3ProviderAssembly) []byte {
	t.Helper()
	resources := make([]string, 0, len(assembly.resourceTypes.byRef))
	for key, resourceType := range assembly.resourceTypes.byRef {
		resources = append(resources, key.String()+"="+resourceType)
	}
	sort.Strings(resources)
	codecs := make([]struct {
		Key     string         `json:"key"`
		Form    any            `json:"form"`
		Desired map[string]any `json:"desiredSchema"`
	}, 0, len(assembly.codecs.codecs))
	for key, codec := range assembly.codecs.codecs {
		codecs = append(codecs, struct {
			Key     string         `json:"key"`
			Form    any            `json:"form"`
			Desired map[string]any `json:"desiredSchema"`
		}{Key: key.String(), Form: codec.Form, Desired: codec.DesiredSchema})
	}
	sort.Slice(codecs, func(i, j int) bool { return codecs[i].Key < codecs[j].Key })
	payload := struct {
		SnapshotDigest string   `json:"snapshotDigest"`
		Forms          any      `json:"forms"`
		Interfaces     any      `json:"interfaces"`
		Bindings       any      `json:"bindings"`
		CurrentForms   any      `json:"currentForms"`
		SupportedRefs  any      `json:"supportedRefs"`
		Resources      []string `json:"resources"`
		Codecs         any      `json:"codecs"`
	}{
		SnapshotDigest: assembly.snapshot.Digest(), Forms: assembly.snapshot.Forms(), Interfaces: assembly.snapshot.Interfaces(),
		Bindings: assembly.snapshot.Bindings(), CurrentForms: assembly.currentForms, SupportedRefs: assembly.registry.SupportedRefs(),
		Resources: resources, Codecs: codecs,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
