// Package formcatalog declares every portable Service Form this release
// carries, as data.
//
// A Form is described once here — its intent, its typed fields, the runtime
// interfaces it exposes — and every other surface is derived from that single
// declaration: the Terraform/OpenTofu resource schema, the wire spec, the
// Draft 2020-12 desired schema inside the Form Definition, and the canonical
// conformance fixtures. A kind is therefore impossible to add to one surface
// and forget in another.
//
// Fields describe what a caller wants, never how or where a host builds it.
// No field carries a credential, a target, a placement, a price, or an
// implementation name, and no field is shaped by one backend's product
// catalogue.
package formcatalog

// FieldType is the portable type of one declared field.
type FieldType string

const (
	TypeString    FieldType = "string"
	TypeInt       FieldType = "integer"
	TypeBool      FieldType = "boolean"
	TypeStringSet FieldType = "string-set"
	TypeIntSet    FieldType = "int-set"
)

// Grammar names a reviewed portable string grammar. Free-form strings carry
// no grammar; every other value is constrained so hosts can agree on meaning.
type Grammar string

const (
	GrammarNone       Grammar = ""
	GrammarToken      Grammar = "token"       // open capability token
	GrammarClass      Grammar = "class"       // runtime class identifier
	GrammarTimezone   Grammar = "timezone"    // IANA-style zone token
	GrammarCron       Grammar = "cron"        // five-field cron expression
	GrammarHostname   Grammar = "hostname"    // DNS hostname
	GrammarDomain     Grammar = "domain"      // DNS domain name
	GrammarPath       Grammar = "path"        // absolute URL path
	GrammarCIDR       Grammar = "cidr"        // IPv4/IPv6 address block
	GrammarOCIDigest  Grammar = "oci-digest"  // OCI reference pinned by digest
	GrammarMailbox    Grammar = "mailbox"     // email address
	GrammarHTTPSURL   Grammar = "https-url"   // absolute https URL
	GrammarRecordData Grammar = "record-data" // DNS record value
)

// ConnectionMode says whether a Form declares connections to other Resources.
type ConnectionMode string

const (
	// ConnectionsAbsent is the zero value: a Form declares no connections
	// unless it says so.
	ConnectionsAbsent   ConnectionMode = ""
	ConnectionsOptional ConnectionMode = "optional"
	ConnectionsRequired ConnectionMode = "required"
)

// Field is one typed portable field of a Form.
type Field struct {
	// HCL is the snake_case attribute name; Wire is the camelCase spec key.
	HCL, Wire string
	Type      FieldType
	Doc       string

	Required  bool
	Immutable bool

	Grammar Grammar
	Enum    []string
	Default string // portable default for an optional string field

	Min, Max *int64 // inclusive integer bounds, also applied to int set members
	MinItems int    // minimum members of a set

	// Example is the value used by the canonical conformance fixture.
	Example any
	// CounterExample is a value that must be rejected. Exactly one field per
	// Form carries it, and it drives the generated negative fixture.
	CounterExample any
	// AltExample is a second valid value. Every immutable field carries one so
	// the lifecycle can prove that changing it really replaces the resource.
	AltExample any
}

// Interface is a runtime interface a Form declares. Names are author-defined
// and open: no registry, no allowlist, no central approval.
type Interface struct {
	Name        string
	Description string
	Operations  []string
	// ExtraInputs names additional output pointers the descriptor resolves,
	// beyond the resource id and name every interface receives.
	ExtraInputs []string
}

// Kind is one portable Service Form.
type Kind struct {
	Kind         string // PascalCase portable kind
	Slug         string // kebab-case package directory
	ResourceType string // takoform_* Terraform resource type
	// DefinitionVersion is the SemVer of this Form's definition. A kind whose
	// name previously carried published bytes starts a new major line so the
	// retired identity is never silently reshaped.
	DefinitionVersion string
	Domain            string
	Title             string
	Description       string

	Fields      []Field
	Connections ConnectionMode
	// Artifact adds the portable prebuilt-artifact source union.
	Artifact bool
	// ArtifactExample is the canonical fixture value for that source.
	ArtifactExample map[string]any
	// ConnectionExample is the canonical fixture value for connections.
	ConnectionExample map[string]any

	Interfaces []Interface
}

func i64(value int64) *int64 { return &value }

// Kinds is the complete portable Form set of this release, in a stable order.
var Kinds = []Kind{
	// ---------------------------------------------------------------- compute
	{
		Kind: "HttpService", Slug: "http-service", ResourceType: "takoform_http_service",
		Domain: "compute", Title: "HTTP Service",
		Description: "Portable HTTP application served from a prebuilt immutable artifact.",
		Artifact:    true, Connections: ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactRef":    "portable-conformance/v1/http-service.tar",
			"artifactSha256": "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
		},
		ConnectionExample: connection("assets", "ObjectBucket/object-bucket", []any{"read"}, "object.binding.v1"),
		Fields: []Field{
			{HCL: "runtime", Wire: "runtime", Type: TypeString, Grammar: GrammarToken,
				Doc:     "Open runtime capability token the artifact expects. The configured host decides support.",
				Example: "javascript", AltExample: "python"},
			{HCL: "runtime_version", Wire: "runtimeVersion", Type: TypeString,
				Doc: "Optional runtime version requested for the artifact."},
			{HCL: "request_timeout_seconds", Wire: "requestTimeoutSeconds", Type: TypeInt, Min: i64(1), Max: i64(3600),
				Doc: "Optional per-request timeout preference in seconds."},
			{HCL: "concurrency", Wire: "concurrency", Type: TypeInt, Min: i64(1),
				Doc: "Optional concurrent-request preference.", CounterExample: 0},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by an application.", Operations: []string{"request"}}},
	},
	{
		Kind: "ContainerService", DefinitionVersion: "2.0.0", Slug: "container-service", ResourceType: "takoform_container_service",
		Domain: "compute", Title: "Container Service",
		Description: "Portable OCI container service pinned to an immutable image digest.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "image", Wire: "image", Type: TypeString, Required: true, Grammar: GrammarOCIDigest,
				Doc:     "Immutable OCI image reference pinned by sha256 digest.",
				Example: "docker.io/library/nginx@sha256:845b5424415de5f77dd5753cbb7c1be8bd8e44cc81f20f9705783a02f8848317",
				// A floating tag is not an immutable image.
				CounterExample: "docker.io/library/nginx:latest"},
			{HCL: "ports", Wire: "ports", Type: TypeIntSet, Min: i64(1), Max: i64(65535),
				Doc: "Container ports requested by the service.", Example: []any{80}},
			{HCL: "public_http", Wire: "publicHttp", Type: TypeBool,
				Doc: "Whether this container asks for public HTTP exposure.", Example: true},
			{HCL: "cpu_millicores", Wire: "cpuMillicores", Type: TypeInt, Min: i64(1),
				Doc: "Optional CPU request in millicores."},
			{HCL: "memory_mib", Wire: "memoryMib", Type: TypeInt, Min: i64(1),
				Doc: "Optional memory request in mebibytes."},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by a container service.", Operations: []string{"request"}}},
	},
	{
		Kind: "ComputeInstance", Slug: "compute-instance", ResourceType: "takoform_compute_instance",
		Domain: "compute", Title: "Compute Instance",
		Description: "Portable long-running machine instance built from an immutable image.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "machine_class", Wire: "machineClass", Type: TypeString, Required: true, Grammar: GrammarToken,
				Doc: "Open machine capability token describing the requested size class.", Example: "general.small",
				CounterExample: "not a token"},
			{HCL: "image", Wire: "image", Type: TypeString, Required: true,
				Doc: "Immutable machine image reference.", Example: "portable-conformance/v1/base-linux"},
			{HCL: "boot_disk_gib", Wire: "bootDiskGib", Type: TypeInt, Required: true, Min: i64(1),
				Doc: "Boot disk size in gibibytes.", Example: 20},
			{HCL: "instance_count", Wire: "instanceCount", Type: TypeInt, Min: i64(1),
				Doc: "Optional identical-instance count preference."},
		},
	},
	{
		Kind: "StaticSite", Slug: "static-site", ResourceType: "takoform_static_site",
		Domain: "compute", Title: "Static Site",
		Description: "Portable static asset site served from a prebuilt immutable artifact.",
		Artifact:    true,
		ArtifactExample: map[string]any{
			"artifactRef":    "portable-conformance/v1/static-site.tar",
			"artifactSha256": "3b1d4c2f9a8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817065f4e",
		},
		Fields: []Field{
			{HCL: "index_document", Wire: "indexDocument", Type: TypeString, Default: "index.html",
				Doc: "Document served for a directory request.", Example: "index.html"},
			{HCL: "error_document", Wire: "errorDocument", Type: TypeString,
				Doc: "Optional document served for a not-found request."},
			{HCL: "single_page_app", Wire: "singlePageApp", Type: TypeBool,
				Doc: "Whether unmatched paths should serve the index document."},
			{HCL: "cache_control_seconds", Wire: "cacheControlSeconds", Type: TypeInt, Min: i64(0),
				Doc:     "Optional freshness lifetime advertised for served assets, in seconds.",
				Example: 300, CounterExample: -1},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by a static site.", Operations: []string{"request"}}},
	},
	{
		Kind: "Workflow", Slug: "workflow", ResourceType: "takoform_workflow",
		Domain: "compute", Title: "Workflow",
		Description: "Portable durable workflow definition and instance-state lifecycle.",
		Artifact:    true, Connections: ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactRef":    "portable-conformance/v1/workflow.mjs",
			"artifactSha256": "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5",
		},
		Fields: []Field{
			{HCL: "entrypoint", Wire: "entrypoint", Type: TypeString, Required: true,
				Doc: "Workflow runtime entrypoint.", Example: "IngestWorkflow"},
			{HCL: "max_attempts", Wire: "maxAttempts", Type: TypeInt, Min: i64(1),
				Doc: "Optional maximum attempts per workflow run.", Example: 3, CounterExample: 0},
			{HCL: "initial_backoff_seconds", Wire: "initialBackoffSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional initial retry backoff in seconds.", Example: 5},
		},
		Interfaces: []Interface{{Name: "workflow.invoke", Description: "Portable durable workflow invocation operations.", Operations: []string{"cancel", "invoke", "status"}}},
	},
	{
		Kind: "StatefulEntity", Slug: "stateful-entity", ResourceType: "takoform_stateful_entity",
		Domain: "compute", Title: "Stateful Entity",
		Description: "Portable namespace of individually addressable, individually persistent entities.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "entity_class", Wire: "entityClass", Type: TypeString, Required: true, Grammar: GrammarClass,
				Doc: "Runtime class identifier owning entity behaviour inside this namespace.", Example: "RoomEntity",
				CounterExample: "not a class"},
			{HCL: "persistence", Wire: "persistence", Type: TypeString, Grammar: GrammarToken,
				Doc: "Open persistence capability token requested for entity state."},
			{HCL: "migration_tag", Wire: "migrationTag", Type: TypeString,
				Doc: "Optional namespace migration tag. It never identifies one entity instance.", Example: "v1",
				AltExample: "v2"},
		},
		Interfaces: []Interface{{Name: "entity.invoke", Description: "Portable stateful entity invocation operations.", Operations: []string{"invoke"}}},
	},
	{
		Kind: "Schedule", DefinitionVersion: "2.0.0", Slug: "schedule", ResourceType: "takoform_schedule",
		Domain: "compute", Title: "Schedule",
		Description:       "Portable cron lifecycle that invokes exactly one connected Resource.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("invocation", "Workflow/workflow", []any{"invoke"}, "schedule.trigger.v1"),
		Fields: []Field{
			{HCL: "cron", Wire: "cron", Type: TypeString, Required: true, Grammar: GrammarCron,
				Doc: "Portable five-field cron expression.", Example: "0 0 * * *", CounterExample: "0 0 * *",
				AltExample: "15 0 * * *"},
			{HCL: "timezone", Wire: "timezone", Type: TypeString, Grammar: GrammarTimezone, Default: "UTC",
				Doc: "Open timezone token. Non-UTC requires explicit support from the configured host.", Example: "UTC"},
		},
	},

	// ------------------------------------------------------------------- data
	{
		Kind: "ObjectBucket", DefinitionVersion: "2.0.0", Slug: "object-bucket", ResourceType: "takoform_object_bucket",
		Domain: "data", Title: "Object Bucket",
		Description: "Portable object storage with a portable default storage class.",
		Fields: []Field{
			{HCL: "storage_class", Wire: "storageClass", Type: TypeString, Default: "standard",
				Enum: []string{"standard", "infrequent_access", "archive"},
				Doc:  "Portable default storage class for newly written objects.", Example: "standard",
				CounterExample: "cold"},
			{HCL: "versioning", Wire: "versioning", Type: TypeBool,
				Doc: "Whether the bucket should retain non-current object versions."},
			{HCL: "access_protocols", Wire: "accessProtocols", Type: TypeStringSet, Grammar: GrammarToken,
				Doc: "Optional access-protocol capability tokens requested from the host.", Example: []any{"s3_api"}},
		},
		Interfaces: []Interface{{Name: "object.storage", Description: "Portable object storage operations.", Operations: []string{"delete", "get", "list", "put"}}},
	},
	{
		Kind: "ObjectLifecycleRule", Slug: "object-lifecycle-rule", ResourceType: "takoform_object_lifecycle_rule",
		Domain: "data", Title: "Object Lifecycle Rule",
		Description:       "Portable retention and transition rule applied to one connected object store.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("store", "ObjectBucket/object-bucket", []any{"administer"}, "object.lifecycle.v1"),
		Fields: []Field{
			{HCL: "prefix", Wire: "prefix", Type: TypeString,
				Doc: "Optional key prefix the rule applies to.", Example: "logs/"},
			{HCL: "expire_after_days", Wire: "expireAfterDays", Type: TypeInt, Min: i64(1),
				Doc: "Optional age in days after which matching objects are deleted.", Example: 90,
				CounterExample: 0},
			{HCL: "transition_after_days", Wire: "transitionAfterDays", Type: TypeInt, Min: i64(1),
				Doc: "Optional age in days after which matching objects change storage class."},
			{HCL: "transition_storage_class", Wire: "transitionStorageClass", Type: TypeString,
				Enum: []string{"infrequent_access", "archive"},
				Doc:  "Storage class matching objects transition into."},
		},
	},
	{
		Kind: "KeyValueStore", Slug: "key-value-store", ResourceType: "takoform_key_value_store",
		Domain: "data", Title: "Key Value Store",
		Description: "Portable key/value state with an optional consistency preference.",
		Fields: []Field{
			{HCL: "consistency", Wire: "consistency", Type: TypeString, Enum: []string{"eventual", "strong"},
				Doc: "Optional consistency preference.", Example: "eventual", CounterExample: "linearizable"},
			{HCL: "default_ttl_seconds", Wire: "defaultTtlSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional default entry lifetime in seconds."},
		},
		Interfaces: []Interface{{Name: "keyvalue.store", Description: "Portable key/value operations.", Operations: []string{"delete", "get", "list", "put"}}},
	},
	{
		Kind: "CacheCluster", Slug: "cache-cluster", ResourceType: "takoform_cache_cluster",
		Domain: "data", Title: "Cache Cluster",
		Description: "Portable in-memory cache sized by an open capability token.",
		Fields: []Field{
			{HCL: "size_class", Wire: "sizeClass", Type: TypeString, Required: true, Grammar: GrammarToken,
				Doc: "Open capability token describing the requested cache size.", Example: "cache.small",
				CounterExample: "not a token"},
			{HCL: "eviction_policy", Wire: "evictionPolicy", Type: TypeString,
				Enum: []string{"least_recently_used", "least_frequently_used", "time_to_live"},
				Doc:  "Optional eviction preference when the cache is full.", Example: "least_recently_used"},
			{HCL: "default_ttl_seconds", Wire: "defaultTtlSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional default entry lifetime in seconds."},
		},
		Interfaces: []Interface{{Name: "cache.store", Description: "Portable cache operations.", Operations: []string{"delete", "get", "put"}}},
	},
	{
		Kind: "RelationalDatabase", Slug: "relational-database", ResourceType: "takoform_relational_database",
		Domain: "data", Title: "Relational Database",
		Description: "Portable relational database addressed through an open engine capability token.",
		Fields: []Field{
			{HCL: "engine", Wire: "engine", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarToken,
				Doc: "Open engine capability token. Changing it replaces the database.", Example: "postgres",
				CounterExample: "not a token", AltExample: "mysql"},
			{HCL: "engine_version", Wire: "engineVersion", Type: TypeString,
				Doc: "Optional engine version requested from the host.", Example: "16", AltExample: "17"},
			{HCL: "storage_gib", Wire: "storageGib", Type: TypeInt, Min: i64(1),
				Doc: "Optional storage request in gibibytes."},
		},
		Interfaces: []Interface{{
			Name: "sql.query", Description: "Portable SQL query and transaction operations.",
			Operations: []string{"execute", "query", "transaction"}, ExtraInputs: []string{"engine"},
		}},
	},
	{
		Kind: "IndexedStore", Slug: "indexed-store", ResourceType: "takoform_indexed_store",
		Domain: "data", Title: "Indexed Store",
		Description: "Portable bounded key/value item store with declared queryable attributes and no query language.",
		Fields: []Field{
			{HCL: "partition_key", Wire: "partitionKey", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarToken,
				Doc: "Attribute that partitions stored items. Changing it replaces the store.", Example: "tenantId",
				CounterExample: "not a key", AltExample: "accountId"},
			{HCL: "sort_key", Wire: "sortKey", Type: TypeString, Immutable: true, Grammar: GrammarToken,
				Doc: "Optional attribute that orders items inside one partition.", Example: "createdAt", AltExample: "updatedAt"},
			{HCL: "indexed_attributes", Wire: "indexedAttributes", Type: TypeStringSet, Grammar: GrammarToken,
				Doc: "Additional attributes the host must make queryable.", Example: []any{"status"}},
		},
		Interfaces: []Interface{{
			Name: "data.indexed", Description: "Portable bounded key and declared-index operations.",
			Operations: []string{"delete", "get", "put", "query"},
		}},
	},
	{
		Kind: "Queue", DefinitionVersion: "2.0.0", Slug: "queue", ResourceType: "takoform_queue",
		Domain: "data", Title: "Queue",
		Description: "Portable asynchronous delivery with at-least-once semantics.",
		Fields: []Field{
			{HCL: "max_retries", Wire: "maxRetries", Type: TypeInt, Min: i64(0),
				Doc: "Optional delivery retry preference.", Example: 5, CounterExample: -1},
			{HCL: "max_batch_size", Wire: "maxBatchSize", Type: TypeInt, Min: i64(1),
				Doc: "Optional consumer batch size preference."},
			{HCL: "visibility_timeout_seconds", Wire: "visibilityTimeoutSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional time a received message stays invisible to other consumers."},
		},
		Interfaces: []Interface{{Name: "queue.messages", Description: "Portable queue delivery operations.", Operations: []string{"acknowledge", "receive", "send"}}},
	},
	{
		Kind: "StreamTopic", Slug: "stream-topic", ResourceType: "takoform_stream_topic",
		Domain: "data", Title: "Stream Topic",
		Description: "Portable published event stream that many independent consumers can read.",
		Fields: []Field{
			{HCL: "partitions", Wire: "partitions", Type: TypeInt, Immutable: true, Min: i64(1),
				Doc: "Ordered partition count fixed for the stream lifecycle.", Example: 3, CounterExample: 0, AltExample: 6},
			{HCL: "retention_hours", Wire: "retentionHours", Type: TypeInt, Min: i64(1),
				Doc: "Optional published-record retention in hours.", Example: 24},
		},
		Interfaces: []Interface{{Name: "stream.publish", Description: "Portable stream publish and subscribe operations.", Operations: []string{"publish", "subscribe"}}},
	},
	{
		Kind: "SearchIndex", Slug: "search-index", ResourceType: "takoform_search_index",
		Domain: "data", Title: "Search Index",
		Description: "Portable full-text index over a declared set of document fields.",
		Fields: []Field{
			{HCL: "fields", Wire: "fields", Type: TypeStringSet, Required: true, MinItems: 1, Grammar: GrammarToken,
				Doc: "Document fields the index must make searchable.", Example: []any{"body", "title"}},
			{HCL: "language", Wire: "language", Type: TypeString, Grammar: GrammarToken,
				Doc: "Optional analysis language token.", Example: "en", CounterExample: "not a token"},
		},
		Interfaces: []Interface{{Name: "search.query", Description: "Portable search index operations.", Operations: []string{"delete", "index", "query"}}},
	},
	{
		Kind: "VectorIndex", DefinitionVersion: "2.0.0", Slug: "vector-index", ResourceType: "takoform_vector_index",
		Domain: "data", Title: "Vector Index",
		Description: "Portable vector index with dimensions fixed for the index lifecycle.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "dimensions", Wire: "dimensions", Type: TypeInt, Required: true, Immutable: true, Min: i64(1),
				Doc: "Positive vector dimensions fixed for the index lifecycle.", Example: 1536, CounterExample: 0, AltExample: 768},
			{HCL: "metric", Wire: "metric", Type: TypeString, Grammar: GrammarToken, Default: "cosine",
				Doc: "Open similarity metric capability token.", Example: "cosine", AltExample: "dot"},
		},
		Interfaces: []Interface{{Name: "vector.query", Description: "Portable vector index operations.", Operations: []string{"delete", "query", "upsert"}}},
	},

	// -------------------------------------------------------- analytics and AI
	{
		Kind: "AnalyticsDataset", Slug: "analytics-dataset", ResourceType: "takoform_analytics_dataset",
		Domain: "analytics", Title: "Analytics Dataset",
		Description: "Portable append-oriented dataset queried for analysis rather than transactions.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "partition_field", Wire: "partitionField", Type: TypeString, Immutable: true, Grammar: GrammarToken,
				Doc: "Optional field the dataset partitions on. Changing it replaces the dataset.", Example: "eventDate",
				CounterExample: "not a token", AltExample: "ingestDate"},
			{HCL: "retention_days", Wire: "retentionDays", Type: TypeInt, Min: i64(1),
				Doc: "Optional record retention in days.", Example: 365},
		},
		Interfaces: []Interface{{Name: "analytics.query", Description: "Portable analytics dataset operations.", Operations: []string{"append", "query"}}},
	},
	{
		Kind: "ModelEndpoint", Slug: "model-endpoint", ResourceType: "takoform_model_endpoint",
		Domain: "analytics", Title: "Model Endpoint",
		Description: "Portable inference endpoint serving one declared model for one declared task.",
		Fields: []Field{
			{HCL: "model", Wire: "model", Type: TypeString, Required: true,
				Doc: "Immutable model reference the endpoint serves.", Example: "portable-conformance/v1/embedding-small",
				AltExample: "portable-conformance/v1/embedding-large"},
			{HCL: "task", Wire: "task", Type: TypeString, Required: true, Grammar: GrammarToken,
				Doc: "Open task capability token, for example text_generation or embedding.", Example: "embedding",
				CounterExample: "not a token"},
			{HCL: "max_concurrency", Wire: "maxConcurrency", Type: TypeInt, Min: i64(1),
				Doc: "Optional concurrent-inference preference."},
		},
		Interfaces: []Interface{{Name: "model.invoke", Description: "Portable model inference operations.", Operations: []string{"invoke"}}},
	},

	// ---------------------------------------------------------------- network
	{
		Kind: "DnsZone", Slug: "dns-zone", ResourceType: "takoform_dns_zone",
		Domain: "network", Title: "DNS Zone",
		Description: "Portable authoritative DNS zone for one domain.",
		Fields: []Field{
			{HCL: "domain", Wire: "domain", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarDomain,
				Doc: "Domain this zone is authoritative for.", Example: "portable-conformance.invalid",
				CounterExample: "not a domain", AltExample: "alt.portable-conformance.invalid"},
			{HCL: "default_ttl_seconds", Wire: "defaultTtlSeconds", Type: TypeInt, Min: i64(1),
				Doc: "Optional default record time to live for this zone, in seconds.", Example: 3600},
		},
		Interfaces: []Interface{{Name: "dns.zone", Description: "Portable authoritative DNS zone operations.", Operations: []string{"list", "resolve"}}},
	},
	{
		Kind: "DnsRecord", Slug: "dns-record", ResourceType: "takoform_dns_record",
		Domain: "network", Title: "DNS Record",
		Description:       "Portable DNS record published into one connected zone.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("parent", "DnsZone/primary", []any{"administer"}, "dns.zone.v1"),
		Fields: []Field{
			{HCL: "record_name", Wire: "recordName", Type: TypeString, Required: true,
				Doc: "Record name relative to the connected zone.", Example: "api"},
			{HCL: "record_type", Wire: "recordType", Type: TypeString, Required: true, Immutable: true,
				Enum: []string{"A", "AAAA", "CNAME", "TXT", "MX", "SRV", "CAA", "NS"},
				Doc:  "Record type. Changing it replaces the record.", Example: "CNAME", CounterExample: "ANY",
				AltExample: "TXT"},
			{HCL: "values", Wire: "values", Type: TypeStringSet, Required: true, MinItems: 1, Grammar: GrammarRecordData,
				Doc: "Record data published for this name.", Example: []any{"service.portable-conformance.invalid"}},
			{HCL: "ttl_seconds", Wire: "ttlSeconds", Type: TypeInt, Min: i64(1),
				Doc: "Optional record time to live in seconds.", Example: 300},
		},
	},
	{
		Kind: "TlsCertificate", Slug: "tls-certificate", ResourceType: "takoform_tls_certificate",
		Domain: "network", Title: "TLS Certificate",
		Description: "Portable managed TLS certificate for a fixed set of domains. Key material stays with the host.",
		Fields: []Field{
			{HCL: "domains", Wire: "domains", Type: TypeStringSet, Required: true, Immutable: true, MinItems: 1, Grammar: GrammarDomain,
				Doc:     "Domains the certificate covers. Changing them replaces the certificate.",
				Example: []any{"portable-conformance.invalid"}, AltExample: []any{"alt.portable-conformance.invalid"}},
			{HCL: "key_algorithm", Wire: "keyAlgorithm", Type: TypeString, Default: "ecdsa_p256",
				Enum:    []string{"ecdsa_p256", "ecdsa_p384", "rsa_2048", "rsa_4096"},
				Doc:     "Requested certificate algorithm. The host generates and holds the key material.",
				Example: "ecdsa_p256", CounterExample: "rsa_1024"},
		},
		Interfaces: []Interface{{Name: "tls.certificate", Description: "Portable managed certificate status operations.", Operations: []string{"status"}}},
	},
	{
		Kind: "HttpRoute", Slug: "http-route", ResourceType: "takoform_http_route",
		Domain: "network", Title: "HTTP Route",
		Description:       "Portable hostname and path binding that sends HTTP traffic to one connected Resource.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("application", "HttpService/http-service", []any{"request"}, "http.route.v1"),
		Fields: []Field{
			{HCL: "hostname", Wire: "hostname", Type: TypeString, Required: true, Grammar: GrammarHostname,
				Doc: "Hostname this route answers.", Example: "api.portable-conformance.invalid",
				CounterExample: "not a hostname"},
			{HCL: "path_prefix", Wire: "pathPrefix", Type: TypeString, Grammar: GrammarPath, Default: "/",
				Doc: "Absolute path prefix this route matches.", Example: "/", AltExample: "/api"},
			{HCL: "strip_path_prefix", Wire: "stripPathPrefix", Type: TypeBool,
				Doc: "Whether the matched prefix is removed before the request reaches the target."},
		},
	},
	{
		Kind: "LoadBalancer", Slug: "load-balancer", ResourceType: "takoform_load_balancer",
		Domain: "network", Title: "Load Balancer",
		Description:       "Portable listener that distributes connections across connected backends.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("upstream", "ContainerService/container-service", []any{"request"}, "network.backend.v1"),
		Fields: []Field{
			{HCL: "protocol", Wire: "protocol", Type: TypeString, Required: true,
				Enum: []string{"tcp", "udp", "http", "https"},
				Doc:  "Listener protocol.", Example: "https", CounterExample: "smtp"},
			{HCL: "listen_port", Wire: "listenPort", Type: TypeInt, Required: true, Min: i64(1), Max: i64(65535),
				Doc: "Port the listener accepts connections on.", Example: 443},
			{HCL: "health_check_path", Wire: "healthCheckPath", Type: TypeString, Grammar: GrammarPath,
				Doc: "Optional HTTP path polled to decide backend health.", Example: "/healthz"},
		},
		Interfaces: []Interface{{Name: "network.endpoint", Description: "Portable network endpoint status operations.", Operations: []string{"status"}}},
	},
	{
		Kind: "PrivateNetwork", Slug: "private-network", ResourceType: "takoform_private_network",
		Domain: "network", Title: "Private Network",
		Description: "Portable private address space that other Resources can attach to.",
		Fields: []Field{
			{HCL: "address_space", Wire: "addressSpace", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarCIDR,
				Doc: "Private address block in CIDR notation.", Example: "10.32.0.0/16",
				CounterExample: "10.32.0.0", AltExample: "10.99.0.0/16"},
			{HCL: "public_egress", Wire: "publicEgress", Type: TypeBool,
				Doc: "Whether attached Resources may open outbound public connections.", Example: false},
		},
		Interfaces: []Interface{{Name: "network.attach", Description: "Portable private network attachment operations.", Operations: []string{"attach", "detach"}}},
	},

	// ------------------------------------------------------------- operations
	{
		Kind: "ContainerRegistry", Slug: "container-registry", ResourceType: "takoform_container_registry",
		Domain: "operations", Title: "Container Registry",
		Description: "Portable OCI artifact registry namespace.",
		Fields: []Field{
			{HCL: "visibility", Wire: "visibility", Type: TypeString, Default: "private",
				Enum: []string{"private", "public"},
				Doc:  "Whether pulls require an authenticated principal.", Example: "private",
				CounterExample: "internal"},
			{HCL: "immutable_tags", Wire: "immutableTags", Type: TypeBool,
				Doc: "Whether an existing tag may be repointed at different bytes.", Example: true},
		},
		Interfaces: []Interface{{Name: "registry.images", Description: "Portable registry artifact operations.", Operations: []string{"list", "pull", "push"}}},
	},
	{
		Kind: "LogSink", Slug: "log-sink", ResourceType: "takoform_log_sink",
		Domain: "operations", Title: "Log Sink",
		Description: "Portable destination that retains structured application logs.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "retention_days", Wire: "retentionDays", Type: TypeInt, Min: i64(1),
				Doc: "Optional log retention in days.", Example: 30, CounterExample: 0},
			{HCL: "format", Wire: "format", Type: TypeString, Default: "json", Enum: []string{"json", "text"},
				Doc: "Record format the sink accepts.", Example: "json"},
		},
		Interfaces: []Interface{{Name: "log.ingest", Description: "Portable log ingest and read operations.", Operations: []string{"query", "write"}}},
	},
	{
		Kind: "MetricSink", Slug: "metric-sink", ResourceType: "takoform_metric_sink",
		Domain: "operations", Title: "Metric Sink",
		Description: "Portable destination that retains numeric time series.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "retention_days", Wire: "retentionDays", Type: TypeInt, Min: i64(1),
				Doc: "Optional series retention in days.", Example: 90},
			{HCL: "resolution_seconds", Wire: "resolutionSeconds", Type: TypeInt, Min: i64(1),
				Doc: "Optional smallest retained sample interval in seconds.", Example: 60,
				CounterExample: 0},
		},
		Interfaces: []Interface{{Name: "metric.ingest", Description: "Portable metric ingest and read operations.", Operations: []string{"query", "write"}}},
	},
	{
		Kind: "EmailSender", Slug: "email-sender", ResourceType: "takoform_email_sender",
		Domain: "operations", Title: "Email Sender",
		Description: "Portable outbound mail identity for one verified domain.",
		Fields: []Field{
			{HCL: "domain", Wire: "domain", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarDomain,
				Doc:     "Domain the host verifies before it accepts outbound mail.",
				Example: "portable-conformance.invalid", CounterExample: "not a domain", AltExample: "alt.portable-conformance.invalid"},
			{HCL: "default_sender", Wire: "defaultSender", Type: TypeString, Grammar: GrammarMailbox,
				Doc:        "Optional default sender mailbox inside the verified domain.",
				Example:    "notifications@portable-conformance.invalid",
				AltExample: "alerts@portable-conformance.invalid"},
		},
		Interfaces: []Interface{{Name: "email.send", Description: "Portable outbound mail operations.", Operations: []string{"send", "status"}}},
	},
	{
		Kind: "WebhookEndpoint", Slug: "webhook-endpoint", ResourceType: "takoform_webhook_endpoint",
		Domain: "operations", Title: "Webhook Endpoint",
		Description:       "Portable inbound HTTP endpoint that forwards received requests to one connected Resource.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("destination", "Queue/queue", []any{"send"}, "queue.producer.v1"),
		Fields: []Field{
			{HCL: "path", Wire: "path", Type: TypeString, Grammar: GrammarPath, Default: "/",
				Doc: "Absolute path the endpoint accepts requests on.", Example: "/hooks"},
			{HCL: "allowed_methods", Wire: "allowedMethods", Type: TypeStringSet, MinItems: 1,
				Enum: []string{"DELETE", "GET", "PATCH", "POST", "PUT"},
				Doc:  "HTTP methods the endpoint accepts.", Example: []any{"POST"},
				CounterExample: []any{"TRACE"}},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by a webhook endpoint.", Operations: []string{"request"}}},
	},
	{
		Kind: "IdentityClient", Slug: "identity-client", ResourceType: "takoform_identity_client",
		Domain: "operations", Title: "Identity Client",
		Description: "Portable OIDC relying-party registration. Issued client material stays with the host.",
		Fields: []Field{
			{HCL: "redirect_uris", Wire: "redirectUris", Type: TypeStringSet, Required: true, MinItems: 1, Grammar: GrammarHTTPSURL,
				Doc:            "Absolute https redirect URIs the client may return to.",
				Example:        []any{"https://app.portable-conformance.invalid/callback"},
				CounterExample: []any{"http://app.portable-conformance.invalid/callback"}},
			{HCL: "grant_types", Wire: "grantTypes", Type: TypeStringSet, MinItems: 1,
				Enum: []string{"authorization_code", "client_credentials", "refresh_token"},
				Doc:  "Grant types the client is registered for.", Example: []any{"authorization_code"}},
			{HCL: "auth_method", Wire: "authMethod", Type: TypeString, Default: "none",
				Enum:    []string{"none", "basic", "jwt"},
				Doc:     "How the client authenticates at the token endpoint. The host issues and holds any material this implies.",
				Example: "none"},
		},
		Interfaces: []Interface{{Name: "identity.oidc", Description: "Portable OIDC relying-party metadata operations.", Operations: []string{"metadata"}}},
	},
	{
		Kind: "FeatureFlag", Slug: "feature-flag", ResourceType: "takoform_feature_flag",
		Domain: "operations", Title: "Feature Flag",
		Description: "Portable named runtime switch with an optional percentage rollout.",
		Fields: []Field{
			{HCL: "flag_key", Wire: "flagKey", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarToken,
				Doc: "Stable key applications evaluate. Changing it replaces the flag.", Example: "new_checkout",
				CounterExample: "not a key", AltExample: "new_checkout_v2"},
			{HCL: "enabled", Wire: "enabled", Type: TypeBool, Required: true,
				Doc: "Whether the flag evaluates true by default.", Example: true},
			{HCL: "rollout_percentage", Wire: "rolloutPercentage", Type: TypeInt, Min: i64(0), Max: i64(100),
				Doc: "Optional share of evaluations that receive the enabled value.", Example: 25},
		},
		Interfaces: []Interface{{Name: "flag.evaluate", Description: "Portable feature flag evaluation operations.", Operations: []string{"evaluate"}}},
	},
	{
		Kind: "RateLimitPolicy", Slug: "rate-limit-policy", ResourceType: "takoform_rate_limit_policy",
		Domain: "operations", Title: "Rate Limit Policy",
		Description:       "Portable request budget applied to one connected Resource.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("subject", "HttpRoute/http-route", []any{"administer"}, "http.route.v1"),
		Fields: []Field{
			{HCL: "requests_per_minute", Wire: "requestsPerMinute", Type: TypeInt, Required: true, Min: i64(1),
				Doc: "Sustained request budget per minute.", Example: 600, CounterExample: 0},
			{HCL: "burst", Wire: "burst", Type: TypeInt, Min: i64(0),
				Doc: "Optional additional requests tolerated above the sustained budget.", Example: 100},
			{HCL: "scope", Wire: "scope", Type: TypeString, Default: "client", Enum: []string{"client", "route"},
				Doc:     "Whether the budget is counted per calling client or across the whole target.",
				Example: "client"},
		},
	},
	{
		Kind: "BackupPolicy", Slug: "backup-policy", ResourceType: "takoform_backup_policy",
		Domain: "operations", Title: "Backup Policy",
		Description:       "Portable scheduled copy and retention rule for one connected Resource.",
		Connections:       ConnectionsRequired,
		ConnectionExample: connection("origin", "RelationalDatabase/relational-database", []any{"administer"}, "sql.admin.v1"),
		Fields: []Field{
			{HCL: "cron", Wire: "cron", Type: TypeString, Required: true, Grammar: GrammarCron,
				Doc:     "Portable five-field cron expression describing when copies are taken.",
				Example: "0 3 * * *", CounterExample: "0 3 * *"},
			{HCL: "retention_days", Wire: "retentionDays", Type: TypeInt, Required: true, Min: i64(1),
				Doc: "How long each copy is retained, in days.", Example: 14},
			{HCL: "timezone", Wire: "timezone", Type: TypeString, Grammar: GrammarTimezone, Default: "UTC",
				Doc: "Open timezone token the schedule is interpreted in.", Example: "UTC"},
		},
	},
}

func connection(name, resource string, permissions []any, projection string) map[string]any {
	return map[string]any{name: map[string]any{
		"resource": resource, "permissions": permissions, "projection": projection,
	}}
}

// ByKind returns the declared Form for a portable kind token.
func ByKind(kind string) (Kind, bool) {
	for _, candidate := range Kinds {
		if candidate.Kind == kind {
			return candidate, true
		}
	}
	return Kind{}, false
}

// ByResourceType returns the declared Form for a Terraform resource type.
func ByResourceType(resourceType string) (Kind, bool) {
	for _, candidate := range Kinds {
		if candidate.ResourceType == resourceType {
			return candidate, true
		}
	}
	return Kind{}, false
}

// KindTokens lists every portable kind token in declaration order.
func KindTokens() []string {
	tokens := make([]string, 0, len(Kinds))
	for _, candidate := range Kinds {
		tokens = append(tokens, candidate.Kind)
	}
	return tokens
}

// DefaultDefinitionVersion is the SemVer of a Form whose kind token carries no
// earlier published identity.
const DefaultDefinitionVersion = "1.0.0"

// Version returns the SemVer of this Form's definition and package.
func (k Kind) Version() string {
	if k.DefinitionVersion != "" {
		return k.DefinitionVersion
	}
	return DefaultDefinitionVersion
}
