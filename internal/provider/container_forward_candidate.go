package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	containerForwardCandidateFormVersion               = "0.1.0-dev.1"
	containerForwardCandidateVectorVersion             = "0.1.0-dev.1"
	containerForwardCandidateWorkerVersion             = "0.6.0-dev.1"
	containerForwardCandidateDeploymentVersion         = "0.5.0-dev.1"
	containerForwardCandidateInterfaceVersion          = "0.1.0"
	containerForwardCandidateWorkerVersionSchemaDigest = "sha256:af31ce7e2e7da88f3281b8e49e35d35054ae33cc3bc3871dfe542f01518d1da5"
	containerCandidateVectorInterfaceDigest            = "sha256:6df8b7680b0ff278cb8fcb6f56c602ec5d6bb9b4ec115a6172e70e6b54d6cada"
	containerCandidateHTTPInterfaceDigest              = "sha256:65039b3ea0223057e5f34eff182098a1ba097ee351e19fcb7626a9af41a8561a"
	containerCandidateVectorBindingDigest              = "sha256:bc367a665405e405ac99091fb7f1908a382123d63c4acbd807de97c10184a809"
	containerCandidateHTTPBindingDigest                = "sha256:caf35e19cd375a9115310dead9e4985f772e044d84dd3449cc347cd0ab49f301"
	containerCandidateActorInterfaceDigest             = "sha256:907b3168365945cc3d03419755110a12547218774d0c56d8af4477c6a88fb638"
	containerCandidateActorBindingDigest               = "sha256:299f450ffe08d78419ab639064c513a166498e203764678bdb39e9bfaee5e7a0"
	containerCandidateRuntimeInterfaceDigest           = "sha256:a3852fd29d798359e2c5ba53926875a195e8b6788f90f54fe14f19385b251d7f"
)

type containerForwardCandidateSourceDocument struct {
	FormRef struct {
		APIVersion        string `json:"apiVersion"`
		Kind              string `json:"kind"`
		DefinitionVersion string `json:"definitionVersion"`
		SchemaDigest      string `json:"schemaDigest"`
	} `json:"formRef"`
	Form struct {
		Kind           string `json:"kind"`
		DefinitionJSON string `json:"definitionJson"`
	} `json:"form"`
	Interface containerForwardCandidateSourceDocumentInterface `json:"interface"`
	Binding   struct {
		Name         string `json:"name"`
		Version      string `json:"version"`
		SchemaDigest string `json:"schemaDigest"`
	} `json:"binding"`
	Vector struct {
		Form struct {
			Kind       string `json:"kind"`
			Definition struct {
				DefinitionVersion string `json:"definitionVersion"`
			} `json:"definition"`
			DefinitionJSON string `json:"definitionJson"`
		} `json:"form"`
		Interface containerForwardCandidateSourceDocumentInterface `json:"interface"`
		Binding   struct {
			Name         string `json:"name"`
			Version      string `json:"version"`
			SchemaDigest string `json:"schemaDigest"`
		} `json:"binding"`
	} `json:"vector"`
	WorkerVersion struct {
		Kind       string `json:"kind"`
		Definition struct {
			APIVersion            string                   `json:"apiVersion"`
			DefinitionVersion     string                   `json:"definitionVersion"`
			Kind                  string                   `json:"kind"`
			DesiredSchema         map[string]any           `json:"desiredSchema"`
			Constraints           []model.Constraint       `json:"constraints"`
			ImmutableFields       []string                 `json:"immutableFields"`
			RequiresHostAPI       string                   `json:"requiresHostApi"`
			Role                  string                   `json:"role"`
			LifecycleCapabilities []string                 `json:"lifecycleCapabilities"`
			AcceptedBindings      []formpackage.BindingRef `json:"acceptedBindings"`
		} `json:"definition"`
	} `json:"workerVersion"`
	WorkerDeployment struct {
		Kind       string `json:"kind"`
		Definition struct {
			DefinitionVersion string `json:"definitionVersion"`
		} `json:"definition"`
		DefinitionJSON string `json:"definitionJson"`
	} `json:"workerDeployment"`
}

type containerForwardCandidateSourceDocumentInterface struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	SchemaDigest string `json:"schemaDigest"`
}

const (
	containerCandidateImagePattern      = `^(?:(?:localhost|(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]*[a-z0-9])?|(?:(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.){3}(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9]))(?::(?:[1-9][0-9]{0,3}|[1-5][0-9]{4}|6[0-4][0-9]{3}|65[0-4][0-9]{2}|655[0-2][0-9]|6553[0-5]))?/)?[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*@sha256:[0-9a-f]{64}$`
	containerCandidateHealthPathPattern = `^/([^/\\#\x00-\x20\x7F-\x9F][^\\#\x00-\x20\x7F-\x9F]*|)$`
	containerCandidateDescription       = "Unpublished Edge install-owned Container Service candidate. It runs one application-selected OCI image identified by a SHA-256 digest and exposes one application HTTP port after the required health path succeeds. image, port, healthPath, environment, requiredSensitiveVars, workloadRevision, and outboundInternet are application intent; provider, registry credential, native IDs, capacity, filesystem, endpoint, and secret values remain outside portable state. Lifecycle is create, read, update, observe, and delete; import is not claimed and no portable outputs are declared. workloadRevision is an application-owned non-secret revision token: changing it deliberately starts a new execution generation and is required for secret-only rotation, while secret bytes arrive through a separate sealed input and never enter the Form. The host starts a new revision before cutover; an unhealthy new generation does not replace the old serving generation, and the old generation may remain Ready while that update is unhealthy. servingGeneration is observed only as a canonical positive-decimal string when selection exists. outboundInternet=true grants isolated outbound Internet and DNS only: it does not grant host networking, metadata access, private sibling access, or inbound publication; a host unable to enforce this isolation refuses the resource. This candidate introduces no Actor, WebSocket, or new stable contract and requires the Host/Provider to reject any environment/requiredSensitiveVars name overlap at instance admission (released Core's closed constraint vocabulary cannot express that cross-list relation). It does not imply a separate family, new namespace, Host support, admission, or publication. Secret-bearing workload support additionally requires a concrete generation-bound sealed-input adapter, which this publisher candidate does not implement."
	vectorCandidateDescription          = "Unpublished install-owned vector index candidate. The resource fixes a positive vector dimension, cosine metric, and bounded optional metadata filter-key set. Native storage, endpoints, credentials, table names, and provider IDs are Host-private. Lifecycle is create, read, delete, and observe only; configuration is immutable and no import or update capability is claimed by this candidate. Deleting a bound Resource is refused. A successful unbound delete reports application-visible absence; physical media retention or purge is operator policy, not a portable promise. Observed state reports dimension, metric, filterKeys, and an eventual total vector count across all namespaces; status outputs are not claimed."
)

// NewContainerCandidate is an opt-in local schema candidate. It shares the
// released Provider implementation but has a separate resource roster and a
// deliberately non-release version. It is not used by New or the normal
// registry binary.
func NewContainerCandidate(source []byte) func() frameworkprovider.Provider {
	if err := validateContainerForwardCandidateSource(source); err != nil {
		panic(err)
	}
	return func() frameworkprovider.Provider {
		return &takoformProvider{
			version:            "0.0.0-dev+35a77bdb37483fccbb365998923824040dad67cc",
			candidateOnly:      true,
			candidateResources: newContainerCandidateResources(),
		}
	}
}

func validateContainerForwardCandidateSource(raw []byte) error {
	var source containerForwardCandidateSourceDocument
	if err := json.Unmarshal(raw, &source); err != nil {
		return err
	}
	if source.FormRef.APIVersion != "edge.forms.takoform.com" || source.FormRef.Kind != "ContainerService" ||
		source.FormRef.DefinitionVersion != containerForwardCandidateFormVersion || source.FormRef.SchemaDigest != "sha256:883002fb7f9cade8e8a817a14b406889afa024907348473fe0de28622b79e464" ||
		source.Form.Kind != source.FormRef.Kind || source.Interface.Name != "container.http" || source.Interface.Version != containerForwardCandidateInterfaceVersion ||
		source.Interface.SchemaDigest != containerCandidateHTTPInterfaceDigest ||
		source.Binding.Name != "module-worker.container-http" || source.Binding.Version != containerForwardCandidateInterfaceVersion ||
		source.Binding.SchemaDigest != containerCandidateHTTPBindingDigest ||
		source.Vector.Form.Kind != "VectorIndex" || source.Vector.Form.Definition.DefinitionVersion != containerForwardCandidateVectorVersion ||
		source.Vector.Interface.Name != "edge.vector" || source.Vector.Interface.Version != containerForwardCandidateInterfaceVersion ||
		source.Vector.Interface.SchemaDigest != containerCandidateVectorInterfaceDigest ||
		source.Vector.Binding.Name != "module-worker.edge-vector" || source.Vector.Binding.Version != "1.0.0" ||
		source.Vector.Binding.SchemaDigest != containerCandidateVectorBindingDigest ||
		source.WorkerVersion.Kind != "WorkerVersion" || source.WorkerVersion.Definition.APIVersion != "edge.forms.takoform.com" ||
		source.WorkerVersion.Definition.DefinitionVersion != containerForwardCandidateWorkerVersion ||
		len(source.WorkerVersion.Definition.AcceptedBindings) != 9 || source.WorkerDeployment.Kind != "WorkerDeployment" ||
		source.WorkerDeployment.Definition.DefinitionVersion != containerForwardCandidateDeploymentVersion {
		return fmt.Errorf("takoform provider: embedded Container forward candidate identities drifted")
	}
	wantBindings := []string{
		"module-worker.edge-kv", "module-worker.object-bucket", "module-worker.sqlite", "module-worker.queue-producer",
		"module-worker.service", "module-worker.workflow", "module-worker.actor", "module-worker.edge-vector", "module-worker.container-http",
	}
	for index, want := range wantBindings {
		if source.WorkerVersion.Definition.AcceptedBindings[index].Name != want {
			return fmt.Errorf("takoform provider: embedded candidate WorkerVersion binding %d drifted", index)
		}
	}
	properties, _ := source.WorkerVersion.Definition.DesiredSchema["properties"].(map[string]any)
	if _, ok := properties["containerHttpBindings"]; !ok {
		return fmt.Errorf("takoform provider: candidate WorkerVersion has no exact containerHttpBindings field")
	}
	assembly := mustPublisherProviderSnapshotAssembly()
	worker := containerCandidateWorkerVersion(assembly)
	if source.WorkerVersion.Definition.Kind != worker.Kind || source.WorkerVersion.Definition.Role != string(worker.Role) ||
		source.WorkerVersion.Definition.RequiresHostAPI != worker.RequiresHostAPI {
		return fmt.Errorf("takoform provider: exact candidate WorkerVersion kind, role, or required Host API drifted")
	}
	acceptedBindings, err := containerCandidateAcceptedBindingRefs(assembly, worker.AcceptedBindings)
	if err != nil {
		return err
	}
	if !canonicalJSONEqual(acceptedBindings, source.WorkerVersion.Definition.AcceptedBindings) {
		return fmt.Errorf("takoform provider: typed WorkerVersion accepted Bindings differ from exact 0.6.0-dev.1 Form refs")
	}
	workerSchema, err := worker.DesiredSchema(containerCandidateTargetResolver{v3SnapshotProjectionResolver{snapshot: assembly.snapshot}})
	if err != nil {
		return fmt.Errorf("takoform provider: typed WorkerVersion desired schema is invalid: %w", err)
	}
	if !canonicalJSONEqual(candidateSchemaSemantics(workerSchema), candidateSchemaSemantics(source.WorkerVersion.Definition.DesiredSchema)) {
		return fmt.Errorf("takoform provider: typed WorkerVersion desired schema differs from exact 0.6.0-dev.1 Form")
	}
	if !canonicalJSONEqual(worker.Constraints(), source.WorkerVersion.Definition.Constraints) ||
		!canonicalJSONEqual(worker.ImmutableFields(), source.WorkerVersion.Definition.ImmutableFields) ||
		!canonicalJSONEqual(worker.LifecycleCapabilities(), source.WorkerVersion.Definition.LifecycleCapabilities) {
		return fmt.Errorf("takoform provider: typed WorkerVersion constraints, immutable fields, or lifecycle differ from exact 0.6.0-dev.1 Form")
	}
	if err := validateContainerCandidateWorkerDeployment(source.WorkerDeployment.DefinitionJSON); err != nil {
		return err
	}
	var vectorDefinition struct {
		ImmutableFields []string `json:"immutableFields"`
	}
	if err := json.Unmarshal([]byte(source.Vector.Form.DefinitionJSON), &vectorDefinition); err != nil {
		return fmt.Errorf("takoform provider: candidate VectorIndex definition is malformed: %w", err)
	}
	vector := containerCandidateVectorIndex()
	if !canonicalJSONEqual(vector.ImmutableFields(), vectorDefinition.ImmutableFields) {
		return fmt.Errorf("takoform provider: typed VectorIndex replacement semantics differ from exact 0.1.0-dev.1 Form semantics")
	}
	if !sameCandidateInterfaces(vector.ProvidedInterfaces, []model.InterfaceRefSource{{Name: source.Vector.Interface.Name, Version: source.Vector.Interface.Version}}) {
		return fmt.Errorf("takoform provider: typed VectorIndex provided Interfaces differ from the exact source Form")
	}
	container := containerCandidateService()
	if !sameCandidateInterfaces(container.ProvidedInterfaces, []model.InterfaceRefSource{{Name: source.Interface.Name, Version: source.Interface.Version}}) {
		return fmt.Errorf("takoform provider: typed ContainerService provided Interfaces differ from the exact source Form")
	}
	if err := validateContainerCandidateFormDefinition("ContainerService", source.Form.DefinitionJSON, container, source.Interface); err != nil {
		return err
	}
	if err := validateContainerCandidateFormDefinition("VectorIndex", source.Vector.Form.DefinitionJSON, vector, source.Vector.Interface); err != nil {
		return err
	}
	return nil
}

func containerCandidateAcceptedBindingRefs(assembly *v3ProviderAssembly, accepted []model.BindingRefSource) ([]formpackage.BindingRef, error) {
	refs := make([]formpackage.BindingRef, 0, len(accepted))
	for _, source := range accepted {
		var ref formpackage.BindingRef
		switch source.Name {
		case "module-worker.actor":
			if source.Version != "2.0.0" {
				return nil, fmt.Errorf("takoform provider: candidate WorkerVersion actor Binding version drifted")
			}
			ref = formpackage.BindingRef{
				APIVersion: "bindings.takoform.com/v1alpha2", Name: source.Name, Version: source.Version,
				SchemaDigest: containerCandidateActorBindingDigest,
			}
		case "module-worker.edge-vector":
			if source.Version != "1.0.0" {
				return nil, fmt.Errorf("takoform provider: candidate WorkerVersion edge-vector Binding version drifted")
			}
			ref = formpackage.BindingRef{
				APIVersion: "bindings.takoform.com/v1alpha2", Name: source.Name, Version: source.Version,
				SchemaDigest: containerCandidateVectorBindingDigest,
			}
		case "module-worker.container-http":
			if source.Version != containerForwardCandidateInterfaceVersion {
				return nil, fmt.Errorf("takoform provider: candidate WorkerVersion container-http Binding version drifted")
			}
			ref = formpackage.BindingRef{
				APIVersion: "bindings.takoform.com/v1alpha2", Name: source.Name, Version: source.Version,
				SchemaDigest: containerCandidateHTTPBindingDigest,
			}
		default:
			matches := make([]formpackage.BindingRef, 0, 1)
			for _, binding := range assembly.snapshot.Bindings() {
				if binding.Ref.Name == source.Name && binding.Ref.Version == source.Version {
					matches = append(matches, binding.Ref)
				}
			}
			if len(matches) != 1 {
				return nil, fmt.Errorf("takoform provider: WorkerVersion Binding %s@%s resolves %d exact publisher Snapshot refs", source.Name, source.Version, len(matches))
			}
			ref = matches[0]
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func validateContainerCandidateFormDefinition(
	kind string,
	raw string,
	form model.Form,
	interfaceRef containerForwardCandidateSourceDocumentInterface,
) error {
	var actual struct {
		Title              string             `json:"title"`
		Description        string             `json:"description"`
		DesiredSchema      map[string]any     `json:"desiredSchema"`
		Constraints        []model.Constraint `json:"constraints"`
		ImmutableFields    []string           `json:"immutableFields"`
		ProvidedInterfaces []struct {
			APIVersion   string `json:"apiVersion"`
			Name         string `json:"name"`
			Version      string `json:"version"`
			SchemaDigest string `json:"schemaDigest"`
		} `json:"providedInterfaces"`
	}
	if err := json.Unmarshal([]byte(raw), &actual); err != nil {
		return fmt.Errorf("takoform provider: exact %s definition is malformed: %w", kind, err)
	}
	desiredSchema, err := form.DesiredSchema(nil)
	if err != nil {
		return fmt.Errorf("takoform provider: typed %s desired schema is invalid: %w", kind, err)
	}
	if !canonicalJSONEqual(candidateSchemaSemantics(desiredSchema), candidateSchemaSemantics(actual.DesiredSchema)) ||
		!canonicalJSONEqual(form.Constraints(), actual.Constraints) ||
		!canonicalJSONEqual(form.ImmutableFields(), actual.ImmutableFields) {
		return fmt.Errorf("takoform provider: typed %s projection differs from the exact candidate definition", kind)
	}
	if len(actual.ProvidedInterfaces) != 1 || len(form.ProvidedInterfaces) != 1 ||
		actual.ProvidedInterfaces[0].APIVersion != "interfaces.takoform.com/v1alpha1" ||
		actual.ProvidedInterfaces[0].Name != interfaceRef.Name || actual.ProvidedInterfaces[0].Version != interfaceRef.Version ||
		actual.ProvidedInterfaces[0].SchemaDigest != interfaceRef.SchemaDigest ||
		form.ProvidedInterfaces[0].Name != interfaceRef.Name || form.ProvidedInterfaces[0].Version != interfaceRef.Version {
		return fmt.Errorf("takoform provider: typed %s provided Interface differs from the exact candidate contract", kind)
	}
	return nil
}

// candidateSchemaSemantics discards only JSON Schema prose annotations. The
// comparison still binds defaults, validators, required members, constraints,
// target FormRefs, and every other behavior-bearing schema key.
func candidateSchemaSemantics(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, member := range typed {
			if key == "description" || key == "title" {
				continue
			}
			out[key] = candidateSchemaSemantics(member)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, member := range typed {
			out[index] = candidateSchemaSemantics(member)
		}
		return out
	default:
		return value
	}
}

func sameCandidateInterfaces(actual, expected []model.InterfaceRefSource) bool {
	return canonicalJSONEqual(actual, expected)
}

func validateContainerCandidateWorkerDeployment(raw string) error {
	var actual struct {
		DesiredSchema         map[string]any     `json:"desiredSchema"`
		Constraints           []model.Constraint `json:"constraints"`
		ImmutableFields       []string           `json:"immutableFields"`
		LifecycleCapabilities []string           `json:"lifecycleCapabilities"`
	}
	if err := json.Unmarshal([]byte(raw), &actual); err != nil {
		return fmt.Errorf("takoform provider: candidate WorkerDeployment definition is malformed: %w", err)
	}
	form := containerCandidateWorkerDeployment()
	desiredSchema, err := form.DesiredSchema(containerCandidateTargetResolver{})
	if err != nil {
		return fmt.Errorf("takoform provider: candidate WorkerDeployment projection is invalid: %w", err)
	}
	if !canonicalJSONEqual(candidateSchemaSemantics(desiredSchema), candidateSchemaSemantics(actual.DesiredSchema)) ||
		!canonicalJSONEqual(form.Constraints(), actual.Constraints) ||
		!canonicalJSONEqual(form.ImmutableFields(), actual.ImmutableFields) ||
		!canonicalJSONEqual(form.LifecycleCapabilities(), actual.LifecycleCapabilities) {
		return fmt.Errorf("takoform provider: typed WorkerDeployment projection differs from exact 0.5.0-dev.1 Form semantics")
	}
	return nil
}

type containerCandidateTargetResolver struct{ v3SnapshotProjectionResolver }

func (resolver containerCandidateTargetResolver) ResolveResourceTarget(target model.ResourceTarget) (model.ResolvedResourceTarget, error) {
	resolved := model.ResolvedResourceTarget{ResourceNamePattern: model.PatternResourceName}
	if target.Contract.Interface != nil && !target.Contract.ExactForm {
		var digest string
		switch {
		case target.Contract.Interface.Name == "worker.runtime" && target.Contract.Interface.Version == "1.2.0":
			digest = containerCandidateRuntimeInterfaceDigest
		case target.Contract.Interface.Name == "worker.actor" && target.Contract.Interface.Version == "2.0.0":
			digest = containerCandidateActorInterfaceDigest
		case target.Contract.Interface.Name == "edge.vector" && target.Contract.Interface.Version == containerForwardCandidateInterfaceVersion:
			digest = containerCandidateVectorInterfaceDigest
		case target.Contract.Interface.Name == "container.http" && target.Contract.Interface.Version == containerForwardCandidateInterfaceVersion:
			digest = containerCandidateHTTPInterfaceDigest
		default:
			return resolver.v3SnapshotProjectionResolver.ResolveResourceTarget(target)
		}
		resolved.RequiredInterface = &model.RequiredInterface{
			APIVersion: "interfaces.takoform.com/v1alpha1", Name: target.Contract.Interface.Name,
			Version: target.Contract.Interface.Version, SchemaDigest: digest,
		}
		return resolved, nil
	}
	if !target.Contract.ExactForm || target.Contract.Interface != nil {
		return model.ResolvedResourceTarget{}, fmt.Errorf("candidate target %s/%s must pin an exact Form or Interface", target.Group, target.Kind)
	}
	var ref model.TargetFormRef
	switch target.Kind {
	case "ModuleWorker":
		ref = model.TargetFormRef{
			APIVersion: "edge.forms.takoform.com", Kind: "ModuleWorker", DefinitionVersion: "0.2.0",
			SchemaDigest: "sha256:f47672eaddb821c6c9c8fae88ebd6e683858afc2026c047ec3e509221cb2e3aa",
		}
	case "WorkerVersion":
		ref = model.TargetFormRef{
			APIVersion: "edge.forms.takoform.com", Kind: "WorkerVersion",
			DefinitionVersion: containerForwardCandidateWorkerVersion,
			SchemaDigest:      containerForwardCandidateWorkerVersionSchemaDigest,
		}
	default:
		return resolver.v3SnapshotProjectionResolver.ResolveResourceTarget(target)
	}
	if ref.APIVersion != target.Group || ref.Kind != target.Kind {
		return model.ResolvedResourceTarget{}, fmt.Errorf("candidate target %s/%s does not match exact identity %s", target.Group, target.Kind, ref.String())
	}
	resolved.TargetFormRefs = []model.TargetFormRef{ref}
	return resolved, nil
}

func newContainerCandidateResources() []func() resource.Resource {
	assembly := mustPublisherProviderSnapshotAssembly()
	factories := newPublisherFormResources()
	worker := containerCandidateWorkerVersion(assembly)
	deployment := containerCandidateWorkerDeployment()
	var out []func() resource.Resource
	for _, factory := range factories {
		instance := factory()
		var metadata resource.MetadataResponse
		instance.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		if metadata.TypeName == "takoform_worker_version" {
			declared := worker
			out = append(out, func() resource.Resource {
				return &v3FormResource{form: declared, resourceType: "takoform_worker_version"}
			})
			continue
		}
		if metadata.TypeName == "takoform_worker_deployment" {
			declared := deployment
			out = append(out, func() resource.Resource {
				return &v3FormResource{form: declared, resourceType: "takoform_worker_deployment"}
			})
			continue
		}
		out = append(out, factory)
	}

	for _, form := range []model.Form{containerCandidateService(), containerCandidateVectorIndex()} {
		declared := form
		resourceType := "takoform_container_service"
		if form.Kind == "VectorIndex" {
			resourceType = "takoform_vector_index"
		}
		if form.Kind == "ContainerService" {
			out = append(out, func() resource.Resource {
				return &containerCandidateServiceResource{v3FormResource: &v3FormResource{form: declared, resourceType: resourceType}}
			})
		} else {
			out = append(out, func() resource.Resource {
				return &v3FormResource{form: declared, resourceType: resourceType}
			})
		}
	}
	return out
}

// containerCandidateServiceResource adds the one cross-attribute rule the
// pinned ContainerService proposal declares: environment names and required
// secret names share one runtime namespace.
type containerCandidateServiceResource struct{ *v3FormResource }

var _ resource.ResourceWithValidateConfig = (*containerCandidateServiceResource)(nil)

func (r *containerCandidateServiceResource) ValidateConfig(
	ctx context.Context,
	req resource.ValidateConfigRequest,
	resp *resource.ValidateConfigResponse,
) {
	if req.Config.Raw.IsNull() {
		return
	}
	var environment types.List
	readEnvironment := req.Config.GetAttribute(ctx, path.Root("environment"), &environment)
	resp.Diagnostics.Append(readEnvironment...)
	if readEnvironment.HasError() || environment.IsNull() || environment.IsUnknown() {
		return
	}
	var sensitive types.Set
	readSensitive := req.Config.GetAttribute(ctx, path.Root("required_sensitive_vars"), &sensitive)
	resp.Diagnostics.Append(readSensitive...)
	if readSensitive.HasError() {
		return
	}
	claimed := map[string]path.Path{}
	for index, element := range environment.Elements() {
		object, ok := element.(types.Object)
		if !ok || object.IsNull() || object.IsUnknown() {
			continue
		}
		name, ok := object.Attributes()["name"].(types.String)
		if !ok || name.IsNull() || name.IsUnknown() {
			continue
		}
		claimed[name.ValueString()] = path.Root("environment").AtListIndex(index).AtName("name")
	}
	if sensitive.IsNull() || sensitive.IsUnknown() {
		return
	}
	for _, element := range sensitive.Elements() {
		name, ok := element.(types.String)
		if !ok || name.IsNull() || name.IsUnknown() {
			continue
		}
		if previous, exists := claimed[name.ValueString()]; exists {
			resp.Diagnostics.AddAttributeError(
				path.Root("required_sensitive_vars"),
				"Duplicate Container environment name",
				"The name "+name.ValueString()+" is already declared at "+previous.String()+". Plain environment values and required sensitive values share one ContainerService environment namespace.",
			)
		}
	}
}

func containerCandidateWorkerDeployment() model.Form {
	return model.Form{
		Family: model.Family{Group: "edge.forms.takoform.com"}, Kind: "WorkerDeployment", Slug: "worker-deployment",
		Role: model.RoleDeployment, RequiresHostAPI: "forms.takoform.com/v1", Title: "Worker Deployment",
		DefinitionVersion: containerForwardCandidateDeploymentVersion,
		Description:       "Selects which Worker Versions of one Module Worker serve traffic and in what proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, never mutating a revision.",
		Fields: []model.Field{
			{HCL: "worker", Wire: "worker", Kind: model.KindResourceRef, Required: true, Immutable: true,
				ResourceTarget: &model.ResourceTarget{Group: "edge.forms.takoform.com", Kind: "ModuleWorker", Contract: model.TargetContract{ExactForm: true}},
				Exclusive:      &model.ExclusiveHold{}, Doc: "Module Worker identity whose traffic this deployment governs."},
			{HCL: "versions", Wire: "versions", Kind: model.KindObjectList, Required: true, MinItems: 1, MaxItems: 8,
				Sum: &model.SummedMember{Member: "weight", Total: 10000},
				Doc: "Active Worker Versions and their traffic weights in basis points. Weights must sum to exactly 10000.",
				Fields: []model.Field{
					{HCL: "weight", Wire: "weight", Kind: model.KindInteger, Required: true, Min: model.I64(1), Max: model.I64(10000), Doc: "Traffic share in basis points (1..10000)."},
					{HCL: "worker_version", Wire: "workerVersion", Kind: model.KindResourceRef, Required: true,
						ResourceTarget: &model.ResourceTarget{Group: "edge.forms.takoform.com", Kind: "WorkerVersion", Contract: model.TargetContract{ExactForm: true}},
						Doc:            "Worker Version receiving this weight."},
				}},
		},
	}
}

func containerCandidateWorkerVersion(assembly *v3ProviderAssembly) model.Form {
	var worker model.Form
	for _, form := range assembly.currentForms {
		if form.Kind == "WorkerVersion" {
			worker = form
			break
		}
	}
	if worker.Kind == "" {
		panic("takoform provider: Container candidate has no existing typed WorkerVersion basis")
	}
	worker.DefinitionVersion = containerForwardCandidateWorkerVersion
	for index := range worker.Fields {
		if worker.Fields[index].HCL == "worker" {
			worker.Fields[index].Target.Interface.Version = "1.2.0"
		}
		if worker.Fields[index].HCL == "actor_bindings" {
			worker.Fields[index].Target.Interface.Version = "2.0.0"
		}
	}
	worker.Fields = append(worker.Fields,
		model.Field{
			HCL: "vector_bindings", Wire: "vectorBindings", Kind: model.KindBindingList,
			ResourceTarget: &model.ResourceTarget{
				Group: "edge.forms.takoform.com", Kind: "VectorIndex",
				Contract: model.TargetContract{Interface: &model.InterfaceRefSource{Name: "edge.vector", Version: containerForwardCandidateInterfaceVersion}},
			},
			BindingType: "module-worker.edge-vector", Default: []any{},
			Doc: "Typed module-worker.edge-vector bindings to exact VectorIndex resources.",
		},
		model.Field{
			HCL: "container_http_bindings", Wire: "containerHttpBindings", Kind: model.KindBindingList,
			ResourceTarget: &model.ResourceTarget{
				Group: "edge.forms.takoform.com", Kind: "ContainerService",
				Contract: model.TargetContract{Interface: &model.InterfaceRefSource{Name: "container.http", Version: containerForwardCandidateInterfaceVersion}},
			},
			BindingType: "module-worker.container-http", Default: []any{},
			Doc: "Typed module-worker.container-http bindings to exact ContainerService resources.",
		},
	)
	worker.AcceptedBindings = append(worker.AcceptedBindings,
		model.BindingRefSource{Name: "module-worker.edge-vector", Version: "1.0.0"},
		model.BindingRefSource{Name: "module-worker.container-http", Version: containerForwardCandidateInterfaceVersion},
	)
	for index := range worker.AcceptedBindings {
		if worker.AcceptedBindings[index].Name == "module-worker.actor" {
			worker.AcceptedBindings[index].Version = "2.0.0"
		}
	}
	return worker
}

func containerCandidateService() model.Form {
	return model.Form{
		Family: model.Family{Group: "edge.forms.takoform.com"}, Kind: "ContainerService", Slug: "container-service",
		Role: model.RoleIdentity, RequiresHostAPI: "forms.takoform.com/v1", Title: "Container Service",
		DefinitionVersion: containerForwardCandidateFormVersion,
		Description:       containerCandidateDescription,
		ProvidedInterfaces: []model.InterfaceRefSource{{
			Name: "container.http", Version: containerForwardCandidateInterfaceVersion,
		}},
		Fields: []model.Field{
			{HCL: "image", Wire: "image", Kind: model.KindString, Doc: "Required OCI image reference pinned to one lowercase sha256 digest (name@sha256:<64 lowercase hex digits>). Repository components use the lowercase distribution/reference subset [a-z0-9]+ with [._], __, or hyphen separators; an optional lowercase DNS/IPv4/localhost registry prefix and port 1..65535 may appear only before a slash. Mutable tags, tag-only references, IPv6 literals, userinfo, registry credentials, and native image IDs are outside this first candidate; a host resolves the exact digest before starting the revision.", Required: true, Pattern: containerCandidateImagePattern, MaxLength: 512},
			{HCL: "port", Wire: "port", Kind: model.KindInteger, Doc: "Required application HTTP port in the inclusive range 1..65535. The host connects to the isolated process on this port; it is not a host listener or a public endpoint.", Required: true, Min: model.I64(1), Max: model.I64(65535)},
			{HCL: "health_path", Wire: "healthPath", Kind: model.KindString, Doc: "Required absolute HTTP path checked with GET before a generation is eligible to serve. It has one leading slash, no authority/URL form, fragment, backslash, whitespace, or control character; the host never treats it as a public endpoint.", Required: true, Pattern: containerCandidateHealthPathPattern, MaxLength: 1024},
			{HCL: "environment", Wire: "environment", Kind: model.KindObjectList, Doc: "Optional non-secret application environment entries. Each entry has a unique ASCII name matching ^[A-Za-z_][A-Za-z0-9_]{0,127}$ and a bounded value without NUL; omission means []. Names must be disjoint from requiredSensitiveVars before mutation.", Default: []any{}, MaxItems: 128,
				Fields: []model.Field{
					{HCL: "name", Wire: "name", Kind: model.KindString, Doc: "ASCII environment identifier; entries are unique by name.", Required: true, Pattern: `^[A-Za-z_][A-Za-z0-9_]{0,127}$`, MaxLength: 128},
					{HCL: "value", Wire: "value", Kind: model.KindString, Doc: "Non-secret environment value without NUL.", Required: true, Pattern: `^[^\x00]*$`, MaxLength: 8192},
				}},
			{HCL: "required_sensitive_vars", Wire: "requiredSensitiveVars", Kind: model.KindStringSet, Doc: "Optional unique set of ASCII environment names whose values the host must provide through a separate generation-bound sealed input. Only names are portable; values never appear in desired, observed, output, logs, or diagnostics. Omission means no required sensitive values, and names must be disjoint from environment.", Default: []any{}, MaxItems: 64, ItemPattern: `^[A-Za-z_][A-Za-z0-9_]{0,127}$`},
			{HCL: "workload_revision", Wire: "workloadRevision", Kind: model.KindString, Doc: "Required application-owned non-secret revision token. It is not a provider nonce, operation key, native identity, or secret-derived value. Changing it is the explicit portable signal for a new execution generation, including a secret-only rotation; an old revision is never mutated under the same serving generation.", Required: true, Pattern: `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`, MaxLength: 64},
			{HCL: "outbound_internet", Wire: "outboundInternet", Kind: model.KindBoolean, Doc: "Whether the isolated process may make outbound Internet and DNS requests. The default false denies egress; true never grants host networking, metadata/private sibling access, inbound exposure, or provider credentials, and a Host unable to enforce those boundaries refuses the resource.", Default: false},
		},
		StructuralConstraints: []model.Constraint{{Kind: model.ConstraintUniqueBy, List: "/environment", Member: "name"}},
	}
}

func containerCandidateVectorIndex() model.Form {
	return model.Form{
		Family: model.Family{Group: "edge.forms.takoform.com"}, Kind: "VectorIndex", Slug: "vector-index",
		Role: model.RoleIdentity, RequiresHostAPI: "forms.takoform.com/v1", Title: "Install-owned vector index",
		DefinitionVersion: containerForwardCandidateVectorVersion,
		Description:       vectorCandidateDescription,
		ProvidedInterfaces: []model.InterfaceRefSource{{
			Name: "edge.vector", Version: containerForwardCandidateInterfaceVersion,
		}},
		Fields: []model.Field{
			{HCL: "dimension", Wire: "dimension", Kind: model.KindInteger, Doc: "Required positive vector length. Every stored and query vector must have exactly this many components; each component is converted to IEEE 754 binary32 before storage and a host rejects wrong length rather than padding or truncating it.", Required: true, Immutable: true, Min: model.I64(1), Max: model.I64(1536)},
			{HCL: "filter_keys", Wire: "filterKeys", Kind: model.KindStringSet, Doc: "Optional immutable set of metadata keys callers may use in exact, type-sensitive equality filters. Omission means an empty set; a filter key not declared here is rejected. Observed keys use lexical ordering matching canonical set defaults. Keys are simple ASCII identifiers and at most eight may be declared.", Immutable: true, Default: []any{}, MaxItems: 8, ItemPattern: `^[A-Za-z][A-Za-z0-9_]{0,63}$`},
			{HCL: "metric", Wire: "metric", Kind: model.KindStringEnum, Doc: "Required immutable similarity metric. This candidate supports cosine similarity only; scores are higher-is-better and a host rejects another metric instead of silently substituting one.", Required: true, Immutable: true, Enum: []string{"cosine"}},
		},
	}
}
