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
	// TypeStringMap is a non-secret configuration map. Its keys are checked
	// with the same data-only forbidden-field policy as declared fields, so a
	// credential cannot be smuggled in as configuration.
	TypeStringMap FieldType = "string-map"
)

// Grammar names a reviewed portable string grammar. Free-form strings carry
// no grammar; every other value is constrained so hosts can agree on meaning.
type Grammar string

const (
	GrammarNone             Grammar = ""
	GrammarToken            Grammar = "token"    // open capability token
	GrammarVersion          Grammar = "version"  // opaque portable version token
	GrammarClass            Grammar = "class"    // runtime class identifier
	GrammarTimezone         Grammar = "timezone" // IANA-style zone token
	GrammarCron             Grammar = "cron"     // five-field cron expression
	GrammarHostname         Grammar = "hostname" // DNS hostname
	GrammarDomain           Grammar = "domain"   // DNS domain name
	GrammarPath             Grammar = "path"     // absolute URL path
	GrammarCIDR             Grammar = "cidr"     // IPv4/IPv6 address block
	GrammarDNSRelativeName  Grammar = "dns-relative-name"
	GrammarOCIDigest        Grammar = "oci-digest" // OCI reference pinned by digest
	GrammarMailbox          Grammar = "mailbox"    // email address
	GrammarMailboxLocalPart Grammar = "mailbox-local-part"
	GrammarHTTPSURL         Grammar = "https-url" // general absolute https URL; query and fragment permitted
	// GrammarCredentialFreeHTTPSURL is an absolute HTTPS URL safe to retain
	// in nonsensitive state. Userinfo, query, and fragment are forbidden.
	GrammarCredentialFreeHTTPSURL Grammar = "credential-free-https-url"
	GrammarRecordData             Grammar = "record-data" // DNS record value
	GrammarRelativePath           Grammar = "relative-path"
	// GrammarSHA256 is a bare or sha256-prefixed 64-hex digest. It binds a
	// fetched URL to exact immutable bytes the same way an artifact source does.
	GrammarSHA256 Grammar = "sha256"
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
	// CounterExample is a value that must be rejected. Every field that states
	// a constraint should carry one: each becomes its own negative fixture, so
	// a constraint a Form declares is a constraint something proves.
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

// ConditionalConstraint binds cross-field meaning into the portable JSON
// Schema. It is intentionally small: Forms can specialize a string-set item
// grammar or forbid fields for selected discriminator values without relying
// on a host-only validator.
type ConditionalConstraint struct {
	Name                  string
	WhenField             string
	WhenValues            []string
	ForbidFields          []string
	StringSetItemPatterns map[string]string
	StringSetMaxItems     map[string]int
	// CounterExample overlays the canonical desired state to prove this
	// condition is enforced by package and host conformance.
	CounterExample map[string]any
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
	// MaxConnections bounds connection cardinality when the Form's intent is
	// singular. Zero means unbounded; ConnectionsRequired always means at least
	// one. JSON and provider projections enforce the same limits.
	MaxConnections int
	// Artifact adds the portable digest-bound HTTPS artifact source.
	Artifact bool
	// ArtifactExample is the canonical fixture value for that source.
	ArtifactExample map[string]any
	// ConnectionExample is the canonical fixture value for connections.
	ConnectionExample map[string]any

	Interfaces  []Interface
	Constraints []ConditionalConstraint
}

func i64(value int64) *int64 { return &value }

// Kinds is the complete portable Form set of this release, in a stable order.
var Kinds = []Kind{
	// ---------------------------------------------------------------- compute
	{
		Kind: "EdgeWorker", DefinitionVersion: "3.0.0", Slug: "edge-worker", ResourceType: "takoform_edge_worker",
		Domain: "compute", Title: "Edge Worker",
		Description: "Portable edge/event-driven application served from a prebuilt immutable artifact.",
		Artifact:    true, Connections: ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/edge-worker.tar",
			"artifactSha256":    "0f2c0c7ec3d0e2f34f1ea1f6b5f04f0b3aa03d0e6f2f2f8a7f0c5d9e4b1a8c37",
			"artifactMediaType": "application/vnd.takoform.edge-worker+tar",
		},
		ConnectionExample: connection("assets", "ObjectBucket/object-bucket", []any{"read"}, "object.binding.v1"),
		Fields: []Field{
			{HCL: "entrypoint", Wire: "entrypoint", Type: TypeString, Required: true, Grammar: GrammarRelativePath,
				Doc: "Relative path of the artifact module the edge runtime starts.", Example: "worker.mjs",
				CounterExample: "../worker.mjs"},
			{HCL: "runtime", Wire: "runtime", Type: TypeString, Grammar: GrammarToken,
				Doc:     "Open runtime capability token the artifact expects. The configured host decides support.",
				Example: "javascript", AltExample: "python"},
			{HCL: "runtime_version", Wire: "runtimeVersion", Type: TypeString, Grammar: GrammarVersion,
				Doc: "Optional runtime version requested for the artifact.", Example: "2026.1"},
			{HCL: "request_timeout_seconds", Wire: "requestTimeoutSeconds", Type: TypeInt, Min: i64(1), Max: i64(3600),
				Doc: "Optional per-request timeout preference in seconds.", Example: 30},
			{HCL: "concurrency", Wire: "concurrency", Type: TypeInt, Min: i64(1),
				Doc: "Optional concurrent-request preference.", Example: 100, CounterExample: 0},
			{HCL: "configuration", Wire: "configuration", Type: TypeStringMap,
				Doc:     "Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.",
				Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by an edge application.", Operations: []string{"request"}}},
	},
	{
		Kind: "ContainerService", DefinitionVersion: "3.0.0", Slug: "container-service", ResourceType: "takoform_container_service",
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
				Doc: "Optional CPU request in millicores.", Example: 250},
			{HCL: "memory_mib", Wire: "memoryMib", Type: TypeInt, Min: i64(1),
				Doc: "Optional memory request in mebibytes.", Example: 512},
			{HCL: "replicas", Wire: "replicas", Type: TypeInt, Min: i64(1),
				Doc: "Requested number of identical running instances.", Example: 2},
			{HCL: "health_check_path", Wire: "healthCheckPath", Type: TypeString, Grammar: GrammarPath,
				Doc: "Optional HTTP path a host polls to decide whether an instance is serving.", Example: "/healthz"},
			{HCL: "configuration", Wire: "configuration", Type: TypeStringMap,
				Doc:     "Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.",
				Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by a container service.", Operations: []string{"request"}}},
	},
	{
		Kind: "ComputeInstance", DefinitionVersion: "2.0.0", Slug: "compute-instance", ResourceType: "takoform_compute_instance",
		Domain: "compute", Title: "Compute Instance",
		Description: "Portable long-running machine instance built from digest-bound boot-image bytes.",
		Artifact:    true, Connections: ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/base-linux.qcow2",
			"artifactSha256":    "a6cbb7e295a8dd89b98f3b8c731047e0e62a4312b8b43c590a8c8662df59e913",
			"artifactMediaType": "application/x-qemu-disk",
		},
		Fields: []Field{
			{HCL: "machine_class", Wire: "machineClass", Type: TypeString, Required: true, Grammar: GrammarToken,
				Doc: "Open machine capability token describing the requested size class.", Example: "general.small",
				CounterExample: "not a token"},
			{HCL: "boot_disk_gib", Wire: "bootDiskGib", Type: TypeInt, Required: true, Min: i64(1),
				Doc: "Boot disk size in gibibytes.", Example: 20},
			{HCL: "instance_count", Wire: "instanceCount", Type: TypeInt, Min: i64(1),
				Doc: "Optional identical-instance count preference.", Example: 1},
			{HCL: "configuration", Wire: "configuration", Type: TypeStringMap,
				Doc:     "Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.",
				Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
	},
	{
		Kind: "StaticSite", DefinitionVersion: "2.0.0", Slug: "static-site", ResourceType: "takoform_static_site",
		Domain: "compute", Title: "Static Site",
		Description: "Portable static asset site served from a prebuilt immutable artifact.",
		Artifact:    true,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/static-site.tar",
			"artifactSha256":    "3b1d4c2f9a8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a392817065f4e",
			"artifactMediaType": "application/vnd.takoform.static-site+tar",
		},
		Fields: []Field{
			{HCL: "index_document", Wire: "indexDocument", Type: TypeString, Default: "index.html", Grammar: GrammarRelativePath,
				Doc: "Document served for a directory request.", Example: "index.html"},
			{HCL: "error_document", Wire: "errorDocument", Type: TypeString, Grammar: GrammarRelativePath,
				Doc: "Optional document served for a not-found request.", Example: "404.html"},
			{HCL: "single_page_app", Wire: "singlePageApp", Type: TypeBool,
				Doc: "Whether unmatched paths should serve the index document.", Example: false},
			{HCL: "cache_control_seconds", Wire: "cacheControlSeconds", Type: TypeInt, Min: i64(0),
				Doc:     "Optional freshness lifetime advertised for served assets, in seconds.",
				Example: 300, CounterExample: -1},
		},
		Interfaces: []Interface{{Name: "http.request", Description: "Portable HTTP request surface exposed by a static site.", Operations: []string{"request"}}},
	},
	{
		Kind: "Workflow", DefinitionVersion: "2.0.0", Slug: "workflow", ResourceType: "takoform_workflow",
		Domain: "compute", Title: "Workflow",
		Description: "Portable durable workflow definition and instance-state lifecycle.",
		Artifact:    true, Connections: ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/workflow.mjs",
			"artifactSha256":    "8712e09089276b497669472eddc0aa425c6fa2bf766037f7351690a3517d5ac5",
			"artifactMediaType": "text/javascript",
		},
		Fields: []Field{
			{HCL: "entrypoint", Wire: "entrypoint", Type: TypeString, Required: true, Grammar: GrammarClass,
				Doc: "Workflow runtime entrypoint.", Example: "IngestWorkflow"},
			{HCL: "max_attempts", Wire: "maxAttempts", Type: TypeInt, Min: i64(1),
				Doc: "Optional maximum attempts per workflow run.", Example: 3, CounterExample: 0},
			{HCL: "initial_backoff_seconds", Wire: "initialBackoffSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional initial retry backoff in seconds.", Example: 5},
			{HCL: "configuration", Wire: "configuration", Type: TypeStringMap,
				Doc:     "Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.",
				Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []Interface{{Name: "workflow.invoke", Description: "Portable durable workflow invocation operations.", Operations: []string{"cancel", "invoke", "status"}}},
	},
	{
		Kind: "StatefulEntity", DefinitionVersion: "3.0.0", Slug: "stateful-entity", ResourceType: "takoform_stateful_entity",
		Domain: "compute", Title: "Stateful Entity",
		Description: "Portable namespace of addressable persistent entities implemented by digest-bound application bytes.",
		Artifact:    true, Connections: ConnectionsOptional,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/stateful-entity.tar",
			"artifactSha256":    "5d877f919bf8db6e6fd819e32f74dff6fc94b06f8914fa1abf5bcd2fb32ae958",
			"artifactMediaType": "application/vnd.takoform.stateful-entity+tar",
		},
		Fields: []Field{
			{HCL: "entity_class", Wire: "entityClass", Type: TypeString, Required: true, Grammar: GrammarClass,
				Doc: "Runtime class identifier owning entity behaviour inside this namespace.", Example: "RoomEntity",
				CounterExample: "not a class"},
			{HCL: "runtime", Wire: "runtime", Type: TypeString, Required: true, Grammar: GrammarToken,
				Doc: "Open runtime capability token required by the entity artifact.", Example: "javascript",
				CounterExample: "not a runtime"},
			{HCL: "runtime_version", Wire: "runtimeVersion", Type: TypeString, Grammar: GrammarVersion,
				Doc: "Optional runtime-version capability token requested for the artifact.", Example: "2026.1"},
			{HCL: "persistence", Wire: "persistence", Type: TypeString, Grammar: GrammarToken,
				Doc: "Open persistence capability token requested for entity state.", Example: "transactional"},
			{HCL: "migration_tag", Wire: "migrationTag", Type: TypeString, Grammar: GrammarVersion,
				Doc: "Optional namespace migration tag. It never identifies one entity instance.", Example: "v1",
				AltExample: "v2"},
			{HCL: "configuration", Wire: "configuration", Type: TypeStringMap,
				Doc:     "Non-secret configuration passed to the running service. Secret material is never portable state: a host injects it through its own credential path.",
				Example: map[string]any{"LOG_LEVEL": "info"}, AltExample: map[string]any{"LOG_LEVEL": "debug"}},
		},
		Interfaces: []Interface{{Name: "entity.invoke", Description: "Portable stateful entity invocation operations.", Operations: []string{"invoke"}}},
	},
	{
		Kind: "Schedule", DefinitionVersion: "3.0.0", Slug: "schedule", ResourceType: "takoform_schedule",
		Domain: "compute", Title: "Schedule",
		Description:       "Portable cron lifecycle that invokes exactly one connected Resource.",
		Connections:       ConnectionsRequired,
		MaxConnections:    1,
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
		Kind: "ObjectBucket", DefinitionVersion: "3.0.0", Slug: "object-bucket", ResourceType: "takoform_object_bucket",
		Domain: "data", Title: "Object Bucket",
		Description: "Portable object storage with a portable default storage class.",
		Fields: []Field{
			{HCL: "storage_class", Wire: "storageClass", Type: TypeString, Default: "standard",
				Enum: []string{"standard", "infrequent_access", "archive"},
				Doc:  "Portable default storage class for newly written objects.", Example: "standard",
				CounterExample: "cold"},
			{HCL: "versioning", Wire: "versioning", Type: TypeBool,
				Doc: "Whether the bucket should retain non-current object versions.", Example: true},
			{HCL: "access_protocols", Wire: "accessProtocols", Type: TypeStringSet, Grammar: GrammarToken,
				Doc: "Optional access-protocol capability tokens requested from the host.", Example: []any{"s3_api"}},
		},
		Interfaces: []Interface{{Name: "object.storage", Description: "Portable object storage operations.", Operations: []string{"delete", "get", "list", "put"}}},
	},
	{
		Kind: "ObjectLifecycleRule", Slug: "object-lifecycle-rule", ResourceType: "takoform_object_lifecycle_rule",
		Domain: "data", Title: "Object Lifecycle Rule",
		Description:       "One portable expiration or storage-transition action applied to a connected object store.",
		Connections:       ConnectionsRequired,
		MaxConnections:    1,
		ConnectionExample: connection("store", "ObjectBucket/object-bucket", []any{"administer"}, "object.lifecycle.v1"),
		Fields: []Field{
			{HCL: "prefix", Wire: "prefix", Type: TypeString,
				Doc: "Optional key prefix the rule applies to.", Example: "logs/"},
			{HCL: "action", Wire: "action", Type: TypeString, Required: true,
				Enum: []string{"expire", "transition_infrequent_access", "transition_archive"},
				Doc:  "Single action performed when matching objects reach after_days.", Example: "expire",
				CounterExample: "transition"},
			{HCL: "after_days", Wire: "afterDays", Type: TypeInt, Required: true, Min: i64(1),
				Doc: "Age in days at which the declared action is performed.", Example: 90,
				CounterExample: 0},
		},
	},
	{
		Kind: "KeyValueStore", DefinitionVersion: "2.0.0", Slug: "key-value-store", ResourceType: "takoform_key_value_store",
		Domain: "data", Title: "Key Value Store",
		Description: "Portable key/value state with an optional consistency preference.",
		Fields: []Field{
			{HCL: "consistency", Wire: "consistency", Type: TypeString, Enum: []string{"eventual", "strong"},
				Doc: "Optional consistency preference.", Example: "eventual", CounterExample: "linearizable"},
			{HCL: "default_ttl_seconds", Wire: "defaultTtlSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional default entry lifetime in seconds.", Example: 3600},
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
				Doc: "Optional default entry lifetime in seconds.", Example: 300},
		},
		Interfaces: []Interface{{Name: "cache.store", Description: "Portable cache operations.", Operations: []string{"delete", "get", "put"}}},
	},
	{
		Kind: "RelationalDatabase", DefinitionVersion: "3.0.0", Slug: "relational-database", ResourceType: "takoform_relational_database",
		Domain: "data", Title: "Relational Database",
		Description: "Portable relational database addressed through an open engine capability token.",
		Fields: []Field{
			{HCL: "engine", Wire: "engine", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarToken,
				Doc: "Open engine capability token. Changing it replaces the database.", Example: "postgres",
				CounterExample: "not a token", AltExample: "mysql"},
			{HCL: "engine_version", Wire: "engineVersion", Type: TypeString, Grammar: GrammarVersion,
				Doc: "Optional engine version requested from the host.", Example: "16", AltExample: "17"},
			{HCL: "storage_gib", Wire: "storageGib", Type: TypeInt, Min: i64(1),
				Doc: "Optional storage request in gibibytes.", Example: 20},
			{HCL: "size_class", Wire: "sizeClass", Type: TypeString, Grammar: GrammarToken,
				Doc: "Open capability token describing the requested compute size.", Example: "db.small"},
			{HCL: "database_name", Wire: "databaseName", Type: TypeString, Immutable: true, Grammar: GrammarToken,
				Doc:     "Initial logical database created inside the instance. Changing it replaces the database.",
				Example: "app", AltExample: "app_v2"},
			{HCL: "high_availability", Wire: "highAvailability", Type: TypeBool,
				Doc: "Whether the host should keep a standby able to take over.", Example: false},
			// A database that declares a schema is converged to it during apply.
			// The bundle is one immutable document carrying its migrations in
			// order: naming a package registry here would put that registry in
			// the portable contract, so a host unable to reach it could not
			// apply a schema its own plan called applicable.
			{HCL: "schema_url", Wire: "schemaUrl", Type: TypeString, Grammar: GrammarCredentialFreeHTTPSURL,
				Doc:            "Optional immutable migration bundle the host applies in order. Userinfo, query, and fragment are forbidden because this value persists in nonsensitive state.",
				Example:        "https://artifacts.portable-conformance.invalid/relational-schema.json",
				AltExample:     "https://artifacts.portable-conformance.invalid/relational-schema-2.json",
				CounterExample: "https://schema.invalid/bundle.json?download=1"},
			{HCL: "schema_sha256", Wire: "schemaSha256", Type: TypeString, Grammar: GrammarSHA256,
				Doc:            "Digest binding schema_url to exact immutable bytes.",
				Example:        "1d2181e213a086ae9e025d235ff5e267c43ec60cf4fc2f966977a21f2a95ef7b",
				AltExample:     "3b3f36501936a84ed19b9bef37e5581c3e04948733b947ebaa002f196e66817c",
				CounterExample: "not-a-sha256"},
			{HCL: "schema_format", Wire: "schemaFormat", Type: TypeString, Grammar: GrammarToken,
				Doc:     "Open capability token naming how the bundle is interpreted.",
				Example: "takosumi.resource-migrations", AltExample: "sql.ordered",
				CounterExample: "not a token"},
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
		Kind: "Queue", DefinitionVersion: "3.0.0", Slug: "queue", ResourceType: "takoform_queue",
		Domain: "data", Title: "Queue",
		Description: "Portable asynchronous delivery with at-least-once semantics.",
		Connections: ConnectionsOptional,
		Fields: []Field{
			{HCL: "max_retries", Wire: "maxRetries", Type: TypeInt, Min: i64(0),
				Doc: "Optional delivery retry preference.", Example: 5, CounterExample: -1},
			{HCL: "max_batch_size", Wire: "maxBatchSize", Type: TypeInt, Min: i64(1),
				Doc: "Optional consumer batch size preference.", Example: 10},
			{HCL: "visibility_timeout_seconds", Wire: "visibilityTimeoutSeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional time a received message stays invisible to other consumers.", Example: 30},
			{HCL: "message_retention_seconds", Wire: "messageRetentionSeconds", Type: TypeInt, Min: i64(1),
				Doc: "Optional time an unacknowledged message is retained, in seconds.", Example: 345600},
			{HCL: "max_message_bytes", Wire: "maxMessageBytes", Type: TypeInt, Min: i64(1),
				Doc: "Optional largest accepted message size in bytes.", Example: 262144},
			{HCL: "delivery_delay_seconds", Wire: "deliveryDelaySeconds", Type: TypeInt, Min: i64(0),
				Doc: "Optional delay before a sent message becomes receivable.", Example: 0},
			{HCL: "ordering", Wire: "ordering", Type: TypeString, Enum: []string{"best_effort", "strict"},
				Doc: "Whether the host must preserve send order.", Example: "best_effort"},
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
		Kind: "VectorIndex", DefinitionVersion: "3.0.0", Slug: "vector-index", ResourceType: "takoform_vector_index",
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
		Kind: "ModelEndpoint", DefinitionVersion: "3.0.0", Slug: "model-endpoint", ResourceType: "takoform_model_endpoint",
		Domain: "analytics", Title: "Model Endpoint",
		Description: "Portable inference endpoint serving digest-bound model bytes for one declared task.",
		Artifact:    true,
		ArtifactExample: map[string]any{
			"artifactUrl":       "https://artifacts.portable-conformance.invalid/embedding-small.safetensors",
			"artifactSha256":    "fd52f6d3dfaa989615128f2049893584cc6f71a4ae5536b86681ae33ae2c072b",
			"artifactMediaType": "application/vnd.safetensors",
		},
		Fields: []Field{
			{HCL: "task", Wire: "task", Type: TypeString, Required: true, Grammar: GrammarToken,
				Doc: "Open task capability token, for example text_generation or embedding.", Example: "embedding",
				CounterExample: "not a token", AltExample: "text_generation"},
			{HCL: "max_concurrency", Wire: "maxConcurrency", Type: TypeInt, Min: i64(1),
				Doc: "Optional concurrent-inference preference.", Example: 8},
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
		MaxConnections:    1,
		ConnectionExample: connection("parent", "DnsZone/primary", []any{"administer"}, "dns.zone.v1"),
		Fields: []Field{
			{HCL: "record_name", Wire: "recordName", Type: TypeString, Required: true, Grammar: GrammarDNSRelativeName,
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
		Constraints: []ConditionalConstraint{
			{Name: "dns-a-values", WhenField: "recordType", WhenValues: []string{"A"}, StringSetItemPatterns: map[string]string{"values": PatternIPv4Address},
				CounterExample: map[string]any{"recordType": "A", "values": []any{"999.999.999.999"}}},
			{Name: "dns-aaaa-values", WhenField: "recordType", WhenValues: []string{"AAAA"}, StringSetItemPatterns: map[string]string{"values": PatternIPv6Address},
				CounterExample: map[string]any{"recordType": "AAAA", "values": []any{"2001:::10"}}},
			{Name: "dns-hostname-values", WhenField: "recordType", WhenValues: []string{"CNAME", "NS"}, StringSetItemPatterns: map[string]string{"values": PatternHostname},
				CounterExample: map[string]any{"recordType": "CNAME", "values": []any{"not a hostname"}}},
			{Name: "dns-cname-cardinality", WhenField: "recordType", WhenValues: []string{"CNAME"}, StringSetMaxItems: map[string]int{"values": 1},
				CounterExample: map[string]any{"recordType": "CNAME", "values": []any{"one.portable-conformance.invalid", "two.portable-conformance.invalid"}}},
			{Name: "dns-mx-values", WhenField: "recordType", WhenValues: []string{"MX"}, StringSetItemPatterns: map[string]string{"values": PatternDNSMX},
				CounterExample: map[string]any{"recordType": "MX", "values": []any{"mail.portable-conformance.invalid"}}},
			{Name: "dns-srv-values", WhenField: "recordType", WhenValues: []string{"SRV"}, StringSetItemPatterns: map[string]string{"values": PatternDNSSRV},
				CounterExample: map[string]any{"recordType": "SRV", "values": []any{"443 service.portable-conformance.invalid"}}},
			{Name: "dns-caa-values", WhenField: "recordType", WhenValues: []string{"CAA"}, StringSetItemPatterns: map[string]string{"values": PatternDNSCAA},
				CounterExample: map[string]any{"recordType": "CAA", "values": []any{"issue ca.portable-conformance.invalid"}}},
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
		MaxConnections:    1,
		ConnectionExample: connection("application", "EdgeWorker/edge-worker", []any{"request"}, "http.route.v1"),
		Fields: []Field{
			{HCL: "hostname", Wire: "hostname", Type: TypeString, Required: true, Grammar: GrammarHostname,
				Doc: "Hostname this route answers.", Example: "api.portable-conformance.invalid",
				CounterExample: "not a hostname"},
			{HCL: "path_prefix", Wire: "pathPrefix", Type: TypeString, Grammar: GrammarPath, Default: "/",
				Doc: "Absolute path prefix this route matches.", Example: "/", AltExample: "/api"},
			{HCL: "strip_path_prefix", Wire: "stripPathPrefix", Type: TypeBool,
				Doc: "Whether the matched prefix is removed before the request reaches the target.", Example: false},
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
				Doc:  "Listener protocol.", Example: "https", CounterExample: "smtp",
				AltExample: "http"},
			{HCL: "listen_port", Wire: "listenPort", Type: TypeInt, Required: true, Min: i64(1), Max: i64(65535),
				Doc: "Port the listener accepts connections on.", Example: 443},
			{HCL: "health_check_path", Wire: "healthCheckPath", Type: TypeString, Grammar: GrammarPath,
				Doc: "Optional HTTP path polled to decide backend health.", Example: "/healthz"},
			{HCL: "internal", Wire: "internal", Type: TypeBool,
				Doc: "Whether the listener is reachable only from inside the host's private network.", Example: false},
			{HCL: "idle_timeout_seconds", Wire: "idleTimeoutSeconds", Type: TypeInt, Min: i64(1), Max: i64(4000),
				Doc: "Optional time an idle connection is held open, in seconds.", Example: 60},
		},
		Constraints: []ConditionalConstraint{{
			Name: "non-http-health-path", WhenField: "protocol", WhenValues: []string{"tcp", "udp"},
			ForbidFields:   []string{"healthCheckPath"},
			CounterExample: map[string]any{"protocol": "tcp"},
		}},
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
			{HCL: "default_local_part", Wire: "defaultLocalPart", Type: TypeString, Grammar: GrammarMailboxLocalPart,
				Doc:        "Optional local part combined with domain to form the default sender mailbox.",
				Example:    "notifications",
				AltExample: "alerts"},
		},
		Interfaces: []Interface{{Name: "email.send", Description: "Portable outbound mail operations.", Operations: []string{"send", "status"}}},
	},
	{
		Kind: "WebhookEndpoint", Slug: "webhook-endpoint", ResourceType: "takoform_webhook_endpoint",
		Domain: "operations", Title: "Webhook Endpoint",
		Description:       "Portable inbound HTTP endpoint that forwards received requests to one connected Resource.",
		Connections:       ConnectionsRequired,
		MaxConnections:    1,
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
			{HCL: "auth_method", Wire: "authMethod", Type: TypeString, Default: "none",
				Enum:    []string{"none", "client_secret_basic", "private_key_jwt"},
				Doc:     "How this authorization-code client authenticates at the token endpoint. The host issues and holds any material this implies.",
				Example: "none"},
		},
		Interfaces: []Interface{{Name: "identity.oidc", Description: "Portable OIDC relying-party metadata operations.", Operations: []string{"metadata"}}},
	},
	{
		Kind: "FeatureFlag", Slug: "feature-flag", ResourceType: "takoform_feature_flag",
		Domain: "operations", Title: "Feature Flag",
		Description: "Portable named runtime switch expressed as one complete enabled percentage.",
		Fields: []Field{
			{HCL: "flag_key", Wire: "flagKey", Type: TypeString, Required: true, Immutable: true, Grammar: GrammarToken,
				Doc: "Stable key applications evaluate. Changing it replaces the flag.", Example: "new_checkout",
				CounterExample: "not a key", AltExample: "new_checkout_v2"},
			{HCL: "enabled_percentage", Wire: "enabledPercentage", Type: TypeInt, Required: true, Min: i64(0), Max: i64(100),
				Doc: "Share of stable evaluation subjects receiving true; 0 is always false and 100 is always true.", Example: 25},
		},
		Interfaces: []Interface{{Name: "flag.evaluate", Description: "Portable feature flag evaluation operations.", Operations: []string{"evaluate"}}},
	},
	{
		Kind: "RateLimitPolicy", Slug: "rate-limit-policy", ResourceType: "takoform_rate_limit_policy",
		Domain: "operations", Title: "Rate Limit Policy",
		Description:       "Portable request budget applied to one connected Resource.",
		Connections:       ConnectionsRequired,
		MaxConnections:    1,
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
		MaxConnections:    1,
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
