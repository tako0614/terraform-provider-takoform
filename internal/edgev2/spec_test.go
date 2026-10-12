package edgev2

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestExactURLRosterAndKindLookup(t *testing.T) {
	if got := len(URLs()); got != 13 {
		t.Fatalf("URLs() = %d, want 13", got)
	}
	for _, tc := range []struct{ name, want string }{
		{"ModuleWorker", ModuleWorkerURL}, {"WorkerBundle", WorkerBundleURL},
		{"WorkerVersion", WorkerVersionURL}, {"WorkerDeployment", WorkerDeploymentURL},
		{"WorkerEndpoint", WorkerEndpointURL}, {"WorkerCronTrigger", WorkerCronTriggerURL},
		{"EdgeKVNamespace", EdgeKVNamespaceURL}, {"ObjectBucket", ObjectBucketURL},
		{"SQLiteDatabase", SQLiteDatabaseURL}, {"SQLiteMigrationSet", SQLiteMigrationSetURL},
		{"SQLiteMigrationApplication", SQLiteMigrationApplicationURL}, {"AtLeastOnceQueue", AtLeastOnceQueueURL},
		{"QueueConsumer", QueueConsumerURL},
	} {
		got, ok := URLForKind(tc.name)
		if !ok || got != tc.want {
			t.Errorf("URLForKind(%s)=(%q,%v), want %q", tc.name, got, ok, tc.want)
		}
		kind, ok := KindForURL(tc.want)
		if !ok || kind != tc.name {
			t.Errorf("KindForURL(%s)=(%q,%v), want %q", tc.want, kind, ok, tc.name)
		}
	}
	if _, ok := URLForKind("takoform_module_worker"); ok {
		t.Fatal("accepted a Terraform resource type as Form kind")
	}
	if _, ok := KindForURL(strings.Replace(ModuleWorkerURL, "0.3.0", "0.3.1", 1)); ok {
		t.Fatal("accepted non-exact URL")
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

func TestDecodeRejectsAmbiguousOrInvalidJSON(t *testing.T) {
	inputs := [][]byte{
		[]byte(`{"worker":{"resourceUid":"first","resourceUid":"second"}}`),
		[]byte(`{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"vars":{"x":"a","x":"b"}}`),
		[]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'},
		[]byte(`{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"vars":{"x":"\ud800"}}`),
	}
	for i, input := range inputs {
		if _, err := Decode(WorkerVersionURL, input); err == nil {
			t.Errorf("case %d accepted ambiguous or invalid JSON %q", i, input)
		}
	}
}

func TestWorkerVersionNumberPrecisionSurvivesDecodeEncode(t *testing.T) {
	const number = "0.1234567890123456789"
	input := []byte(`{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"vars":{"ratio":` + number + `}}`)
	spec, err := Decode(WorkerVersionURL, input)
	if err != nil {
		t.Fatal(err)
	}
	version := spec.(*WorkerVersionSpec)
	if got, ok := version.Vars["ratio"].(json.Number); !ok || got.String() != number {
		t.Fatalf("decoded ratio = %#v, want json.Number(%s)", version.Vars["ratio"], number)
	}
	encoded, err := Encode(WorkerVersionURL, version)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"ratio":`+number) {
		t.Fatalf("encoded JSON lost numeric precision: %s", encoded)
	}
}

func TestEncodeRejectsInvalidUnicode(t *testing.T) {
	spec := &WorkerEndpointSpec{Worker: ResourceRef{ResourceUID: string([]byte{'b', 0xff, 'd'})}}
	if _, err := Encode(WorkerEndpointURL, spec); err == nil {
		t.Fatal("Encode accepted invalid UTF-8 in a typed spec")
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
		{"empty uid", SQLiteMigrationApplicationURL, `{"database":{"resourceUid":""},"migrationSet":{"resourceUid":"s"}}`, "resourceUid must match"},
		{"invalid uid character", WorkerEndpointURL, `{"worker":{"resourceUid":"has space"}}`, "resourceUid must match"},
		{"uid too long", WorkerEndpointURL, `{"worker":{"resourceUid":"` + strings.Repeat("a", 129) + `"}}`, "resourceUid must match"},
		{"retention below minimum", AtLeastOnceQueueURL, `{"messageRetentionSeconds":59}`, "60..1209600"},
		{"delay over maximum", AtLeastOnceQueueURL, `{"deliveryDelaySeconds":43201,"messageRetentionSeconds":60}`, "0..43200"},
		{"decimal integer", QueueConsumerURL, `{"queue":{"resourceUid":"q"},"worker":{"resourceUid":"w"},"maxBatchSize":1.5,"maxBatchTimeoutSeconds":0,"maxConcurrency":1,"maxRetries":0,"retryDelaySeconds":0}`, "integer"},
		{"bad artifact url", WorkerBundleURL, `{"artifact":{"url":"https://user:pass@artifacts.example/a?token=x","sha256":"` + strings.Repeat("a", 64) + `"}}`, `absolute HTTPS`},
		{"bad digest", SQLiteMigrationSetURL, `{"artifact":{"url":"https://artifacts.example/a","sha256":"ABC"}}`, `lowercase`},
		{"migration manifest url query", SQLiteMigrationSetURL, `{"artifact":{"url":"https://artifacts.example/manifest?sig=secret","sha256":"` + strings.Repeat("a", 64) + `"}}`, `absolute HTTPS`},
		{"empty hostname", WorkerBundleURL, `{"artifact":{"url":"https://:443/path","sha256":"` + strings.Repeat("a", 64) + `"}}`, `absolute HTTPS`},
		{"private value is not public", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":["fetch"],"privateInputs":{"TOKEN":"secret"}}`, "unknown field"},
		{"worker binding null nested ref", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"kvBindings":[{"name":"KV","resource":null}]}`, "object"},
		{"duplicate env name", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"bundle":{"resourceUid":"b"},"handlers":[],"vars":{"KV":"public"},"kvBindings":[{"name":"KV","resource":{"resourceUid":"r"}}]}`, "duplicate environment"},
		{"static plus handler", WorkerVersionURL, `{"worker":{"resourceUid":"w"},"handlers":["fetch"],"assets":{"bundle":{"resourceUid":"a"},"runWorkerFirst":false,"notFoundHandling":"none"}}`, "empty handlers"},
		{"weight sum", WorkerDeploymentURL, `{"worker":{"resourceUid":"w"},"versions":[{"workerVersion":{"resourceUid":"v"},"weight":9999}]}`, "sum to exactly 10000"},
		{"bad cron", WorkerCronTriggerURL, `{"worker":{"resourceUid":"w"},"cron":"61 * * * *"}`, "out of range"},
		{"overflow cron", WorkerCronTriggerURL, `{"worker":{"resourceUid":"w"},"cron":"18446744073709551616 * * * *"}`, "out of range"},
		{"signed cron integer", WorkerCronTriggerURL, `{"worker":{"resourceUid":"w"},"cron":"+1 * * * *"}`, "out of range"},
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
