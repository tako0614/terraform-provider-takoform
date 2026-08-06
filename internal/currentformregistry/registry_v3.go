package currentformregistry

import (
	"fmt"
	"sort"
)

// V3Ref identifies one exact Form Family candidate Definition and package for
// the provider-v3 (v1alpha3 wire) lane. Unlike the v1alpha2 Ref, its
// APIVersion is a namespaced family group, so one kind name can exist in
// several groups without colliding.
type V3Ref struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	PackageDigest     string `json:"packageDigest"`
}

func v3Key(apiVersion, kind string) string { return apiVersion + "/" + kind }

// V3ForKind returns the recommended create target for one exact family kind.
// State written by an older FormRef of the same kind remains readable through
// V3SupportedFormRefs when a Form line advances.
func V3ForKind(apiVersion, kind string) (V3Ref, error) {
	value, ok := v3Refs[v3Key(apiVersion, kind)]
	if !ok {
		return V3Ref{}, fmt.Errorf("takoform: provider v3 has no Form candidate for %s kind %q", apiVersion, kind)
	}
	return value, nil
}

// V3SupportedFormRefs returns every exact family FormRef this provider build
// can read, observe, update, and delete, in a stable order.
func V3SupportedFormRefs() []V3Ref {
	keys := make([]string, 0, len(v3Refs))
	for key := range v3Refs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]V3Ref, 0, len(keys))
	for _, key := range keys {
		out = append(out, v3Refs[key])
	}
	return out
}

// V3All returns a defensive copy of the exact family candidate set, keyed
// "<group>/<groupVersion>/<Kind>".
func V3All() map[string]V3Ref {
	out := make(map[string]V3Ref, len(v3Refs))
	for key, value := range v3Refs {
		out[key] = value
	}
	return out
}
