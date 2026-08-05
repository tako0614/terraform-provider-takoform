package currentformcatalog

import (
	"slices"
	"strings"
	"testing"
)

func TestCurrentCatalogIsExactNineFormReset(t *testing.T) {
	t.Parallel()
	if len(Kinds) != len(orderedKinds) {
		t.Fatalf("current kinds = %d, want %d", len(Kinds), len(orderedKinds))
	}
	for index, kind := range Kinds {
		if kind.Kind != orderedKinds[index] || kind.DefinitionVersion != "0.1.0" {
			t.Fatalf("current kind[%d] = %s@%s", index, kind.Kind, kind.DefinitionVersion)
		}
		if kind.ProposalID == "" {
			t.Fatalf("current kind[%d] %s has no Proposal identity", index, kind.Kind)
		}
	}
}

func TestCurrentCatalogExcludesSubstrateOperations(t *testing.T) {
	t.Parallel()
	forbidden := map[string]struct{}{
		"accessProtocols": {}, "assetsNotFoundHandling": {}, "assetsPath": {},
		"concurrency": {}, "cpuMillicores": {}, "deliveryDelaySeconds": {},
		"healthCheckPath": {}, "highAvailability": {}, "maxBatchSize": {},
		"maxMessageBytes": {}, "maxRetries": {}, "memoryMib": {},
		"migrationTag": {}, "ports": {}, "publicHttp": {}, "replicas": {},
		"requestTimeoutSeconds": {}, "sizeClass": {}, "storageClass": {},
		"storageGib": {}, "visibilityTimeoutSeconds": {},
	}
	for _, kind := range Kinds {
		for _, field := range kind.Fields {
			if _, rejected := forbidden[field.Wire]; rejected {
				t.Errorf("%s carries host/operator field %s", kind.Kind, field.Wire)
			}
		}
	}
}

func TestCurrentCatalogHasReviewedSemanticFields(t *testing.T) {
	t.Parallel()
	want := map[string][]string{
		"EdgeWorker":         {"configuration", "entrypoint", "runtime", "runtimeVersion"},
		"RelationalDatabase": {"databaseName", "engine", "engineVersion", "schemaFormat", "schemaSha256", "schemaUrl"},
		"ObjectBucket":       {"versioning"},
		"KeyValueStore":      {"consistency", "defaultTtlSeconds"},
		"Queue":              {"messageRetentionSeconds", "ordering"},
		"Schedule":           {"cron", "timezone"},
		"ContainerService":   {"configuration", "image"},
		"StatefulEntity":     {"configuration", "entrypoint", "persistence", "runtime", "runtimeVersion"},
		"VectorIndex":        {"dimensions", "metric"},
	}
	for _, current := range Kinds {
		got := make([]string, 0, len(current.Fields))
		for _, field := range current.Fields {
			got = append(got, field.Wire)
		}
		slices.Sort(got)
		if !slices.Equal(got, want[current.Kind]) {
			t.Errorf("%s fields = %v, want reviewed semantic fields %v", current.Kind, got, want[current.Kind])
		}
	}
}

func TestRelationalDatabaseSchemaMaterialIsAllOrNone(t *testing.T) {
	kind, ok := ByKind("RelationalDatabase")
	if !ok {
		t.Fatal("RelationalDatabase is not declared")
	}
	desired := kind.CanonicalDesired()
	delete(desired, "schemaSha256")
	if err := kind.ValidateDesired(desired); err == nil {
		t.Fatal("RelationalDatabase accepted schemaUrl/schemaFormat without schemaSha256")
	}
}

func TestKeyValueStoreRejectsZeroDefaultTTL(t *testing.T) {
	kind, ok := ByKind("KeyValueStore")
	if !ok {
		t.Fatal("KeyValueStore is not declared")
	}
	desired := kind.CanonicalDesired()
	desired["defaultTtlSeconds"] = 0
	if err := kind.ValidateDesired(desired); err == nil {
		t.Fatal("KeyValueStore accepted an ambiguous zero default TTL")
	}
}

func TestCurrentCatalogUsesOneCanonicalDigestSpelling(t *testing.T) {
	t.Parallel()
	for _, kind := range Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()
			desired := kind.CanonicalDesired()
			if kind.Artifact {
				source := desired["source"].(map[string]any)
				source["artifactSha256"] = "sha256:" + strings.Repeat("A", 64)
				if err := kind.ValidateDesired(desired); err == nil {
					t.Fatal("current artifact accepted a non-canonical uppercase digest")
				}
			}
			if kind.Kind == "RelationalDatabase" {
				desired["schemaSha256"] = strings.Repeat("a", 64)
				if err := kind.ValidateDesired(desired); err == nil {
					t.Fatal("current schema bundle accepted a digest without its canonical algorithm prefix")
				}
			}
			if kind.Kind == "ContainerService" {
				desired["image"] = "registry.invalid/app@sha256:" + strings.Repeat("A", 64)
				if err := kind.ValidateDesired(desired); err == nil {
					t.Fatal("current OCI reference accepted an uppercase digest")
				}
			}
		})
	}
}
