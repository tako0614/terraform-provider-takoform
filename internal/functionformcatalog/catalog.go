// Package functionformcatalog declares the Regional Function Form Family.
//
// The package is deliberately a data-only source catalog.  It describes the
// portable shape of the four function Forms and their exact same-family
// references; it does not contain a provider resource mapping, an execution
// runtime, or a standard-service implementation.
package functionformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the versionless Function family group.  A Form's own definition
// SemVer and digest are its identity; the family never carries a latest or a
// fallback version.
var Family = model.Family{Group: "function.forms.takoform.com"}

const (
	definitionVersion = "0.1.0"
	firstHostAPI      = "forms.takoform.com/v1"
	currentHostAPI    = "forms.takoform.com/v1"

	// The artifact is a committed manifest, not a URL or a mutable tag.
	artifactDigestPattern = model.PatternCanonicalSHA256
	// Function handlers are JavaScript exports.  Keep the name closed and
	// bounded without pretending to define the runtime's full ABI here.
	handlerMaxLength = 128
	// Ordered command/argument tokens are intentionally opaque to this family;
	// the host passes them to the process exactly in their declared order.
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

func commandField(hcl, wire, doc string) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindStringList,
		AbsenceIsSemantic: true, ItemPattern: commandTokenPattern, MaxLength: commandTokenLength, MaxItems: 64,
		Doc: doc,
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

// Forms is the complete Regional Function Family MVP set, in a stable order.
// Provider resource names are intentionally absent: they are not Form
// identity and are assigned by the provider-owned registry.
var Forms = []model.Form{
	{
		Family: Family, Kind: "Function", Slug: "function", Role: model.RoleIdentity,
		DefinitionVersion: definitionVersion, RequiresHostAPI: firstHostAPI,
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: FunctionRuntimeInterfaceName, Version: interfaceVersion}},
		Title:              "Function", Description: "Logical identity of one regional JavaScript function. " +
			"Code, configuration, resource bounds, and traffic are represented by the surrounding " +
			"revision and deployment Forms; this identity carries no desired fields.",
	},
	{
		Family: Family, Kind: "FunctionVersion", Slug: "function-version", Role: model.RoleRevision,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Function Version", Description: "Immutable content-addressed executable snapshot of one Function: " +
			"artifact manifest, handler, environment declarations, sealed slots, and invocation bounds.",
		Fields: []model.Field{
			{HCL: "function", Wire: "function", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("Function"), Required: true, Immutable: true,
				Doc: "Function identity this immutable version belongs to.", Example: ref("Function", "function")},
			{HCL: "artifact", Wire: "artifact", Kind: model.KindString,
				Pattern: artifactDigestPattern, MaxLength: 71, Required: true, Immutable: true,
				Doc: "Digest of the committed FunctionBundle artifact manifest. The host resolves and verifies " +
					"the manifest before storing this revision; a mutable URL or tag is not portable state.",
				Example:        "sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43",
				AltExample:     "sha256:8624fd492ece196a5048414afd598275b811beafc00aab602bfb59978f880765",
				CounterExample: "sha256:not-a-function-bundle"},
			{HCL: "handler", Wire: "handler", Kind: model.KindString,
				Pattern: model.PatternBindingName, MaxLength: handlerMaxLength, Required: true, Immutable: true,
				Doc:     "Named export of the artifact's main module invoked for each event.",
				Example: "handle", CounterExample: "not a handler"},
			varsField(), sensitiveVarsField(), externalServicesField(),
			{HCL: "memory_mib", Wire: "memoryMiB", Kind: model.KindInteger,
				Min: model.I64(128), Max: model.I64(10240), Required: true, Immutable: true,
				Doc: "Usable memory bound for one invocation environment, in MiB.", Example: 512},
			{HCL: "timeout_seconds", Wire: "timeoutSeconds", Kind: model.KindInteger,
				Min: model.I64(1), Max: model.I64(900), Required: true, Immutable: true,
				Doc: "Wall-clock invocation budget, in seconds. Expiry terminates the invocation.", Example: 30},
			{HCL: "max_concurrency", Wire: "maxConcurrency", Kind: model.KindInteger,
				Min: model.I64(1), Max: model.I64(1000), Required: true, Immutable: true,
				Doc: "Maximum simultaneous invocations served by one version environment.", Example: 10},
		},
	},
	{
		Family: Family, Kind: "FunctionDeployment", Slug: "function-deployment", Role: model.RoleDeployment,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Function Deployment", Description: "The one active traffic selection for a Function. " +
			"One or two immutable versions carry positive basis-point weights totaling exactly 10000.",
		Fields: []model.Field{
			{HCL: "function", Wire: "function", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("Function"), Required: true, Immutable: true,
				Exclusive: &model.ExclusiveHold{},
				Doc:       "Function identity governed by this deployment. There is at most one deployment per identity.",
				Example:   ref("Function", "function")},
			{HCL: "versions", Wire: "versions", Kind: model.KindObjectList,
				Required: true, MinItems: 1, MaxItems: 2, Sum: &model.SummedMember{Member: "weight", Total: 10000},
				Doc: "One or two selected Function Versions and their positive basis-point weights. " +
					"Weights are host-validated and must sum to exactly 10000.",
				Fields: []model.Field{
					{HCL: "function_version", Wire: "functionVersion", Kind: model.KindResourceRef,
						ResourceTarget: resourceTarget("FunctionVersion"), Required: true,
						Doc: "Immutable version selected by this traffic entry."},
					{HCL: "weight", Wire: "weight", Kind: model.KindInteger,
						Min: model.I64(1), Max: model.I64(10000), Required: true,
						Doc: "Positive traffic weight in basis points.", Example: 10000},
				},
				Example: []any{map[string]any{
					"functionVersion": ref("FunctionVersion", "function-version"), "weight": 10000,
				}},
			},
		},
		ResolvedUIDConstraints: []model.Constraint{{
			Kind:   model.ConstraintSameResolvedTarget,
			Anchor: "/function", Members: "/versions/*/functionVersion", Through: "/function",
		}},
	},
	{
		Family: Family, Kind: "FunctionEndpoint", Slug: "function-endpoint", Role: model.RoleAttachment,
		DefinitionVersion: definitionVersion, RequiresHostAPI: currentHostAPI,
		Title: "Function Endpoint", Description: "Host-assigned HTTPS reachability for the active Function " +
			"Deployment. The address is output, not desired state, and remains stable for the attachment UID.",
		Fields: []model.Field{
			{HCL: "function", Wire: "function", Kind: model.KindResourceRef,
				ResourceTarget: resourceTarget("Function"), Required: true, Immutable: true,
				Exclusive: &model.ExclusiveHold{}, RequiredEntrypoint: FunctionRuntimeEntrypoint,
				Doc:     "Function whose active deployment receives HTTPS requests. Changing it replaces the endpoint.",
				Example: ref("Function", "function")},
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
}

// Validate proves the family source is closed and every Form's declaration is
// internally coherent. Cross-resource contract and digest checks happen when
// RenderForms resolves the exact target set.
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
			return fmt.Errorf("duplicate Function family identity %s/%s", form.Kind, form.Slug)
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
