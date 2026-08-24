package provider

import (
	"fmt"
	"regexp"
	"sort"
	"sync"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
	"github.com/tako0614/terraform-provider-takoform/internal/edgeformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/retainededgeformcatalog"
)

// v3ResourceTypeLine is provider-owned authoring metadata. A Form Definition
// does not name Terraform, and changing this mapping cannot change the Form's
// canonical bytes or schema digest. The provider expands each declared line to
// every exact FormRef it supports on that line, so runtime dispatch remains
// exact even while an additive Form evolution keeps one Terraform type.
type v3ResourceTypeLine struct {
	GroupKind    currentformregistry.GroupKind
	ResourceType string
}

// v3ResourceTypeRegistry is the provider's exact FormRef -> Terraform resource
// type mapping. It is deliberately separate from currentformmodel.Form: an
// alternative client can consume the same Form declarations without carrying
// or agreeing with any Terraform name.
type v3ResourceTypeRegistry struct {
	byRef map[currentformregistry.ExactFormKey]string
}

var terraformResourceTypePattern = regexp.MustCompile(`^takoform_[a-z0-9_]+$`)

func newV3ResourceTypeRegistry(
	forms *currentformregistry.V3Registry,
	lines []v3ResourceTypeLine,
) (*v3ResourceTypeRegistry, error) {
	if forms == nil {
		return nil, fmt.Errorf("takoform provider: exact Form registry is nil")
	}
	registry := &v3ResourceTypeRegistry{byRef: map[currentformregistry.ExactFormKey]string{}}
	seenLines := map[currentformregistry.GroupKind]struct{}{}
	typeKinds := map[string]string{}
	for _, line := range lines {
		if line.GroupKind.APIVersion == "" || line.GroupKind.Kind == "" {
			return nil, fmt.Errorf("takoform provider: Terraform resource type mapping has an incomplete Form line")
		}
		if !terraformResourceTypePattern.MatchString(line.ResourceType) {
			return nil, fmt.Errorf("takoform provider: resource type %q for %s/%s is invalid", line.ResourceType, line.GroupKind.APIVersion, line.GroupKind.Kind)
		}
		if _, duplicate := seenLines[line.GroupKind]; duplicate {
			return nil, fmt.Errorf("takoform provider: duplicate Terraform mapping for %s/%s", line.GroupKind.APIVersion, line.GroupKind.Kind)
		}
		seenLines[line.GroupKind] = struct{}{}
		if priorKind, taken := typeKinds[line.ResourceType]; taken && priorKind != line.GroupKind.Kind {
			return nil, fmt.Errorf("takoform provider: resource type %q maps both %s and %s", line.ResourceType, priorKind, line.GroupKind.Kind)
		}
		typeKinds[line.ResourceType] = line.GroupKind.Kind

		refs := forms.SupportedRefsFor(line.GroupKind)
		if len(refs) == 0 {
			return nil, fmt.Errorf("takoform provider: resource type %q maps a Form line with no supported exact FormRef: %s/%s", line.ResourceType, line.GroupKind.APIVersion, line.GroupKind.Kind)
		}
		for _, ref := range refs {
			key := ref.ExactKey()
			if _, duplicate := registry.byRef[key]; duplicate {
				return nil, fmt.Errorf("takoform provider: duplicate Terraform mapping for exact FormRef %s", key)
			}
			registry.byRef[key] = line.ResourceType
		}
	}
	return registry, nil
}

func (r *v3ResourceTypeRegistry) Lookup(key currentformregistry.ExactFormKey) (string, bool) {
	if r == nil {
		return "", false
	}
	resourceType, ok := r.byRef[key]
	return resourceType, ok
}

// providerV3ResourceTypeLines is the official provider's own naming policy.
// Current and retained group identities are listed independently; sharing a
// type across the two exact lines is a provider compatibility choice, not a
// statement made by either Form Family.
func providerV3ResourceTypeLines() []v3ResourceTypeLine {
	retainedGroup := retainededgeformcatalog.Family.APIVersion()
	// Terraform names are provider-owned metadata. Current Forms stay
	// provider-neutral and are keyed by their complete family Group+Kind here.
	// In particular, no Kind-only fallback is possible when another family
	// happens to introduce the same Kind.
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
			"ContainerService":      "takoform_container_service",
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
			"Schedule": "takoform_schedule",
		},
		"vector.forms.takoform.com": {
			"VectorIndex": "takoform_vector_index",
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

var v3TerraformResourceTypes = sync.OnceValue(func() *v3ResourceTypeRegistry {
	registry, err := newV3ResourceTypeRegistry(currentformregistry.V3Current(), providerV3ResourceTypeLines())
	if err != nil {
		panic(err)
	}
	return registry
})

// compileV3FormResources is provider registration. Missing or duplicate
// Terraform mappings fail here only; Form validation and canonical rendering
// remain completely independent of the official provider.
func compileV3FormResources(
	forms []model.Form,
	formRegistry *currentformregistry.V3Registry,
	resourceTypes *v3ResourceTypeRegistry,
	codecs *v3CodecTable,
) ([]func() frameworkresource.Resource, error) {
	if formRegistry == nil || resourceTypes == nil || codecs == nil {
		return nil, fmt.Errorf("takoform provider: resource registration dependency is nil")
	}
	factories := make([]func() frameworkresource.Resource, 0, len(forms))
	registeredTypes := map[string]currentformregistry.GroupKind{}
	for _, form := range forms {
		line := currentformregistry.GroupKind{APIVersion: form.Family.APIVersion(), Kind: form.Kind}
		ref, err := formRegistry.DefaultCreate(line)
		if err != nil {
			return nil, fmt.Errorf("takoform provider: registering %s: %w", form.Kind, err)
		}
		resourceType, mapped := resourceTypes.Lookup(ref.ExactKey())
		if !mapped {
			return nil, fmt.Errorf("takoform provider: registering %s requires an exact Terraform mapping for %s", form.Kind, ref.ExactKey())
		}
		if prior, duplicate := registeredTypes[resourceType]; duplicate {
			return nil, fmt.Errorf("takoform provider: resource type %q is registered for both %s/%s and %s/%s", resourceType, prior.APIVersion, prior.Kind, line.APIVersion, line.Kind)
		}
		registeredTypes[resourceType] = line
		declared := form
		typeName := resourceType
		factories = append(factories, func() frameworkresource.Resource {
			return &v3FormResource{form: declared, resourceType: typeName, codecs: codecs}
		})
	}
	return factories, nil
}
