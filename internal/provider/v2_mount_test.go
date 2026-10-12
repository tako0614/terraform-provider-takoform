package provider

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
)

func TestV2ProviderRegistersOnlyExactPublishedEdgeForms(t *testing.T) {
	got := make([]string, 0)
	for _, factory := range New("5.0.0")().Resources(context.Background()) {
		var metadata resource.MetadataResponse
		factory().Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "takoform"}, &metadata)
		got = append(got, metadata.TypeName)
	}
	sort.Strings(got)
	want := []string{
		"takoform_at_least_once_queue", "takoform_edge_kv_namespace", "takoform_edge_object_bucket",
		"takoform_module_worker", "takoform_queue_consumer", "takoform_sqlite_database",
		"takoform_sqlite_migration_application", "takoform_sqlite_migration_set", "takoform_worker_bundle",
		"takoform_worker_cron_trigger", "takoform_worker_deployment", "takoform_worker_endpoint", "takoform_worker_version",
	}
	if len(got) != len(want) {
		t.Fatalf("registered %d resources, want %d: %v", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("resource %d = %q, want %q", i, got[i], want[i])
		}
	}
}
