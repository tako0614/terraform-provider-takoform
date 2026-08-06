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

// ForKind returns the recommended create target (the default line) for one
// kind. It is not the only supported identity: existing state written by an
// older FormRef of the same kind remains readable through SupportedFormRefs.
func ForKind(kind string) (Ref, error) {
	value, ok := refs[kind]
	if !ok {
		return Ref{}, fmt.Errorf("takoform: provider v2 has no v1alpha2 Form candidate for kind %q", kind)
	}
	return value, nil
}

// SupportedFormRefs returns every exact FormRef this provider build can read,
// observe, update, and delete. The current generation has one entry per kind;
// when a Form line advances, the generated refs map gains the new FormRef
// while retaining the old one, so existing state never becomes unreadable.
func SupportedFormRefs() []Ref {
	out := make([]Ref, 0, len(refs))
	for _, value := range refs {
		out = append(out, value)
	}
	return out
}

// All returns a defensive copy of the exact candidate set.
func All() map[string]Ref {
	out := make(map[string]Ref, len(refs))
	for kind, value := range refs {
		out[kind] = value
	}
	return out
}
