// Package formregistry embeds the exact data-only Legacy Form identities
// supported by this provider build. The host independently decides whether an
// exact package is installed, supported, activated, executable, and visible to
// the calling principal. None of those decisions changes Form maturity.
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

// candidate-refs.json is the retained pre-reset projection of
// forms/standard-package-set.json. The old generator is retired.
//
//go:embed candidate-refs.json
var candidateRefsJSON []byte

// successor-refs.json is generated beside candidate-refs.json and retains
// additional major-version candidates without replacing supported historical
// identities for the same Kind.
//
//go:embed successor-refs.json
var successorRefsJSON []byte

var candidateRefs = mustDecodeCandidateRefs(candidateRefsJSON)
var successorRefs = mustDecodeSuccessorRefs(successorRefsJSON)

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
	var refs map[string]map[string]Ref
	if err := json.Unmarshal(raw, &refs); err != nil {
		panic(fmt.Errorf("takoform: decode embedded successor FormRefs: %w", err))
	}
	for kind, versions := range refs {
		if kind == "" || len(versions) == 0 {
			panic(fmt.Errorf("takoform: embedded successor FormRefs for %q are incomplete", kind))
		}
		for version, ref := range versions {
			if ref.APIVersion != APIVersion || ref.Kind != kind || ref.DefinitionVersion != version ||
				ref.SchemaDigest == "" || ref.PackageDigest == "" {
				panic(fmt.Errorf("takoform: embedded successor FormRef for %s@%s is incomplete", kind, version))
			}
		}
	}
	return refs
}

// ForKind returns the exact Legacy compatibility Form identity compiled into
// this provider build. Support, activation, and availability are host-owned.
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
	if versions, ok := successorRefs[kind]; ok {
		if ref, ok := versions[definitionVersion]; ok {
			return ref, nil
		}
	}
	return Ref{}, fmt.Errorf("takoform: provider build has no candidate FormRef for %s@%s", kind, definitionVersion)
}

// All returns a defensive copy of every embedded candidate identity.
func All() map[string]Ref {
	out := make(map[string]Ref, len(candidateRefs))
	for kind, ref := range candidateRefs {
		out[kind] = ref
	}
	return out
}
