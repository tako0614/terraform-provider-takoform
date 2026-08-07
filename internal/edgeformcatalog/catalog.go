// Package edgeformcatalog declares, as data, the Edge Platform Family:
// the first official Form Family (edge.forms.takoform.com/v1alpha1, decision
// 0009) together with its exact Interface and Binding contracts (decision
// 0010). Every member fixes the application-visible semantics of one proven
// edge service primitive completely (decision 0008); no field selects a
// vendor, an account, a region, or an implementation.
package edgeformcatalog

import (
	"fmt"

	model "github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

// Family is the Edge Platform Family group.
var Family = model.Family{Group: "edge.forms.takoform.com", Version: "v1alpha1"}

// edgeDefinitionVersion is the definition SemVer every MVP member starts at.
const edgeDefinitionVersion = "0.1.0"

// modulePathPattern is a non-escaping relative bundle module path.
const modulePathPattern = model.PatternRelativePath

func moduleWorkerRef(hcl, wire, doc string, required, immutable bool) model.Field {
	return model.Field{
		HCL: hcl, Wire: wire, Kind: model.KindResourceRef, TargetKind: "ModuleWorker",
		Required: required, Immutable: immutable, Doc: doc,
		Example: map[string]any{"kind": "ModuleWorker", "name": "module-worker"},
	}
}

// Forms is the complete Edge Platform Family MVP set, in a stable order.
var Forms = []model.Form{
	{
		Kind: "ModuleWorker", Slug: "module-worker", ResourceType: "takoform_module_worker",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "Module Worker",
		Description: "Long-lived logical identity of one ES Module Worker application. The Form fixes the " +
			"ES module worker ABI by identity: handlers are exported module functions receiving typed " +
			"events and a binding environment. Code, configuration, and bindings live on Worker Version " +
			"revisions; traffic selection lives on Worker Deployments.",
	},
	{
		Kind: "WorkerBundle", Slug: "worker-bundle", ResourceType: "takoform_worker_bundle",
		Role: model.RoleRevision, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Bundle",
		Description: "Immutable content-addressed module bundle of one worker build: a main module plus " +
			"additional modules, each pinned by size and digest and resolved through the content-addressed " +
			"artifact upload API (decision 0012). Different bytes are a different bundle.",
		Fields: []model.Field{
			{HCL: "main_module", Wire: "mainModule", Kind: model.KindString, Required: true,
				Pattern: modulePathPattern, MaxLength: 240,
				Doc:     "Relative path of the ES module the runtime instantiates first. It must name one declared module.",
				Example: "worker.mjs", AltExample: "src/index.mjs", CounterExample: "../worker.mjs"},
			{HCL: "modules", Wire: "modules", Kind: model.KindObjectList, Required: true, MinItems: 1,
				Doc: "Every module of the bundle, exactly as committed to the artifact manifest: relative path, closed media type, size, and sha256 digest.",
				Fields: []model.Field{
					{HCL: "name", Wire: "name", Kind: model.KindString, Required: true,
						Pattern: modulePathPattern, MaxLength: 240,
						Doc: "Relative module path inside the bundle."},
					{HCL: "media_type", Wire: "mediaType", Kind: model.KindStringEnum, Required: true,
						Enum: []string{"application/javascript+module", "application/wasm", "text/plain", "application/octet-stream", "application/source-map+json"},
						Doc:  "Closed media type deciding how the runtime links the module."},
					{HCL: "size", Wire: "size", Kind: model.KindInteger, Required: true,
						Min: model.I64(0), Max: model.I64(268435456),
						Doc: "Exact module size in bytes."},
					{HCL: "digest", Wire: "digest", Kind: model.KindString, Required: true,
						Pattern: model.PatternCanonicalSHA256,
						Doc:     "Canonical lowercase sha256 digest of the module bytes."},
				},
				Example: []any{map[string]any{
					"name":      "worker.mjs",
					"mediaType": "application/javascript+module",
					"size":      2048,
					"digest":    "sha256:6a5cbf24f5d0c86479ae13b9d1731a626a1729f01aef65403c5c8ac82ed85f43",
				}},
				CounterExample: []any{}},
		},
	},
	{
		Kind: "WorkerVersion", Slug: "worker-version", ResourceType: "takoform_worker_version",
		Role: model.RoleRevision, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Version",
		Description: "Immutable executable snapshot of one Module Worker: a bundle, a runtime compatibility " +
			"date, declared handlers, non-secret vars, and the typed capability bindings the code may use. " +
			"A change is a new Worker Version; traffic moves only through Worker Deployments.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "worker.service", Version: "1.0.0"}},
		AcceptedBindings: []model.BindingRefSource{
			{Name: "module-worker.edge-kv", Version: "1.0.0"},
			{Name: "module-worker.object-bucket", Version: "1.0.0"},
			{Name: "module-worker.sqlite", Version: "1.0.0"},
			{Name: "module-worker.queue-producer", Version: "1.0.0"},
			{Name: "module-worker.service", Version: "1.0.0"},
		},
		Fields: []model.Field{
			{HCL: "worker", Wire: "worker", Kind: model.KindResourceRef, TargetKind: "ModuleWorker", Required: true,
				Doc:     "Module Worker identity this version belongs to.",
				Example: map[string]any{"kind": "ModuleWorker", "name": "module-worker"}},
			{HCL: "bundle", Wire: "bundle", Kind: model.KindResourceRef, TargetKind: "WorkerBundle", Required: true,
				Doc:     "Worker Bundle carrying the exact module bytes this version executes.",
				Example: map[string]any{"kind": "WorkerBundle", "name": "worker-bundle"}},
			{HCL: "compatibility_date", Wire: "compatibilityDate", Kind: model.KindDateString, Required: true,
				Doc:     "Runtime compatibility date fixing default runtime behavior for this version.",
				Example: "2026-08-06", AltExample: "2026-01-01"},
			{HCL: "compatibility_flags", Wire: "compatibilityFlags", Kind: model.KindStringSet,
				Enum:    []string{"nodejs_compat"},
				Default: []any{},
				Doc:     "Closed runtime compatibility flags enabled for this version. Omitting it enables no flag.",
				Example: []any{"nodejs_compat"}},
			{HCL: "handlers", Wire: "handlers", Kind: model.KindStringSet, Required: true, MinItems: 1,
				Enum:    []string{"fetch", "scheduled", "queue", "tail"},
				Doc:     "Module event handlers this version exports. A host rejects an attachment whose event kind is not declared here.",
				Example: []any{"fetch"}},
			{HCL: "vars", Wire: "vars", Kind: model.KindJSONMap,
				Default: map[string]any{},
				Doc: "Non-secret configuration values projected into the module environment. Sensitive material never enters portable state. " +
					"Omitting it projects no variable.",
				Example: map[string]any{"LOG_LEVEL": "info"}},
			{HCL: "kv_bindings", Wire: "kvBindings", Kind: model.KindBindingList,
				TargetKind: "EdgeKVNamespace", BindingType: "module-worker.edge-kv",
				Default: []any{},
				Doc:     "Typed module-worker.edge-kv bindings projecting the edge.kv API under JavaScript identifier names. Omitting it declares no such binding.",
				Example: []any{map[string]any{"name": "CACHE", "resource": map[string]any{"kind": "EdgeKVNamespace", "name": "edge-kv-namespace"}}}},
			{HCL: "bucket_bindings", Wire: "bucketBindings", Kind: model.KindBindingList,
				TargetKind: "ObjectBucket", BindingType: "module-worker.object-bucket",
				Default: []any{},
				Doc:     "Typed module-worker.object-bucket bindings projecting the edge.objects API. Omitting it declares no such binding.",
				Example: []any{map[string]any{"name": "MEDIA", "resource": map[string]any{"kind": "ObjectBucket", "name": "object-bucket"}}}},
			{HCL: "sqlite_bindings", Wire: "sqliteBindings", Kind: model.KindBindingList,
				TargetKind: "SQLiteDatabase", BindingType: "module-worker.sqlite",
				Default: []any{},
				Doc:     "Typed module-worker.sqlite bindings projecting the edge.sql API. Omitting it declares no such binding.",
				Example: []any{map[string]any{"name": "DB", "resource": map[string]any{"kind": "SQLiteDatabase", "name": "sqlite-database"}}}},
			{HCL: "queue_producer_bindings", Wire: "queueProducerBindings", Kind: model.KindBindingList,
				TargetKind: "AtLeastOnceQueue", BindingType: "module-worker.queue-producer",
				Default: []any{},
				Doc:     "Typed module-worker.queue-producer bindings projecting only send and sendBatch. Omitting it declares no such binding.",
				Example: []any{map[string]any{"name": "EVENTS", "resource": map[string]any{"kind": "AtLeastOnceQueue", "name": "at-least-once-queue"}}}},
			{HCL: "service_bindings", Wire: "serviceBindings", Kind: model.KindBindingList,
				TargetKind: "ModuleWorker", BindingType: "module-worker.service",
				Default: []any{},
				Doc:     "Typed module-worker.service bindings projecting worker.service fetch toward another Module Worker. Omitting it declares no such binding.",
				Example: []any{map[string]any{"name": "AUTH", "resource": map[string]any{"kind": "ModuleWorker", "name": "auth-worker"}}}},
			{HCL: "required_sensitive_vars", Wire: "requiredSensitiveVars", Kind: model.KindStringSet,
				ItemPattern: model.PatternSensitiveVarName,
				Default:     []any{},
				Doc: "Names of sensitive values this version requires the host to supply out-of-band. " +
					"Only the names are portable state; values travel through each host's own sealed path. " +
					"Omitting it requires no sensitive value.",
				Example: []any{"API_SIGNING_TOKEN_NAME"}, CounterExample: []any{"lowercase name"}},
		},
	},
	{
		Kind: "WorkerDeployment", Slug: "worker-deployment", ResourceType: "takoform_worker_deployment",
		Role: model.RoleDeployment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Deployment",
		Description: "Selects which Worker Versions of one Module Worker serve traffic and in what " +
			"proportion. Weights are basis points and must sum to exactly 10000 across entries; the sum " +
			"is host-validated semantics because a schema cannot add weights. Rollback is re-weighting, " +
			"never mutating a revision.",
		Fields: []model.Field{
			moduleWorkerRef("worker", "worker", "Module Worker identity whose traffic this deployment governs.", true, true),
			{HCL: "versions", Wire: "versions", Kind: model.KindObjectList, Required: true, MinItems: 1, MaxItems: 8,
				Doc: "Active Worker Versions and their traffic weights in basis points. Weights must sum to exactly 10000.",
				Fields: []model.Field{
					{HCL: "worker_version", Wire: "workerVersion", Kind: model.KindResourceRef, TargetKind: "WorkerVersion", Required: true,
						Doc: "Worker Version receiving this weight."},
					{HCL: "weight", Wire: "weight", Kind: model.KindInteger, Required: true,
						Min: model.I64(1), Max: model.I64(10000),
						Doc: "Traffic share in basis points (1..10000)."},
				},
				Example: []any{map[string]any{
					"workerVersion": map[string]any{"kind": "WorkerVersion", "name": "worker-version"},
					"weight":        10000,
				}},
				CounterExample: []any{map[string]any{
					"workerVersion": map[string]any{"kind": "WorkerVersion", "name": "worker-version"},
					"weight":        0,
				}}},
		},
	},
	{
		Kind: "WorkerCustomDomain", Slug: "worker-custom-domain", ResourceType: "takoform_worker_custom_domain",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Custom Domain",
		Description: "Attaches one DNS hostname to a Module Worker so its active deployment serves that " +
			"hostname over HTTPS. Inward activation is an attachment, never a binding; deleting the " +
			"attachment detaches the hostname and never deletes the worker.",
		Fields: []model.Field{
			moduleWorkerRef("worker", "worker", "Module Worker served on this hostname.", true, true),
			{HCL: "hostname", Wire: "hostname", Kind: model.KindString, Required: true, Immutable: true,
				Pattern: model.PatternHostname, MaxLength: 253,
				Doc:     "Dotted DNS hostname this attachment serves. Changing it replaces the attachment.",
				Example: "app.portable-conformance.invalid", AltExample: "alt.portable-conformance.invalid",
				CounterExample: "not a hostname"},
		},
	},
	{
		Kind: "WorkerCronTrigger", Slug: "worker-cron-trigger", ResourceType: "takoform_worker_cron_trigger",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Worker Cron Trigger",
		Description: "Attaches one cron schedule to a Module Worker, invoking its scheduled handler at " +
			"each match. Schedules are interpreted in UTC only; there is no timezone field, so two hosts " +
			"can never fire the same trigger at different instants. The accepted grammar is exactly five " +
			"single-value fields separated by single spaces: minute is a literal 0-59 and hour a literal " +
			"0-23, day-of-month is `*` or 1-31, month is `*` or 1-12, and day-of-week is `*` or 0-6. " +
			"Ranges, lists, steps such as `*/5`, names, and `*` in the minute or hour field are all " +
			"rejected, so the most frequent representable schedule is once per day at one fixed UTC " +
			"time. Hourly and " +
			"sub-hourly schedules are not expressible and need a future grammar revision, which is a new " +
			"definition version of this Form.",
		Fields: []model.Field{
			moduleWorkerRef("worker", "worker", "Module Worker whose scheduled handler this trigger invokes.", true, true),
			{HCL: "cron", Wire: "cron", Kind: model.KindString, Required: true,
				Pattern: model.PatternCron, MaxLength: 64,
				Doc:     "Portable five-field cron expression, interpreted in UTC only.",
				Example: "0 3 * * *", AltExample: "15 0 * * *", CounterExample: "0 3 * *"},
		},
	},
	{
		Kind: "EdgeKVNamespace", Slug: "edge-kv-namespace", ResourceType: "takoform_edge_kv_namespace",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "Edge KV Namespace",
		Description: "Globally replicated key/value namespace with eventual consistency, exactly as fixed " +
			"by the edge.kv Interface. Eventual consistency is the Form's semantics, not an option: a " +
			"store with different convergence behavior is a different Form.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.kv", Version: "1.0.0"}},
	},
	{
		// The resource type is takoform_edge_object_bucket, not
		// takoform_object_bucket: the retained v2 lane still owns that name
		// while both lanes are co-registered in one provider binary. This is a
		// transitional name; a future provider release that drops the retained
		// v2 lane (a major, per versioning.md) reclaims takoform_object_bucket
		// for this Form.
		Kind: "ObjectBucket", Slug: "object-bucket", ResourceType: "takoform_edge_object_bucket",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "Object Bucket",
		Description: "Flat-namespace object store with read-after-write consistency, exactly as fixed by " +
			"the edge.objects Interface. Operating rules such as CORS, lifecycle, and lock are separate " +
			"policy resources, never desired fields of the bucket identity.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.objects", Version: "1.0.0"}},
	},
	{
		Kind: "SQLiteDatabase", Slug: "sqlite-database", ResourceType: "takoform_sqlite_database",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "SQLite Database",
		Description: "Embedded SQLite database with serializable transactions, exactly as fixed by the " +
			"edge.sql Interface. SQLite semantics are the identity: a database with different SQL, typing, " +
			"or isolation behavior is a different Form, never an engine token.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.sql", Version: "1.0.0"}},
	},
	{
		Kind: "AtLeastOnceQueue", Slug: "at-least-once-queue", ResourceType: "takoform_at_least_once_queue",
		Role: model.RoleIdentity, DefinitionVersion: edgeDefinitionVersion,
		Title: "At-Least-Once Queue",
		Description: "Message queue with at-least-once delivery and no ordering guarantee, exactly as fixed " +
			"by the edge.queue Interface. There is no ordering field: a FIFO queue is a different Form.",
		ProvidedInterfaces: []model.InterfaceRefSource{{Name: "edge.queue", Version: "1.0.0"}},
		Fields: []model.Field{
			// Retention has no portable default: how long a host keeps an
			// undelivered message is the whole operational meaning of the queue,
			// and no value is right for every workload. It is therefore required
			// rather than silently chosen.
			{HCL: "message_retention_seconds", Wire: "messageRetentionSeconds", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(60), Max: model.I64(1209600),
				Doc:     "How long an undelivered message is retained before it is dropped, in seconds.",
				Example: 345600},
			{HCL: "delivery_delay_seconds", Wire: "deliveryDelaySeconds", Kind: model.KindInteger,
				Min: model.I64(0), Max: model.I64(43200), Default: 0,
				Doc:     "Default delay before a sent message becomes deliverable, in seconds. Omitting it delivers immediately.",
				Example: 0},
		},
	},
	{
		Kind: "QueueConsumer", Slug: "queue-consumer", ResourceType: "takoform_queue_consumer",
		Role: model.RoleAttachment, DefinitionVersion: edgeDefinitionVersion,
		Title: "Queue Consumer",
		Description: "Attaches one Module Worker as the batch consumer of one At-Least-Once Queue, invoking " +
			"its queue handler with message batches and redelivering failed batches. Consumption is inward " +
			"activation and therefore an attachment, never a binding.",
		Fields: []model.Field{
			{HCL: "queue", Wire: "queue", Kind: model.KindResourceRef, TargetKind: "AtLeastOnceQueue",
				Required: true, Immutable: true,
				Doc:     "Queue this consumer drains. Changing it replaces the attachment.",
				Example: map[string]any{"kind": "AtLeastOnceQueue", "name": "at-least-once-queue"}},
			moduleWorkerRef("worker", "worker", "Module Worker whose queue handler receives the batches. Changing it replaces the attachment.", true, true),
			// Batching, retry, and concurrency decide throughput, duplicate
			// exposure, and downstream load together. No single value is portable
			// across workloads, so the consumer states all five rather than
			// inheriting whatever a host would otherwise pick.
			{HCL: "max_batch_size", Wire: "maxBatchSize", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(1), Max: model.I64(100),
				Doc:     "Largest number of messages delivered in one batch.",
				Example: 10},
			{HCL: "max_batch_timeout_seconds", Wire: "maxBatchTimeoutSeconds", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(0), Max: model.I64(60),
				Doc:     "Longest time the host waits to fill a batch before delivering it, in seconds.",
				Example: 5},
			{HCL: "max_retries", Wire: "maxRetries", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(0), Max: model.I64(100),
				Doc:     "How many times a failed batch is redelivered before its messages go to the dead-letter queue or are dropped.",
				Example: 3},
			{HCL: "retry_delay_seconds", Wire: "retryDelaySeconds", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(0), Max: model.I64(43200),
				Doc:     "Delay before a failed batch becomes deliverable again, in seconds.",
				Example: 60},
			// The one field whose ABSENCE is the semantics: there is no queue that
			// means "drop exhausted messages", so no default can express it.
			{HCL: "dead_letter_queue", Wire: "deadLetterQueue", Kind: model.KindResourceRef, TargetKind: "AtLeastOnceQueue",
				AbsenceIsSemantic: true,
				Doc:               "Queue receiving messages that exhausted their retries. Without it, exhausted messages are dropped.",
				Example:           map[string]any{"kind": "AtLeastOnceQueue", "name": "dead-letters"}},
			{HCL: "max_concurrency", Wire: "maxConcurrency", Kind: model.KindInteger,
				Required: true,
				Min:      model.I64(1), Max: model.I64(250),
				Doc:     "Largest number of concurrent batch invocations.",
				Example: 4},
		},
	},
}

// Validate proves every structural catalog rule: per-form model rules, the
// open-token guard, unique identities, and resolvable contract references.
func Validate() error {
	if err := model.ValidateNoOpenTokens(Forms); err != nil {
		return err
	}
	kinds := map[string]struct{}{}
	slugs := map[string]struct{}{}
	resourceTypes := map[string]struct{}{}
	for _, form := range Forms {
		if err := form.Validate(); err != nil {
			return err
		}
		if form.DefinitionVersion != edgeDefinitionVersion {
			return fmt.Errorf("form %s declares definition version %q; the MVP family line is %q", form.Kind, form.DefinitionVersion, edgeDefinitionVersion)
		}
		for name, set := range map[string]map[string]struct{}{form.Kind: kinds, form.Slug: slugs, form.ResourceType: resourceTypes} {
			if _, duplicate := set[name]; duplicate {
				return fmt.Errorf("duplicate catalog identity %q", name)
			}
			set[name] = struct{}{}
		}
		for _, source := range form.ProvidedInterfaces {
			if _, err := interfaceDefinitionByName(source.Name); err != nil {
				return fmt.Errorf("form %s: %w", form.Kind, err)
			}
		}
	}
	for _, field := range bindingListTargets() {
		if _, known := kinds[field]; !known {
			return fmt.Errorf("binding target kind %q is not a catalog Form", field)
		}
	}
	return validateAbsenceSemanticExemptions()
}

// absenceSemanticExemptions is the complete, reviewed list of family fields
// whose absence carries portable meaning instead of a default. It is written
// out so the exemption is an auditable fact rather than a marker anyone can
// add: a new AbsenceIsSemantic field fails the catalog until it is listed
// here, which forces the review that decides whether it deserves the
// exemption at all.
var absenceSemanticExemptions = map[string]struct{}{
	"QueueConsumer/deadLetterQueue": {},
}

func validateAbsenceSemanticExemptions() error {
	for _, form := range Forms {
		for _, field := range form.Fields {
			if !field.AbsenceIsSemantic {
				continue
			}
			key := form.Kind + "/" + field.Wire
			if _, allowed := absenceSemanticExemptions[key]; !allowed {
				return fmt.Errorf(
					"form %s field %s marks AbsenceIsSemantic without a reviewed exemption; "+
						"declare a portable Default instead, or add %s to absenceSemanticExemptions",
					form.Kind, field.Wire, key,
				)
			}
		}
	}
	return nil
}

func bindingListTargets() []string {
	var targets []string
	for _, form := range Forms {
		for _, field := range form.Fields {
			if field.Kind == model.KindBindingList || field.Kind == model.KindResourceRef || field.Kind == model.KindResourceRefList {
				targets = append(targets, field.TargetKind)
			}
		}
	}
	return targets
}

// ByKind returns the declared Form for a portable kind token.
func ByKind(kind string) (model.Form, bool) {
	for _, candidate := range Forms {
		if candidate.Kind == kind {
			return candidate, true
		}
	}
	return model.Form{}, false
}
