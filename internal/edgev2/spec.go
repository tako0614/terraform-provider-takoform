// Package edgev2 contains an unmounted, publisher-specific projection of the
// exact Edge Form URLs used by the v2 install path.
package edgev2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tako0614/terraform-provider-takoform/internal/jsonv2"
)

const (
	ModuleWorkerURL               = "https://edge.forms.takoform.com/forms/ModuleWorker/0.3.0/"
	WorkerBundleURL               = "https://edge.forms.takoform.com/forms/WorkerBundle/0.2.0/"
	WorkerVersionURL              = "https://edge.forms.takoform.com/forms/WorkerVersion/0.5.0/"
	WorkerDeploymentURL           = "https://edge.forms.takoform.com/forms/WorkerDeployment/0.4.0/"
	WorkerEndpointURL             = "https://edge.forms.takoform.com/forms/WorkerEndpoint/0.3.0/"
	WorkerCronTriggerURL          = "https://edge.forms.takoform.com/forms/WorkerCronTrigger/0.3.0/"
	EdgeKVNamespaceURL            = "https://edge.forms.takoform.com/forms/EdgeKVNamespace/0.2.0/"
	ObjectBucketURL               = "https://edge.forms.takoform.com/forms/ObjectBucket/0.2.0/"
	SQLiteDatabaseURL             = "https://edge.forms.takoform.com/forms/SQLiteDatabase/0.2.0/"
	SQLiteMigrationSetURL         = "https://edge.forms.takoform.com/forms/SQLiteMigrationSet/0.2.0/"
	SQLiteMigrationApplicationURL = "https://edge.forms.takoform.com/forms/SQLiteMigrationApplication/0.2.0/"
	AtLeastOnceQueueURL           = "https://edge.forms.takoform.com/forms/AtLeastOnceQueue/0.2.0/"
	QueueConsumerURL              = "https://edge.forms.takoform.com/forms/QueueConsumer/0.3.0/"
)

var urls = []string{ModuleWorkerURL, WorkerBundleURL, WorkerVersionURL, WorkerDeploymentURL, WorkerEndpointURL, WorkerCronTriggerURL, EdgeKVNamespaceURL, ObjectBucketURL, SQLiteDatabaseURL, SQLiteMigrationSetURL, SQLiteMigrationApplicationURL, AtLeastOnceQueueURL, QueueConsumerURL}

// URLs returns the exact published URL identities in stable order.
func URLs() []string { return append([]string(nil), urls...) }

// URLForKind returns the exact published Form URL for an exact Form kind.
func URLForKind(kind string) (string, bool) {
	for _, entry := range []struct{ name, url string }{
		{"ModuleWorker", ModuleWorkerURL}, {"WorkerBundle", WorkerBundleURL},
		{"WorkerVersion", WorkerVersionURL}, {"WorkerDeployment", WorkerDeploymentURL},
		{"WorkerEndpoint", WorkerEndpointURL}, {"WorkerCronTrigger", WorkerCronTriggerURL},
		{"EdgeKVNamespace", EdgeKVNamespaceURL}, {"ObjectBucket", ObjectBucketURL},
		{"SQLiteDatabase", SQLiteDatabaseURL}, {"SQLiteMigrationSet", SQLiteMigrationSetURL},
		{"SQLiteMigrationApplication", SQLiteMigrationApplicationURL}, {"AtLeastOnceQueue", AtLeastOnceQueueURL},
		{"QueueConsumer", QueueConsumerURL},
	} {
		if entry.name == kind {
			return entry.url, true
		}
	}
	return "", false
}

// KindForURL returns the exact Form kind for a URL in this bounded roster.
func KindForURL(formURL string) (string, bool) {
	for _, kind := range []string{"ModuleWorker", "WorkerBundle", "WorkerVersion", "WorkerDeployment", "WorkerEndpoint", "WorkerCronTrigger", "EdgeKVNamespace", "ObjectBucket", "SQLiteDatabase", "SQLiteMigrationSet", "SQLiteMigrationApplication", "AtLeastOnceQueue", "QueueConsumer"} {
		if url, ok := URLForKind(kind); ok && url == formURL {
			return kind, true
		}
	}
	return "", false
}

type ResourceRef struct {
	ResourceUID string `json:"resourceUid"`
}
type Artifact struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}
type ArtifactSpec struct {
	Artifact Artifact `json:"artifact"`
}
type EmptySpec struct{}
type WorkerEndpointSpec struct {
	Worker ResourceRef `json:"worker"`
}
type WorkerCronTriggerSpec struct {
	Worker ResourceRef `json:"worker"`
	Cron   string      `json:"cron"`
}
type QueueSpec struct {
	DeliveryDelaySeconds    *int `json:"deliveryDelaySeconds,omitempty"`
	MessageRetentionSeconds int  `json:"messageRetentionSeconds"`
}
type QueueConsumerSpec struct {
	Queue                  ResourceRef  `json:"queue"`
	Worker                 ResourceRef  `json:"worker"`
	MaxBatchSize           int          `json:"maxBatchSize"`
	MaxBatchTimeoutSeconds int          `json:"maxBatchTimeoutSeconds"`
	MaxConcurrency         int          `json:"maxConcurrency"`
	MaxRetries             int          `json:"maxRetries"`
	RetryDelaySeconds      int          `json:"retryDelaySeconds"`
	DeadLetterQueue        *ResourceRef `json:"deadLetterQueue,omitempty"`
}
type WeightedVersion struct {
	WorkerVersion ResourceRef `json:"workerVersion"`
	Weight        int         `json:"weight"`
}
type WorkerDeploymentSpec struct {
	Worker   ResourceRef       `json:"worker"`
	Versions []WeightedVersion `json:"versions"`
}
type NamedBinding struct {
	Name     string      `json:"name"`
	Resource ResourceRef `json:"resource"`
}
type Assets struct {
	Bundle           ResourceRef `json:"bundle"`
	RunWorkerFirst   bool        `json:"runWorkerFirst"`
	NotFoundHandling string      `json:"notFoundHandling"`
}
type WorkerVersionSpec struct {
	Worker                ResourceRef    `json:"worker"`
	Bundle                *ResourceRef   `json:"bundle,omitempty"`
	Handlers              []string       `json:"handlers"`
	Vars                  map[string]any `json:"vars,omitempty"`
	RequiredSensitiveVars []string       `json:"requiredSensitiveVars,omitempty"`
	KVBindings            []NamedBinding `json:"kvBindings,omitempty"`
	SQLiteBindings        []NamedBinding `json:"sqliteBindings,omitempty"`
	BucketBindings        []NamedBinding `json:"bucketBindings,omitempty"`
	QueueProducerBindings []NamedBinding `json:"queueProducerBindings,omitempty"`
	ServiceBindings       []NamedBinding `json:"serviceBindings,omitempty"`
	ActorBindings         []NamedBinding `json:"actorBindings,omitempty"`
	WorkflowBindings      []NamedBinding `json:"workflowBindings,omitempty"`
	Assets                *Assets        `json:"assets,omitempty"`
}
type MigrationApplicationSpec struct {
	Database     ResourceRef `json:"database"`
	MigrationSet ResourceRef `json:"migrationSet"`
}

var nameRE = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]{0,63}$`)
var varRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,63}$`)
var secretNameRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)
var digestRE = regexp.MustCompile(`^[0-9a-f]{64}$`)
var uidRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Decode validates a closed desired-input shape and returns its typed spec.
// privateInputs are deliberately not part of this public spec codec.
func Decode(formURL string, data []byte) (any, error) {
	value, err := jsonv2.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode spec: %w", err)
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("spec must be an object")
	}
	if err := validate(formURL, obj); err != nil {
		return nil, err
	}
	var out any
	switch formURL {
	case ModuleWorkerURL, EdgeKVNamespaceURL, ObjectBucketURL, SQLiteDatabaseURL:
		out = &EmptySpec{}
	case WorkerBundleURL, SQLiteMigrationSetURL:
		out = &ArtifactSpec{}
	case WorkerVersionURL:
		out = &WorkerVersionSpec{}
	case WorkerDeploymentURL:
		out = &WorkerDeploymentSpec{}
	case WorkerEndpointURL:
		out = &WorkerEndpointSpec{}
	case WorkerCronTriggerURL:
		out = &WorkerCronTriggerSpec{}
	case AtLeastOnceQueueURL:
		out = &QueueSpec{}
	case QueueConsumerURL:
		out = &QueueConsumerSpec{}
	case SQLiteMigrationApplicationURL:
		out = &MigrationApplicationSpec{}
	default:
		return nil, fmt.Errorf("unsupported exact Form URL %q", formURL)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	if err := d.Decode(out); err != nil {
		return nil, fmt.Errorf("decode typed spec: %w", err)
	}
	return out, nil
}

// Encode accepts only the URL-specific typed spec and validates its JSON form.
func Encode(formURL string, spec any) ([]byte, error) {
	b, err := jsonv2.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("encode spec: %w", err)
	}
	decoded, err := Decode(formURL, b)
	if err != nil {
		return nil, err
	}
	if fmt.Sprintf("%T", decoded) != fmt.Sprintf("%T", spec) {
		return nil, fmt.Errorf("wrong typed spec for %s", formURL)
	}
	return b, nil
}

func validate(formURL string, o map[string]any) error {
	var err error
	closed := func(keys ...string) error { return only(o, keys...) }
	switch formURL {
	case ModuleWorkerURL, EdgeKVNamespaceURL, ObjectBucketURL, SQLiteDatabaseURL:
		err = closed()
	case WorkerBundleURL, SQLiteMigrationSetURL:
		if err = closed("artifact"); err == nil {
			err = artifact(o["artifact"])
		}
	case WorkerEndpointURL:
		if err = closed("worker"); err == nil {
			err = ref(o["worker"])
		}
	case WorkerCronTriggerURL:
		if err = closed("worker", "cron"); err == nil {
			err = ref(o["worker"])
		}
		if err == nil {
			err = cron(o["cron"])
		}
	case AtLeastOnceQueueURL:
		if err = closed("deliveryDelaySeconds", "messageRetentionSeconds"); err == nil {
			if v, ok := o["deliveryDelaySeconds"]; ok {
				err = integer(v, 0, 43200, "deliveryDelaySeconds")
			}
		}
		if err == nil {
			err = integer(o["messageRetentionSeconds"], 60, 1209600, "messageRetentionSeconds")
		}
	case QueueConsumerURL:
		if err = closed("queue", "worker", "maxBatchSize", "maxBatchTimeoutSeconds", "maxConcurrency", "maxRetries", "retryDelaySeconds", "deadLetterQueue"); err == nil {
			err = ref(o["queue"])
		}
		if err == nil {
			err = ref(o["worker"])
		}
		for _, f := range []struct {
			k        string
			min, max int
		}{{"maxBatchSize", 1, 100}, {"maxBatchTimeoutSeconds", 0, 60}, {"maxConcurrency", 1, 250}, {"maxRetries", 0, 100}, {"retryDelaySeconds", 0, 43200}} {
			if err == nil {
				err = integer(o[f.k], f.min, f.max, f.k)
			}
		}
		if err == nil {
			if v, ok := o["deadLetterQueue"]; ok {
				err = ref(v)
			}
		}
	case WorkerDeploymentURL:
		err = validateDeployment(o)
	case WorkerVersionURL:
		err = validateVersion(o)
	case SQLiteMigrationApplicationURL:
		if err = closed("database", "migrationSet"); err == nil {
			err = ref(o["database"])
		}
		if err == nil {
			err = ref(o["migrationSet"])
		}
	default:
		return fmt.Errorf("unsupported exact Form URL %q", formURL)
	}
	return err
}

func only(o map[string]any, keys ...string) error {
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	for k := range o {
		if !allowed[k] {
			return fmt.Errorf("unknown field %q", k)
		}
	}
	return nil
}
func object(v any, keys ...string) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok || m == nil {
		return nil, errors.New("must be an object (null is not allowed)")
	}
	if err := only(m, keys...); err != nil {
		return nil, err
	}
	return m, nil
}
func ref(v any) error {
	m, e := object(v, "resourceUid")
	if e != nil {
		return e
	}
	s, ok := m["resourceUid"].(string)
	if !ok || !uidRE.MatchString(s) {
		return errors.New("resourceUid must match [A-Za-z0-9][A-Za-z0-9._-]{0,127}")
	}
	return nil
}
func integer(v any, min, max int, field string) error {
	n, ok := v.(json.Number)
	if !ok {
		return fmt.Errorf("%s must be an integer", field)
	}
	i, e := n.Int64()
	if e != nil || i < int64(min) || i > int64(max) {
		return fmt.Errorf("%s must be an integer in %d..%d", field, min, max)
	}
	return nil
}
func artifact(v any) error {
	m, e := object(v, "url", "sha256")
	if e != nil {
		return fmt.Errorf("artifact: %w", e)
	}
	raw, ok := m["url"].(string)
	if !ok || len(raw) > 8192 || !isArtifactURL(raw) {
		return errors.New("artifact.url must be an absolute HTTPS URL without userinfo, query, or fragment (max 8192 ASCII chars)")
	}
	sha, ok := m["sha256"].(string)
	if !ok || !digestRE.MatchString(sha) {
		return errors.New("artifact.sha256 must be 64 lowercase hexadecimal characters")
	}
	return nil
}
func isArtifactURL(raw string) bool {
	for _, r := range raw {
		if r > 127 {
			return false
		}
	}
	u, e := url.Parse(raw)
	return e == nil && !strings.Contains(raw, "#") && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == ""
}

func validateDeployment(o map[string]any) error {
	if e := only(o, "worker", "versions"); e != nil {
		return e
	}
	if e := ref(o["worker"]); e != nil {
		return e
	}
	a, ok := o["versions"].([]any)
	if !ok || len(a) < 1 || len(a) > 8 {
		return errors.New("versions must contain 1..8 entries")
	}
	seen := map[string]bool{}
	total := 0
	for _, v := range a {
		m, e := object(v, "workerVersion", "weight")
		if e != nil {
			return e
		}
		if e = ref(m["workerVersion"]); e != nil {
			return e
		}
		uid := m["workerVersion"].(map[string]any)["resourceUid"].(string)
		if seen[uid] {
			return errors.New("duplicate workerVersion UID")
		}
		seen[uid] = true
		if e = integer(m["weight"], 1, 10000, "weight"); e != nil {
			return e
		}
	}
	// Re-read exact integer values; the range check above already excludes fractional numbers.
	total = 0
	for _, v := range a {
		n, _ := oInt(v.(map[string]any)["weight"])
		total += n
	}
	if total != 10000 {
		return errors.New("versions weights must sum to exactly 10000")
	}
	return nil
}
func oInt(v any) (int, error) { n := v.(json.Number); i, e := n.Int64(); return int(i), e }

func validateVersion(o map[string]any) error {
	keys := []string{"worker", "bundle", "handlers", "vars", "requiredSensitiveVars", "kvBindings", "sqliteBindings", "bucketBindings", "queueProducerBindings", "serviceBindings", "actorBindings", "workflowBindings", "assets"}
	if e := only(o, keys...); e != nil {
		return e
	}
	if e := ref(o["worker"]); e != nil {
		return e
	}
	if v, ok := o["bundle"]; ok {
		if e := ref(v); e != nil {
			return e
		}
	}
	handlers, ok := o["handlers"].([]any)
	if !ok || len(handlers) > 3 {
		return errors.New("handlers is required and must contain at most 3 entries")
	}
	handlerSet := map[string]bool{}
	for _, v := range handlers {
		s, ok := v.(string)
		if !ok || !oneOf(s, "fetch", "scheduled", "queue") || handlerSet[s] {
			return errors.New("handlers must be unique values from fetch, scheduled, queue")
		}
		handlerSet[s] = true
	}
	if v, ok := o["vars"]; ok {
		m, ok := v.(map[string]any)
		if !ok || len(m) > 64 {
			return errors.New("vars must be an object with at most 64 keys")
		}
		for k, x := range m {
			if !varRE.MatchString(k) {
				return fmt.Errorf("invalid vars key %q", k)
			}
			if e := jsonValue(x, 1); e != nil {
				return fmt.Errorf("vars.%s: %w", k, e)
			}
		}
	}
	used := map[string]string{}
	if v, ok := o["requiredSensitiveVars"]; ok {
		a, ok := v.([]any)
		if !ok || len(a) > 64 {
			return errors.New("requiredSensitiveVars must be an array with at most 64 names")
		}
		for _, x := range a {
			s, ok := x.(string)
			if !ok || !secretNameRE.MatchString(s) {
				return errors.New("invalid requiredSensitiveVars name")
			}
			if _, dup := used[s]; dup {
				return fmt.Errorf("duplicate environment name %q", s)
			}
			used[s] = "secret"
		}
	}
	if m, ok := o["vars"].(map[string]any); ok {
		for k := range m {
			if _, dup := used[k]; dup {
				return fmt.Errorf("duplicate environment name %q", k)
			}
			used[k] = "var"
		}
	}
	for _, field := range []string{"kvBindings", "sqliteBindings", "bucketBindings", "queueProducerBindings", "serviceBindings", "actorBindings", "workflowBindings"} {
		if v, ok := o[field]; ok {
			a, ok := v.([]any)
			if !ok || len(a) > 64 {
				return fmt.Errorf("%s must be an array with at most 64 bindings", field)
			}
			seen := map[string]bool{}
			for _, item := range a {
				m, e := object(item, "name", "resource")
				if e != nil {
					return fmt.Errorf("%s: %w", field, e)
				}
				name, ok := m["name"].(string)
				if !ok || !nameRE.MatchString(name) {
					return fmt.Errorf("%s binding name is invalid", field)
				}
				if seen[name] {
					return fmt.Errorf("duplicate %s binding name %q", field, name)
				}
				seen[name] = true
				if _, dup := used[name]; dup {
					return fmt.Errorf("duplicate environment name %q", name)
				}
				used[name] = field
				if e = ref(m["resource"]); e != nil {
					return fmt.Errorf("%s binding resource: %w", field, e)
				}
			}
		}
	}
	if v, ok := o["assets"]; ok {
		m, e := object(v, "bundle", "runWorkerFirst", "notFoundHandling")
		if e != nil {
			return fmt.Errorf("assets: %w", e)
		}
		if e = ref(m["bundle"]); e != nil {
			return e
		}
		if _, ok := m["runWorkerFirst"].(bool); !ok {
			return errors.New("assets.runWorkerFirst must be boolean")
		}
		s, ok := m["notFoundHandling"].(string)
		if !ok || !oneOf(s, "none", "single_page_application") {
			return errors.New("assets.notFoundHandling must be none or single_page_application")
		}
	}
	_, bundle := o["bundle"]
	_, assets := o["assets"]
	if !bundle && !assets {
		return errors.New("bundle or assets is required")
	}
	if !bundle && assets {
		if len(handlers) != 0 {
			return errors.New("static-only WorkerVersion requires empty handlers")
		}
		if o["vars"] != nil && len(o["vars"].(map[string]any)) != 0 {
			return errors.New("static-only WorkerVersion requires empty vars")
		}
		if v, ok := o["requiredSensitiveVars"].([]any); ok && len(v) != 0 {
			return errors.New("static-only WorkerVersion requires empty requiredSensitiveVars")
		}
		for _, f := range []string{"kvBindings", "sqliteBindings", "bucketBindings", "queueProducerBindings", "serviceBindings", "actorBindings", "workflowBindings"} {
			if a, ok := o[f].([]any); ok && len(a) != 0 {
				return fmt.Errorf("static-only WorkerVersion requires empty %s", f)
			}
		}
		if o["assets"].(map[string]any)["runWorkerFirst"] == true {
			return errors.New("static-only WorkerVersion requires assets.runWorkerFirst=false")
		}
	}
	return nil
}
func jsonValue(v any, depth int) error {
	if depth > 8 {
		return errors.New("JSON value nesting exceeds 8")
	}
	switch x := v.(type) {
	case nil, bool, string, json.Number:
		if s, ok := x.(string); ok && utf8.RuneCountInString(s) > 8192 {
			return errors.New("string exceeds 8192 Unicode scalar values")
		}
	case []any:
		if len(x) > 64 {
			return errors.New("array exceeds 64 items")
		}
		for _, v := range x {
			if e := jsonValue(v, depth+1); e != nil {
				return e
			}
		}
	case map[string]any:
		if len(x) > 64 {
			return errors.New("object exceeds 64 keys")
		}
		for k, v := range x {
			if !varRE.MatchString(k) {
				return fmt.Errorf("invalid object key %q", k)
			}
			if e := jsonValue(v, depth+1); e != nil {
				return e
			}
		}
	default:
		return errors.New("value is not JSON")
	}
	return nil
}
func oneOf(v string, opts ...string) bool {
	for _, o := range opts {
		if v == o {
			return true
		}
	}
	return false
}
func cron(v any) error {
	s, ok := v.(string)
	if !ok || len(s) == 0 || len(s) > 64 || strings.Contains(s, "\t") || strings.Contains(s, "  ") || strings.TrimSpace(s) != s {
		return errors.New("cron must be at most 64 characters with single ASCII spaces")
	}
	p := strings.Split(s, " ")
	if len(p) != 5 {
		return errors.New("cron must have five fields")
	}
	ranges := [][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for i, f := range p {
		if e := cronField(f, ranges[i][0], ranges[i][1]); e != nil {
			return fmt.Errorf("cron field %d: %w", i+1, e)
		}
	}
	return nil
}
func cronField(f string, min, max int) error {
	seen := map[string]bool{}
	for _, part := range strings.Split(f, ",") {
		if part == "" || seen[part] {
			return errors.New("empty or duplicate element")
		}
		seen[part] = true
		step := 0
		if strings.Contains(part, "/") {
			bits := strings.Split(part, "/")
			if len(bits) != 2 || bits[0] == "" || bits[1] == "" {
				return errors.New("invalid step")
			}
			part = bits[0]
			n, e := parseInt(bits[1])
			if e != nil || n < 1 || n > max-min+1 {
				return errors.New("step out of range")
			}
			step = n
		}
		if part == "*" {
			continue
		}
		if strings.Contains(part, "-") {
			bits := strings.Split(part, "-")
			if len(bits) != 2 {
				return errors.New("invalid range")
			}
			a, e := parseInt(bits[0])
			if e != nil {
				return e
			}
			b, e := parseInt(bits[1])
			if e != nil || a < min || b > max || a > b {
				return errors.New("range out of bounds")
			}
			continue
		}
		if step != 0 {
			return errors.New("step requires * or a range")
		}
		n, e := parseInt(part)
		if e != nil || n < min || n > max {
			return errors.New("value out of range")
		}
	}
	return nil
}
func parseInt(s string) (int, error) {
	if s == "" {
		return 0, errors.New("empty integer")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("expected decimal integer")
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil || n > uint64(^uint(0)>>1) {
		return 0, errors.New("decimal integer out of range")
	}
	return int(n), nil
}
