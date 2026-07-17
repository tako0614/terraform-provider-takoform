package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

// VerifyStandardFormStructure inspects the actual provider resource schema
// used by this build. This is structural coverage only: it does not run the
// Terraform protocol lifecycle, contact a host, or emit admission evidence.
func VerifyStandardFormStructure(kind string, desired map[string]any) error {
	name, ok := desired["name"].(string)
	if !ok || !validPortableName(name) || validPortableName("") {
		return fmt.Errorf("provider portable-name validation does not cover canonical positive/negative fixtures for %s", kind)
	}
	constructor, ok := standardResourceConstructors()[kind]
	if !ok {
		return fmt.Errorf("provider has no standard resource for %q", kind)
	}
	implementation := constructor()
	var response resource.SchemaResponse
	implementation.Schema(context.Background(), resource.SchemaRequest{}, &response)
	if response.Diagnostics.HasError() {
		return fmt.Errorf("provider schema for %s: %s", kind, response.Diagnostics.Errors()[0].Detail())
	}
	if _, ok := implementation.(resource.ResourceWithImportState); !ok {
		return fmt.Errorf("provider resource %s lacks import", kind)
	}
	for field := range desired {
		for _, providerField := range providerFieldsForDesired(kind, field) {
			if _, ok := response.Schema.Attributes[providerField]; !ok {
				return fmt.Errorf("provider schema for %s lacks %s projected from %s", kind, providerField, field)
			}
		}
	}
	if err := requireReplace(response.Schema.Attributes["name"], "name"); err != nil {
		return fmt.Errorf("provider schema for %s: %w", kind, err)
	}
	if kind == client.KindVectorIndex {
		if err := requireReplace(response.Schema.Attributes["dimensions"], "dimensions"); err != nil {
			return fmt.Errorf("provider schema for %s: %w", kind, err)
		}
	}
	if kind == client.KindSQLDatabase {
		if err := requireReplace(response.Schema.Attributes["engine"], "engine"); err != nil {
			return fmt.Errorf("provider schema for %s: %w", kind, err)
		}
	}
	return nil
}

func standardResourceConstructors() map[string]func() resource.Resource {
	return map[string]func() resource.Resource{
		client.KindEdgeWorker:             NewEdgeWorkerResource,
		client.KindObjectBucket:           NewObjectBucketResource,
		client.KindKVStore:                NewKVStoreResource,
		client.KindSQLDatabase:            NewSQLDatabaseResource,
		client.KindQueue:                  NewQueueResource,
		client.KindVectorIndex:            NewVectorIndexResource,
		client.KindDurableWorkflow:        NewDurableWorkflowResource,
		client.KindContainerService:       NewContainerServiceResource,
		client.KindStatefulActorNamespace: NewStatefulActorNamespaceResource,
		client.KindSchedule:               NewScheduleResource,
	}
}

func providerFieldsForDesired(kind, field string) []string {
	if field == "source" {
		return []string{"artifact_path", "artifact_url", "artifact_ref", "artifact_sha256"}
	}
	mapping := map[string]string{
		"compatibilityDate": "compatibility_date", "compatibilityFlags": "compatibility_flags",
		"storageClass": "storage_class", "migrationsPath": "migrations_path",
		"publicHttp": "public_http", "dimensions": "dimensions", "entrypoint": "entrypoint",
		"className": "class_name", "storageProfile": "storage_profile", "migrationTag": "migration_tag",
		"connections": "connections", "interfaces": "interfaces", "consistency": "consistency",
		"image": "image", "ports": "ports", "environment": "environment", "metric": "metric",
		"name": "name", "profiles": "profiles", "cron": "cron", "timezone": "timezone", "engine": "engine",
	}
	if field == "delivery" {
		return []string{"max_retries", "max_batch_size"}
	}
	if field == "retry" {
		return []string{"max_attempts", "initial_backoff_seconds"}
	}
	if mapped, ok := mapping[field]; ok {
		return []string{mapped}
	}
	return []string{camelToSnake(field)}
}

func requireReplace(attribute schema.Attribute, field string) error {
	switch typed := attribute.(type) {
	case schema.StringAttribute:
		for _, modifier := range typed.PlanModifiers {
			if strings.Contains(strings.ToLower(fmt.Sprintf("%T", modifier)), "requiresreplace") {
				return nil
			}
		}
	case schema.Int64Attribute:
		for _, modifier := range typed.PlanModifiers {
			if strings.Contains(strings.ToLower(fmt.Sprintf("%T", modifier)), "requiresreplace") {
				return nil
			}
		}
	}
	return fmt.Errorf("%s lacks RequiresReplace", field)
}

func camelToSnake(value string) string {
	var result strings.Builder
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(character)
	}
	return strings.ToLower(result.String())
}
