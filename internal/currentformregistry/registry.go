// Package currentformregistry embeds the exact local v1alpha2 publication
// candidates selected by provider v2. Candidate presence is not publication,
// maturity, host activation, or commercial availability.
package currentformregistry

import "fmt"

const APIVersion = "forms.takoform.com/v1alpha2"

// Ref identifies one exact candidate Form Definition and package.
type Ref struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	PackageDigest     string `json:"packageDigest"`
}

func ref(kind, schemaDigest, packageDigest string) Ref {
	return Ref{APIVersion: APIVersion, Kind: kind, DefinitionVersion: "0.1.0", SchemaDigest: schemaDigest, PackageDigest: packageDigest}
}

// ForKind returns the exact candidate selected by this provider build.
func ForKind(kind string) (Ref, error) {
	value, ok := refs[kind]
	if !ok {
		return Ref{}, fmt.Errorf("takoform: provider v2 has no v1alpha2 Form candidate for kind %q", kind)
	}
	return value, nil
}

// All returns a defensive copy of the exact candidate set.
func All() map[string]Ref {
	out := make(map[string]Ref, len(refs))
	for kind, value := range refs {
		out[kind] = value
	}
	return out
}
