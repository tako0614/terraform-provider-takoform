package containerformcatalog

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

func TestCatalogValidates(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogIdentityAndRoles(t *testing.T) {
	wantKinds := []string{"ContainerService", "ContainerRevision", "ContainerTraffic", "ContainerEndpoint", "ContainerCustomDomain"}
	if Family.APIVersion() != "container.forms.takoform.com" {
		t.Fatalf("family apiVersion = %q", Family.APIVersion())
	}
	if len(Forms) != len(wantKinds) {
		t.Fatalf("forms = %d, want %d", len(Forms), len(wantKinds))
	}
	wantRoles := []model.Role{model.RoleIdentity, model.RoleRevision, model.RoleDeployment, model.RoleAttachment, model.RoleAttachment}
	for index, form := range Forms {
		if form.Kind != wantKinds[index] || form.Role != wantRoles[index] {
			t.Fatalf("form[%d] = %s/%s, want %s/%s", index, form.Kind, form.Role, wantKinds[index], wantRoles[index])
		}
	}
}

func TestContainerRevisionFieldsAndConstraints(t *testing.T) {
	revision, ok := ByKind("ContainerRevision")
	if !ok {
		t.Fatal("ContainerRevision not found")
	}
	var names []string
	for _, field := range revision.Fields {
		names = append(names, field.Wire)
	}
	want := []string{"service", "image", "command", "args", "vars", "requiredSensitiveVars", "externalServices", "memoryMiB", "cpu", "concurrencyTarget", "minInstances", "maxInstances", "timeoutSeconds"}
	if !slices.Equal(names, want) {
		t.Fatalf("ContainerRevision fields = %v, want %v", names, want)
	}
	if target := revision.Fields[0].ResourceTarget; target == nil || target.Group != Family.APIVersion() || target.Kind != "ContainerService" || !target.Contract.ExactForm {
		t.Fatalf("ContainerRevision.service target = %#v, want exact same-family ResourceTarget", target)
	}
	if got := revision.StructuralConstraints; len(got) != 1 || got[0].Kind != model.ConstraintOrderedPair ||
		!slices.Equal(got[0].References, []string{"/minInstances", "/maxInstances"}) {
		t.Fatalf("ContainerRevision structural constraints = %#v, want minInstances <= maxInstances", got)
	}
	traffic, ok := ByKind("ContainerTraffic")
	if !ok {
		t.Fatal("ContainerTraffic not found")
	}
	constraints := traffic.Constraints()
	if len(constraints) != 3 {
		t.Fatalf("ContainerTraffic constraints = %#v, want exclusive, sum, sameResolvedTarget", constraints)
	}
	if constraints[0].Kind != model.ConstraintExclusive || constraints[0].Reference != "/service" {
		t.Fatalf("traffic exclusive constraint = %#v", constraints[0])
	}
	if constraints[1].Kind != model.ConstraintSum || constraints[1].List != "/revisions" || constraints[1].Member != "weight" || constraints[1].Total != 10000 {
		t.Fatalf("traffic sum constraint = %#v", constraints[1])
	}
	if constraints[2].Kind != model.ConstraintSameResolvedTarget || constraints[2].Anchor != "/service" || constraints[2].Members != "/revisions/*/containerRevision" || constraints[2].Through != "/service" {
		t.Fatalf("traffic same-target constraint = %#v", constraints[2])
	}
}

func TestContainerRevisionRejectsInvertedInstanceBounds(t *testing.T) {
	t.Parallel()
	revision, ok := ByKind("ContainerRevision")
	if !ok {
		t.Fatal("ContainerRevision not found")
	}
	desired := revision.CanonicalDesired()
	desired["minInstances"] = 21
	desired["maxInstances"] = 20
	if err := model.ValidateStructuralConstraintValues(revision.StructuralConstraints, desired); err == nil {
		t.Fatal("inverted minInstances/maxInstances pair was accepted")
	}
}

func TestRenderedExactIdentitiesAndDomainClaim(t *testing.T) {
	rendered, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != len(Forms) {
		t.Fatalf("rendered = %d, want %d", len(rendered), len(Forms))
	}
	for _, item := range rendered {
		if item.Definition.APIVersion != Family.APIVersion() || item.Definition.DefinitionVersion != definitionVersion {
			t.Fatalf("%s identity = %s@%s", item.Kind, item.Definition.APIVersion, item.Definition.DefinitionVersion)
		}
		if _, ok := item.Fixtures["desired.json"]; !ok {
			t.Fatalf("%s has no canonical desired fixture", item.Kind)
		}
	}
	seenDigests := map[string]string{}
	for _, item := range rendered {
		digest, err := catalogDefinitionDigest(item.DefinitionJSON)
		if err != nil {
			t.Fatal(err)
		}
		if previous, duplicate := seenDigests[digest]; duplicate {
			t.Fatalf("Forms %s and %s share digest %s", previous, item.Kind, digest)
		}
		seenDigests[digest] = item.Kind
	}
	domain, ok := ByKind("ContainerCustomDomain")
	if !ok {
		t.Fatal("ContainerCustomDomain not found")
	}
	var foundClaim bool
	for _, field := range domain.Fields {
		if field.Wire == "hostname" {
			foundClaim = field.Claimed && field.Immutable && field.Pattern == model.PatternHostname
		}
	}
	if !foundClaim {
		t.Fatal("custom-domain hostname is not an immutable canonical claimed field")
	}
	resolver := NewTargetResolver()
	refs, err := resolver.TargetFormRefs("ContainerService")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].APIVersion != Family.APIVersion() || refs[0].Kind != "ContainerService" || refs[0].DefinitionVersion != definitionVersion || len(refs[0].SchemaDigest) != len("sha256:")+64 {
		t.Fatalf("ContainerService exact ref = %#v", refs)
	}
}

func TestContainerRuntimeInterfaceIsExactAndResolverRejectsDrift(t *testing.T) {
	t.Parallel()
	definitions := InterfaceDefinitions()
	if err := ValidateInterfaceDefinitions(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Name != ContainerRuntimeInterfaceName || definitions[0].Version != "1.0.0" {
		t.Fatalf("runtime interface definitions = %#v", definitions)
	}
	rendered, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 || rendered[0].Name != ContainerRuntimeInterfaceName || rendered[0].Version != "1.0.0" {
		t.Fatalf("rendered runtime interfaces = %#v", rendered)
	}
	ref, err := InterfaceRefFor(ContainerRuntimeInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if ref.APIVersion != InterfaceAPIVersion || ref.Name != ContainerRuntimeInterfaceName || ref.Version != "1.0.0" || ref.SchemaDigest != rendered[0].SchemaDigest {
		t.Fatalf("runtime interface ref = %#v, rendered = %#v", ref, rendered[0])
	}
	identity, ok := ByKind("ContainerService")
	if !ok || len(identity.ProvidedInterfaces) != 1 || identity.ProvidedInterfaces[0] != (model.InterfaceRefSource{Name: ContainerRuntimeInterfaceName, Version: "1.0.0"}) {
		t.Fatalf("ContainerService provided interfaces = %#v", identity.ProvidedInterfaces)
	}
	forms, err := RenderForms()
	if err != nil {
		t.Fatal(err)
	}
	if len(forms[0].Definition.ProvidedInterfaces) != 1 || forms[0].Definition.ProvidedInterfaces[0] != ref {
		t.Fatalf("rendered ContainerService provided interfaces = %#v, want %#v", forms[0].Definition.ProvidedInterfaces, ref)
	}
	resolver := NewTargetResolver()
	target := model.ResourceTarget{
		Group: Family.APIVersion(), Kind: "ContainerService",
		Contract: model.TargetContract{Interface: &model.InterfaceRefSource{Name: ContainerRuntimeInterfaceName, Version: "1.0.0"}},
	}
	resolved, err := resolver.ResolveResourceTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.RequiredInterface == nil || resolved.RequiredInterface.SchemaDigest != ref.SchemaDigest {
		t.Fatalf("resolved runtime interface = %#v, want digest %s", resolved.RequiredInterface, ref.SchemaDigest)
	}
	for _, wrong := range []model.InterfaceRefSource{
		{Name: "container.other", Version: "1.0.0"},
		{Name: ContainerRuntimeInterfaceName, Version: "1.0.1"},
	} {
		wrong := wrong
		t.Run(wrong.Name+"@"+wrong.Version, func(t *testing.T) {
			_, err := resolver.ResolveResourceTarget(model.ResourceTarget{
				Group: Family.APIVersion(), Kind: "ContainerService",
				Contract: model.TargetContract{Interface: &wrong},
			})
			if err == nil {
				t.Fatalf("wrong runtime interface %s@%s unexpectedly resolved", wrong.Name, wrong.Version)
			}
		})
	}
	if _, err := resolver.RequiredInterface(ContainerRuntimeInterfaceName, "1.0.1"); err == nil {
		t.Fatal("wrong runtime Interface version unexpectedly resolved directly")
	}
}

func TestContainerRuntimeInterfaceSatisfiesNormativeSchema(t *testing.T) {
	t.Parallel()
	definitions := InterfaceDefinitions()
	if len(definitions) != 1 || len(definitions[0].Operations) != 1 || definitions[0].Operations[0].Name != ContainerRuntimeEntrypoint {
		t.Fatalf("container runtime operations = %#v, want one %q entrypoint", definitions, ContainerRuntimeEntrypoint)
	}
	endpoint, ok := ByKind("ContainerEndpoint")
	if !ok || len(endpoint.Fields) != 1 || endpoint.Fields[0].RequiredEntrypoint != ContainerRuntimeEntrypoint {
		t.Fatalf("ContainerEndpoint entrypoint = %#v, want %q", endpoint.Fields, ContainerRuntimeEntrypoint)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	normative, err := os.ReadFile(filepath.Join("..", "..", "spec", "schemas", "interface-definition-v1alpha1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	normativeValue, err := jsonschema.UnmarshalJSON(bytes.NewReader(normative))
	if err != nil {
		t.Fatal(err)
	}
	const normativeID = "https://forms.takoform.com/schemas/interfaces/v1alpha1/interface-definition.schema.json"
	if err := compiler.AddResource(normativeID, normativeValue); err != nil {
		t.Fatal(err)
	}
	normativeSchema, err := compiler.Compile(normativeID)
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := RenderInterfaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(rendered) != 1 {
		t.Fatalf("rendered runtime interfaces = %d, want 1", len(rendered))
	}
	definitionValue, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(rendered[0].DefinitionJSON)))
	if err != nil {
		t.Fatal(err)
	}
	if err := normativeSchema.Validate(definitionValue); err != nil {
		t.Fatalf("container.runtime definition violates normative Interface schema: %v", err)
	}
	ref, err := InterfaceRefFor(ContainerRuntimeInterfaceName, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := formpackage.DigestCanonicalJSON([]byte(rendered[0].DefinitionJSON))
	if err != nil {
		t.Fatal(err)
	}
	if digest != ref.SchemaDigest || digest != rendered[0].SchemaDigest {
		t.Fatalf("container.runtime digest = %s, ref = %s, rendered = %s", digest, ref.SchemaDigest, rendered[0].SchemaDigest)
	}
}

func catalogDefinitionDigest(definition string) (string, error) {
	return formpackage.DigestCanonicalJSON([]byte(definition))
}

func TestOptionalFieldsDeclareMeaning(t *testing.T) {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.Required && field.Default == nil && !field.AbsenceIsSemantic {
				t.Errorf("%s.%s is optional without Default or AbsenceIsSemantic", form.Kind, field.Wire)
			}
		}
	}
}
