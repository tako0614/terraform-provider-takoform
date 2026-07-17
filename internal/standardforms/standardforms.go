package standardforms

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/portableconformance"
	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

const (
	definitionVersion = "1.0.0"
	packageVersion    = "1.0.0"
)

type Spec struct {
	Kind        string
	Slug        string
	Title       string
	Description string
	Immutable   []string
}

var Specs = []Spec{
	{Kind: "EdgeWorker", Slug: "edge-worker", Title: "Edge Worker", Description: "Provider-neutral edge application backed by a prebuilt immutable artifact.", Immutable: []string{"/name"}},
	{Kind: "ObjectBucket", Slug: "object-bucket", Title: "Object Bucket", Description: "Provider-neutral object storage with a portable default storage class.", Immutable: []string{"/name"}},
	{Kind: "KVStore", Slug: "kv-store", Title: "Key Value Store", Description: "Provider-neutral key/value state with an optional consistency preference.", Immutable: []string{"/name"}},
	{Kind: "SQLDatabase", Slug: "sql-database", Title: "SQL Database", Description: "Provider-neutral SQL storage with an open engine capability token.", Immutable: []string{"/name", "/engine"}},
	{Kind: "Queue", Slug: "queue", Title: "Queue", Description: "Provider-neutral asynchronous delivery and event fan-out.", Immutable: []string{"/name"}},
	{Kind: "VectorIndex", Slug: "vector-index", Title: "Vector Index", Description: "Provider-neutral vector index with dimensions fixed for the materialized index lifecycle.", Immutable: []string{"/name", "/dimensions"}},
	{Kind: "DurableWorkflow", Slug: "durable-workflow", Title: "Durable Workflow", Description: "Provider-neutral versioned durable workflow definition and instance-state lifecycle.", Immutable: []string{"/name"}},
	{Kind: "ContainerService", Slug: "container-service", Title: "Container Service", Description: "Provider-neutral OCI container service, separate from an edge worker.", Immutable: []string{"/name"}},
	{Kind: "StatefulActorNamespace", Slug: "stateful-actor-namespace", Title: "Stateful Actor Namespace", Description: "Provider-neutral namespace, class, and storage contract for stateful actors.", Immutable: []string{"/name"}},
	{Kind: "Schedule", Slug: "schedule", Title: "Schedule", Description: "Provider-neutral five-field cron lifecycle with one explicit invokable connection.", Immutable: []string{"/name"}},
}

var externalRequirements = []string{
	"immutable-release-tag",
	"registry-install-readback",
	"sigstore-signature-and-provenance",
	"takosumi-portable-host-lifecycle-proof",
	"terraform-provider-protocol-lifecycle-proof",
	"portable-invalid-argument-negative-lifecycle-proof",
	"signed-standard-admission-evidence",
}

type Inventory struct {
	Format              string           `json:"format"`
	Classification      string           `json:"classification"`
	DefinitionVersion   string           `json:"definitionVersion"`
	PackageVersion      string           `json:"packageVersion"`
	LocalConformance    string           `json:"localConformance"`
	PublicationReady    bool             `json:"publicationReady"`
	AdmissionStatus     string           `json:"admissionStatus"`
	ExternalRequired    []string         `json:"externalRequired"`
	ConformanceManifest string           `json:"conformanceManifest"`
	Packages            []InventoryEntry `json:"packages"`
}

type InventoryEntry struct {
	Kind            string              `json:"kind"`
	Path            string              `json:"path"`
	AdmissionStatus string              `json:"admissionStatus"`
	ConformanceCase string              `json:"conformanceCase"`
	FormRef         formpackage.FormRef `json:"formRef"`
	PackageDigest   string              `json:"packageDigest"`
}

func Generate(root string) error {
	entries := make([]InventoryEntry, 0, len(Specs))
	for _, spec := range Specs {
		entry, err := generatePackage(root, spec)
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	inventory := Inventory{
		Format: "takoform.standard-package-set@v1", Classification: "structural-candidate",
		DefinitionVersion: definitionVersion, PackageVersion: packageVersion, LocalConformance: "structural-only",
		PublicationReady: false, AdmissionStatus: "external-required",
		ExternalRequired:    append([]string(nil), externalRequirements...),
		ConformanceManifest: "conformance/form-package-v1/manifest.json", Packages: entries,
	}
	if err := writeJSON(filepath.Join(root, "forms", "standard-package-set.json"), inventory); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(root, "conformance", "standard-form-admission-v1")); err != nil {
		return err
	}
	if err := updateConformanceManifest(root, entries); err != nil {
		return err
	}
	return updatePortableHostContract(root, entries)
}

func generatePackage(root string, spec Spec) (InventoryEntry, error) {
	desiredSchema, err := desiredSchema(spec.Kind)
	if err != nil {
		return InventoryEntry{}, err
	}
	desired, err := canonicalDesired(spec.Kind)
	if err != nil {
		return InventoryEntry{}, err
	}
	name, _ := desired["name"].(string)
	definition := formpackage.FormDefinition{
		APIVersion: formpackage.FormAPIVersion, Kind: spec.Kind, DefinitionVersion: definitionVersion,
		Title: spec.Title, Description: spec.Description, Status: "standard",
		DesiredSchema: desiredSchema, ObservedSchema: observedSchema(desiredSchema), OutputSchema: outputSchema(spec.Kind, desiredSchema),
		ImmutableFields:       append([]string(nil), spec.Immutable...),
		LifecycleCapabilities: []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		ConformanceFixtures: []formpackage.ConformanceFixture{{
			Name: "canonical", DesiredPath: "fixtures/desired.json", ObservedPath: "fixtures/observed.json", OutputPath: "fixtures/output.json",
		}},
		NegativeFixtures: []formpackage.NegativeFixture{{
			Name: "reject-invalid-semantics", Stage: "desired", InputPath: "fixtures/negative.json", ExpectedFailure: "schema_validation_failed",
		}},
	}
	observed := map[string]any{
		"applied": desired, "driftedFields": []any{}, "generation": 1, "id": spec.Kind + "/" + name,
		"imported": true, "portability": "portable", "ready": true,
	}
	output := map[string]any{
		"generation": 1, "id": spec.Kind + "/" + name, "kind": spec.Kind, "name": name,
		"portability": "portable", "portableSpec": desired,
	}
	negative, err := negativeDesired(spec.Kind, desired)
	if err != nil {
		return InventoryEntry{}, err
	}
	packageRoot := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", spec.Slug)
	if err := os.RemoveAll(packageRoot); err != nil {
		return InventoryEntry{}, err
	}
	files := map[string]any{
		"definition.json": definition, "fixtures/desired.json": desired, "fixtures/negative.json": negative,
		"fixtures/observed.json": observed, "fixtures/output.json": output,
	}
	for relative, value := range files {
		if err := writeJSON(filepath.Join(packageRoot, filepath.FromSlash(relative)), value); err != nil {
			return InventoryEntry{}, err
		}
	}
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, "definition.json"))
	if err != nil {
		return InventoryEntry{}, err
	}
	schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		return InventoryEntry{}, err
	}
	ref := formpackage.FormRef{APIVersion: formpackage.FormAPIVersion, Kind: spec.Kind, DefinitionVersion: definitionVersion, SchemaDigest: schemaDigest}
	paths := make([]string, 0, len(files))
	for relative := range files {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	packageFiles := make([]formpackage.PackageFile, 0, len(paths))
	for _, relative := range paths {
		raw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(relative)))
		if err != nil {
			return InventoryEntry{}, err
		}
		mediaType := "application/json"
		if relative == "definition.json" {
			mediaType = formpackage.DefinitionMediaType
		}
		packageFiles = append(packageFiles, formpackage.PackageFile{Path: relative, MediaType: mediaType, Size: int64(len(raw)), Digest: formpackage.DigestBytes(raw)})
	}
	index := formpackage.PackageIndex{APIVersion: formpackage.PackageAPIVersion, Kind: formpackage.PackageKind, PackageVersion: packageVersion, FormRef: ref, DefinitionPath: "definition.json", Files: packageFiles}
	if err := writeJSON(filepath.Join(packageRoot, formpackage.PackageIndexFilename), index); err != nil {
		return InventoryEntry{}, err
	}
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return InventoryEntry{}, fmt.Errorf("verify generated %s: %w", spec.Kind, err)
	}
	if err := provider.VerifyStandardFormStructure(spec.Kind, desired); err != nil {
		return InventoryEntry{}, err
	}
	return InventoryEntry{
		Kind: spec.Kind, Path: filepath.ToSlash(filepath.Join("conformance", "form-package-v1", "positive", "standard", spec.Slug)),
		AdmissionStatus: "external-required", ConformanceCase: "standard-" + spec.Slug + "-package", FormRef: report.FormRef, PackageDigest: report.PackageDigest,
	}, nil
}

func Verify(root string) error {
	var inventory Inventory
	if err := readJSON(filepath.Join(root, "forms", "standard-package-set.json"), &inventory); err != nil {
		return err
	}
	if inventory.Format != "takoform.standard-package-set@v1" || inventory.Classification != "structural-candidate" || inventory.PublicationReady || inventory.LocalConformance != "structural-only" || inventory.AdmissionStatus != "external-required" || !reflect.DeepEqual(inventory.ExternalRequired, externalRequirements) || len(inventory.Packages) != len(Specs) {
		return fmt.Errorf("standard package inventory identity or release truth is invalid")
	}
	if _, err := os.Stat(filepath.Join(root, "conformance", "standard-form-admission-v1")); err == nil {
		return fmt.Errorf("structural-only verification must not emit passed standard-admission evidence")
	} else if !os.IsNotExist(err) {
		return err
	}
	for _, entry := range inventory.Packages {
		if entry.AdmissionStatus != "external-required" {
			return fmt.Errorf("%s admission evidence status is not external-required", entry.Kind)
		}
		packageRoot := filepath.Join(root, filepath.FromSlash(entry.Path))
		report, err := formpackage.VerifyDirectory(packageRoot)
		if err != nil {
			return fmt.Errorf("%s package: %w", entry.Kind, err)
		}
		if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
			return fmt.Errorf("%s inventory digest drift", entry.Kind)
		}
		compiled, err := formregistry.ForKind(entry.Kind)
		if err != nil {
			return err
		}
		if compiled.APIVersion != entry.FormRef.APIVersion || compiled.Kind != entry.FormRef.Kind ||
			compiled.DefinitionVersion != entry.FormRef.DefinitionVersion || compiled.SchemaDigest != entry.FormRef.SchemaDigest ||
			compiled.PackageDigest != entry.PackageDigest {
			return fmt.Errorf("%s provider release ref drift", entry.Kind)
		}
		var desired map[string]any
		if err := readJSON(filepath.Join(packageRoot, "fixtures", "desired.json"), &desired); err != nil {
			return err
		}
		if err := provider.VerifyStandardFormStructure(entry.Kind, desired); err != nil {
			return err
		}
	}
	return nil
}

func desiredSchema(kind string) (map[string]any, error) {
	name := map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": ".*\\S.*"}
	properties := map[string]any{"name": name}
	required := []string{"name"}
	defs := map[string]any{}
	addConnections := func(requiredConnection bool) {
		for key, value := range connectionDefinitions() {
			defs[key] = value
		}
		properties["connections"] = map[string]any{"$ref": "#/$defs/connections"}
		if requiredConnection {
			required = append(required, "connections")
		}
	}
	addArtifact := func() {
		for key, value := range artifactDefinitions() {
			defs[key] = value
		}
		properties["source"] = map[string]any{"$ref": "#/$defs/artifactSource"}
		required = append(required, "source")
	}
	switch kind {
	case "EdgeWorker":
		addArtifact()
		addConnections(false)
		properties["compatibilityDate"] = map[string]any{"type": "string", "minLength": 1}
		properties["compatibilityFlags"] = tokenArraySchema()
		properties["profiles"] = tokenArraySchema()
	case "ObjectBucket":
		properties["storageClass"] = map[string]any{"type": "string", "enum": []string{"standard", "infrequent_access"}, "default": "standard"}
		properties["interfaces"] = tokenArraySchema()
	case "KVStore":
		properties["consistency"] = map[string]any{"type": "string", "enum": []string{"eventual", "strong"}}
	case "SQLDatabase":
		properties["engine"] = tokenSchema("sqlite")
		properties["migrationsPath"] = map[string]any{"type": "string", "minLength": 1}
	case "Queue":
		properties["delivery"] = map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"maxRetries":   map[string]any{"type": "integer", "minimum": 0},
				"maxBatchSize": map[string]any{"type": "integer", "minimum": 0},
			},
		}
	case "VectorIndex":
		addConnections(false)
		properties["dimensions"] = map[string]any{"type": "integer", "minimum": 1}
		properties["metric"] = tokenSchema("cosine")
		required = append(required, "dimensions")
	case "DurableWorkflow":
		addArtifact()
		addConnections(false)
		properties["entrypoint"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 256, "pattern": ".*\\S.*"}
		properties["retry"] = map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"maxAttempts":           map[string]any{"type": "integer", "minimum": 1},
				"initialBackoffSeconds": map[string]any{"type": "integer", "minimum": 0},
			},
		}
		required = append(required, "entrypoint")
	case "ContainerService":
		addConnections(false)
		properties["image"] = map[string]any{"type": "string", "pattern": "^[^@\\s]+@sha256:[A-Fa-f0-9]{64}$"}
		properties["ports"] = map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 1, "maximum": 65535}}
		properties["publicHttp"] = map[string]any{"type": "boolean"}
		properties["environment"] = typedStringMapSchema()
		required = append(required, "image")
	case "StatefulActorNamespace":
		addConnections(false)
		properties["className"] = map[string]any{"type": "string", "pattern": "^[A-Za-z_$][A-Za-z0-9_$]*$"}
		properties["storageProfile"] = tokenSchema("durable_sqlite")
		properties["migrationTag"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128, "pattern": ".*\\S.*"}
		required = append(required, "className")
	case "Schedule":
		for key, value := range scheduleConnectionDefinitions() {
			defs[key] = value
		}
		properties["cron"] = map[string]any{"type": "string", "pattern": "^[0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)? [0-9*,-]+(?:/[1-9][0-9]*)?$"}
		properties["timezone"] = map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:/+-]{0,127}$", "default": "UTC"}
		properties["connections"] = map[string]any{"$ref": "#/$defs/connections"}
		required = append(required, "cron", "connections")
	default:
		return nil, fmt.Errorf("no explicit standard desired schema for %s", kind)
	}
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": required, "properties": properties,
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}
	return schema, nil
}

func observedSchema(desired map[string]any) map[string]any {
	applied := cloneJSONMap(desired)
	delete(applied, "$schema")
	defs, _ := applied["$defs"].(map[string]any)
	delete(applied, "$defs")
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": []string{"id", "ready", "generation", "imported", "portability", "applied", "driftedFields"},
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "minLength": 1},
			"ready":       map[string]any{"type": "boolean"},
			"generation":  map[string]any{"type": "integer", "minimum": 1},
			"imported":    map[string]any{"type": "boolean"},
			"portability": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
			"applied":     applied,
			"driftedFields": map[string]any{
				"type": "array", "uniqueItems": true,
				"items": map[string]any{"type": "string", "pattern": "^(?:/(?:[^~/]|~0|~1)*)+$"},
			},
		},
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}
	return schema
}

func outputSchema(kind string, desired map[string]any) map[string]any {
	portableSpec := cloneJSONMap(desired)
	delete(portableSpec, "$schema")
	defs, _ := portableSpec["$defs"].(map[string]any)
	delete(portableSpec, "$defs")
	schema := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": false,
		"required": []string{"id", "kind", "name", "generation", "portability", "portableSpec"},
		"properties": map[string]any{
			"id":           map[string]any{"type": "string", "minLength": 1},
			"kind":         map[string]any{"type": "string", "const": kind},
			"name":         map[string]any{"type": "string", "minLength": 1},
			"generation":   map[string]any{"type": "integer", "minimum": 1},
			"portability":  map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
			"portableSpec": portableSpec,
		},
	}
	if len(defs) > 0 {
		schema["$defs"] = defs
	}
	return schema
}

func canonicalDesired(kind string) (map[string]any, error) {
	connections := func(name, resource string, permissions []any, projection string) map[string]any {
		return map[string]any{name: map[string]any{"resource": resource, "permissions": permissions, "projection": projection}}
	}
	switch kind {
	case "EdgeWorker":
		return map[string]any{
			"name": "edge", "source": map[string]any{"artifactUrl": "https://artifacts.example.test/edge.js", "artifactSha256": strings.Repeat("a", 64)},
			"compatibilityDate": "2026-07-16", "compatibilityFlags": []any{"nodejs_compat"}, "profiles": []any{"workers"},
			"connections": connections("ASSETS", "ObjectBucket/assets", []any{"read", "write"}, "binding"),
		}, nil
	case "ObjectBucket":
		return map[string]any{"name": "assets", "storageClass": "standard", "interfaces": []any{"s3_api"}}, nil
	case "KVStore":
		return map[string]any{"name": "cache", "consistency": "strong"}, nil
	case "SQLDatabase":
		return map[string]any{"name": "main", "engine": "sqlite", "migrationsPath": "migrations"}, nil
	case "Queue":
		return map[string]any{"name": "jobs", "delivery": map[string]any{"maxRetries": 5, "maxBatchSize": 10}}, nil
	case "VectorIndex":
		return map[string]any{"name": "embeddings", "dimensions": 1536, "metric": "cosine", "connections": connections("DATABASE", "SQLDatabase/main", []any{"read"}, "metadata")}, nil
	case "DurableWorkflow":
		return map[string]any{
			"name": "ingest", "source": map[string]any{"artifactRef": "artifact:workflow:ingest", "artifactSha256": strings.Repeat("b", 64)},
			"entrypoint": "IngestWorkflow", "retry": map[string]any{"maxAttempts": 3, "initialBackoffSeconds": 5},
			"connections": connections("JOBS", "Queue/jobs", []any{"consume"}, "binding"),
		}, nil
	case "ContainerService":
		return map[string]any{
			"name": "agent", "image": "ghcr.io/example/agent@sha256:" + strings.Repeat("c", 64), "ports": []any{8080}, "publicHttp": true,
			"environment": map[string]any{"LOG_LEVEL": "info"},
			"connections": connections("DATABASE", "SQLDatabase/main", []any{"read", "write"}, "environment"),
		}, nil
	case "StatefulActorNamespace":
		return map[string]any{
			"name": "rooms", "className": "RoomActor", "storageProfile": "durable_sqlite", "migrationTag": "v1",
			"connections": connections("DATABASE", "SQLDatabase/main", []any{"read", "write"}, "storage"),
		}, nil
	case "Schedule":
		return map[string]any{
			"name": "nightly", "cron": "0 0 * * *", "timezone": "UTC",
			"connections": connections("workflow", "DurableWorkflow/ingest", []any{"invoke"}, "schedule_trigger"),
		}, nil
	default:
		return nil, fmt.Errorf("no canonical standard desired fixture for %s", kind)
	}
}

func negativeDesired(kind string, canonical map[string]any) (map[string]any, error) {
	negative := cloneJSONMap(canonical)
	switch kind {
	case "EdgeWorker":
		negative["source"] = map[string]any{"artifactUrl": "https://artifacts.example.test/edge.js"}
	case "ObjectBucket":
		negative["storageClass"] = "cold"
	case "KVStore":
		negative["consistency"] = "linearizable"
	case "SQLDatabase":
		negative["engine"] = "not a token"
	case "Queue":
		negative["delivery"] = map[string]any{"maxRetries": -1}
	case "VectorIndex":
		negative["dimensions"] = 0
	case "DurableWorkflow":
		negative["retry"] = map[string]any{"maxAttempts": 0}
	case "ContainerService":
		negative["ports"] = []any{0}
	case "StatefulActorNamespace":
		negative["className"] = "not a class"
	case "Schedule":
		negative["cron"] = "0 0 * *"
	default:
		return nil, fmt.Errorf("no negative standard fixture for %s", kind)
	}
	return negative, nil
}

func connectionDefinitions() map[string]any {
	return map[string]any{
		"connections": map[string]any{
			"type": "object", "propertyNames": portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/connection"},
		},
		"connection": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"resource", "permissions", "projection"},
			"properties": map[string]any{
				"resource":    map[string]any{"type": "string", "pattern": "^\\S+$"},
				"permissions": map[string]any{"type": "array", "minItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"}},
				"projection":  map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
			},
		},
	}
}

func scheduleConnectionDefinitions() map[string]any {
	return map[string]any{
		"connections": map[string]any{
			"type": "object", "minProperties": 1, "maxProperties": 1, "propertyNames": portableMapKeys(),
			"additionalProperties": map[string]any{"$ref": "#/$defs/scheduleConnection"},
		},
		"scheduleConnection": map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{"resource", "permissions", "projection"},
			"properties": map[string]any{
				"resource": map[string]any{"type": "string", "pattern": "^\\S+$"},
				"permissions": map[string]any{
					"type": "array", "minItems": 1, "uniqueItems": true,
					"contains": map[string]any{"const": "invoke", "type": "string"},
					"items":    map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$"},
				},
				"projection": map[string]any{"const": "schedule_trigger", "type": "string"},
			},
		},
	}
}

func artifactDefinitions() map[string]any {
	return map[string]any{
		"artifactSource": map[string]any{"oneOf": []any{
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"artifactPath"}, "properties": map[string]any{"artifactPath": map[string]any{"type": "string", "minLength": 1}}},
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"artifactUrl", "artifactSha256"}, "properties": map[string]any{"artifactUrl": map[string]any{"type": "string", "format": "uri", "pattern": "^https://"}, "artifactSha256": map[string]any{"$ref": "#/$defs/sha256"}}},
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"artifactRef", "artifactSha256"}, "properties": map[string]any{"artifactRef": map[string]any{"type": "string", "minLength": 1}, "artifactSha256": map[string]any{"$ref": "#/$defs/sha256"}}},
		}},
		"sha256": map[string]any{"type": "string", "pattern": "^(sha256:)?[A-Fa-f0-9]{64}$"},
	}
}

func tokenArraySchema() map[string]any {
	return map[string]any{"type": "array", "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^\\S+$"}}
}

func tokenSchema(defaultValue string) map[string]any {
	return map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:-]{0,127}$", "default": defaultValue}
}

func typedStringMapSchema() map[string]any {
	return map[string]any{"type": "object", "propertyNames": portableMapKeys(), "additionalProperties": map[string]any{"type": "string"}}
}

func portableMapKeys() map[string]any {
	return map[string]any{
		"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._-]{0,63}$",
		"x-takoform-fieldPolicy": "portable-data-only-v1",
	}
}

func cloneJSONMap(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	var result map[string]any
	_ = json.Unmarshal(raw, &result)
	return result
}

func updateConformanceManifest(root string, entries []InventoryEntry) error {
	path := filepath.Join(root, "conformance", "form-package-v1", "manifest.json")
	var manifest struct {
		SchemaVersion int `json:"schemaVersion"`
		Positive      []struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"positive"`
		Negative []map[string]any `json:"negative"`
	}
	if err := readJSON(path, &manifest); err != nil {
		return err
	}
	kept := manifest.Positive[:0]
	for _, item := range manifest.Positive {
		if !strings.HasPrefix(item.Name, "standard-") {
			kept = append(kept, item)
		}
	}
	manifest.Positive = kept
	for _, entry := range entries {
		manifest.Positive = append(manifest.Positive, struct {
			Name string `json:"name"`
			Path string `json:"path"`
			Kind string `json:"kind"`
		}{Name: entry.ConformanceCase, Path: strings.TrimPrefix(entry.Path, "conformance/form-package-v1/"), Kind: entry.Kind})
	}
	return writeJSON(path, manifest)
}

func updatePortableHostContract(root string, entries []InventoryEntry) error {
	var bucket *InventoryEntry
	for index := range entries {
		if entries[index].Kind == "ObjectBucket" {
			bucket = &entries[index]
			break
		}
	}
	if bucket == nil {
		return fmt.Errorf("standard ObjectBucket missing")
	}
	contractPath := filepath.Join(root, "conformance", "portable-host-v1", "contract.json")
	var contract portableconformance.Contract
	if err := readJSON(contractPath, &contract); err != nil {
		return err
	}
	contract.RunnerInput.Identity = portableconformance.InstalledFormReference{
		FormRef: portableconformance.FormRef{
			APIVersion: bucket.FormRef.APIVersion, Kind: bucket.FormRef.Kind,
			DefinitionVersion: bucket.FormRef.DefinitionVersion, SchemaDigest: bucket.FormRef.SchemaDigest,
		},
		PackageDigest: bucket.PackageDigest,
	}
	if err := writeJSON(contractPath, contract); err != nil {
		return err
	}
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(raw)
	manifest := struct {
		Format   string `json:"format"`
		Contract string `json:"contract"`
		SHA256   string `json:"sha256"`
	}{Format: "takoform.portable-host-conformance-manifest@v1", Contract: "contract.json", SHA256: hex.EncodeToString(digest[:])}
	return writeJSON(filepath.Join(root, "conformance", "portable-host-v1", "manifest.json"), manifest)
}

func readJSON(path string, value any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
