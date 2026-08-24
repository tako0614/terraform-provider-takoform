package standardforms

import (
	"fmt"
	"strings"

	"github.com/tako0614/terraform-provider-takoform/internal/containerformcatalog"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/functionformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/queueformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/scheduleformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/tableformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/topicformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/vectorformcatalog"
)

// currentFamilyInventory is the provider-neutral current Form roster. Its
// order is the authoring/dependency order used by cmd/current-form-source;
// the publication index has its own required lexical ordering.
type currentFamilyInventory struct {
	Group string
	Forms []model.Form
}

func currentFamilies() []currentFamilyInventory {
	return []currentFamilyInventory{
		{Group: edgeformcatalog.Family.APIVersion(), Forms: edgeformcatalog.Forms},
		{Group: functionformcatalog.Family.APIVersion(), Forms: functionformcatalog.Forms},
		{Group: containerformcatalog.Family.APIVersion(), Forms: containerformcatalog.Forms},
		{Group: tableformcatalog.Family.APIVersion(), Forms: tableformcatalog.Forms},
		{Group: queueformcatalog.Family.APIVersion(), Forms: queueformcatalog.Forms},
		{Group: topicformcatalog.Family.APIVersion(), Forms: topicformcatalog.Forms},
		{Group: scheduleformcatalog.Family.APIVersion(), Forms: scheduleformcatalog.Forms},
		{Group: vectorformcatalog.Family.APIVersion(), Forms: vectorformcatalog.Forms},
	}
}

func currentFormCount() int {
	total := 0
	for _, family := range currentFamilies() {
		total += len(family.Forms)
	}
	return total
}

// providerReferenceTerraformTypes is metadata for generated official-provider
// reference docs and examples only. It is deliberately outside Form and is
// keyed by the complete family Group+Kind so two families can never collide
// through a Kind-only fallback. It is not used to render, validate, identify,
// or digest a Form Definition. The provider owns the production exact-FormRef
// mapping and tests it independently.
var providerReferenceTerraformTypes = map[string]map[string]string{
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
	functionformcatalog.Family.APIVersion(): {
		"Function":           "takoform_function",
		"FunctionVersion":    "takoform_function_version",
		"FunctionDeployment": "takoform_function_deployment",
		"FunctionEndpoint":   "takoform_function_endpoint",
	},
	containerformcatalog.Family.APIVersion(): {
		"ContainerService":      "takoform_serverless_container_service",
		"ContainerRevision":     "takoform_container_revision",
		"ContainerTraffic":      "takoform_container_traffic",
		"ContainerEndpoint":     "takoform_container_endpoint",
		"ContainerCustomDomain": "takoform_container_custom_domain",
	},
	tableformcatalog.Family.APIVersion(): {
		"Table": "takoform_table",
	},
	queueformcatalog.Family.APIVersion(): {
		"PullQueue": "takoform_pull_queue",
	},
	topicformcatalog.Family.APIVersion(): {
		"Topic":             "takoform_topic",
		"TopicSubscription": "takoform_topic_subscription",
	},
	scheduleformcatalog.Family.APIVersion(): {
		"Schedule": "takoform_message_schedule",
	},
	vectorformcatalog.Family.APIVersion(): {
		"VectorIndex": "takoform_dense_vector_index",
	},
}

func providerReferenceTerraformType(form model.Form) (string, error) {
	byKind, ok := providerReferenceTerraformTypes[form.Family.APIVersion()]
	if !ok {
		return "", fmt.Errorf("official-provider reference surface has no Terraform mapping for %s/%s", form.Family.APIVersion(), form.Kind)
	}
	resourceType, ok := byKind[form.Kind]
	if !ok {
		return "", fmt.Errorf("official-provider reference surface has no Terraform mapping for %s/%s", form.Family.APIVersion(), form.Kind)
	}
	return resourceType, nil
}

func mustProviderReferenceTerraformType(form model.Form) string {
	resourceType, err := providerReferenceTerraformType(form)
	if err != nil {
		panic(err)
	}
	return resourceType
}

func providerDocBasename(resourceType string) string {
	return strings.TrimPrefix(resourceType, "takoform_") + ".md"
}
