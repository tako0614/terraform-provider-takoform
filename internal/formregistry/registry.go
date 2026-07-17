// Package formregistry pins the exact data-only Form identities owned by this
// provider release. Hosts still decide whether an exact package is installed,
// activated, executable, and available to the calling principal.
package formregistry

import "fmt"

const APIVersion = "forms.takoform.com/v1alpha1"

// Ref identifies one immutable Form Definition and package.
type Ref struct {
	APIVersion        string `json:"apiVersion"`
	Kind              string `json:"kind"`
	DefinitionVersion string `json:"definitionVersion"`
	SchemaDigest      string `json:"schemaDigest"`
	PackageDigest     string `json:"packageDigest"`
}

var releaseRefs = map[string]Ref{
	"EdgeWorker": {
		APIVersion: APIVersion, Kind: "EdgeWorker", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:ce55ac9ea700ac391637ca29f149439ee0fcc54a9983d4513023f097cccf02b0",
		PackageDigest: "sha256:c44e2ad933de36d77c61b9b24df76a56b9ee9ff265e3085e88061ce755d6f8b6",
	},
	"ObjectBucket": {
		APIVersion: APIVersion, Kind: "ObjectBucket", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:ee32286a40681296fc6f3db9ece79c2d651821aa2e947d1fa1cd6e28e8be8391",
		PackageDigest: "sha256:0c43dfbf565c959ad627a6cd8d19aa77bf56d9e3655f44f71bb207fb79b264f2",
	},
	"KVStore": {
		APIVersion: APIVersion, Kind: "KVStore", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:3b3f8d369eba1e41c4de7093229698ecc54c30103351e670422f2da4d8a033d6",
		PackageDigest: "sha256:7bdc1933764bcd7687980acb97b8fcb82f12ce7a5e853c988b508997a60895dd",
	},
	"Queue": {
		APIVersion: APIVersion, Kind: "Queue", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:313fc48201f2b324519d5869a2b819df31a09411704265f3ad633bc0d7384a15",
		PackageDigest: "sha256:87dcc5fb75f980ade0b0775751a0ee6a49cd8ff3aab4523f4bf8651719c7dd0e",
	},
	"SQLDatabase": {
		APIVersion: APIVersion, Kind: "SQLDatabase", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:8ba271241cca83d802c0e3e2e3fc1ee488ef912389364ca35e7db54abbb6c17c",
		PackageDigest: "sha256:1e206b6bcaf069a4fd6aea48cc5a2262b2758e47bf29dddef690ba4ba3f97a90",
	},
	"ContainerService": {
		APIVersion: APIVersion, Kind: "ContainerService", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:85f290f96799788f8bd544894b9a26f8e0b2551b6537ad8eb4c7e11fea52d9d7",
		PackageDigest: "sha256:13fee163873e9ed84c6be612b28c1df273fdeeedfbc8ddbb405f14e897c0d075",
	},
	"VectorIndex": {
		APIVersion: APIVersion, Kind: "VectorIndex", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:328c601c50511184d46266f28cdb09a46d3b526127d46ca66f2a8f41f04bc884",
		PackageDigest: "sha256:968bdd942bfa404f38eb33cc9adca185b108c7653b7030d236186b9c5521cb00",
	},
	"DurableWorkflow": {
		APIVersion: APIVersion, Kind: "DurableWorkflow", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:fb713cdaa4db5da7dbfae1b106fc9ac498566356026f3c3427eb90a2c356ba7d",
		PackageDigest: "sha256:8bc7d360e007a1d69ba8ba9aae2356da3a03c0e32ca9a6dbffe173fa42fcef59",
	},
	"StatefulActorNamespace": {
		APIVersion: APIVersion, Kind: "StatefulActorNamespace", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:feb30f237bee2f8baaeefb70147c96ed84d3f3d7a71963e290b135bdec83962f",
		PackageDigest: "sha256:b9a1fea011dcc1b049c21abeef214519982f138ff832812e9546f3918c7bb1ea",
	},
	"Schedule": {
		APIVersion: APIVersion, Kind: "Schedule", DefinitionVersion: "0.0.0-legacy.1",
		SchemaDigest:  "sha256:04595cf30f2e92f899bda8655db3e6677c408a30f615d79b4273e4b0f98bf7ba",
		PackageDigest: "sha256:50f97f5bf7f62763103bf716e3a9efba8856ef5fc7d117934c0dd6f896eea4c6",
	},
}

// ForKind returns the exact Form identity compiled into this provider release.
func ForKind(kind string) (Ref, error) {
	ref, ok := releaseRefs[kind]
	if !ok {
		return Ref{}, fmt.Errorf("takoform: provider release has no FormRef for kind %q", kind)
	}
	return ref, nil
}

// All returns a defensive copy of every release-owned identity.
func All() map[string]Ref {
	out := make(map[string]Ref, len(releaseRefs))
	for kind, ref := range releaseRefs {
		out[kind] = ref
	}
	return out
}
