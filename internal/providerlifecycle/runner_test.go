package providerlifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/formcatalog"
	"github.com/tako0614/terraform-provider-takoform/standardform"
)

func TestFormHostPreviewBindsReviewToCanonicalRequestedSpec(t *testing.T) {
	host := newFormHost()
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	formClient := client.New(server.URL, "", server.Client())
	ctx := context.Background()
	if _, err := formClient.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket declaration is missing")
	}
	form := candidateForms()[kind.Kind]
	desired := &client.Resource{
		APIVersion: client.APIVersion, Kind: kind.Kind, Form: &form,
		Metadata: client.Metadata{Name: kind.FixtureName(), Space: "prod"},
		Spec:     kind.CanonicalDesired(),
	}
	specRaw, err := json.Marshal(desired.Spec)
	if err != nil {
		t.Fatal(err)
	}
	wantDigest, err := formpackage.DigestCanonicalJSON(specRaw)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := formClient.PreviewResource(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Review.SpecDigest != wantDigest {
		t.Fatalf("preview specDigest = %q, want RFC 8785 requested spec digest %q", preview.Review.SpecDigest, wantDigest)
	}
}

func TestFormHostProjectsExactFormDescriptorsOnlyForReadyResourceLifecycle(t *testing.T) {
	host := newFormHost()
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	formClient := client.New(server.URL, "", server.Client())
	if _, err := formClient.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}

	kind, ok := currentformcatalog.ByKind("ObjectBucket")
	if !ok {
		t.Fatal("ObjectBucket declaration is missing")
	}
	form := candidateForms()[kind.Kind]
	selector := client.InterfaceSelector{
		Name: "object.storage", Version: "1",
		ResourceKind: kind.Kind, ResourceName: kind.FixtureName(),
	}
	if listed, err := formClient.ListInterfaces(context.Background(), "prod"); err != nil || len(listed) != 0 {
		t.Fatalf("interfaces before Resource create = %#v, %v", listed, err)
	}
	if _, err := formClient.GetInterface(context.Background(), "prod", selector); !errors.Is(err, client.ErrNotFound) {
		t.Fatalf("exact interface before Resource create error = %v, want not found", err)
	}

	created, err := formClient.PutResource(context.Background(), kind.Kind, kind.FixtureName(), &client.Resource{
		APIVersion: client.APIVersion, Kind: kind.Kind, Form: &form,
		Metadata: client.Metadata{Name: kind.FixtureName(), Space: "prod"},
		Spec:     kind.CanonicalDesired(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !resourceIsReady(*created) {
		t.Fatalf("created Resource is not Ready: %#v", created.Status)
	}
	listed, err := formClient.ListInterfaces(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("Ready ObjectBucket interfaces = %#v, want one exact descriptor", listed)
	}
	wantDocument := map[string]any{"operations": []any{"delete", "get", "list", "put"}}
	if listed[0].Name != selector.Name || listed[0].Version != selector.Version ||
		listed[0].Resource.Kind != selector.ResourceKind || listed[0].Resource.Name != selector.ResourceName ||
		!reflect.DeepEqual(listed[0].Document, wantDocument) || listed[0].Form == nil || *listed[0].Form != form {
		t.Fatalf("materialized Interface does not match exact Form descriptor: %#v", listed[0])
	}
	if listed[0].Name == "s3.api" || listed[0].Version == "2025-11-25" {
		t.Fatalf("undeclared legacy Interface leaked from test host: %#v", listed[0])
	}
	exact, err := formClient.GetInterface(context.Background(), "prod", selector)
	if err != nil || !reflect.DeepEqual(exact, listed[0]) {
		t.Fatalf("exact Interface read = %#v, %v; list item = %#v", exact, err, listed[0])
	}

	if err := host.setResourceReady(kind.Kind, kind.FixtureName(), false); err != nil {
		t.Fatal(err)
	}
	if listed, err := formClient.ListInterfaces(context.Background(), "prod"); err != nil || len(listed) != 0 {
		t.Fatalf("non-Ready Resource interfaces = %#v, %v", listed, err)
	}
	if err := host.setResourceReady(kind.Kind, kind.FixtureName(), true); err != nil {
		t.Fatal(err)
	}

	if err := formClient.DeleteResource(context.Background(), kind.Kind, kind.FixtureName(), "prod", client.MutationFence{
		ResourceVersion: created.Metadata.ResourceVersion,
		Form:            form,
	}); err != nil {
		t.Fatal(err)
	}
	if listed, err := formClient.ListInterfaces(context.Background(), "prod"); err != nil || len(listed) != 0 {
		t.Fatalf("interfaces after Resource delete = %#v, %v", listed, err)
	}
	if _, err := formClient.GetInterface(context.Background(), "prod", selector); !errors.Is(err, client.ErrNotFound) {
		t.Fatalf("exact interface after Resource delete error = %v, want not found", err)
	}
}

func TestFormHostProjectsEveryCurrentDescriptorThroughReadyLifecycle(t *testing.T) {
	host := newFormHost()
	server := httptest.NewServer(host)
	t.Cleanup(server.Close)
	formClient := client.New(server.URL, "", server.Client())
	ctx := context.Background()
	if _, err := formClient.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	if err := verifyNoMaterializedInterfaces(ctx, formClient); err != nil {
		t.Fatal(err)
	}
	forms := candidateForms()
	created := make(map[string]*client.Resource, len(resourceCases))
	for _, item := range resourceCases {
		kind, ok := currentformcatalog.ByKind(item.Kind)
		if !ok {
			t.Fatalf("resource case names undeclared Form %s", item.Kind)
		}
		form := forms[item.Kind]
		resource, err := formClient.PutResource(ctx, item.Kind, item.Name, &client.Resource{
			APIVersion: client.APIVersion, Kind: item.Kind, Form: &form,
			Metadata: client.Metadata{Name: item.Name, Space: "prod"},
			Spec:     kind.CanonicalDesired(),
		})
		if err != nil {
			t.Fatalf("create %s: %v", item.Kind, err)
		}
		created[item.Kind] = resource
	}
	if err := verifyReadyMaterializedInterfaces(ctx, formClient, host); err != nil {
		t.Fatal(err)
	}
	for _, item := range resourceCases {
		form := forms[item.Kind]
		if err := formClient.DeleteResource(ctx, item.Kind, item.Name, "prod", client.MutationFence{
			ResourceVersion: created[item.Kind].Metadata.ResourceVersion,
			Form:            form,
		}); err != nil {
			t.Fatalf("delete %s: %v", item.Kind, err)
		}
	}
	if err := verifyNoMaterializedInterfaces(ctx, formClient); err != nil {
		t.Fatal(err)
	}
}

func TestOpenTofuValidateAcceptsYurucommuComputedConfigurationValues(t *testing.T) {
	root, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	version, err := loadProviderVersion(root)
	if err != nil {
		t.Fatal(err)
	}
	temp := t.TempDir()
	binDir := filepath.Join(temp, "bin")
	stackDir := filepath.Join(temp, "stack")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	providerBinary := filepath.Join(binDir, "terraform-provider-takoform")
	ctx := context.Background()
	if output, err := runCommand(
		ctx,
		root,
		nil,
		"go",
		"build",
		"-trimpath",
		"-buildvcs=false",
		"-ldflags",
		"-buildid= -X main.version="+version,
		"-o",
		providerBinary,
		".",
	); err != nil {
		t.Fatalf("build provider binary: %v\n%s", err, output)
	}
	cliConfig := filepath.Join(temp, "terraformrc")
	if err := os.WriteFile(cliConfig, []byte(fmt.Sprintf(`provider_installation {
  dev_overrides {
    %q = %q
  }
  direct {}
}
`, OpenTofuProviderAddress, binDir)), 0o600); err != nil {
		t.Fatal(err)
	}
	// This is the configuration shape used by Yurucommu's portable module.
	// Module variables remain unknown during `tofu validate`, so values derived
	// from project_name must remain framework String values until planning.
	stack := fmt.Sprintf(`terraform {
  required_providers {
    takoform = {
      source = %q
    }
  }
}

variable "project_name" {
  type    = string
  default = "yurucommu"
}

locals {
  prefix = var.project_name
}

resource "takoform_edge_worker" "worker" {
  name                = local.prefix
  artifact_url        = "https://example.test/yurucommu-worker.js"
  artifact_sha256     = "sha256:683c5ed5bc5f537087b703bf24ad3b306508dd3778918d0c31eb4561777fbe13"
  artifact_media_type = "text/javascript"
  entrypoint          = "worker.js"
  runtime             = "javascript"

  configuration = {
    DELIVERY_QUEUE_NAME = "${local.prefix}-delivery"
    DELIVERY_DLQ_NAME   = "${local.prefix}-delivery-dlq"
  }
}
`, OpenTofuProviderAddress)
	if err := os.WriteFile(filepath.Join(stackDir, "main.tf"), []byte(stack), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := runCommand(
		ctx,
		root,
		terraformRunnerEnvironment(cliConfig),
		"tofu",
		"-chdir="+stackDir,
		"validate",
		"-no-color",
	)
	if err != nil {
		t.Fatalf("tofu validate rejected Yurucommu computed configuration values: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Success! The configuration is valid") {
		t.Fatalf("tofu validate did not report success:\n%s", output)
	}
}

func TestStandardFixtureCasesRequireExactExecutedFormIdentity(t *testing.T) {
	forms := candidateForms()
	cases := make([]StandardFixtureCase, 0, len(resourceCases))
	for _, item := range resourceCases {
		form := forms[item.Kind]
		cases = append(cases, StandardFixtureCase{
			Kind: item.Kind,
			Identity: standardform.InstalledFormReference{
				FormRef: formpackage.FormRef{
					APIVersion: form.FormRef.APIVersion, Kind: form.FormRef.Kind,
					DefinitionVersion: form.FormRef.DefinitionVersion, SchemaDigest: form.FormRef.SchemaDigest,
				},
				PackageDigest: form.PackageDigest,
			},
			PositiveName: "canonical", Positive: map[string]any{"name": item.Name},
			Negatives: []StandardNegativeFixture{{
				Name: "reject-name", Stage: "desired", Input: map[string]any{"name": item.Name},
			}},
		})
	}
	if _, err := validateAndOrderStandardFixtureCases(cases); err != nil {
		t.Fatalf("exact current candidate cases: %v", err)
	}
	forged := append([]StandardFixtureCase(nil), cases...)
	forged[0].Identity.FormRef.Kind = resourceCases[1].Kind
	if _, err := validateAndOrderStandardFixtureCases(forged); err == nil || !strings.Contains(err.Error(), "incomplete or unknown") {
		t.Fatalf("mislabeled fixture identity was accepted: %v", err)
	}
}

func TestStandardFixtureDiagnosticsCoverEveryRequiredDesiredOmission(t *testing.T) {
	t.Parallel()

	for _, kind := range currentformcatalog.Kinds {
		kind := kind
		t.Run(kind.Kind, func(t *testing.T) {
			t.Parallel()
			cases, err := kind.NegativeCases()
			if err != nil {
				t.Fatal(err)
			}
			for _, negative := range cases {
				if !strings.HasPrefix(negative.Name, "missing-") {
					continue
				}
				field, detail, ok := standardNegativeDiagnostic(kind.Kind, "reject-"+negative.Name, negative.Desired)
				if !ok || field == "" || detail != "required" {
					t.Errorf("%s diagnostic = %q/%q/%t", negative.Name, field, detail, ok)
				}
			}
		})
	}
}

func TestStandardFixtureDiagnosticsCoverCredentialFreeArtifactURLs(t *testing.T) {
	t.Parallel()

	wantDetail := formcatalog.GrammarCredentialFreeHTTPSURL.Message("artifact_url")
	for _, kind := range currentformcatalog.Kinds {
		if !kind.Artifact {
			continue
		}
		for _, suffix := range []string{"userinfo", "query", "fragment"} {
			fixtureName := "reject-artifact-url-" + suffix
			field, detail, ok := standardNegativeDiagnostic(
				kind.Kind,
				fixtureName,
				kind.CanonicalDesired(),
			)
			if !ok || field != "artifact_url" || detail != wantDetail {
				t.Errorf(
					"%s %s diagnostic = %q/%q/%t, want artifact_url/%q/true",
					kind.Kind,
					fixtureName,
					field,
					detail,
					ok,
					wantDetail,
				)
			}
		}
	}
}

func TestLoadCLIMatrixPinsOneCanonicalProviderAddress(t *testing.T) {
	root, err := RepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	requirements, descriptorDigest, err := LoadCLIMatrix(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 2 || !validDigest(descriptorDigest) {
		t.Fatalf("unexpected matrix identity: %#v %q", requirements, descriptorDigest)
	}
	seen := map[string]CLIRequirement{}
	for _, requirement := range requirements {
		seen[requirement.Product] = requirement
	}
	if seen["OpenTofu"].Version != "1.12.3" || seen["OpenTofu"].ProviderAddress != OpenTofuProviderAddress {
		t.Fatalf("unexpected OpenTofu matrix entry: %#v", seen["OpenTofu"])
	}
	if seen["Terraform"].Version != "1.15.8" || seen["Terraform"].ProviderAddress != TerraformProviderAddress {
		t.Fatalf("unexpected Terraform matrix entry: %#v", seen["Terraform"])
	}
	if CanonicalProviderAddress != TerraformProviderAddress || OpenTofuProviderAddress != CanonicalProviderAddress {
		t.Fatalf("provider distribution identities differ: canonical=%q opentofu=%q terraform=%q", CanonicalProviderAddress, OpenTofuProviderAddress, TerraformProviderAddress)
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"one provider source and state identity",
		"pins the published provider",
		"canonical Terraform Registry listing",
		"through CLI",
		"development overrides",
		"release-descriptor metadata",
	} {
		if !strings.Contains(string(readme), required) {
			t.Fatalf("provider distribution guidance lacks %q", required)
		}
	}
}

func TestStackConfigUsesExactCLIProviderAddress(t *testing.T) {
	openTofu := stackConfig("https://forms.example.test", OpenTofuProviderAddress, "1.0.0", 1)
	if !strings.Contains(openTofu, `source = "`+CanonicalProviderAddress+`"`) || !strings.Contains(openTofu, `version = "1.0.0"`) {
		t.Fatalf("OpenTofu config did not retain its exact FQN:\n%s", openTofu)
	}
	terra := stackConfig("https://forms.example.test", TerraformProviderAddress, "1.0.0", 1)
	if !strings.Contains(terra, `source = "`+CanonicalProviderAddress+`"`) {
		t.Fatalf("Terraform config did not retain its exact FQN:\n%s", terra)
	}
}

func TestFindInstalledProviderBinaryRequiresOneExecutableRegularFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	providerDir := filepath.Join(root, ".terraform", "providers", "registry.example.test", "tako0614", "takoform", "1.0.0", "linux_amd64")
	if err := os.MkdirAll(providerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(providerDir, "terraform-provider-takoform_v1.0.0")
	if err := os.WriteFile(binary, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findInstalledProviderBinary(root, "1.0.0")
	if err != nil || got != binary {
		t.Fatalf("installed binary = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(providerDir, "terraform-provider-takoform_duplicate"), []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := findInstalledProviderBinary(root, "1.0.0"); err == nil || !strings.Contains(err.Error(), "2 provider binaries") {
		t.Fatalf("duplicate binary error = %v", err)
	}
}

func TestTerraformRunnerEnvironmentRemovesProviderAndCLITaint(t *testing.T) {
	for key, value := range map[string]string{
		"TF_REATTACH_PROVIDERS": "forged-provider", "TF_CLI_ARGS_apply": "-target=forged",
		"TF_DATA_DIR": "/tmp/forged-data", "TOFU_CLI_CONFIG_FILE": "/tmp/forged-tofurc",
		"TAKOFORM_ENDPOINT": "https://forged.example.test", "TAKOFORM_TOKEN": "secret", "CHECKPOINT_DISABLE": "0",
	} {
		t.Setenv(key, value)
	}
	environment := terraformRunnerEnvironment("/tmp/exact-terraformrc")
	values := map[string]string{}
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[key] = value
		}
	}
	for _, forbidden := range []string{"TF_REATTACH_PROVIDERS", "TF_CLI_ARGS_apply", "TF_DATA_DIR", "TOFU_CLI_CONFIG_FILE", "TAKOFORM_ENDPOINT", "TAKOFORM_TOKEN"} {
		if _, ok := values[forbidden]; ok {
			t.Fatalf("sanitized provider runner retained %s", forbidden)
		}
	}
	if values["TF_CLI_CONFIG_FILE"] != "/tmp/exact-terraformrc" || values["TF_IN_AUTOMATION"] != "1" || values["CHECKPOINT_DISABLE"] != "1" {
		t.Fatalf("sanitized provider runner overrides = %#v", values)
	}
	for _, entry := range sanitizedTerraformBaseEnvironment() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "TF_") || strings.HasPrefix(key, "TOFU_") || strings.HasPrefix(key, "TAKOFORM_") || key == "CHECKPOINT_DISABLE" {
			t.Fatalf("sanitized CLI identity environment retained %s", key)
		}
	}
}

func TestValidateMatrixRejectsNonCanonicalAddressAndEvidenceDrift(t *testing.T) {
	requirements := []CLIRequirement{
		{Product: "OpenTofu", Version: "1.12.1", ProviderAddress: OpenTofuProviderAddress},
		{Product: "Terraform", Version: "1.15.8", ProviderAddress: TerraformProviderAddress},
	}
	openTofu := completeReport("OpenTofu", "1.12.1", OpenTofuProviderAddress)
	terra := completeReport("Terraform", "1.15.8", TerraformProviderAddress)
	matrix := MatrixReport{
		Format: MatrixReportFormat, Classification: "supported-cli-fqn-candidate-matrix", PublicationReady: false,
		InstallationSource:      LocalDevOverride,
		ReleaseDescriptorSHA256: "sha256:" + strings.Repeat("a", 64),
		CandidateSetSHA256:      candidateSetSHA256(), ProviderSchemaSHA256: "sha256:" + strings.Repeat("b", 64),
		Reports: []Report{openTofu, terra},
	}
	if err := ValidateMatrix(matrix, requirements); err != nil {
		t.Fatalf("valid matrix: %v", err)
	}
	if err := ValidateRegistryMatrix(matrix, requirements); err == nil {
		t.Fatal("Registry matrix accepted local dev-override evidence")
	}
	registry := matrix
	registry.InstallationSource = DirectRegistryInstall
	registry.Reports = append([]Report(nil), matrix.Reports...)
	for index := range registry.Reports {
		registry.Reports[index].InstallationSource = DirectRegistryInstall
	}
	if err := ValidateRegistryMatrix(registry, requirements); err != nil {
		t.Fatalf("valid direct Registry matrix: %v", err)
	}

	nonCanonical := matrix
	nonCanonical.Reports = append([]Report(nil), matrix.Reports...)
	nonCanonical.Reports[0].CLI.ProviderAddress = "registry.opentofu.org/tako0614/takoform"
	if err := ValidateMatrix(nonCanonical, requirements); err == nil {
		t.Fatal("matrix accepted a non-canonical OpenTofu provider address")
	}

	drifted := matrix
	drifted.Reports = append([]Report(nil), matrix.Reports...)
	drifted.Reports[1].Resources = append([]ResourceEvidence(nil), matrix.Reports[1].Resources...)
	drifted.Reports[1].Resources[0].Checks.Delete = false
	if err := ValidateMatrix(drifted, requirements); err == nil {
		t.Fatal("matrix accepted divergent lifecycle evidence")
	}

	missingInterfaceCheck := matrix
	missingInterfaceCheck.Reports = append([]Report(nil), matrix.Reports...)
	missingInterfaceCheck.Reports[1].InterfaceChecks.RequiredReadiness = false
	if err := ValidateMatrix(missingInterfaceCheck, requirements); err == nil {
		t.Fatal("matrix accepted incomplete or divergent Interface lifecycle evidence")
	}

	duplicateResource := registry
	duplicateResource.Reports = append([]Report(nil), registry.Reports...)
	duplicateResource.Reports[0].Resources = append([]ResourceEvidence(nil), registry.Reports[0].Resources...)
	duplicateResource.Reports[0].Resources[1] = duplicateResource.Reports[0].Resources[0]
	if err := ValidateRegistryMatrix(duplicateResource, requirements); err == nil {
		t.Fatal("Registry matrix accepted a duplicated resource identity")
	}

	unknownNegative := registry
	unknownNegative.Reports = append([]Report(nil), registry.Reports...)
	unknownNegative.Reports[0].NegativeChecks = append([]NegativeEvidence(nil), registry.Reports[0].NegativeChecks...)
	unknownNegative.Reports[0].NegativeChecks[0].Name = "unreviewed-negative"
	if err := ValidateRegistryMatrix(unknownNegative, requirements); err == nil {
		t.Fatal("Registry matrix accepted an unreviewed negative fixture")
	}
}

func completeReport(product, version, address string) Report {
	checks := CheckEvidence{Create: true, Read: true, Update: true, Observe: true, Refresh: true, NativeImport: true, CLIImport: true, Delete: true, DriftState: true, NameReplace: true}
	resources := make([]ResourceEvidence, 0, len(resourceCases))
	immutable := make([]ImmutableReplaceEvidence, 0, len(resourceCases)+len(declaredImmutableFieldPointers()))
	for _, item := range resourceCases {
		resources = append(resources, ResourceEvidence{Kind: item.Kind, ResourceType: item.ResourceType, Checks: checks})
		immutable = append(immutable, ImmutableReplaceEvidence{Kind: item.Kind, Field: "/name", Passed: true})
	}
	for _, pointer := range declaredImmutableFieldPointers() {
		kind, field, _ := strings.Cut(pointer, "/")
		immutable = append(immutable, ImmutableReplaceEvidence{Kind: kind, Field: "/" + field, Passed: true})
	}
	return Report{
		Format: ReportFormat, Classification: "generic-lifecycle-candidate", PublicationReady: false,
		BindingStatus: "exact-structural-candidate-set", RunnerSubject: RunnerSubject,
		Protocol: providerProtocol, InstallationSource: LocalDevOverride,
		CandidateSetSHA256: candidateSetSHA256(), ProviderSchemaSHA256: "sha256:" + strings.Repeat("b", 64),
		ProviderBinary: ProviderBinaryIdentity{Version: "0.1.0-rc.2", SHA256: "sha256:" + strings.Repeat("d", 64)},
		CLI:            CLIIdentity{Product: product, Version: version, ProviderAddress: address, ExecutableName: strings.ToLower(product), ExecutableSHA256: "sha256:" + strings.Repeat("c", 64)},
		Resources:      resources,
		InterfaceChecks: InterfaceCheckEvidence{
			AbsentBeforeCreate: true, DescriptorDerivedList: true, ExactGet: true,
			RequiredReadiness: true, AbsentAfterDelete: true,
		},
		NegativeChecks: []NegativeEvidence{
			{Name: "response-name-substitution-rejected", Kind: resourceCases[0].Kind, Fixture: "fixture", Passed: true},
			{Name: "response-package-digest-substitution-rejected", Kind: resourceCases[1].Kind, Fixture: "fixture", Passed: true},
		},
		ImmutableReplace: immutable,
	}
}
