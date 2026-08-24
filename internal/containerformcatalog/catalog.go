// Package containerformcatalog declares the Serverless Container Form Family.
// It is a provider-neutral, data-only source catalog: no provider resource
// names, runtime implementation, image registry behavior, or standard
// protocol semantics live here.
package containerformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the versionless Container family group. Each Form carries its own
// SemVer and exact digest; there is no family version/latest fallback.
var Family = model.Family{Group: "container.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	firstHostAPI      = "forms.takoform.com/v1"
	currentHostAPI    = "forms.takoform.com/v1"

	// OCI references are accepted only when their immutable digest is present.
	imagePattern = `^[A-Za-z0-9][A-Za-z0-9._:/-]*@sha256:[0-9a-f]{64}$`
	// Command and argument arrays preserve order and pass opaque non-control
	// tokens to the container process; shell parsing is not a Form concern.
	commandTokenPattern = `^[^\x00\r\n]{1,256}$`
	commandTokenLength  = 256
)

func ref(kind, name string) map[string]any {
	return map[string]any{"apiVersion": Family.APIVersion(), "kind": kind, "name": name}
}

func exactTarget(kind string) model.TargetContract { return model.TargetContract{ExactForm: true} }

func resourceTarget(kind string) *model.ResourceTarget {
	return &model.ResourceTarget{Group: Family.APIVersion(), Kind: kind, Contract: exactTarget(kind)}
}

func externalServiceSlot(name, protocol string, required bool) map[string]any {
	return map[string]any{
		"name": name,
		"service": map[string]any{
			"apiVersion": model.StandardServiceAPIVersion,
			"protocol":   protocol,
		},
		"required": required,
	}
}

func commandField(hcl, wire, doc string, example []any) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindStringList,
		AbsenceIsSemantic: true, ItemPattern: commandTokenPattern, MaxLength: commandTokenLength, MaxItems: 64,
		Doc: doc, Example: example,
	}
}

func varsField() model.Field {
	return model.Field{
		HCL: "vars", Wire: "vars", Kind: model.KindJSONMap,
		Default: map[string]any{}, ProjectsEnvironmentNames: true,
		Doc: "Non-secret configuration values projected into the process environment. " +
			"Omitting it projects no variable; sensitive material never enters portable state.",
		Example: map[string]any{"LOG_LEVEL": "info"},
	}
}

func sensitiveVarsField() model.Field {
	return model.Field{
		HCL: "required_sensitive_vars", Wire: "requiredSensitiveVars", Kind: model.KindStringSet,
		ItemPattern: model.PatternSensitiveVarName, MaxItems: 64,
		Default: []any{}, ProjectsEnvironmentNames: true,
		Doc: "Names of sensitive values the host must supply through its sealed path. " +
			"Only names are portable state; omitting it requires no sensitive value.",
		Example: []any{"API_SIGNING_TOKEN"}, CounterExample: []any{"not-a-sensitive-name"},
	}
}

func externalServicesField() model.Field {
	return model.Field{
		HCL: "external_services", Wire: "externalServices", Kind: model.KindExternalServiceList,
		MaxItems: 16, Default: []any{}, ProjectsEnvironmentNames: true,
		Doc: "Sealed slots naming only a standard protocol and a projected NAME. The host resolves " +
			"endpoint and credentials out of band; neither is portable state. Omitting it declares no " +
			"external service.",
		Example: []any{externalServiceSlot("PRIMARY_DB", "org.postgresql.wire", true)},
	}
}

// Forms is the complete Serverless Container Family MVP set, in stable order.
// Provider resource mappings are intentionally absent: they are not Form
// identity and belong to the provider-owned exact registry.
var Forms = []model.Form{
	{
		Family: Family, Kind: "ContainerService", Slug: "container-service", Role: model.RoleIdentity,
		DefinitionVersion: definitionVersion, RequiresHostAPI: firstHostAPI,
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: ContainerRuntimeInterfaceName, Version: interfaceVersion}},
		Title:              "Container Service", Description: "Logical identity of one request-driven serverless " +
			"container service. Image, process configuration, resources, scaling, and traffic are " +
			"represented by immutable revisions and the active traffic Form around this identity.",
	},
	{
		Family: Family, Kind: "ContainerRevision", Slug: "container-revision", Role: model.RoleRevision,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Container Revision", Description: "Immutable serving snapshot of one Container Service: a " +
			"digest-pinned OCI image, process arguments, environment declarations, sealed slots, and " +
			"resource and scaling bounds.",
		Fields: []model.Field{
			{HCL: "service", Wire: "service", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("ContainerService"), Required: true, Immutable: true,
				Doc: "Container Service identity this immutable revision belongs to.", Example: ref("ContainerService", "container-service")},
			{HCL: "image", Wire: "image", Kind: model.KindString,
				Pattern: imagePattern, MaxLength: 512, Required: true, Immutable: true,
				Doc: "OCI image reference pinned by a sha256 digest. A mutable tag is not portable state; " +
					"the host resolves and retains the digest-pinned image before serving.",
				Example:        "registry.example/app@sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43",
				AltExample:     "registry.example/app@sha256:8624fd492ece196a5048414afd598275b811beafc00aab602bfb59978f880765",
				CounterExample: "registry.example/app:latest"},
			commandField("command", "command", "Optional process entrypoint override. Without it the image's declared entrypoint applies; argument order is preserved.", []any{"/app/server"}),
			commandField("args", "args", "Optional process argument override. Without it the image's default arguments apply; argument order is preserved.", []any{"--port", "8080"}),
			varsField(), sensitiveVarsField(), externalServicesField(),
			{HCL: "memory_mib", Wire: "memoryMiB", Kind: model.KindInteger,
				Min: model.I64(128), Max: model.I64(32768), Required: true, Immutable: true,
				Doc: "Usable memory bound for one serving instance, in MiB.", Example: 512},
			{HCL: "cpu", Wire: "cpu", Kind: model.KindInteger,
				Min: model.I64(1), Max: model.I64(16000), Required: true, Immutable: true,
				Doc: "Compute allocation for one serving instance, in millicpu units.", Example: 1000},
			{HCL: "concurrency_target", Wire: "concurrencyTarget", Kind: model.KindInteger,
				Min: model.I64(1), Max: model.I64(1000), Required: true, Immutable: true,
				Doc: "Maximum concurrent HTTP requests delivered to one ready instance.", Example: 80},
			{HCL: "min_instances", Wire: "minInstances", Kind: model.KindInteger,
				Min: model.I64(0), Max: model.I64(1000), Required: true, Immutable: true,
				Doc: "Lower bound on serving instances. Zero permits scale-to-zero.", Example: 0},
			{HCL: "max_instances", Wire: "maxInstances", Kind: model.KindInteger,
				Min: model.I64(1), Max: model.I64(1000), Required: true, Immutable: true,
				Doc: "Upper bound on serving instances.", Example: 20},
			{HCL: "timeout_seconds", Wire: "timeoutSeconds", Kind: model.KindInteger,
				Min: model.I64(1), Max: model.I64(3600), Required: true, Immutable: true,
				Doc: "Maximum wall-clock time for one request before the host fails it, in seconds.", Example: 60},
		},
		StructuralConstraints: []model.Constraint{{
			Kind: model.ConstraintOrderedPair, References: []string{"/minInstances", "/maxInstances"},
		}},
	},
	{
		Family: Family, Kind: "ContainerTraffic", Slug: "container-traffic", Role: model.RoleDeployment,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Container Traffic", Description: "The one active basis-point traffic selection for a Container " +
			"Service. One to eight immutable revisions carry positive weights totaling exactly 10000.",
		Fields: []model.Field{
			{HCL: "service", Wire: "service", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("ContainerService"), Required: true, Immutable: true,
				Exclusive: &model.ExclusiveHold{},
				Doc:       "Container Service governed by this traffic resource. There is at most one traffic resource per identity.",
				Example:   ref("ContainerService", "container-service")},
			{HCL: "revisions", Wire: "revisions", Kind: model.KindObjectList,
				Required: true, MinItems: 1, MaxItems: 8, Sum: &model.SummedMember{Member: "weight", Total: 10000},
				Doc: "One to eight selected Container Revisions and their positive basis-point weights. " +
					"Weights are host-validated and must sum to exactly 10000.",
				Fields: []model.Field{
					{HCL: "container_revision", Wire: "containerRevision", Kind: model.KindResourceRef,
						ResourceTarget: resourceTarget("ContainerRevision"), Required: true,
						Doc: "Immutable revision selected by this traffic entry."},
					{HCL: "weight", Wire: "weight", Kind: model.KindInteger,
						Min: model.I64(1), Max: model.I64(10000), Required: true,
						Doc: "Positive traffic weight in basis points.", Example: 10000},
				},
				Example: []any{map[string]any{
					"containerRevision": ref("ContainerRevision", "container-revision"), "weight": 10000,
				}},
			},
		},
		ResolvedUIDConstraints: []model.Constraint{{
			Kind:   model.ConstraintSameResolvedTarget,
			Anchor: "/service", Members: "/revisions/*/containerRevision", Through: "/service",
		}},
	},
	{
		Family: Family, Kind: "ContainerEndpoint", Slug: "container-endpoint", Role: model.RoleAttachment,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Container Endpoint", Description: "Host-assigned HTTPS reachability for the active Container " +
			"Traffic. The address is output, not desired state, and remains stable for the attachment UID.",
		Fields: []model.Field{
			{HCL: "service", Wire: "service", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("ContainerService"), Required: true, Immutable: true,
				Exclusive: &model.ExclusiveHold{}, RequiredEntrypoint: "http",
				Doc:     "Container Service whose active traffic receives HTTPS requests. Changing it replaces the endpoint.",
				Example: ref("ContainerService", "container-service")},
		},
		Outputs: []model.Field{
			{HCL: "hostname", Wire: "hostname", Kind: model.KindString, HostAssigned: true,
				Pattern: model.PatternCanonicalHostname, MaxLength: 253,
				Doc: "Canonical DNS hostname assigned by the host. Its shape is host detail and must not be reconstructed."},
			{HCL: "url", Wire: "url", Kind: model.KindString, HostAssigned: true,
				Pattern: `^https://[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+/$`, MaxLength: 262,
				Doc: "Exactly https:// plus the assigned hostname and a root slash."},
		},
	},
	{
		Family: Family, Kind: "ContainerCustomDomain", Slug: "container-custom-domain", Role: model.RoleAttachment,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Container Custom Domain", Description: "HTTPS attachment that serves one canonical customer-owned " +
			"hostname from the active Container Traffic. ACME certificate issuance and TLS termination are host duties.",
		Fields: []model.Field{
			{HCL: "service", Wire: "service", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("ContainerService"), Required: true, Immutable: true,
				RequiredEntrypoint: "http",
				Doc:                "Container Service served on this hostname. Changing it replaces the attachment.",
				Example:            ref("ContainerService", "container-service")},
			{HCL: "hostname", Wire: "hostname", Kind: model.KindString,
				Pattern: model.PatternHostname, MaxLength: 253, Required: true, Immutable: true, Claimed: true,
				Doc: "Customer-owned DNS hostname served by this attachment. The host canonicalizes it before " +
					"comparison and storage; ACME certificate issuance is the host's duty.",
				Example: "app.example.invalid", AltExample: "alt.example.invalid", CounterExample: "not a hostname"},
		},
	},
}

// Validate proves the family source is closed and every Form declaration is
// internally coherent. Exact reference identities are resolved by RenderForms.
func Validate() error {
	if err := model.ValidateNoOpenTokens(Forms); err != nil {
		return err
	}
	seenKinds, seenSlugs := map[string]bool{}, map[string]bool{}
	for _, form := range Forms {
		if err := form.Validate(); err != nil {
			return err
		}
		if form.Family != Family {
			return fmt.Errorf("form %s belongs to family %s, want %s", form.Kind, form.Family.APIVersion(), Family.APIVersion())
		}
		if seenKinds[form.Kind] || seenSlugs[form.Slug] {
			return fmt.Errorf("duplicate Container family identity %s/%s", form.Kind, form.Slug)
		}
		seenKinds[form.Kind], seenSlugs[form.Slug] = true, true
		if form.DefinitionVersion != definitionVersion {
			return fmt.Errorf("form %s definition version %q, want %q", form.Kind, form.DefinitionVersion, definitionVersion)
		}
		if err := model.ValidateEnvironmentNamespace(form); err != nil {
			return err
		}
	}
	return nil
}

// ByKind returns one source Form by its exact portable kind.
func ByKind(kind string) (model.Form, bool) {
	for _, form := range Forms {
		if form.Kind == kind {
			return form, true
		}
	}
	return model.Form{}, false
}
