package standardforms

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/formregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/indexedsql"
	"github.com/tako0614/terraform-provider-takoform/internal/provider"
)

const sqlDatabaseV2InventoryPath = "forms/sql-database-v2-package.json"

type SQLDatabaseV2Inventory struct {
	Format                      string              `json:"format"`
	Classification              string              `json:"classification"`
	Kind                        string              `json:"kind"`
	SupersedesDefinitionVersion string              `json:"supersedesDefinitionVersion"`
	DefinitionVersion           string              `json:"definitionVersion"`
	PackageVersion              string              `json:"packageVersion"`
	AdmissionStatus             string              `json:"admissionStatus"`
	PublicationReady            bool                `json:"publicationReady"`
	Path                        string              `json:"path"`
	ConformanceCase             string              `json:"conformanceCase"`
	DataInterface               string              `json:"dataInterface"`
	FormRef                     formpackage.FormRef `json:"formRef"`
	PackageDigest               string              `json:"packageDigest"`
}

type semanticNegativeCase struct {
	Name      string         `json:"name"`
	Desired   map[string]any `json:"desired"`
	WantError string         `json:"wantError"`
}

type semanticNegativeSet struct {
	Format string                 `json:"format"`
	Cases  []semanticNegativeCase `json:"cases"`
}

type indexedContractManifest struct {
	Format           string   `json:"format"`
	RequestSchema    string   `json:"requestSchema"`
	ResponseSchema   string   `json:"responseSchema"`
	SuccessStatus    int      `json:"successStatus"`
	ConflictStatus   int      `json:"conflictStatus"`
	RequestPositive  []string `json:"requestPositive"`
	RequestNegative  []string `json:"requestNegative"`
	ResponseSuccess  []string `json:"responseSuccess"`
	ResponseConflict []string `json:"responseConflict"`
	ResponseNegative []string `json:"responseNegative"`
}

func generateSQLDatabaseV2(root string) (InventoryEntry, error) {
	desired := canonicalSQLDatabaseV2Desired()
	if err := indexedsql.ValidateDesired(desired); err != nil {
		return InventoryEntry{}, fmt.Errorf("canonical SQLDatabase@2.0.0 desired: %w", err)
	}
	definition := formpackage.FormDefinition{
		APIVersion: formpackage.FormAPIVersion, Kind: "SQLDatabase",
		DefinitionVersion: indexedsql.DefinitionVersion,
		Title:             "Indexed SQL Database",
		Description:       "Provider-neutral bounded indexed data with declared tables, keys, and indexes; no caller-provided SQL or DDL.",
		Status:            "standard",
		DesiredSchema:     indexedsql.DesiredSchema(), ObservedSchema: observedSchema(), OutputSchema: indexedsql.OutputSchema(),
		ImmutableFields:       []string{"/name", "/schemaVersion", "/tables"},
		LifecycleCapabilities: []string{"create", "read", "update", "delete", "import", "observe", "refresh", "drift"},
		Interfaces:            []formpackage.InterfaceDescriptor{indexedsql.InterfaceDescriptor()},
		ConformanceFixtures: []formpackage.ConformanceFixture{{
			Name: "canonical-indexed-schema", DesiredPath: "fixtures/desired.json", ObservedPath: "fixtures/observed.json", OutputPath: "fixtures/output.json",
		}},
		NegativeFixtures: []formpackage.NegativeFixture{{
			Name: "reject-unsupported-schema-version", Stage: "desired", InputPath: "fixtures/negative.json", ExpectedFailure: "schema_validation_failed",
		}},
	}
	observed := map[string]any{
		"driftedFields": []any{}, "generation": 1, "id": "SQLDatabase/main",
		"imported": true, "portability": "portable", "ready": true,
	}
	output := map[string]any{
		"generation": 1, "id": "SQLDatabase/main", "kind": "SQLDatabase", "name": "main",
		"portability": "portable", "schemaVersion": 1, "tables": cloneJSONValue(desired["tables"]),
	}
	negative := cloneJSONMap(desired)
	negative["schemaVersion"] = 2
	semanticNegatives := sqlDatabaseV2SemanticNegatives(desired)

	packageRoot := filepath.Join(root, "conformance", "form-package-v1", "positive", "standard", "sql-database-v2")
	if err := os.RemoveAll(packageRoot); err != nil {
		return InventoryEntry{}, err
	}
	files := map[string]any{
		"definition.json": definition, "fixtures/desired.json": desired, "fixtures/negative.json": negative,
		"fixtures/observed.json": observed, "fixtures/output.json": output,
		"fixtures/semantic-negative.json": semanticNegatives,
		indexedsql.RequestSchemaPath:      indexedsql.RequestSchema(), indexedsql.ResponseSchemaPath: indexedsql.ResponseSchema(),
	}
	for relative, value := range files {
		if err := writeJSON(filepath.Join(packageRoot, filepath.FromSlash(relative)), value); err != nil {
			return InventoryEntry{}, err
		}
	}
	entry, err := finishSQLDatabaseV2Package(packageRoot, files)
	if err != nil {
		return InventoryEntry{}, err
	}
	if err := syncSQLDatabaseV2ReleaseSource(root, entry); err != nil {
		return InventoryEntry{}, err
	}
	inventory := SQLDatabaseV2Inventory{
		Format: "takoform.form-successor@v1", Classification: "structural-candidate", Kind: "SQLDatabase",
		SupersedesDefinitionVersion: "1.0.1", DefinitionVersion: indexedsql.DefinitionVersion,
		PackageVersion: indexedsql.PackageVersion, AdmissionStatus: "external-required", PublicationReady: false,
		Path: entry.Path, ConformanceCase: entry.ConformanceCase, DataInterface: indexedsql.InterfaceName + "@" + indexedsql.InterfaceVersion,
		FormRef: entry.FormRef, PackageDigest: entry.PackageDigest,
	}
	if err := writeJSON(filepath.Join(root, filepath.FromSlash(sqlDatabaseV2InventoryPath)), inventory); err != nil {
		return InventoryEntry{}, err
	}
	successors := map[string]map[string]formregistry.Ref{
		"SQLDatabase": {
			indexedsql.DefinitionVersion: {
				APIVersion: entry.FormRef.APIVersion, Kind: entry.FormRef.Kind,
				DefinitionVersion: entry.FormRef.DefinitionVersion, SchemaDigest: entry.FormRef.SchemaDigest,
				PackageDigest: entry.PackageDigest,
			},
		},
	}
	if err := writeJSON(filepath.Join(root, "internal", "formregistry", "successor-refs.json"), successors); err != nil {
		return InventoryEntry{}, err
	}
	if err := generateIndexedConformance(root); err != nil {
		return InventoryEntry{}, err
	}
	return entry, nil
}

func finishSQLDatabaseV2Package(packageRoot string, files map[string]any) (InventoryEntry, error) {
	definitionRaw, err := os.ReadFile(filepath.Join(packageRoot, "definition.json"))
	if err != nil {
		return InventoryEntry{}, err
	}
	schemaDigest, err := formpackage.DigestCanonicalJSON(definitionRaw)
	if err != nil {
		return InventoryEntry{}, err
	}
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
		} else if strings.HasSuffix(relative, ".schema.json") {
			mediaType = "application/schema+json"
		}
		packageFiles = append(packageFiles, formpackage.PackageFile{
			Path: relative, MediaType: mediaType, Size: int64(len(raw)), Digest: formpackage.DigestBytes(raw),
		})
	}
	ref := formpackage.FormRef{
		APIVersion: formpackage.FormAPIVersion, Kind: "SQLDatabase",
		DefinitionVersion: indexedsql.DefinitionVersion, SchemaDigest: schemaDigest,
	}
	index := formpackage.PackageIndex{
		APIVersion: formpackage.PackageAPIVersion, Kind: formpackage.PackageKind,
		PackageVersion: indexedsql.PackageVersion, FormRef: ref,
		DefinitionPath: "definition.json", Files: packageFiles,
	}
	if err := writeJSON(filepath.Join(packageRoot, formpackage.PackageIndexFilename), index); err != nil {
		return InventoryEntry{}, err
	}
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return InventoryEntry{}, fmt.Errorf("verify generated SQLDatabase@2.0.0: %w", err)
	}
	return InventoryEntry{
		Kind: "SQLDatabase", Path: "conformance/form-package-v1/positive/standard/sql-database-v2",
		AdmissionStatus: "external-required", ConformanceCase: "standard-sql-database-v2-package",
		FormRef: report.FormRef, PackageDigest: report.PackageDigest,
	}, nil
}

func syncSQLDatabaseV2ReleaseSource(root string, entry InventoryEntry) error {
	source := filepath.Join(root, filepath.FromSlash(entry.Path))
	destination := filepath.Join(root, "forms", "releases", releaseIDForKind(entry.Kind), indexedsql.PackageVersion)
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.CopyFS(destination, os.DirFS(source)); err != nil {
		return fmt.Errorf("sync SQLDatabase@2.0.0 candidate release source: %w", err)
	}
	return nil
}

func verifySQLDatabaseV2(root string) (InventoryEntry, error) {
	var inventory SQLDatabaseV2Inventory
	if err := readJSON(filepath.Join(root, filepath.FromSlash(sqlDatabaseV2InventoryPath)), &inventory); err != nil {
		return InventoryEntry{}, err
	}
	if inventory.Format != "takoform.form-successor@v1" || inventory.Classification != "structural-candidate" ||
		inventory.Kind != "SQLDatabase" || inventory.SupersedesDefinitionVersion != "1.0.1" ||
		inventory.DefinitionVersion != indexedsql.DefinitionVersion || inventory.PackageVersion != indexedsql.PackageVersion ||
		inventory.AdmissionStatus != "external-required" || inventory.PublicationReady ||
		inventory.DataInterface != indexedsql.InterfaceName+"@"+indexedsql.InterfaceVersion {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 inventory identity or release truth is invalid")
	}
	entry := InventoryEntry{
		Kind: inventory.Kind, Path: inventory.Path, AdmissionStatus: inventory.AdmissionStatus,
		ConformanceCase: inventory.ConformanceCase, FormRef: inventory.FormRef, PackageDigest: inventory.PackageDigest,
	}
	packageRoot := filepath.Join(root, filepath.FromSlash(entry.Path))
	report, err := formpackage.VerifyDirectory(packageRoot)
	if err != nil {
		return InventoryEntry{}, err
	}
	if report.FormRef != entry.FormRef || report.PackageDigest != entry.PackageDigest {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 inventory digest drift")
	}
	releaseRoot := filepath.Join(root, "forms", "releases", releaseIDForKind(entry.Kind), indexedsql.PackageVersion)
	if err := verifyReleaseSource(packageRoot, releaseRoot, entry); err != nil {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 release source: %w", err)
	}
	var definition formpackage.FormDefinition
	if err := readJSON(filepath.Join(packageRoot, "definition.json"), &definition); err != nil {
		return InventoryEntry{}, err
	}
	if !slices.Equal(definition.ImmutableFields, []string{"/name", "/schemaVersion", "/tables"}) {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 immutable table schema drift")
	}
	if len(definition.Interfaces) != 1 {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 data.indexed@1 descriptor drift")
	}
	wantDescriptor, err := json.Marshal(indexedsql.InterfaceDescriptor())
	if err != nil {
		return InventoryEntry{}, err
	}
	gotDescriptor, err := json.Marshal(definition.Interfaces[0])
	if err != nil {
		return InventoryEntry{}, err
	}
	if string(gotDescriptor) != string(wantDescriptor) {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 data.indexed@1 descriptor drift")
	}
	if err := verifyIndexedSchemaClosure(packageRoot, definition.Interfaces[0]); err != nil {
		return InventoryEntry{}, err
	}
	var desired map[string]any
	if err := readJSON(filepath.Join(packageRoot, "fixtures", "desired.json"), &desired); err != nil {
		return InventoryEntry{}, err
	}
	if err := indexedsql.ValidateDesired(desired); err != nil {
		return InventoryEntry{}, err
	}
	if err := provider.VerifyStandardFormStructure("SQLDatabase", desired); err != nil {
		return InventoryEntry{}, err
	}
	var semantic semanticNegativeSet
	if err := readJSON(filepath.Join(packageRoot, "fixtures", "semantic-negative.json"), &semantic); err != nil {
		return InventoryEntry{}, err
	}
	if semantic.Format != "takoform.sql-database.semantic-negative@v1" || len(semantic.Cases) < 6 {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 semantic-negative fixture set is incomplete")
	}
	for _, fixture := range semantic.Cases {
		err := indexedsql.ValidateDesired(fixture.Desired)
		if err == nil || !strings.Contains(err.Error(), fixture.WantError) {
			return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 semantic fixture %q error = %v, want %q", fixture.Name, err, fixture.WantError)
		}
	}
	ref, err := formregistry.ForKindVersion("SQLDatabase", indexedsql.DefinitionVersion)
	if err != nil || ref.SchemaDigest != entry.FormRef.SchemaDigest || ref.PackageDigest != entry.PackageDigest {
		return InventoryEntry{}, fmt.Errorf("SQLDatabase@2.0.0 embedded FormRef drift: %v", err)
	}
	if err := verifyIndexedConformance(root); err != nil {
		return InventoryEntry{}, err
	}
	return entry, nil
}

func verifyIndexedSchemaClosure(packageRoot string, descriptor formpackage.InterfaceDescriptor) error {
	schemas, ok := descriptor.Document["schemas"].(map[string]any)
	if !ok {
		return fmt.Errorf("data.indexed@1 descriptor schemas are missing")
	}
	for name, expected := range map[string]struct {
		path   string
		schema map[string]any
	}{
		"request":  {path: indexedsql.RequestSchemaPath, schema: indexedsql.RequestSchema()},
		"response": {path: indexedsql.ResponseSchemaPath, schema: indexedsql.ResponseSchema()},
	} {
		entry, ok := schemas[name].(map[string]any)
		if !ok {
			return fmt.Errorf("data.indexed@1 %s schema declaration is missing", name)
		}
		packagePath, _ := entry["packagePath"].(string)
		if packagePath != expected.path {
			return fmt.Errorf("data.indexed@1 %s packagePath %q is non-canonical or permits path traversal", name, packagePath)
		}
		inline, ok := entry["schema"].(map[string]any)
		if !ok {
			return fmt.Errorf("data.indexed@1 %s inline schema is missing", name)
		}
		inlineRaw, err := json.Marshal(inline)
		if err != nil {
			return err
		}
		inlineDigest, err := formpackage.DigestCanonicalJSON(inlineRaw)
		if err != nil {
			return err
		}
		if entry["schemaDigest"] != inlineDigest {
			return fmt.Errorf("data.indexed@1 %s inline schema digest drift", name)
		}
		fileRaw, err := os.ReadFile(filepath.Join(packageRoot, filepath.FromSlash(packagePath)))
		if err != nil {
			return fmt.Errorf("data.indexed@1 %s package schema: %w", name, err)
		}
		fileDigest, err := formpackage.DigestCanonicalJSON(fileRaw)
		if err != nil {
			return err
		}
		expectedRaw, _ := json.Marshal(expected.schema)
		expectedDigest, _ := formpackage.DigestCanonicalJSON(expectedRaw)
		if fileDigest != inlineDigest || fileDigest != expectedDigest {
			return fmt.Errorf("data.indexed@1 %s inline, package, and generated schema bytes drift", name)
		}
	}
	return nil
}

func canonicalSQLDatabaseV2Desired() map[string]any {
	return map[string]any{
		"name": "main", "schemaVersion": 1,
		"tables": []any{map[string]any{
			"name": "records",
			"columns": []any{
				map[string]any{"name": "id", "type": "string", "nullable": false},
				map[string]any{"name": "tenant_id", "type": "string", "nullable": false},
				map[string]any{"name": "created_at", "type": "integer", "nullable": false},
				map[string]any{"name": "score", "type": "number", "nullable": true},
			},
			"primaryKey": []any{"id"},
			"indexes": []any{
				map[string]any{"name": "by_tenant_created", "columns": []any{"tenant_id", "created_at"}, "unique": true},
			},
		}},
	}
}

func sqlDatabaseV2SemanticNegatives(canonical map[string]any) semanticNegativeSet {
	caseWith := func(name, want string, mutate func(map[string]any)) semanticNegativeCase {
		desired := cloneJSONMap(canonical)
		mutate(desired)
		return semanticNegativeCase{Name: name, Desired: desired, WantError: want}
	}
	firstTable := func(value map[string]any) map[string]any { return value["tables"].([]any)[0].(map[string]any) }
	return semanticNegativeSet{Format: "takoform.sql-database.semantic-negative@v1", Cases: []semanticNegativeCase{
		caseWith("duplicate-table", "duplicate table", func(value map[string]any) {
			table := cloneJSONMap(firstTable(value))
			value["tables"] = append(value["tables"].([]any), table)
		}),
		caseWith("unknown-primary-key", "unknown column", func(value map[string]any) { firstTable(value)["primaryKey"] = []any{"missing"} }),
		caseWith("nullable-primary-key", "must be non-null", func(value map[string]any) {
			firstTable(value)["columns"].([]any)[0].(map[string]any)["nullable"] = true
		}),
		caseWith("number-primary-key", "not indexable", func(value map[string]any) {
			firstTable(value)["columns"].([]any)[0].(map[string]any)["type"] = "number"
		}),
		caseWith("duplicate-index", "duplicate index", func(value map[string]any) {
			table := firstTable(value)
			indexes := table["indexes"].([]any)
			table["indexes"] = append(indexes, cloneJSONMap(indexes[0].(map[string]any)))
		}),
		caseWith("nullable-index", "must be non-null", func(value map[string]any) {
			firstTable(value)["columns"].([]any)[1].(map[string]any)["nullable"] = true
		}),
		caseWith("number-index", "not indexable", func(value map[string]any) {
			firstTable(value)["columns"].([]any)[1].(map[string]any)["type"] = "number"
		}),
	}}
}

func generateIndexedConformance(root string) error {
	if err := writeJSON(filepath.Join(root, "spec", "data-indexed", "request.schema.json"), indexedsql.RequestSchema()); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, "spec", "data-indexed", "response.schema.json"), indexedsql.ResponseSchema()); err != nil {
		return err
	}
	base := filepath.Join(root, "conformance", "data-indexed-v1")
	if err := os.RemoveAll(base); err != nil {
		return err
	}
	requestPositive := map[string]any{
		"get.json":        map[string]any{"operation": "get", "table": "records", "key": map[string]any{"id": "r1"}},
		"get-unique.json": map[string]any{"operation": "get_unique", "table": "records", "index": "by_tenant_created", "key": map[string]any{"tenant_id": "t1", "created_at": 1}},
		"page.json":       map[string]any{"operation": "page", "table": "records", "index": "by_tenant_created", "prefix": map[string]any{"tenant_id": "t1"}, "range": map[string]any{"column": "created_at", "gte": 1}, "limit": indexedsql.MaxPageSize},
		"put.json":        map[string]any{"operation": "put", "table": "records", "row": map[string]any{"id": "r1", "tenant_id": "t1", "created_at": 1}},
		"delete.json":     map[string]any{"operation": "delete", "table": "records", "key": map[string]any{"id": "r1"}, "expectedRevision": 1},
		"batch.json": map[string]any{"operation": "batch", "mutations": []any{
			map[string]any{"operation": "put", "table": "records", "row": map[string]any{"id": "r1", "tenant_id": "t1", "created_at": 1}},
			map[string]any{"operation": "delete", "table": "records", "key": map[string]any{"id": "r2"}},
		}},
	}
	tooManyMutations := make([]any, indexedsql.MaxBatchMutations+1)
	for index := range tooManyMutations {
		tooManyMutations[index] = map[string]any{"operation": "delete", "table": "records", "key": map[string]any{"id": index}}
	}
	requestNegative := map[string]any{
		"raw-sql.json":           map[string]any{"operation": "query", "sql": "select * from records"},
		"filter.json":            map[string]any{"operation": "page", "table": "records", "index": "by_tenant_created", "prefix": map[string]any{}, "filter": map[string]any{"active": true}},
		"order.json":             map[string]any{"operation": "page", "table": "records", "index": "by_tenant_created", "prefix": map[string]any{}, "order": "desc"},
		"offset.json":            map[string]any{"operation": "page", "table": "records", "index": "by_tenant_created", "prefix": map[string]any{}, "offset": 100},
		"page-limit.json":        map[string]any{"operation": "page", "table": "records", "index": "by_tenant_created", "prefix": map[string]any{}, "limit": indexedsql.MaxPageSize + 1},
		"batch-limit.json":       map[string]any{"operation": "batch", "mutations": tooManyMutations},
		"number-key.json":        map[string]any{"operation": "get", "table": "records", "key": map[string]any{"id": 1.5}},
		"revision-type.json":     map[string]any{"operation": "delete", "table": "records", "key": map[string]any{"id": "r1"}, "expectedRevision": "1"},
		"cursor-limit.json":      map[string]any{"operation": "page", "table": "records", "index": "by_tenant_created", "prefix": map[string]any{}, "cursor": strings.Repeat("c", indexedsql.MaxCursorBytes+1)},
		"integer-key-limit.json": map[string]any{"operation": "get", "table": "records", "key": map[string]any{"id": indexedsql.MaxSafeInteger + 1}},
		"integer-row-limit.json": map[string]any{"operation": "put", "table": "records", "row": map[string]any{"id": "r1", "created_at": indexedsql.MinSafeInteger - 1}},
	}
	row := map[string]any{"id": "r1", "tenant_id": "t1", "created_at": 1, "score": nil}
	item := map[string]any{"row": row, "revision": 1}
	responseSuccess := map[string]any{
		"get-missing.json": map[string]any{"operation": "get", "item": nil},
		"get.json":         map[string]any{"operation": "get", "item": item},
		"get-unique.json":  map[string]any{"operation": "get_unique", "item": item},
		"page.json":        map[string]any{"operation": "page", "items": []any{item}, "nextCursor": "opaque-cursor"},
		"put.json":         map[string]any{"operation": "put", "item": item},
		"delete.json":      map[string]any{"operation": "delete", "deleted": true},
		"batch.json": map[string]any{"operation": "batch", "results": []any{
			map[string]any{"operation": "put", "item": item}, map[string]any{"operation": "delete", "deleted": false},
		}},
	}
	responseConflict := map[string]any{
		"revision-conflict.json": map[string]any{"operation": "delete", "conflict": map[string]any{
			"reason": "revision_conflict", "table": "records", "key": map[string]any{"id": "r1"},
		}},
		"unique-conflict.json": map[string]any{"operation": "batch", "conflict": map[string]any{
			"reason": "unique_conflict", "table": "records", "index": "by_tenant_created",
		}},
	}
	tooManyItems := make([]any, indexedsql.MaxPageSize+1)
	for index := range tooManyItems {
		tooManyItems[index] = item
	}
	tooManyResults := make([]any, indexedsql.MaxBatchMutations+1)
	for index := range tooManyResults {
		tooManyResults[index] = map[string]any{"operation": "delete", "deleted": true}
	}
	responseNegative := map[string]any{
		"untagged.json":              map[string]any{"found": false},
		"get-missing-item.json":      map[string]any{"operation": "get"},
		"item-missing-revision.json": map[string]any{"operation": "get", "item": map[string]any{"row": row}},
		"page-missing-cursor.json":   map[string]any{"operation": "page", "items": []any{}},
		"page-limit.json":            map[string]any{"operation": "page", "items": tooManyItems, "nextCursor": nil},
		"cursor-limit.json":          map[string]any{"operation": "page", "items": []any{}, "nextCursor": strings.Repeat("c", indexedsql.MaxCursorBytes+1)},
		"revision-zero.json":         map[string]any{"operation": "put", "item": map[string]any{"row": row, "revision": 0}},
		"revision-limit.json":        map[string]any{"operation": "put", "item": map[string]any{"row": row, "revision": indexedsql.MaxRevision + 1}},
		"delete-extra.json":          map[string]any{"operation": "delete", "deleted": true, "revision": 1},
		"batch-empty.json":           map[string]any{"operation": "batch", "results": []any{}},
		"batch-limit.json":           map[string]any{"operation": "batch", "results": tooManyResults},
		"revision-conflict-operation.json": map[string]any{"operation": "get", "conflict": map[string]any{
			"reason": "revision_conflict", "table": "records", "key": map[string]any{"id": "r1"},
		}},
		"unique-conflict-operation.json": map[string]any{"operation": "delete", "conflict": map[string]any{
			"reason": "unique_conflict", "table": "records", "index": "by_tenant_created",
		}},
		"revision-conflict-number-key.json": map[string]any{"operation": "put", "conflict": map[string]any{
			"reason": "revision_conflict", "table": "records", "key": map[string]any{"id": 1.5},
		}},
	}
	manifest := indexedContractManifest{
		Format:        "takoform.data-indexed-conformance@v1",
		RequestSchema: "../../spec/data-indexed/request.schema.json", ResponseSchema: "../../spec/data-indexed/response.schema.json",
		SuccessStatus: 200, ConflictStatus: 409,
	}
	sets := []struct {
		directory string
		values    map[string]any
		paths     *[]string
	}{
		{"positive", requestPositive, &manifest.RequestPositive},
		{"negative", requestNegative, &manifest.RequestNegative},
		{"response/200", responseSuccess, &manifest.ResponseSuccess},
		{"response/409", responseConflict, &manifest.ResponseConflict},
		{"response/negative", responseNegative, &manifest.ResponseNegative},
	}
	for _, set := range sets {
		for name, value := range set.values {
			path := filepath.Join(set.directory, name)
			if err := writeJSON(filepath.Join(base, path), value); err != nil {
				return err
			}
			*set.paths = append(*set.paths, filepath.ToSlash(path))
		}
		sort.Strings(*set.paths)
	}
	return writeJSON(filepath.Join(base, "manifest.json"), manifest)
}

func verifyIndexedConformance(root string) error {
	base := filepath.Join(root, "conformance", "data-indexed-v1")
	var manifest indexedContractManifest
	if err := readJSON(filepath.Join(base, "manifest.json"), &manifest); err != nil {
		return err
	}
	if manifest.Format != "takoform.data-indexed-conformance@v1" ||
		manifest.RequestSchema != "../../spec/data-indexed/request.schema.json" ||
		manifest.ResponseSchema != "../../spec/data-indexed/response.schema.json" ||
		manifest.SuccessStatus != 200 || manifest.ConflictStatus != 409 ||
		len(manifest.RequestPositive) != 6 || len(manifest.RequestNegative) < 11 ||
		len(manifest.ResponseSuccess) < 7 || len(manifest.ResponseConflict) != 2 || len(manifest.ResponseNegative) < 14 {
		return fmt.Errorf("data.indexed@1 conformance manifest is incomplete")
	}
	requestSchema, err := compileIndexedSchema(root, "request", indexedsql.RequestSchema())
	if err != nil {
		return err
	}
	responseSchema, err := compileIndexedSchema(root, "response", indexedsql.ResponseSchema())
	if err != nil {
		return err
	}
	if err := verifyIndexedFixtures(base, requestSchema, manifest.RequestPositive, true, "request positive"); err != nil {
		return err
	}
	if err := verifyIndexedFixtures(base, requestSchema, manifest.RequestNegative, false, "request negative"); err != nil {
		return err
	}
	if err := verifyIndexedFixtures(base, responseSchema, manifest.ResponseSuccess, true, "HTTP 200 response"); err != nil {
		return err
	}
	if err := verifyIndexedFixtures(base, responseSchema, manifest.ResponseConflict, true, "HTTP 409 response"); err != nil {
		return err
	}
	return verifyIndexedFixtures(base, responseSchema, manifest.ResponseNegative, false, "response negative")
}

func compileIndexedSchema(root, name string, generated map[string]any) (*jsonschema.Schema, error) {
	var committed any
	if err := readJSON(filepath.Join(root, "spec", "data-indexed", name+".schema.json"), &committed); err != nil {
		return nil, err
	}
	generatedRaw, _ := json.Marshal(generated)
	committedRaw, _ := json.Marshal(committed)
	if string(generatedRaw) != string(committedRaw) {
		return nil, fmt.Errorf("data.indexed@1 %s schema drift", name)
	}
	urn := "urn:takoform:data-indexed:" + name
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(urn, committed); err != nil {
		return nil, err
	}
	return compiler.Compile(urn)
}

func verifyIndexedFixtures(base string, schema *jsonschema.Schema, paths []string, wantValid bool, label string) error {
	for _, path := range paths {
		var value any
		if err := readJSON(filepath.Join(base, filepath.FromSlash(path)), &value); err != nil {
			return err
		}
		err := schema.Validate(value)
		if wantValid && err != nil {
			return fmt.Errorf("data.indexed@1 %s %s: %w", label, path, err)
		}
		if !wantValid && err == nil {
			return fmt.Errorf("data.indexed@1 %s %s unexpectedly passed", label, path)
		}
	}
	return nil
}

func cloneJSONValue(value any) any {
	raw, _ := json.Marshal(value)
	var result any
	_ = json.Unmarshal(raw, &result)
	return result
}
