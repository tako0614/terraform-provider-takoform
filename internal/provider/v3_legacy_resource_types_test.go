package provider

// This test-only file is the comparison-only Provider 3 resource roster. Production
// registration consumes the exact mappings in artifacts/v3/projection.json;
// W08 removes this roster after the W02-W07 parity review.

import (
	"sort"
	"sync"

	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

// providerV3ResourceTypeLines is the legacy provider naming roster used only
// by comparison tests and explicitly constructed compatibility helpers.
func providerV3ResourceTypeLines() []v3ResourceTypeLine {
	retainedGroup := retainededgeformcatalog.Family.APIVersion()
	current := map[string]map[string]string{
		edgeformcatalog.Family.APIVersion(): {
			"ActorNamespace":             "takoform_actor_namespace",
			"AtLeastOnceQueue":           "takoform_at_least_once_queue",
			"DurableWorkflow":            "takoform_durable_workflow",
			"EdgeKVNamespace":            "takoform_edge_kv_namespace",
			"ModuleWorker":               "takoform_module_worker",
			"QueueConsumer":              "takoform_queue_consumer",
			"SQLiteDatabase":             "takoform_sqlite_database",
			"SQLiteMigrationApplication": "takoform_sqlite_migration_application",
			"SQLiteMigrationSet":         "takoform_sqlite_migration_set",
			"StaticAssetBundle":          "takoform_static_asset_bundle",
			"WorkerBundle":               "takoform_worker_bundle",
			"WorkerCronTrigger":          "takoform_worker_cron_trigger",
			"WorkerCustomDomain":         "takoform_worker_custom_domain",
			"WorkerDeployment":           "takoform_worker_deployment",
			"WorkerEndpoint":             "takoform_worker_endpoint",
			"WorkerVersion":              "takoform_worker_version",
		},
		"function.forms.takoform.com": {
			"Function":           "takoform_function",
			"FunctionVersion":    "takoform_function_version",
			"FunctionDeployment": "takoform_function_deployment",
			"FunctionEndpoint":   "takoform_function_endpoint",
		},
		"container.forms.takoform.com": {
			"ContainerService":      "takoform_serverless_container_service",
			"ContainerRevision":     "takoform_container_revision",
			"ContainerTraffic":      "takoform_container_traffic",
			"ContainerEndpoint":     "takoform_container_endpoint",
			"ContainerCustomDomain": "takoform_container_custom_domain",
		},
		"table.forms.takoform.com": {
			"Table": "takoform_table",
		},
		"queue.forms.takoform.com": {
			"PullQueue": "takoform_pull_queue",
		},
		"topic.forms.takoform.com": {
			"Topic":             "takoform_topic",
			"TopicSubscription": "takoform_topic_subscription",
		},
		"schedule.forms.takoform.com": {
			"Schedule": "takoform_message_schedule",
		},
		"vector.forms.takoform.com": {
			"VectorIndex": "takoform_dense_vector_index",
		},
	}
	retained := map[string]string{
		"AtLeastOnceQueue":           "takoform_at_least_once_queue",
		"EdgeKVNamespace":            "takoform_edge_kv_namespace",
		"ModuleWorker":               "takoform_module_worker",
		"QueueConsumer":              "takoform_queue_consumer",
		"SQLiteDatabase":             "takoform_sqlite_database",
		"SQLiteMigrationApplication": "takoform_sqlite_migration_application",
		"SQLiteMigrationSet":         "takoform_sqlite_migration_set",
		"StaticAssetBundle":          "takoform_static_asset_bundle",
		"WorkerBundle":               "takoform_worker_bundle",
		"WorkerCronTrigger":          "takoform_worker_cron_trigger",
		"WorkerCustomDomain":         "takoform_worker_custom_domain",
		"WorkerDeployment":           "takoform_worker_deployment",
		"WorkerEndpoint":             "takoform_worker_endpoint",
		"WorkerVersion":              "takoform_worker_version",
	}
	lines := make([]v3ResourceTypeLine, 0, len(current)+len(retained))
	appendGroup := func(group string, mapping map[string]string) {
		kinds := make([]string, 0, len(mapping))
		for kind := range mapping {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		for _, kind := range kinds {
			lines = append(lines, v3ResourceTypeLine{
				GroupKind:    currentformregistry.GroupKind{APIVersion: group, Kind: kind},
				ResourceType: mapping[kind],
			})
		}
	}
	groups := make([]string, 0, len(current))
	for group := range current {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		appendGroup(group, current[group])
	}
	appendGroup(retainedGroup, retained)
	return lines
}

var legacyV3TerraformResourceTypes = sync.OnceValue(func() *v3ResourceTypeRegistry {
	registry, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), providerV3ResourceTypeLines())
	if err != nil {
		panic(err)
	}
	return registry
})
