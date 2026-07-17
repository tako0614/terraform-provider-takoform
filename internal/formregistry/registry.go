// Package formregistry embeds the exact data-only Form identities proposed by
// this provider build. The embedded set remains a structural candidate until
// an external host/provider admission process authenticates it; a host still
// decides whether an exact package is installed, admitted, executable, and
// available to the calling principal.
package formregistry

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

const APIVersion = "forms.takoform.com/v1alpha1"

// Ref identifies one immutable Form Definition and package.
type Ref struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	PackageDigest     string `json:"packageDigest"`
}

// candidate-refs.json is generated from forms/standard-package-set.json by
// `go run ./cmd/standard-form-conformance generate`.
//
//go:embed candidate-refs.json
var candidateRefsJSON []byte

var candidateRefs = mustDecodeCandidateRefs(candidateRefsJSON)

func mustDecodeCandidateRefs(raw []byte) map[string]Ref {
	var refs map[string]Ref
	if err := json.Unmarshal(raw, &refs); err != nil {
		panic(fmt.Errorf("takoform: decode embedded candidate FormRefs: %w", err))
	}
	for kind, ref := range refs {
		if kind == "" || ref.APIVersion != APIVersion || ref.Kind != kind ||
			ref.DefinitionVersion == "" || ref.SchemaDigest == "" || ref.PackageDigest == "" {
			panic(fmt.Errorf("takoform: embedded candidate FormRef for %q is incomplete", kind))
		}
	}
	return refs
}

// ForKind returns the exact structural-candidate Form identity compiled into
// this provider build. Availability and admission remain host-owned checks.
func ForKind(kind string) (Ref, error) {
	ref, ok := candidateRefs[kind]
	if !ok {
		return Ref{}, fmt.Errorf("takoform: provider build has no candidate FormRef for kind %q", kind)
	}
	return ref, nil
}

// All returns a defensive copy of every embedded candidate identity.
func All() map[string]Ref {
	out := make(map[string]Ref, len(candidateRefs))
	for kind, ref := range candidateRefs {
		out[kind] = ref
	}
	return out
}
