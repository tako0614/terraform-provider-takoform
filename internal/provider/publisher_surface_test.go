package provider

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func TestPublisherProviderBinaryEmbedsOnlyPublisherArtifacts(t *testing.T) {
	source, err := fs.Sub(providerPublisherEmbeddedArtifacts, "artifacts/publisher")
	if err != nil {
		t.Fatalf("open publisher-selected embedded artifacts: %v", err)
	}
	var closure v3ArtifactClosure
	raw, err := fs.ReadFile(source, "closure.json")
	if err != nil {
		t.Fatalf("read publisher-selected embedded closure: %v", err)
	}
	if err := formpackage.DecodeStrictIJSON(raw, &closure); err != nil {
		t.Fatalf("decode publisher-selected embedded closure: %v", err)
	}
	if len(closure.Packages) != publisherProviderFormCount {
		t.Fatalf("publisher-selected embedded packages = %d, want %d", len(closure.Packages), publisherProviderFormCount)
	}
	if len(closure.Interfaces) != 8 || len(closure.Bindings) != 7 {
		t.Fatalf("publisher-selected embedded contracts = %d Interfaces/%d Bindings, want 8/7", len(closure.Interfaces), len(closure.Bindings))
	}
	for _, entry := range closure.Packages {
		if entry.FormRef.APIVersion != publisherProviderFamilyGroup {
			t.Errorf("publisher-selected binary embeds non-publisher-selected package %s for %s/%s", entry.Root, entry.FormRef.APIVersion, entry.FormRef.Kind)
		}
	}
	if err := fs.WalkDir(source, "packages", func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name != "packages" && !strings.HasPrefix(name, "packages/"+publisherProviderFamilyGroup) {
			t.Errorf("publisher-selected binary artifact closure contains non-publisher-selected package path %s", name)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk publisher-selected embedded packages: %v", err)
	}
}

func TestCurrentProviderRegistersOnlyPublisherEdgeForms(t *testing.T) {
	forms := currentPublisherProviderForms()
	if len(forms) != publisherProviderFormCount {
		t.Fatalf("publisher-selected provider Form count = %d, want %d", len(forms), publisherProviderFormCount)
	}
	for _, form := range forms {
		if form.Family.Group != publisherProviderFamilyGroup {
			t.Errorf("publisher-selected provider registered non-publisher-selected Form %s/%s", form.Family.APIVersion(), form.Kind)
		}
	}

	resources := newPublisherFormResources()
	if len(resources) != publisherProviderFormCount {
		t.Fatalf("publisher-selected provider resource count = %d, want %d", len(resources), publisherProviderFormCount)
	}
	got := make([]string, 0, len(resources))
	for _, factory := range resources {
		var response frameworkresource.MetadataResponse
		factory().Metadata(context.Background(), frameworkresource.MetadataRequest{ProviderTypeName: "takoform"}, &response)
		got = append(got, response.TypeName)
	}
	sort.Strings(got)
	want := []string{
		"takoform_actor_namespace",
		"takoform_at_least_once_queue",
		"takoform_durable_workflow",
		"takoform_edge_kv_namespace",
		"takoform_edge_object_bucket",
		"takoform_module_worker",
		"takoform_queue_consumer",
		"takoform_sqlite_database",
		"takoform_sqlite_migration_application",
		"takoform_sqlite_migration_set",
		"takoform_static_asset_bundle",
		"takoform_worker_bundle",
		"takoform_worker_cron_trigger",
		"takoform_worker_custom_domain",
		"takoform_worker_deployment",
		"takoform_worker_endpoint",
		"takoform_worker_version",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("publisher-selected provider resource set = %v, want exact publisher-selected set %v", got, want)
	}
	for _, withdrawn := range []string{
		"takoform_serverless_container_service",
		"takoform_function",
		"takoform_pull_queue",
		"takoform_message_schedule",
		"takoform_table",
		"takoform_topic",
		"takoform_dense_vector_index",
	} {
		if index := sort.SearchStrings(got, withdrawn); index < len(got) && got[index] == withdrawn {
			t.Errorf("withdrawn aggregate Form remains registered: %s", withdrawn)
		}
	}
}

func TestProvider3AggregateRemainsReadableHistory(t *testing.T) {
	if got := len(providerV3CurrentForms()); got != providerV3CurrentFormCount {
		t.Fatalf("retained Provider 3 Form history = %d, want %d", got, providerV3CurrentFormCount)
	}
	if got := len(v3ProviderResourceTypeNames()); got != providerV3CurrentFormCount {
		t.Fatalf("retained Provider 3 resource history = %d, want %d", got, providerV3CurrentFormCount)
	}
}

func TestNativeProviderCompositionExampleUsesIndependentProviders(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean("../../examples/native-provider-composition/main.tf"))
	if err != nil {
		t.Fatalf("read native provider composition example: %v", err)
	}
	text := string(raw)
	for _, required := range []string{
		`registry.terraform.io/tako0614/takoform`,
		`registry.terraform.io/hashicorp/aws`,
		`resource "aws_s3_bucket"`,
		`resource "takoform_edge_kv_namespace"`,
		`depends_on = [aws_s3_bucket.artifacts]`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("native provider composition example is missing %q", required)
		}
	}
	for _, forbidden := range []string{"takoform_s3", "takoform_aws", "provider_catalog", "install-options"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("native provider composition example adds a Takoform wrapper or catalog: %q", forbidden)
		}
	}
}
