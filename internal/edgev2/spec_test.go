package edgev2

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestExactURLRosterAndResourceLookup(t *testing.T) {
	if got := len(URLs()); got != 13 {
		t.Fatalf("URLs() = %d, want 13", got)
	}
	for _, tc := range []struct{ name, want string }{
		{"takoform_module_worker", ModuleWorkerURL}, {"takoform_worker_bundle", WorkerBundleURL},
		{"takoform_worker_version", WorkerVersionURL}, {"takoform_worker_deployment", WorkerDeploymentURL},
		{"takoform_worker_endpoint", WorkerEndpointURL}, {"takoform_worker_cron_trigger", WorkerCronTriggerURL},
		{"takoform_edge_kv_namespace", EdgeKVNamespaceURL}, {"takoform_edge_object_bucket", ObjectBucketURL},
		{"takoform_sqlite_database", SQLiteDatabaseURL}, {"takoform_sqlite_migration_set", SQLiteMigrationSetURL},
		{"takoform_sqlite_migration_application", SQLiteMigrationApplicationURL}, {"takoform_at_least_once_queue", AtLeastOnceQueueURL},
		{"takoform_queue_consumer", QueueConsumerURL},
	} {
		got, ok := URLForResource(tc.name)
		if !ok || got != tc.want {
			t.Errorf("URLForResource(%s)=(%q,%v), want %q", tc.name, got, ok, tc.want)
		}
	}
	if _, ok := URLForResource("takoform_v1_module_worker"); ok {
		t.Fatal("accepted a non-current resource identity")
	}
}

func TestEveryFormRejectsUnknownTopLevelFields(t *testing.T) {
	for _, formURL := range URLs() {
		t.Run(formURL, func(t *testing.T) {
			if _, err := Decode(formURL, []byte(`{"unexpected":true}`)); err == nil || !strings.Contains(err.Error(), `unknown field "unexpected"`) {
				t.Fatalf("Decode accepted an unknown field: %v", err)
			}
		})
	}
}

func TestDecodeAndEncodeTypedSpecsForAllForms(t *testing.T) {
	cases := []struct {
		url, body string
		typ       any
	}{
		{ModuleWorkerURL, `{}`, &EmptySpec{}}, {WorkerBundleURL, `{"artifact":{"url":"https://artifacts.example/build/manifest.json","sha256":"` + strings.Repeat("a", 64) + `"}}`, &ArtifactSpec{}},
		{WorkerVersionURL, `{"worker":{"resourceUid":"worker"},"bundle":{"resourceUid":"bundle"},"handlers":["fetch"]}`, &WorkerVersionSpec{}},
		{WorkerDeploymentURL, `{"worker":{"resourceUid":"worker"},"versions":[{"workerVersion":{"resourceUid":"version"},"weight":10000}]}`, &WorkerDeploymentSpec{}},
		{WorkerEndpointURL, `{"worker":{"resourceUid":"worker"}}`, &WorkerEndpointSpec{}},
		{WorkerCronTriggerURL, `{"worker":{"resourceUid":"worker"},"cron":"*/5 * * * *"}`, &WorkerCronTriggerSpec{}},
		{EdgeKVNamespaceURL, `{}`, &EmptySpec{}}, {ObjectBucketURL, `{}`, &EmptySpec{}}, {SQLiteDatabaseURL, `{}`, &EmptySpec{}},
		{SQLiteMigrationSetURL, `{"artifact":{"url":"https://artifacts.example/migrations/manifest.json","sha256":"` + strings.Repeat("b", 64) + `"}}`, &ArtifactSpec{}},
		{SQLiteMigrationApplicationURL, `{"database":{"resourceUid":"db"},"migrationSet":{"resourceUid":"set"}}`, &MigrationApplicationSpec{}},
		{AtLeastOnceQueueURL, `{"messageRetentionSeconds":60}`, &QueueSpec{}},
		{QueueConsumerURL, `{"queue":{"resourceUid":"queue"},"worker":{"resourceUid":"worker"},"maxBatchSize":1,"maxBatchTimeoutSeconds":0,"maxConcurrency":1,"maxRetries":0,"retryDelaySeconds":0}`, &QueueConsumerSpec{}},
	}
	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			spec, e := Decode(tc.url, []byte(tc.body))
			if e != nil {
				t.Fatal(e)
			}
			if _, ok := spec.(interface{}); !ok {
				t.Fatal("not typed")
			}
			if got := fmtType(spec); got != fmtType(tc.typ) {
				t.Fatalf("type=%s want %s", got, fmtType(tc.typ))
			}
			encoded, e := Encode(tc.url, spec)
			if e != nil {
				t.Fatal(e)
			}
			var a, b any
			if e = json.Unmarshal(encoded, &a); e != nil {
				t.Fatal(e)
			}
			if e = json.Unmarshal([]byte(tc.body), &b); e != nil {
				t.Fatal(e)
			}
			if !jsonEqual(a, b) {
				t.Fatalf("round trip changed spec: %s", encoded)
			}
		})
	}
}

func TestClosedShapesBoundsReferencesAndArtifactInputs(t *testing.T) {
	bad := []struct{ name, url, body, contains string }{
		{"unknown field", ModuleWorkerURL, `{"name":"x"}`, "unknown field"},
		{"nested null ref", WorkerEndpointURL, `{"worker":null}`, "object"},
		{"extra ref field", WorkerEndpointURL, `{"worker":{"resourceUid":"u","name":"x"}}`, "unknown field"},
		{"empty uid", SQLiteMigrationApplicationURL, `{"database":{"resourceUid":""},"migrationSet":{"resourceUid":"s"}}`, "non-empty"},
		{"retention below minimum", AtLeastOnceQueueURL, `{"messageRetentionSeconds":59}`, "60..1209600"},
		{"delay over maximum", AtLeastOnceQueueURL, `{"deliveryDelaySeconds":43201,"messageRetentionSeconds":60}`, "0..43200"},
		{"decimal integer", QueueConsumerURL, `{"queue":{"resourceUid":"q"},"worker":{"resourceUid":"w"},"maxBatchSize":1.5,"maxBatchTimeoutSeconds":0,"maxConcurrency":1,"maxRetries":0,"retryDelaySeconds":0}`, "integer"},
		{"bad artifact url", WorkerBundleURL, `{"artifact":{"url":"https://user:pass@artifacts.example/a?token=x","sha256":"` + strings.Repeat("a", 64) + `"}}`, `absolute HTTPS`},
		{"bad digest", SQLiteMigrationSetURL, `{"artifact":{"url":"https://artifacts.example/a","sha256":"ABC"}}`, `lowercase`},
		{"migration manifest url query", SQLiteMigrationSetURL, `{"artifact":{"url":"https://artifacts.example/manifest?sig=secret","sha256":"` + strings.Repeat("a", 64) + `"}}`, `absolute HTTPS`},
		{"private value is not public", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":["fetch"],"privateInputs":{"TOKEN":"secret"}}`, "unknown field"},
		{"worker binding null nested ref", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"kvBindings":[{"name":"KV","resource":null}]}`, "object"},
		{"duplicate env name", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"vars":{"KV":"public"},"kvBindings":[{"name":"KV","resource":{"resourceUid":"r"}}]}`, "duplicate environment"},
		{"static plus handler", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"handlers":["fetch"],"assets":{"bundle":{"resourceUid":"a"},"runWorkerFirst":false,"notFoundHandling":"none"}}`, "empty handlers"},
		{"weight sum", WorkerDeploymentURL, `{"worker":{"resourceUid":"w"},"versions":[{"workerVersion":{"resourceUid":"v"},"weight":9999}]}`, "sum to exactly 10000"},
		{"bad cron", WorkerCronTriggerURL, `{"worker":{"resourceUid":"w"},"cron":"61 * * * *"}`, "out of range"},
		{"wrong exact URL", strings.Replace(ModuleWorkerURL, "0.3.0", "0.3.1", 1), `{}`, "unsupported exact Form URL"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			_, e := Decode(tc.url, []byte(tc.body))
			if e == nil || !strings.Contains(e.Error(), tc.contains) {
				t.Fatalf("Decode error=%v, want substring %q", e, tc.contains)
			}
		})
	}
}

func fmtType(v any) string { return fmt.Sprintf("%T", v) }
func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
