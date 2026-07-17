package migrationproof

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	oldProvider = "registry.terraform.io/takosjp/takosumi"
	newProvider = "registry.terraform.io/tako0614/takoform"
)

type ResourceMapping struct {
	Kind     string `json:"kind"`
	FromType string `json:"fromType"`
	ToType   string `json:"toType"`
}

type Mapping struct {
	Format       string            `json:"format"`
	FromProvider string            `json:"fromProvider"`
	ToProvider   string            `json:"toProvider"`
	ApprovedPath string            `json:"approvedPath"`
	Resources    []ResourceMapping `json:"resources"`
}

type Phase struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Evidence string `json:"evidence"`
}

type Evidence struct {
	Format              string   `json:"format"`
	MappingSHA256       string   `json:"mappingSha256"`
	LegacyStateSHA256   string   `json:"legacyStateSha256"`
	GoldenStateSHA256   string   `json:"goldenStateSha256"`
	LegacyResourceCount int      `json:"legacyResourceCount"`
	ResourceCount       int      `json:"resourceCount"`
	Phases              []Phase  `json:"phases"`
	ExternalBlockers    []string `json:"externalBlockers"`
}

type tfState struct {
	Version          int               `json:"version"`
	TerraformVersion string            `json:"terraform_version"`
	Serial           int               `json:"serial"`
	Lineage          string            `json:"lineage"`
	Outputs          map[string]any    `json:"outputs"`
	Resources        []tfStateResource `json:"resources"`
	CheckResults     any               `json:"check_results"`
}

type tfStateResource struct {
	Mode      string            `json:"mode"`
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Instances []tfStateInstance `json:"instances"`
}

type tfStateInstance struct {
	SchemaVersion int            `json:"schema_version"`
	Attributes    map[string]any `json:"attributes"`
}

type Report struct {
	MigratedResourceCount int      `json:"migratedResourceCount"`
	ResourceCount         int      `json:"resourceCount"`
	StructuralStatus      string   `json:"structuralStatus"`
	Phases                []Phase  `json:"phases"`
	ExternalBlockers      []string `json:"externalBlockers"`
}

func Verify(root string) (Report, error) {
	mappingRaw, err := os.ReadFile(filepath.Join(root, "mapping.json"))
	if err != nil {
		return Report{}, err
	}
	legacyRaw, err := os.ReadFile(filepath.Join(root, "legacy-state.json"))
	if err != nil {
		return Report{}, err
	}
	stateRaw, err := os.ReadFile(filepath.Join(root, "golden-state.json"))
	if err != nil {
		return Report{}, err
	}
	var mapping Mapping
	if err := decodeStrict(mappingRaw, &mapping); err != nil {
		return Report{}, fmt.Errorf("mapping: %w", err)
	}
	var legacy tfState
	if err := decodeStrict(legacyRaw, &legacy); err != nil {
		return Report{}, fmt.Errorf("legacy state: %w", err)
	}
	var state tfState
	if err := decodeStrict(stateRaw, &state); err != nil {
		return Report{}, fmt.Errorf("golden state: %w", err)
	}
	var evidence Evidence
	evidenceRaw, err := os.ReadFile(filepath.Join(root, "evidence.json"))
	if err != nil {
		return Report{}, err
	}
	if err := decodeStrict(evidenceRaw, &evidence); err != nil {
		return Report{}, fmt.Errorf("evidence: %w", err)
	}
	if evidence.MappingSHA256 != digest(mappingRaw) || evidence.LegacyStateSHA256 != digest(legacyRaw) ||
		evidence.GoldenStateSHA256 != digest(stateRaw) {
		return Report{}, errors.New("provider migration evidence digest drifted")
	}
	if err := validateMapping(mapping); err != nil {
		return Report{}, err
	}
	if len(legacy.Resources) != len(mapping.Resources) || evidence.LegacyResourceCount != len(mapping.Resources) {
		return Report{}, errors.New("legacy provider migration coverage is incomplete")
	}
	if len(state.Resources) != 10 || evidence.ResourceCount != len(state.Resources) {
		return Report{}, errors.New("provider migration resource count is incomplete")
	}
	byOld := map[string]ResourceMapping{}
	for _, item := range mapping.Resources {
		byOld[item.FromType] = item
	}
	legacyByKind := map[string]tfStateResource{}
	for index := range legacy.Resources {
		resource := &legacy.Resources[index]
		item, ok := byOld[resource.Type]
		if !ok {
			return Report{}, fmt.Errorf("state resource type %s has no approved mapping", resource.Type)
		}
		if resource.Mode != "managed" || resource.Provider != providerAddress(oldProvider) || len(resource.Instances) != 1 {
			return Report{}, fmt.Errorf("state resource %s.%s has an invalid old-provider identity", resource.Type, resource.Name)
		}
		if err := rejectSensitiveState(resource.Instances[0].Attributes); err != nil {
			return Report{}, fmt.Errorf("state resource %s.%s: %w", resource.Type, resource.Name, err)
		}
		legacyByKind[item.Kind] = *resource
	}
	byNew := map[string]ResourceMapping{}
	for _, item := range mapping.Resources {
		byNew[item.ToType] = item
	}
	newTypes := expectedNewTypes()
	seenNew := map[string]bool{}
	for index := range state.Resources {
		resource := &state.Resources[index]
		kind, ok := newTypes[resource.Type]
		if !ok || seenNew[resource.Type] || resource.Mode != "managed" ||
			resource.Provider != providerAddress(newProvider) || len(resource.Instances) != 1 {
			return Report{}, fmt.Errorf("new state resource %s.%s has an invalid provider/type identity", resource.Type, resource.Name)
		}
		seenNew[resource.Type] = true
		attributes := resource.Instances[0].Attributes
		if err := rejectPrivateState(attributes); err != nil {
			return Report{}, fmt.Errorf("new state resource %s.%s: %w", resource.Type, resource.Name, err)
		}
		if resourceVersion, ok := attributes["resource_version"].(string); !ok || !positiveDecimal(resourceVersion) {
			return Report{}, fmt.Errorf("new state resource %s.%s has no resource_version fence", resource.Type, resource.Name)
		}
		for _, key := range []string{"id", "name", "space"} {
			if !hasStateValue(attributes[key]) {
				return Report{}, fmt.Errorf("new state resource %s.%s has no required %s value", resource.Type, resource.Name, key)
			}
		}
		for _, key := range requiredGoldenAttributes()[resource.Type] {
			if !hasStateValue(attributes[key]) {
				return Report{}, fmt.Errorf("new state resource %s.%s has no required %s value", resource.Type, resource.Name, key)
			}
		}
		if item, migrates := byNew[resource.Type]; migrates {
			old := legacyByKind[item.Kind]
			oldAttributes := old.Instances[0].Attributes
			for _, key := range []string{"id", "name", "space"} {
				if attributes[key] != oldAttributes[key] {
					return Report{}, fmt.Errorf("migration identity %s drifted for %s", key, item.Kind)
				}
			}
		}
		_ = kind
	}
	if len(seenNew) != len(newTypes) {
		return Report{}, errors.New("new-provider golden state does not cover all ten typed resources")
	}
	if err := validatePhases(evidence); err != nil {
		return Report{}, err
	}
	return Report{
		MigratedResourceCount: len(mapping.Resources), ResourceCount: len(state.Resources),
		StructuralStatus: "complete", Phases: evidence.Phases, ExternalBlockers: evidence.ExternalBlockers,
	}, nil
}

func validateMapping(mapping Mapping) error {
	if mapping.Format != "takoform.provider-migration-map@v1" || mapping.FromProvider != oldProvider ||
		mapping.ToProvider != newProvider || mapping.ApprovedPath != "backup-remove-import-refresh-or-restore-backup" {
		return errors.New("provider migration map identity is invalid")
	}
	if len(mapping.Resources) != 6 {
		return errors.New("provider migration map must cover the six real legacy resources")
	}
	oldTypes, newTypes, kinds := map[string]bool{}, map[string]bool{}, map[string]bool{}
	expected := expectedLegacyMappings()
	for _, item := range mapping.Resources {
		if item.Kind == "" || !strings.HasPrefix(item.FromType, "takosumi_") || !strings.HasPrefix(item.ToType, "takoform_") ||
			oldTypes[item.FromType] || newTypes[item.ToType] || kinds[item.Kind] {
			return errors.New("provider migration mapping is not a bijection")
		}
		want, ok := expected[item.Kind]
		if !ok || item.FromType != want.FromType || item.ToType != want.ToType {
			return fmt.Errorf("provider migration mapping for %s is not an exact legacy resource mapping", item.Kind)
		}
		oldTypes[item.FromType], newTypes[item.ToType], kinds[item.Kind] = true, true, true
	}
	return nil
}

func validatePhases(evidence Evidence) error {
	if evidence.Format != "takoform.provider-migration-evidence@v1" {
		return errors.New("migration evidence identity is invalid")
	}
	want := []struct{ name, status string }{
		{"state-backup", "complete"}, {"old-refresh-no-op", "external-required"},
		{"approved-remove-import", "complete"}, {"new-refresh-no-op", "external-required"},
		{"old-artifact-lock-rollback", "external-required"},
	}
	if len(evidence.Phases) != len(want) || len(evidence.ExternalBlockers) != 3 {
		return errors.New("migration evidence phases are incomplete")
	}
	for index, expected := range want {
		if evidence.Phases[index].Name != expected.name || evidence.Phases[index].Status != expected.status || evidence.Phases[index].Evidence == "" {
			return errors.New("migration evidence phase drifted")
		}
	}
	return nil
}

func rejectPrivateState(attributes map[string]any) error {
	return rejectStateKeys(attributes, []string{
		"credential", "secret", "token", "password", "price", "quote", "billing",
		"backend", "selected_implementation", "target", "locked",
	})
}

func rejectSensitiveState(attributes map[string]any) error {
	return rejectStateKeys(attributes, []string{"credential", "secret", "token", "password", "price", "quote", "billing"})
}

func rejectStateKeys(value any, forbidden []string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			for _, term := range forbidden {
				if strings.Contains(normalized, term) {
					return fmt.Errorf("forbidden state key %q", key)
				}
			}
			if err := rejectStateKeys(nested, forbidden); err != nil {
				return err
			}
		}
	case []any:
		for _, nested := range typed {
			if err := rejectStateKeys(nested, forbidden); err != nil {
				return err
			}
		}
	}
	return nil
}

func expectedNewTypes() map[string]string {
	return map[string]string{
		"takoform_edge_worker": "EdgeWorker", "takoform_object_bucket": "ObjectBucket",
		"takoform_kv_store": "KVStore", "takoform_queue": "Queue",
		"takoform_sql_database": "SQLDatabase", "takoform_container_service": "ContainerService",
		"takoform_vector_index": "VectorIndex", "takoform_durable_workflow": "DurableWorkflow",
		"takoform_stateful_actor_namespace": "StatefulActorNamespace", "takoform_schedule": "Schedule",
	}
}

func expectedLegacyMappings() map[string]ResourceMapping {
	return map[string]ResourceMapping{
		"EdgeWorker":       {Kind: "EdgeWorker", FromType: "takosumi_edge_worker", ToType: "takoform_edge_worker"},
		"ObjectBucket":     {Kind: "ObjectBucket", FromType: "takosumi_object_bucket", ToType: "takoform_object_bucket"},
		"KVStore":          {Kind: "KVStore", FromType: "takosumi_kv_store", ToType: "takoform_kv_store"},
		"Queue":            {Kind: "Queue", FromType: "takosumi_queue", ToType: "takoform_queue"},
		"SQLDatabase":      {Kind: "SQLDatabase", FromType: "takosumi_sql_database", ToType: "takoform_sql_database"},
		"ContainerService": {Kind: "ContainerService", FromType: "takosumi_container_service", ToType: "takoform_container_service"},
	}
}

func requiredGoldenAttributes() map[string][]string {
	return map[string][]string{
		"takoform_edge_worker":              {"artifact_url", "artifact_sha256"},
		"takoform_object_bucket":            {"storage_class"},
		"takoform_kv_store":                 {"consistency"},
		"takoform_queue":                    {"max_retries", "max_batch_size"},
		"takoform_sql_database":             {"engine"},
		"takoform_container_service":        {"image"},
		"takoform_vector_index":             {"dimensions", "metric"},
		"takoform_durable_workflow":         {"artifact_url", "artifact_sha256", "entrypoint"},
		"takoform_stateful_actor_namespace": {"class_name", "storage_profile"},
		"takoform_schedule":                 {"cron", "timezone", "connections"},
	}
}

func hasStateValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	default:
		return value != nil
	}
}

func positiveDecimal(value string) bool {
	if value == "" || value[0] == '0' {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func providerAddress(address string) string { return `provider["` + address + `"]` }
func digest(raw []byte) string              { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}
