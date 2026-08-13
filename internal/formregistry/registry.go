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

// successor-refs.json is generated beside candidate-refs.json and retains
// additional major-version candidates without replacing supported historical
// identities for the same Kind.
//
//go:embed successor-refs.json
var successorRefsJSON []byte

// retained-refs.json is deliberately hand-pinned to exact identities from
// already-published provider releases. It is not generated from the current
// package set: regeneration must never rewrite a historical state codec into
// the current Form merely because its Kind still exists.
//
//go:embed retained-refs.json
var retainedRefsJSON []byte

var candidateRefs = mustDecodeCandidateRefs(candidateRefsJSON)
var successorRefs = mustDecodeSuccessorRefs(successorRefsJSON)
var retainedRefs = mustDecodeVersionedRefs("retained", retainedRefsJSON)

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

func mustDecodeSuccessorRefs(raw []byte) map[string]map[string]Ref {
	return mustDecodeVersionedRefs("successor", raw)
}

func mustDecodeVersionedRefs(label string, raw []byte) map[string]map[string]Ref {
	var refs map[string]map[string]Ref
	if err := json.Unmarshal(raw, &refs); err != nil {
		panic(fmt.Errorf("takoform: decode embedded %s FormRefs: %w", label, err))
	}
	for kind, versions := range refs {
		if kind == "" || len(versions) == 0 {
			panic(fmt.Errorf("takoform: embedded %s FormRefs for %q are incomplete", label, kind))
		}
		for version, ref := range versions {
			if ref.APIVersion != APIVersion || ref.Kind != kind || ref.DefinitionVersion != version ||
				ref.SchemaDigest == "" || ref.PackageDigest == "" {
				panic(fmt.Errorf("takoform: embedded %s FormRef for %s@%s is incomplete", label, kind, version))
			}
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

// ForKindVersion returns one exact supported identity without changing the
// historical default returned by ForKind.
func ForKindVersion(kind, definitionVersion string) (Ref, error) {
	if current, ok := candidateRefs[kind]; ok && current.DefinitionVersion == definitionVersion {
		return current, nil
	}
	if versions, ok := retainedRefs[kind]; ok {
		if ref, ok := versions[definitionVersion]; ok {
			return ref, nil
		}
	}
	if versions, ok := successorRefs[kind]; ok {
		if ref, ok := versions[definitionVersion]; ok {
			return ref, nil
		}
	}
	return Ref{}, fmt.Errorf("takoform: provider build has no candidate FormRef for %s@%s", kind, definitionVersion)
}

// ForExact returns a supported reference only when all five identity fields
// are byte-for-byte equal to a compiled registry entry. In particular it does
// not use SemVer ranges, select a nearest version, or trust a familiar Kind.
func ForExact(want Ref) (Ref, error) {
	ref, err := ForKindVersion(want.Kind, want.DefinitionVersion)
	if err != nil || ref != want {
		return Ref{}, fmt.Errorf(
			"takoform: provider build has no exact FormRef for %s@%s schema=%s package=%s",
			want.Kind,
			want.DefinitionVersion,
			want.SchemaDigest,
			want.PackageDigest,
		)
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
