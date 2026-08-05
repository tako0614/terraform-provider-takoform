// Package currentformcatalog owns the independently authored v1alpha2 Form
// candidates used by provider v2. It shares the small schema vocabulary with
// the frozen Legacy catalog, but it does not copy a Legacy Kind or fixture.
package currentformcatalog

import "github.com/tako0614/terraform-provider-takoform/internal/formcatalog"

func i64(value int64) *int64 { return &value }

func connection(name, resource string, permissions []any, projection string) map[string]any {
	return map[string]any{name: map[string]any{
		"resource": resource, "permissions": permissions, "projection": projection,
	}}
}

var orderedKinds = []string{
	"EdgeWorker",
	"RelationalDatabase",
	"ObjectBucket",
	"KeyValueStore",
	"Queue",
	"Schedule",
	"ContainerService",
	"StatefulEntity",
	"VectorIndex",
}

// Kinds is the complete current provider-v2 resource set. Each field expresses
// workload semantics a consumer can rely on. Placement, capacity, scaling,
// routing, health policy, backend compatibility, and commercial offerings are
// deliberately absent: a host or adapter owns those choices.
var Kinds = []formcatalog.Kind{
	{
		Kind: "EdgeWorker", ProposalID: "cloud-edge-worker", DefinitionVersion: "0.1.0", Slug: "edge-worker", ResourceType: "takoform_edge_worker",
		Domain: "compute", Title: "Edge Worker",
		Description: "Portable request/event application executed from digest-bound artifact bytes near an ingress boundary.",
		Artifact:    true, Connections: formcatalog.ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/edge-worker.tar",
			"artifactSha256":    "sha256:0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
			"artifactMediaType": "application/vnd.takoform.edge-worker+tar",
		},
		ConnectionExample: connection("data", "KeyValueStore/key-value-store", []any{"read", "write"}, "keyvalue.binding.v1"),
		Fields: []formcatalog.Field{
			{HCL: "entrypoint", Wire: "entrypoint", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarRelativePath,
				Doc: "Relative path of the artifact module started by the selected runtime.", Example: "worker.mjs", CounterExample: "../worker.mjs"},
			{HCL: "runtime", Wire: "runtime", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarToken,
				Doc: "Open runtime capability token required by the artifact; each host declares support independently.", Example: "javascript", CounterExample: "not a runtime"},
			{HCL: "runtime_version", Wire: "runtimeVersion", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarVersion,
				Doc: "Optional runtime compatibility version required by the artifact.", Example: "2026.1", AltExample: "2026.2"},
			{HCL: "configuration", Wire: "configuration", Type: formcatalog.TypeStringMap,
				Doc: "Non-secret application configuration. Credentials remain host-owned.", Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []formcatalog.Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by the application.", Operations: []string{"request"}}},
	},
	{
		Kind: "RelationalDatabase", ProposalID: "cloud-relational-database", DefinitionVersion: "0.1.0", Slug: "relational-database", ResourceType: "takoform_relational_database",
		Domain: "data", Title: "Relational Database",
		Description: "Portable relational database identified by an open engine capability token.",
		Fields: []formcatalog.Field{
			{HCL: "engine", Wire: "engine", Type: formcatalog.TypeString, Required: true, Immutable: true, Grammar: formcatalog.GrammarToken,
				Doc: "Open relational engine token. Changing it replaces the database.", Example: "postgres", CounterExample: "not a token", AltExample: "mysql"},
			{HCL: "engine_version", Wire: "engineVersion", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarVersion,
				Doc: "Optional engine compatibility version required by the workload.", Example: "16", AltExample: "17"},
			{HCL: "database_name", Wire: "databaseName", Type: formcatalog.TypeString, Immutable: true, Grammar: formcatalog.GrammarToken,
				Doc: "Initial logical database name. Changing it replaces the resource.", Example: "app", AltExample: "app_v2"},
			{HCL: "schema_url", Wire: "schemaUrl", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarCredentialFreeHTTPSURL,
				Doc: "Optional immutable schema or migration bundle applied in document order.", Example: "https://artifacts.portable-conformance.invalid/relational-schema.json", AltExample: "https://artifacts.portable-conformance.invalid/relational-schema-2.json", CounterExample: "https://schema.invalid/bundle.json?download=1"},
			{HCL: "schema_sha256", Wire: "schemaSha256", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarCanonicalSHA256,
				Doc: "Canonical lowercase digest binding schemaUrl to exact immutable bytes.", Example: "sha256:1d2181e213a086ae9e025d235ff5e267c43ec60cf4fc2f966977a21f2a95ef7b", AltExample: "sha256:3b3f36501936a84ed19b9bef37e5581c3e04948733b947ebaa002f196e66817c", CounterExample: "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
			{HCL: "schema_format", Wire: "schemaFormat", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarToken,
				Doc: "Open token naming the schema bundle format.", Example: "sql.ordered", AltExample: "sql.migrations", CounterExample: "not a token"},
		},
		Interfaces:       []formcatalog.Interface{{Name: "sql.query", Description: "Portable SQL query and transaction operations.", Operations: []string{"execute", "query", "transaction"}, ExtraInputs: []string{"engine"}}},
		CoRequiredFields: [][]string{{"schemaUrl", "schemaSha256", "schemaFormat"}},
	},
	{
		Kind: "ObjectBucket", ProposalID: "cloud-object-bucket", DefinitionVersion: "0.1.0", Slug: "object-bucket", ResourceType: "takoform_object_bucket",
		Domain: "data", Title: "Object Bucket", Description: "Portable object storage namespace.",
		Fields: []formcatalog.Field{{HCL: "versioning", Wire: "versioning", Type: formcatalog.TypeBool,
			Doc: "When present, requires retention of non-current object versions when true and disables creation of new non-current versions when false; omission states no portable version-retention requirement.", Example: true}},
		Interfaces: []formcatalog.Interface{{Name: "object.storage", Description: "Portable object storage operations.", Operations: []string{"delete", "get", "list", "put"}}},
	},
	{
		Kind: "KeyValueStore", ProposalID: "cloud-key-value-store", DefinitionVersion: "0.1.0", Slug: "key-value-store", ResourceType: "takoform_key_value_store",
		Domain: "data", Title: "Key Value Store", Description: "Portable key/value state with declared consistency and expiry semantics.",
		Fields: []formcatalog.Field{
			{HCL: "consistency", Wire: "consistency", Type: formcatalog.TypeString, Enum: []string{"eventual", "per_key_linearizable"}, Doc: "Optional read consistency requirement: eventual permits stale reads; per_key_linearizable requires each key's completed writes and subsequent reads to appear in one real-time order. Omission states no portable consistency guarantee.", Example: "eventual", CounterExample: "strong"},
			{HCL: "default_ttl_seconds", Wire: "defaultTtlSeconds", Type: formcatalog.TypeInt, Min: i64(1), Doc: "Optional positive default entry lifetime in seconds; omission requests no default expiry requirement.", Example: 3600, CounterExample: 0},
		},
		Interfaces: []formcatalog.Interface{{Name: "keyvalue.store", Description: "Portable key/value operations.", Operations: []string{"delete", "get", "list", "put"}}},
	},
	{
		Kind: "Queue", ProposalID: "cloud-queue", DefinitionVersion: "0.1.0", Slug: "queue", ResourceType: "takoform_queue",
		Domain: "data", Title: "Queue", Description: "Portable asynchronous at-least-once message delivery.",
		Connections: formcatalog.ConnectionsOptional,
		Fields: []formcatalog.Field{
			{HCL: "message_retention_seconds", Wire: "messageRetentionSeconds", Type: formcatalog.TypeInt, Min: i64(1), Doc: "Optional minimum retention for an unacknowledged message, in seconds.", Example: 345600, CounterExample: 0},
			{HCL: "ordering", Wire: "ordering", Type: formcatalog.TypeString, Enum: []string{"best_effort", "strict"}, Doc: "Delivery ordering requirement: best_effort gives no relative-order guarantee; strict follows host acceptance order within this queue, apart from duplicate redelivery allowed by at-least-once delivery.", Example: "best_effort", CounterExample: "fifo"},
		},
		Interfaces: []formcatalog.Interface{{Name: "queue.messages", Description: "Portable queue delivery operations.", Operations: []string{"acknowledge", "receive", "send"}}},
	},
	{
		Kind: "Schedule", ProposalID: "cloud-schedule", DefinitionVersion: "0.1.0", Slug: "schedule", ResourceType: "takoform_schedule",
		Domain: "compute", Title: "Schedule", Description: "Portable cron lifecycle that invokes exactly one connected Resource.",
		Connections: formcatalog.ConnectionsRequired, MaxConnections: 1,
		ConnectionExample: connection("invocation", "EdgeWorker/edge-worker", []any{"request"}, "schedule.trigger.v1"),
		Fields: []formcatalog.Field{
			{HCL: "cron", Wire: "cron", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarCron, Doc: "Portable five-field cron expression.", Example: "0 0 * * *", CounterExample: "0 0 * *", AltExample: "15 0 * * *"},
			{HCL: "timezone", Wire: "timezone", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarTimezone, Default: "UTC", Doc: "IANA timezone required by the schedule; support is host-declared.", Example: "UTC"},
		},
	},
	{
		Kind: "ContainerService", ProposalID: "cloud-container-service", DefinitionVersion: "0.1.0", Slug: "container-service", ResourceType: "takoform_container_service",
		Domain: "compute", Title: "Container Service", Description: "Portable service executed from an immutable OCI image digest.",
		Connections: formcatalog.ConnectionsOptional,
		Fields: []formcatalog.Field{
			{HCL: "image", Wire: "image", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarCanonicalOCIDigest, Doc: "Immutable OCI image reference pinned by one lowercase sha256 digest.", Example: "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317", CounterExample: "docker.io/library/nginx@sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
			{HCL: "configuration", Wire: "configuration", Type: formcatalog.TypeStringMap, Doc: "Non-secret application configuration. Credentials remain host-owned.", Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []formcatalog.Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by the service.", Operations: []string{"request"}}},
	},
	{
		Kind: "StatefulEntity", ProposalID: "cloud-stateful-entity", DefinitionVersion: "0.1.0", Slug: "stateful-entity", ResourceType: "takoform_stateful_entity",
		Domain: "compute", Title: "Stateful Entity", Description: "Portable namespace of addressable persistent entities implemented by digest-bound application bytes.",
		Artifact: true, Connections: formcatalog.ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/stateful-entity.tar",
			"artifactSha256":    "sha256:5d877f919bf8db6e6fd819e32f74dff6fc94b06f8914fa1abf5bcd2fb32ae958",
			"artifactMediaType": "application/vnd.takoform.stateful-entity+tar",
		},
		Fields: []formcatalog.Field{
			{HCL: "entrypoint", Wire: "entrypoint", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarClass, Doc: "Artifact entrypoint implementing entity behavior.", Example: "RoomEntity", CounterExample: "not a class"},
			{HCL: "runtime", Wire: "runtime", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarToken, Doc: "Open runtime capability token required by the artifact.", Example: "javascript", CounterExample: "not a runtime"},
			{HCL: "runtime_version", Wire: "runtimeVersion", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarVersion, Doc: "Optional runtime compatibility version required by the artifact.", Example: "2026.1", AltExample: "2026.2"},
			{HCL: "persistence", Wire: "persistence", Type: formcatalog.TypeString, Required: true, Grammar: formcatalog.GrammarToken, Doc: "Open persistence capability token required by the workload; transactional means invocations for one entity observe serializable state transitions committed atomically with that invocation.", Example: "transactional", CounterExample: "not a token"},
			{HCL: "configuration", Wire: "configuration", Type: formcatalog.TypeStringMap, Doc: "Non-secret application configuration. Credentials remain host-owned.", Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []formcatalog.Interface{{Name: "entity.invoke", Description: "Portable stateful entity invocation operations.", Operations: []string{"invoke"}}},
	},
	{
		Kind: "VectorIndex", ProposalID: "cloud-vector-index", DefinitionVersion: "0.1.0", Slug: "vector-index", ResourceType: "takoform_vector_index",
		Domain: "data", Title: "Vector Index", Description: "Portable vector index with dimensions fixed for the index lifecycle.",
		Connections: formcatalog.ConnectionsOptional,
		Fields: []formcatalog.Field{
			{HCL: "dimensions", Wire: "dimensions", Type: formcatalog.TypeInt, Required: true, Immutable: true, Min: i64(1), Doc: "Positive vector dimensions fixed for the index lifecycle.", Example: 1536, CounterExample: 0, AltExample: 768},
			{HCL: "metric", Wire: "metric", Type: formcatalog.TypeString, Grammar: formcatalog.GrammarToken, Default: "cosine", Doc: "Open similarity metric capability token.", Example: "cosine", AltExample: "dot"},
		},
		Interfaces: []formcatalog.Interface{{Name: "vector.query", Description: "Portable vector index operations.", Operations: []string{"delete", "query", "upsert"}}},
	},
}

// ByKind returns one current provider-v2 schema declaration.
func ByKind(name string) (formcatalog.Kind, bool) {
	for _, kind := range Kinds {
		if kind.Kind == name {
			return kind, true
		}
	}
	return formcatalog.Kind{}, false
}
