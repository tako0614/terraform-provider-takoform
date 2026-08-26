package provider

import (
	"fmt"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

// v3FormRef is the Provider-owned exact Form identity plus package provenance
// carried by the embedded Provider projection. FormRef is the portable
// identity; PackageDigest is distribution evidence and is deliberately not
// part of the state lookup key.
type v3FormRef struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	PackageDigest     string `json:"packageDigest"`
}

func (ref v3FormRef) FormRef() formpackage.FormRef {
	return formpackage.FormRef{
		APIVersion: ref.APIVersion, Kind: ref.Kind,
		DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
	}
}

func (ref v3FormRef) ExactKey() v3ExactFormKey {
	return v3ExactFormKey{
		APIVersion: ref.APIVersion, Kind: ref.Kind,
		DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
	}
}

// v3ExactFormKey is the whole contract identity used for state dispatch.
// Package provenance is intentionally excluded.
type v3ExactFormKey struct {
	APIVersion        string
	Kind              string
	DefinitionVersion string
	SchemaDigest      string
}

func (key v3ExactFormKey) GroupKind() v3GroupKind {
	return v3GroupKind{APIVersion: key.APIVersion, Kind: key.Kind}
}

func (key v3ExactFormKey) String() string {
	return fmt.Sprintf("%s/%s@%s schema=%s", key.APIVersion, key.Kind, key.DefinitionVersion, key.SchemaDigest)
}

// v3GroupKind identifies a Provider Form line's namespaced group and kind.
// It is only used to select a default create target; state always uses the
// complete v3ExactFormKey.
type v3GroupKind struct {
	APIVersion string
	Kind       string
}
