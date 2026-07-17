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
		APIVersion: APIVersion, Kind: "EdgeWorker", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:7cf4e5eb8d41b069cd790f624bbf5805c8689cb8b33b8c41cb21718aebe99906",
		PackageDigest: "sha256:fb4e5eb74392a986841f6030411474b3132ca9ec52f1beb33f46937ccb2cf17a",
	},
	"ObjectBucket": {
		APIVersion: APIVersion, Kind: "ObjectBucket", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:bf69d69780cd382784c974257514bbed27e9bf0e04b7ffca8f3e20507c8ea4cd",
		PackageDigest: "sha256:bd52b33978b33fa3b62a7bd484fd43f261e87c6ff19a8223047de578b25d0e48",
	},
	"KVStore": {
		APIVersion: APIVersion, Kind: "KVStore", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:ea3cd77484cb969e1ffaa2e57d11a75536063a641da2e72835a87327e1e50fc3",
		PackageDigest: "sha256:4142dec1ce4664a651cf532b10c6be73d1e017c05ec75551d72979f70dca6d3e",
	},
	"Queue": {
		APIVersion: APIVersion, Kind: "Queue", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:62fbabb0058bf9459f1fa328c7da6e412a63f76c14545f1fd218f0fb74b32288",
		PackageDigest: "sha256:d5f8009e5deeae74a7b10527c420928d5f7597cdc0175eb6cfc7e984ccf69670",
	},
	"SQLDatabase": {
		APIVersion: APIVersion, Kind: "SQLDatabase", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:f6a931e2de4cec3b934f1597bc1cbb0dfada416d448d84e2018ef7935b162bde",
		PackageDigest: "sha256:9c1220a2fc24ff7b3ab5617d3037e1f25c9ac3058fbf212de8e9da7fcd0c9dba",
	},
	"ContainerService": {
		APIVersion: APIVersion, Kind: "ContainerService", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:99bf9cea2c3b79b9accc703d6f7a2b23f9a19efb11d077ec29de8e6ec037b459",
		PackageDigest: "sha256:7bddce8a51762e6c2005ba251dc2845ca7ff4d94704eac6595743a8c53ec5d81",
	},
	"VectorIndex": {
		APIVersion: APIVersion, Kind: "VectorIndex", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:23e5a3b8ee40cdeea7c2a00bc7c382b6c8796c09aa90ffcf91b19e45a043b484",
		PackageDigest: "sha256:9eae6a727b8b53ae6a5b082687c7d867c70443cb3d69ee01b67293963fcfcfd7",
	},
	"DurableWorkflow": {
		APIVersion: APIVersion, Kind: "DurableWorkflow", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:6d0f1a9022c875dcb5d538819c90c763662ee5ac2d07880bb2503b24ad94f492",
		PackageDigest: "sha256:72d9bf771a470fd6e7a0cba0258dbe89e5401c11f3590c7c4a2e0c5482ebb40c",
	},
	"StatefulActorNamespace": {
		APIVersion: APIVersion, Kind: "StatefulActorNamespace", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:15e2871d15f705dd76357340e44713dde0266b19123da1f309a754c7234b8e6d",
		PackageDigest: "sha256:85bb04b52f82fba652b1568dc9c03e0353774d415e55e288a319ab2b71017860",
	},
	"Schedule": {
		APIVersion: APIVersion, Kind: "Schedule", DefinitionVersion: "1.0.0",
		SchemaDigest:  "sha256:9ec8c9650e42d2f2f0fb456b0cf718270dbfca7dd754fe929f1fa734f9c111b3",
		PackageDigest: "sha256:49af4f16cbd3f6f0de23044c5522343fc4f34dd0ee2d5584bb0be7644dbaae7b",
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
