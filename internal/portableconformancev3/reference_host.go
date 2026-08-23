package portableconformancev3

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tako0614/terraform-provider-takoform/formpackage"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformmodel"
)

const (
	// ReferencePrimaryToken is exported so cmd/reference-host can print the
	// credential a reader needs. It is a constant compiled into this
	// repository and authenticates nobody in particular, which is one of the
	// reasons this host belongs on loopback.
	ReferencePrimaryToken = referencePrimaryToken

	referencePrimaryToken         = "reference-primary-token"
	referenceAlternateToken       = "reference-alternate-token"
	referenceAlternateTenantToken = "reference-alternate-tenant-token"

	// ErrorProbeHeader is runner-harness instrumentation, not part of the
	// portable host API: a disposable conformance endpoint uses it to make
	// every stable error deterministic. Production endpoints must not
	// implement it.
	ErrorProbeHeader = "Takoform-Conformance-Probe"
	// ProbeAsync asks the disposable host to take the 202 Operation path for
	// one apply or delete.
	ProbeAsync = "async"
	// ProbeTouchStatus asks the disposable host to perform one host-side
	// status touch during observe, so the runner can prove that revision
	// advances while generation does not.
	ProbeTouchStatus = "touch-status"
	// ProbeExternalChange asks the disposable host to perform one delete as an
	// out-of-band backend change: the relation dependency scan is skipped, as
	// if the underlying resource had vanished outside the host API. It exists
	// so a runner can reach the one state deletion protection otherwise makes
	// unreachable — a live relation whose target was destroyed and recreated.
	ProbeExternalChange = "external-change"
	// ProbeErrorPrefix carries one stable error code to return verbatim,
	// e.g. "error:permission_denied".
	ProbeErrorPrefix = "error:"

	expectedGenerationHeader = "Takoform-Expected-Generation"
	operationAPIVersion      = "operations.takoform.com/v1alpha1"
	supportAPIVersion        = "support.takoform.com/v1alpha1"
	artifactAPIVersion       = "artifacts.takoform.com/v1alpha1"
	fixedTransitionTime      = "2026-08-06T00:00:00Z"
	maxBodyBytes             = 8 * 1024 * 1024

	// asyncOperationPolls is how many operation reads a 202 mutation stays
	// pending for before the reference host completes it.
	asyncOperationPolls = 2

	// edgeFormsGroup is the one namespaced group whose cross-resource
	// semantics this reference host enforces.
	edgeFormsGroup                 = "edge.forms.takoform.com/v1beta2"
	workerBundleKind               = "WorkerBundle"
	staticAssetBundleKind          = "StaticAssetBundle"
	sqliteMigrationSetKind         = "SQLiteMigrationSet"
	sqliteMigrationApplicationKind = "SQLiteMigrationApplication"
	migrationBundleKind            = "MigrationBundle"
	// workerDeploymentWeightTotal is the exact basis-point sum a
	// WorkerDeployment versions[] must carry. A desired-state schema cannot
	// add numbers, so the sum is host-validated semantics.
	workerDeploymentWeightTotal = 10000
	// maximumBundleBytes is the published module-size ceiling of the
	// WorkerVersion and artifact-backed Form support profiles. The artifact
	// manifest validator enforces exactly the limits those support surfaces
	// advertise.
	maximumBundleBytes = 10485760
	// maximumBundleFiles is the published entry-count ceiling for file-backed
	// artifact Forms. WorkerBundle modules retain the public manifest schema's
	// 4096-entry ceiling; the provider and host enforce that separately.
	maximumBundleFiles         = 16384
	maximumWorkerBundleModules = 4096
	// sourceMapMediaType names a module that is source-map evidence for
	// another declared module rather than executable code.
	sourceMapMediaType = "application/source-map+json"
	// sourceMapSuffix is the portable naming rule that binds a source map to
	// its target module: "<module>.map" describes "<module>".
	sourceMapSuffix = ".map"
)

var artifactPathPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`)

// artifactModuleMediaTypes is what a WorkerBundle manifest admits. It is
// DERIVED from the lane's single media-type statement rather than listed here,
// so this host cannot commit a bundle the runtime contract could not load, nor
// refuse one the published manifest schema admits (spec/decisions/0012, 0014,
// and 0019). The loadable/auxiliary split the same statement carries is what
// makes `mainModule` and the source-map rule below decidable.
var artifactModuleMediaTypes = func() map[string]bool {
	admitted := map[string]bool{}
	for _, mediaType := range currentformmodel.BundleModuleMediaTypes() {
		admitted[mediaType] = true
	}
	return admitted
}()

// hostError is one closed stable error outcome.
type hostError struct {
	Code    string
	Message string
}

func stableError(code, message string) *hostError { return &hostError{Code: code, Message: message} }

type storedResource struct {
	// Ref is the EXACT Form identity this resource was created under, and the
	// only identity it is ever answered about. A stored resource that carried
	// only a group and a kind would be served under whichever definition version
	// a request happened to name, which is the substitution decision 0022 closes:
	// the response would describe the resource under a contract it was never
	// applied under, successfully, with nothing downstream able to detect it.
	Ref  FormRef
	Name string
	// Tenant is the authenticated tenant this resource belongs to, and the first
	// member of its ADDRESS (spec/decisions/0028).
	//
	// It never travels on the wire: a reference carries `{apiVersion, kind, name}`
	// and a request carries a space, so nothing a client writes names a tenant. It
	// comes from the authenticated credential of the request instead, which is why
	// it is written at create and copied forward rather than re-read from whatever
	// mutation happens to be in flight — exactly like the exact ref.
	//
	// Because it is part of the address, no request authenticated as another
	// tenant can reach this record at all: a foreign read, update, fence, delete,
	// relation resolution, prepare binding, or idempotency replay addresses a
	// different key, which holds nothing.
	Tenant        string
	Space         string
	UID           string
	Generation    int64
	Revision      int64
	Spec          map[string]any
	SpecDigest    string
	StatusTouches int64
	Imported      bool
	// NativeID is the backend object this resource was ADOPTED onto, recorded
	// at the adoption that named it and never rewritten afterwards. A resource
	// this host created holds none until an import names one, which is the
	// ordinary `terraform import` onto an address a configuration already
	// manages; that import records a first claim rather than changing one.
	//
	// It is carried forward by every later mutation, because an update is not a
	// release. The whole record is copied on update (nextResource), so this comes
	// for free here; a host that rebuilt its stored resource from the request
	// document would drop the claim on the next apply, and its import path would
	// look perfectly correct while doing it.
	//
	// It never travels on the wire in either direction beyond the `nativeId` a
	// client writes on `import`: a native identifier is host detail, outside the
	// portable Form contract (spec/portability-boundary.md), so the only thing
	// portable about it is that this host holds at most one resource per
	// identity within one tenant — under any kind, in any space. That is what
	// makes adoption observable at all without putting a vendor's identifier
	// format in a portable author's path (spec/decisions/0011).
	NativeID      string
	PackageDigest string
	// Relations is the resolved cross-resource reference set of this exact
	// stored spec, pinned by target UID and by the target's exact FormRef.
	Relations []storedRelation
	// DerivedRendering is the exact derived part of the representation this
	// resource was last serving — the conditions this host renders from OTHER
	// resources. It is what the revision of that representation was issued for,
	// so a mutation elsewhere that changes it must move the revision
	// (derived_rendering.go).
	DerivedRendering string
}

// group and kind are the two members of the recorded exact ref that address a
// resource in the store. A resource NAME is unique within one space, group, and
// kind — a reference is `{apiVersion, kind, name}` and carries no definition
// version, so a host holding two same-named resources of one kind under
// different contracts could not resolve a reference to either (decision 0015).
// The definition version and digest therefore decide what a request is ANSWERED
// about, never where the resource is stored.
func (resource *storedResource) group() string { return resource.Ref.APIVersion }
func (resource *storedResource) kind() string  { return resource.Ref.Kind }

// scope is the boundary half of this resource's address.
func (resource *storedResource) scope() resourceScope {
	return resourceScope{Tenant: resource.Tenant, Space: resource.Space}
}

// key is this resource's store key.
func (resource *storedResource) key() string {
	return resourceKey(resource.scope(), resource.group(), resource.kind(), resource.Name)
}

type recordedReplay struct {
	Fingerprint string
	Status      int
	ETag        string
	Body        []byte
	// Binding is the incarnation this recorded answer reports as LIVE. A record
	// does not outlive it (spec/decisions/0011, replayBinding).
	Binding replayBinding
}

// replayBinding is what one recorded answer says about a live incarnation, and
// therefore what retires the record.
//
// A recorded response either reports a resource that exists — a create, an
// update, an adoption, an observe — or it reports none: a delete's 204 says the
// incarnation is GONE, and a refusal says nothing happened. Only the first kind
// is bound, and a bound record is retired the moment its uid stops existing,
// because from then on replaying it would report a resource the host does not
// have.
//
// An accepted 202 carries no uid at accept time: the mutation has not run. It is
// bound to its OPERATION instead, and follows that operation's outcome — a
// commit that created or rewrote a resource binds the record to that uid, and a
// commit that removed one or failed leaves it bound to nothing.
type replayBinding struct {
	UID       string
	Operation string
}

type artifactUpload struct {
	ID string
	// Owner is the authenticated tenant and principal that started this upload
	// session. An upload id is a handle, never a capability: a caller who is not
	// the owner is answered as if the session did not exist
	// (spec/decisions/0018).
	Owner          hostAuthContext
	ManifestRaw    []byte
	Manifest       artifactManifest
	ManifestDigest string
}

type artifactManifest struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	MainModule string           `json:"mainModule,omitempty"`
	Modules    []artifactModule `json:"modules,omitempty"`
	Files      []artifactFile   `json:"files,omitempty"`
}

type artifactModule struct {
	Name      string      `json:"name"`
	MediaType string      `json:"mediaType"`
	Size      json.Number `json:"size"`
	Digest    string      `json:"digest"`
}

type artifactFile struct {
	Path      string      `json:"path"`
	MediaType string      `json:"mediaType"`
	Size      json.Number `json:"size"`
	Digest    string      `json:"digest"`
}

type hostOperation struct {
	ID string
	// Owner is the authenticated tenant and principal the mutation was accepted
	// from. An operation id is a resumption handle, never a capability: a caller
	// who is not the owner is answered operation_not_found, because a 403 would
	// confirm that the id names a real operation (spec/decisions/0018).
	Owner          hostAuthContext
	Done           bool
	PollsRemaining int
	// DeleteTarget is the resource key an accepted-but-unfinished DELETE is
	// removing, empty for every other accepted mutation. It is what makes
	// "being deleted" observable to the aggregate rules without a marker on the
	// resource that a cancel would have to unwind.
	DeleteTarget string
	// Accepted is the exact incarnation this operation was accepted against. A
	// 202 is an acceptance of a mutation to ONE resource, so the identity that
	// mutation may land on is recorded here rather than resolved again from a
	// name at commit time (acceptedTarget).
	Accepted acceptedTarget
	// CommittedUID is the incarnation this operation left live when it settled,
	// empty for one that removed a resource, created nothing, or failed. The
	// idempotency record that handed out this operation's 202 follows it
	// (replayBinding).
	CommittedUID string
	commit       func() (map[string]any, *hostError)
	terminalBody []byte
}

// ReferenceHost is a deliberately small, deterministic v1beta1 host used to
// prove the runner over real HTTP. It is not a reusable host and its reports
// are never publication-ready.
type ReferenceHost struct {
	mu sync.Mutex

	contract Contract
	catalog  *Catalog

	// resources is keyed by TENANT, space, group, kind, and name — the whole of a
	// resource's address (spec/decisions/0028). The tenant is first because it is
	// the outermost boundary: two tenants naming one resource in one space are two
	// records with two uids, and no request authenticated as one of them can name
	// the other's key at all.
	//
	// The tenant is not on the wire — a reference carries `{apiVersion, kind,
	// name}` and a request carries a space — so it comes from the authenticated
	// credential and is supplied to every lookup as a resourceScope. That is the
	// point of the type: a key cannot be built without one, so a read, a fence, a
	// delete, or a relation scan cannot silently address the whole host.
	resources map[string]*storedResource
	// relationHolders is the reverse index: target uid -> holder resource keys.
	// Both halves are already tenant-scoped: a uid names one record inside one
	// tenant, and a holder key carries its tenant.
	relationHolders map[string]map[string]struct{}
	// prepares maps a prepare binding to the canonical document it binds. The
	// digest is computed over a payload that carries the minting TENANT, so a
	// binding is spendable only by the tenant it was minted for; a digest that
	// leaked to another tenant recomputes to a different payload and buys nothing.
	prepares map[string][]byte
	replays  map[string]recordedReplay
	uploads  map[string]*artifactUpload
	// blobs and manifests are content-addressed and PHYSICALLY deduplicated:
	// one copy of the bytes, no matter how many tenants hold it.
	blobs      map[string][]byte
	manifests  map[string][]byte
	operations map[string]*hostOperation
	// migrationLedgers are database-local durable history. The key is the
	// database UID, never its reusable name; entries pin both path and digest.
	migrationLedgers map[string][]migrationLedgerEntry
	// blobTenants and manifestTenants are the LOGICAL access facts layered over
	// that dedup: which tenants hold each content address. A digest is a name for
	// bytes, not an entitlement to them, so a caller reads a manifest or a blob
	// only when its own tenant already holds that address — by having uploaded
	// the bytes, or by having committed a manifest naming them
	// (spec/decisions/0018).
	blobTenants     map[string]map[string]bool
	manifestTenants map[string]map[string]bool
	// moduleExports maps one module BLOB digest to the handlers that module's
	// default export exposes. See exportedHandlerViolation for why a reference
	// host that runs no JavaScript can still hold the contract's
	// handler_not_exported rule, and for exactly how far that reaches.
	moduleExports map[string][]string
	// moduleClasses maps one module BLOB digest to the CLASS names that
	// module exports, for the same reason and by the same key as
	// moduleExports: a Durable Workflow and an Actor Namespace name a class,
	// not a handler, and what serves it is the code the deployment selects.
	moduleClasses  map[string][]string
	uidCounter     int
	opCounter      int
	uploadCounter  int
	requestCounter int
}

// NewReferenceHost constructs the deterministic reference host over the
// verified contract and an installed catalog.
func NewReferenceHost(contract Contract, catalog *Catalog) *ReferenceHost {
	host := &ReferenceHost{
		contract:         contract,
		catalog:          catalog,
		resources:        map[string]*storedResource{},
		relationHolders:  map[string]map[string]struct{}{},
		prepares:         map[string][]byte{},
		replays:          map[string]recordedReplay{},
		uploads:          map[string]*artifactUpload{},
		blobs:            map[string][]byte{},
		manifests:        map[string][]byte{},
		operations:       map[string]*hostOperation{},
		migrationLedgers: map[string][]migrationLedgerEntry{},
		blobTenants:      map[string]map[string]bool{},
		manifestTenants:  map[string]map[string]bool{},
		moduleExports:    map[string][]string{},
		moduleClasses:    map[string][]string{},
	}
	input := contract.RunnerInput
	host.declareModuleExports(input.WorkerBundle.ModuleSource, input.WorkerBundle.ExportedHandlers)
	host.declareModuleExports(input.FetchOnlyBundle.ModuleSource, input.FetchOnlyBundle.ExportedHandlers)
	host.declareModuleClasses(input.WorkerBundle.ModuleSource, input.WorkerBundle.ExportedClasses)
	host.declareModuleClasses(input.FetchOnlyBundle.ModuleSource, input.FetchOnlyBundle.ExportedClasses)
	return host
}

// declareModuleExports records what one module's default export exposes, keyed
// by the content address of its bytes. Keying on the DIGEST rather than on a
// bundle or a resource name is what makes the fact a property of the code: the
// same bytes uploaded again, under any manifest, by any tenant, export the same
// handlers.
func (h *ReferenceHost) declareModuleExports(moduleSource string, handlers []string) {
	if moduleSource == "" || len(handlers) == 0 {
		return
	}
	h.moduleExports[formpackage.DigestBytes([]byte(moduleSource))] = append([]string(nil), handlers...)
}

// declareModuleClasses records which classes one module's bytes export, keyed
// by their content address. A module with no class export records NOTHING
// rather than an empty list, so "this host has never been told" stays
// distinguishable from "these bytes export no class" — the second is a refusal
// and the first must not be.
func (h *ReferenceHost) declareModuleClasses(moduleSource string, classes []string) {
	if moduleSource == "" {
		return
	}
	// An empty list is RECORDED, unlike declareModuleExports's. "These bytes
	// export no class" is the fact the refusal is built on, and it must stay
	// distinguishable from "this host was never told about these bytes" —
	// which is the answer that must never refuse.
	h.moduleClasses[formpackage.DigestBytes([]byte(moduleSource))] = append([]string{}, classes...)
}

// holdsBlob and holdsManifest answer the ONE authorization question the
// artifact surface asks: does this caller's tenant already hold this content
// address? Physical storage is shared; these sets are not.
func (h *ReferenceHost) holdsBlob(tenant, digest string) bool {
	return h.blobTenants[digest][tenant]
}

func (h *ReferenceHost) holdsManifest(tenant, digest string) bool {
	return h.manifestTenants[digest][tenant]
}

func (h *ReferenceHost) grantBlob(tenant, digest string) {
	if h.blobTenants[digest] == nil {
		h.blobTenants[digest] = map[string]bool{}
	}
	h.blobTenants[digest][tenant] = true
}

func (h *ReferenceHost) grantManifest(tenant, digest string) {
	if h.manifestTenants[digest] == nil {
		h.manifestTenants[digest] = map[string]bool{}
	}
	h.manifestTenants[digest][tenant] = true
}

// resourceScope is the boundary half of a resource's address: the authenticated
// tenant the record belongs to, and the space inside it.
//
// It exists as one value rather than as two parameters so that the tenant
// cannot be forgotten. Every store lookup, every store-wide scan, and every
// derived-rendering pass takes a scope, so a host-wide question is not something
// this code can ask by accident — which is exactly how a documented tenant
// boundary becomes an unmeasured one (spec/decisions/0028).
type resourceScope struct {
	Tenant string
	Space  string
}

func resourceKey(scope resourceScope, group, kind, name string) string {
	return strings.Join([]string{scope.Tenant, scope.Space, group, kind, name}, "\x00")
}

// groupOf rejoins the two ordinary path segments a namespaced Form group
// travels as — {formGroup}/{formVersion} — into the exact FormRef apiVersion
// string the wire, the queries, and the stored identities use unchanged
// (spec/decisions/0018).
func groupOf(name, version string) string { return name + "/" + version }

type hostAuthContext struct {
	Tenant    string
	Principal string
}

// scope is the resource address boundary one authenticated caller may address:
// its own tenant, and the space the request named. Nothing else this host stores
// is reachable through it.
func (auth hostAuthContext) scope(space string) resourceScope {
	return resourceScope{Tenant: auth.Tenant, Space: space}
}

// The three identities this host authenticates, named once so every ownership,
// holding, and claim fact can be stated in one place: two principals of one
// tenant, and one principal of another. The pair inside one tenant is what
// separates a PRINCIPAL rule from a TENANT rule; the second tenant is what
// separates a tenant rule from a host-wide one.
var (
	referencePrimaryAuth     = hostAuthContext{Tenant: "reference-tenant", Principal: "reference-primary"}
	referenceAlternateAuth   = hostAuthContext{Tenant: "reference-tenant", Principal: "reference-alternate"}
	referenceOtherTenantAuth = hostAuthContext{Tenant: "reference-other-tenant", Principal: "reference-primary"}
)

func hostRequestAuth(request *http.Request) (hostAuthContext, bool) {
	const prefix = "Bearer "
	authorization := request.Header.Get("Authorization")
	if !strings.HasPrefix(authorization, prefix) {
		return hostAuthContext{}, false
	}
	switch strings.TrimPrefix(authorization, prefix) {
	case referencePrimaryToken:
		return referencePrimaryAuth, true
	case referenceAlternateToken:
		return referenceAlternateAuth, true
	case referenceAlternateTenantToken:
		return referenceOtherTenantAuth, true
	default:
		return hostAuthContext{}, false
	}
}

// resourcePlaneRoute names one PATH-addressed resource surface: the method,
// and the action segment that follows the name (empty when the name is the last
// segment).
type resourcePlaneRoute struct {
	Method string
	Action string
}

// resourcePlaneHandlers is the complete set of routes this host serves under a
// resource NAME. It is a table rather than a switch so that the set is data the
// enumeration gate can read: every entry must appear in
// nameAddressedResourceSurfaces with a tenant check against it, and every
// path-addressed surface enumerated there must appear here. A name-addressed
// endpoint therefore cannot be added to this host without also being measured
// against the tenant boundary (spec/decisions/0028).
var resourcePlaneHandlers = map[resourcePlaneRoute]func(
	*ReferenceHost, http.ResponseWriter, *http.Request, string, string, string,
){
	{Method: http.MethodPut}:                     (*ReferenceHost).handleApply,
	{Method: http.MethodGet}:                     (*ReferenceHost).handleRead,
	{Method: http.MethodDelete}:                  (*ReferenceHost).handleDelete,
	{Method: http.MethodPost, Action: "observe"}: (*ReferenceHost).handleObserve,
	{Method: http.MethodPost, Action: "import"}:  (*ReferenceHost).handleImport,
}

// ResourceAddress is one stored resource's complete internal address: the
// tenant, the space, and the exact group, kind, and name it is keyed by
// (spec/decisions/0028).
type ResourceAddress struct {
	Tenant     string `json:"tenant"`
	Space      string `json:"space"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
}

// SnapshotResources is every resource this host currently holds, in stable key
// order, across every tenant.
//
// No wire surface answers this, deliberately: the lane has no list route, so a
// client can only ask about names it already knows. That is the right contract
// and the wrong tool for one job — proving that a real `terraform destroy` left
// NOTHING behind. Probing the names a configuration declared cannot see an
// orphan, which is exactly the failure a teardown proof exists to exclude.
//
// It is a read-only view for the authoring harness
// (internal/workerauthoring), and it is not a route. The v1beta1 conformance
// lane is black box by construction, so no required check may use it: a check
// that reached into the store would be measuring this implementation instead of
// the contract every other host is held to.
func (h *ReferenceHost) SnapshotResources() []ResourceAddress {
	h.mu.Lock()
	defer h.mu.Unlock()
	stored := h.sortedResources()
	out := make([]ResourceAddress, 0, len(stored))
	for _, resource := range stored {
		out = append(out, ResourceAddress{
			Tenant:     resource.Tenant,
			Space:      resource.Space,
			APIVersion: resource.group(),
			Kind:       resource.kind(),
			Name:       resource.Name,
		})
	}
	return out
}

func (h *ReferenceHost) ServeHTTP(w http.ResponseWriter, request *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")

	if _, ok := hostRequestAuth(request); !ok {
		h.writeError(w, "unauthenticated", "valid bearer authentication is required")
		return
	}
	probe := request.Header.Get(ErrorProbeHeader)
	if strings.HasPrefix(probe, ProbeErrorPrefix) {
		code := strings.TrimPrefix(probe, ProbeErrorPrefix)
		if _, known := h.contract.lane.ErrorHTTPStatus[code]; !known {
			h.writeError(w, "invalid_argument", "unknown conformance error probe")
			return
		}
		h.writeError(w, code, "deterministic conformance error probe")
		return
	}
	if probe != "" && probe != ProbeAsync && probe != ProbeTouchStatus && probe != ProbeExternalChange {
		h.writeError(w, "invalid_argument", "unknown conformance probe")
		return
	}

	path := request.URL.EscapedPath()
	if path == h.contract.DiscoveryPath && request.Method == http.MethodGet {
		h.handleDiscovery(w, request)
		return
	}
	prefix := h.contract.APIPath + "/"
	if !strings.HasPrefix(path, prefix) {
		h.writeError(w, "resource_not_found", "unknown route")
		return
	}
	rawParts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	parts := make([]string, len(rawParts))
	for index, rawPart := range rawParts {
		part, err := url.PathUnescape(rawPart)
		if err != nil {
			h.writeError(w, "invalid_argument", "invalid percent-encoding in request path")
			return
		}
		// A namespaced Form group travels as TWO ordinary path segments
		// (spec/decisions/0018). A segment that percent-encodes a slash names no
		// route this lane defines, and accepting it would make the host's routing
		// depend on whether an intermediary decoded %2F on the way in.
		if strings.Contains(part, "/") {
			h.writeError(w, "resource_not_found", "path segments must not percent-encode a slash")
			return
		}
		parts[index] = part
	}
	switch {
	case parts[0] == "forms" && len(parts) == 1 && request.Method == http.MethodGet:
		h.handleForms(w, request)
	case parts[0] == "form-definitions" && len(parts) == 4 && request.Method == http.MethodGet:
		h.handleFormDefinition(w, request, groupOf(parts[1], parts[2]), parts[3])
	case parts[0] == "resources" && len(parts) == 2 && parts[1] == "validate" && request.Method == http.MethodPost:
		h.handleValidate(w, request)
	case parts[0] == "resources" && len(parts) == 2 && parts[1] == "prepare" && request.Method == http.MethodPost:
		h.handlePrepare(w, request)
	// The two path-addressed cases are dispatched from resourcePlaneHandlers
	// rather than from a switch of their own. The map is one half of the pair
	// that makes the tenant matrix's coverage mechanical: a route this host
	// serves and nameAddressedResourceSurfaces does not enumerate, or an
	// enumerated surface this host does not serve, fails
	// TestEveryNameAddressedSurfaceIsRouted. There is no refresh action in the
	// v1beta1 lane — observe is the one fenced read-only re-observation — so
	// /refresh reaches no entry and is an unknown operation, exactly like any
	// other action the lane does not define.
	case parts[0] == "resources" && len(parts) == 5:
		group, kind, name := groupOf(parts[1], parts[2]), parts[3], parts[4]
		handler, defined := resourcePlaneHandlers[resourcePlaneRoute{Method: request.Method}]
		if !defined {
			h.writeError(w, "resource_not_found", "unknown operation")
			return
		}
		handler(h, w, request, group, kind, name)
	case parts[0] == "resources" && len(parts) == 6 && request.Method == http.MethodPost:
		group, kind, name, action := groupOf(parts[1], parts[2]), parts[3], parts[4], parts[5]
		handler, defined := resourcePlaneHandlers[resourcePlaneRoute{Method: http.MethodPost, Action: action}]
		if !defined {
			h.writeError(w, "resource_not_found", "unknown operation")
			return
		}
		handler(h, w, request, group, kind, name)
	case parts[0] == "operations" && len(parts) == 2 && request.Method == http.MethodGet:
		h.handleOperationGet(w, request, parts[1])
	case parts[0] == "operations" && len(parts) == 3 && parts[2] == "cancel" && request.Method == http.MethodPost:
		h.handleOperationCancel(w, request, parts[1])
	case parts[0] == "artifacts":
		h.routeArtifacts(w, request, parts)
	case parts[0] == "support":
		h.routeSupport(w, request, parts)
	default:
		h.writeError(w, "resource_not_found", "unknown route")
	}
}

func (h *ReferenceHost) handleDiscovery(w http.ResponseWriter, request *http.Request) {
	origin := "http://" + request.Host
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"api_versions": []string{h.contract.APIVersion},
		"features": map[string]bool{
			"service_forms":          true,
			"exact_form_ref":         true,
			"optimistic_concurrency": true,
			"idempotent_lifecycle":   true,
			"operations":             true,
			"artifact_upload":        true,
			"support_profiles":       true,
		},
		"endpoints": map[string]string{
			"api":        origin + h.contract.APIPath,
			"artifacts":  origin + h.contract.APIPath + "/artifacts",
			"operations": origin + h.contract.APIPath + "/operations",
			"support":    origin + h.contract.APIPath + "/support",
		},
	})
}

// exactQueryForm resolves the closed exact-FormRef query vocabulary. The
// packageDigest deliberately has no query key: audit evidence never enters
// identity, so an unknown key (including packageDigest) fails closed.
func (h *ReferenceHost) exactQueryForm(query url.Values) (*InstalledForm, string, *hostError) {
	allowed := map[string]bool{
		"space": true, "group": true, "kind": true,
		"definitionVersion": true, "schemaDigest": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			return nil, "", stableError("invalid_argument", "exact Form query vocabulary is closed")
		}
	}
	space := query.Get("space")
	if !validSpaceID(space) {
		return nil, "", stableError("invalid_argument", "exactly one valid space is required")
	}
	// The query is answered about the WHOLE identity or not at all: the four
	// members are one key, and a host that resolved the group and kind first
	// would be one `if` away from answering about a contract it was not asked
	// about (decision 0022).
	form := h.catalog.exact(FormRef{
		APIVersion:        query.Get("group"),
		Kind:              query.Get("kind"),
		DefinitionVersion: query.Get("definitionVersion"),
		SchemaDigest:      query.Get("schemaDigest"),
	})
	if form == nil {
		return nil, "", stableError("form_unknown", "exact Form is unknown")
	}
	return form, space, nil
}

func (h *ReferenceHost) handleForms(w http.ResponseWriter, request *http.Request) {
	if h.contract.lane.FormsResponseEnumerates {
		h.handleFormsEnumerated(w, request)
		return
	}
	form, _, hostErr := h.exactQueryForm(request.URL.Query())
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"forms": []map[string]any{h.availabilityJSON(form)},
	})
}

// handleFormsEnumerated answers the v1beta2 route: every one of the six query
// keys is OPTIONAL and narrows the answer, so the route enumerates the
// installed set and the fully-keyed probe is just the narrowest case of it.
// The v1beta1 shape required the whole identity and capped the array at one,
// which made a route named "forms" unable to tell anyone which Forms existed.
func (h *ReferenceHost) handleFormsEnumerated(w http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	allowed := map[string]bool{
		"space": true, "group": true, "version": true, "kind": true,
		"definitionVersion": true, "schemaDigest": true,
	}
	for key, values := range query {
		if !allowed[key] || len(values) != 1 {
			h.writeError(w, "invalid_argument", "Form query vocabulary is closed")
			return
		}
	}
	if space := query.Get("space"); space != "" && !validSpaceID(space) {
		h.writeError(w, "invalid_argument", "space is malformed")
		return
	}
	// A narrowing key is matched against the whole exact identity it names.
	// `group` and `version` are the two halves of an apiVersion, because the
	// path shape already splits them and a client that had to rejoin them
	// would be re-deriving what the lane took apart.
	group, version := query.Get("group"), query.Get("version")
	matches := make([]map[string]any, 0, len(h.catalog.Forms))
	for _, form := range h.catalog.sortedForms() {
		formGroup, formVersion, split := splitAPIVersion(form.Ref.APIVersion)
		if !split {
			continue
		}
		switch {
		case group != "" && group != formGroup,
			version != "" && version != formVersion,
			query.Get("kind") != "" && query.Get("kind") != form.Ref.Kind,
			query.Get("definitionVersion") != "" && query.Get("definitionVersion") != form.Ref.DefinitionVersion,
			query.Get("schemaDigest") != "" && query.Get("schemaDigest") != form.Ref.SchemaDigest:
			continue
		}
		matches = append(matches, h.availabilityJSON(form))
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{"forms": matches})
}

// availabilityJSON renders one availability answer. `deprecated` is a member
// of the v1beta1 shape only: it was published with no source of truth behind
// it, so v1beta2 removed it rather than invent one.
func (h *ReferenceHost) availabilityJSON(form *InstalledForm) map[string]any {
	answer := map[string]any{
		"identity":             h.identityJSON(form),
		"definitionKnown":      true,
		"installed":            true,
		"executable":           true,
		"activated":            true,
		"availableToPrincipal": true,
		"operations":           form.operations(),
	}
	if h.contract.lane.AvailabilityCarriesDeprecated {
		answer["deprecated"] = false
	}
	return answer
}

// splitAPIVersion splits a namespaced Form group into its group name and group
// version, the two path segments the lane carries them as.
func splitAPIVersion(apiVersion string) (string, string, bool) {
	cut := strings.LastIndex(apiVersion, "/")
	if cut <= 0 || cut == len(apiVersion)-1 {
		return "", "", false
	}
	return apiVersion[:cut], apiVersion[cut+1:], true
}

func (h *ReferenceHost) handleFormDefinition(w http.ResponseWriter, request *http.Request, group, kind string) {
	form, _, hostErr := h.exactQueryForm(request.URL.Query())
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if form.Ref.APIVersion != group || form.Ref.Kind != kind {
		h.writeError(w, "invalid_argument", "path and exact query identities differ")
		return
	}
	response := map[string]any{
		"identity":      h.identityJSON(form),
		"displayName":   form.Title,
		"desiredSchema": form.DesiredSchema,
	}
	if form.Description != "" {
		response["description"] = form.Description
	}
	h.writeJSON(w, http.StatusOK, "", response)
}

func (h *ReferenceHost) identityJSON(form *InstalledForm) map[string]any {
	identity := map[string]any{"formRef": refJSON(form.Ref)}
	if form.PackageDigest != "" {
		identity["packageDigest"] = form.PackageDigest
	}
	return identity
}

func refJSON(ref FormRef) map[string]any {
	return map[string]any{
		"apiVersion":        ref.APIVersion,
		"kind":              ref.Kind,
		"definitionVersion": ref.DefinitionVersion,
		"schemaDigest":      ref.SchemaDigest,
	}
}

// Wire request shapes. Strict I-JSON decoding rejects duplicate members and
// unknown envelope fields before any mutation.
type wireFormReference struct {
	FormRef       FormRef `json:"formRef"`
	PackageDigest string  `json:"packageDigest,omitempty"`
}

type wireMetadata struct {
	Name       string `json:"name"`
	Space      string `json:"space"`
	UID        string `json:"uid,omitempty"`
	Generation string `json:"generation,omitempty"`
	Revision   string `json:"revision,omitempty"`
}

type resourceWire struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Form       *wireFormReference `json:"form"`
	Metadata   wireMetadata       `json:"metadata"`
	Spec       map[string]any     `json:"spec"`
}

type reviewWire struct {
	PrepareDigest string `json:"prepareDigest"`
	// SpecDigest is the OPTIONAL echo of what prepare answered. A client that
	// sends it is asking the host to confirm that the spec it is applying is
	// the spec it prepared, which is worth something precisely because the
	// prepare digest alone cannot say so to the client's own satisfaction.
	SpecDigest string `json:"specDigest,omitempty"`
}

type applyWire struct {
	resourceWire
	Review             *reviewWire `json:"review"`
	ExpectedUID        string      `json:"expectedUid,omitempty"`
	ExpectedGeneration string      `json:"expectedGeneration,omitempty"`
}

type importWire struct {
	resourceWire
	NativeID string `json:"nativeId"`
}

func (h *ReferenceHost) readBody(request *http.Request) ([]byte, *hostError) {
	raw, err := io.ReadAll(io.LimitReader(request.Body, maxBodyBytes+1))
	if err != nil || len(raw) > maxBodyBytes {
		return nil, stableError("invalid_argument", "invalid request body")
	}
	return raw, nil
}

// resolveResourceWire enforces the request-side identity contract shared by
// validate, prepare, apply, and import bodies.
func (h *ReferenceHost) resolveResourceWire(body *resourceWire) (*InstalledForm, *hostError) {
	if body.Form == nil {
		return nil, stableError("invalid_argument", "an exact FormRef is required")
	}
	form := h.catalog.exact(body.Form.FormRef)
	if form == nil {
		return nil, stableError("form_unknown", "exact Form is unknown")
	}
	if body.APIVersion != form.Ref.APIVersion || body.Kind != form.Ref.Kind {
		return nil, stableError("invalid_argument", "resource apiVersion/kind and exact FormRef identities differ")
	}
	// The packageDigest is audit evidence only: it must be well-formed when
	// present, but it never has to match the installed digest and never
	// changes which resource is addressed.
	if body.Form.PackageDigest != "" && !formpackage.ValidDigest(body.Form.PackageDigest) {
		return nil, stableError("invalid_argument", "form.packageDigest must be a lowercase sha256:<hex> digest")
	}
	if !resourceNamePattern.MatchString(body.Metadata.Name) {
		return nil, stableError("invalid_argument", "resource metadata.name is invalid")
	}
	if !validSpaceID(body.Metadata.Space) {
		return nil, stableError("invalid_argument", "resource metadata.space is invalid")
	}
	return form, nil
}

// validSpaceID ports $defs/spaceId of
// spec/schemas/host-api-wire-v1beta1.schema.json into the host: a SpaceID is
// valid UTF-8 of 1..255 Unicode code points, carries no leading or trailing
// Unicode White_Space code point or U+FEFF, and contains no C0/C1 control
// character and no slash. The value is opaque and case-sensitive: the host
// never trims, normalizes, or case-folds it, so anything outside the grammar
// fails closed instead of being repaired into a different space.
func validSpaceID(space string) bool {
	if !utf8.ValidString(space) {
		return false
	}
	runes := []rune(space)
	if len(runes) < 1 || len(runes) > 255 {
		return false
	}
	if isSpaceBoundaryRune(runes[0]) || isSpaceBoundaryRune(runes[len(runes)-1]) {
		return false
	}
	for _, candidate := range runes {
		if candidate == '/' || candidate <= 0x1f || (candidate >= 0x7f && candidate <= 0x9f) {
			return false
		}
	}
	return true
}

func isSpaceBoundaryRune(candidate rune) bool {
	return unicode.IsSpace(candidate) || candidate == '\uFEFF'
}

func (h *ReferenceHost) specDiagnostics(form *InstalledForm, spec map[string]any) ([]map[string]any, *hostError) {
	if spec == nil {
		spec = map[string]any{}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, stableError("invalid_argument", "spec is not encodable JSON")
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, stableError("invalid_argument", "spec is not decodable JSON")
	}
	if err := form.compiled.Validate(value); err != nil {
		return []map[string]any{{
			"severity": "error",
			"message":  "desired spec violates the installed Form Definition: " + err.Error(),
		}}, nil
	}
	// Schema validity is never sufficient (spec/conformance.md, decision 0014).
	// The runtime ABI closes the handler surface, and it does so from the exact
	// contract the host installed rather than from the Form's enum, so a host
	// whose installed schema drifted laxer still refuses (decision 0019).
	if violation := h.declaredHandlerViolation(form, spec); violation != "" {
		return []map[string]any{{"severity": "error", "message": violation}}, nil
	}
	// The cron grammar is decided by a parser rather than by the pattern the
	// Definition carries, for the same reason: a structural minimum admits
	// shapes that name no schedule (decision 0020).
	if violation := cronExpressionViolation(form, spec); violation != "" {
		return []map[string]any{{"severity": "error", "message": violation}}, nil
	}
	return []map[string]any{}, nil
}

func (h *ReferenceHost) handleValidate(w http.ResponseWriter, request *http.Request) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body resourceWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	form, hostErr := h.resolveResourceWire(&body)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"valid":       len(diagnostics) == 0,
		"diagnostics": diagnostics,
	})
}

// Create markers bound into a prepareDigest when the target resource does
// not exist yet: there is no host-issued uid, and generation "0" precedes
// every real generation.
const (
	prepareCreateUID        = ""
	prepareCreateGeneration = "0"
)

// prepareBindingPayload is the canonical document the prepareDigest binds:
// the exact spec digest, the exact ADDRESS — tenant and space included — the
// CURRENT uid and generation of the target resource (create markers when it does
// not exist), and a host plan marker. The packageDigest is deliberately excluded
// because it is audit evidence, not identity.
//
// The tenant is in the payload for the same reason it is in the store key. A
// prepare binding is a short-lived permission to mutate one exact resource, and
// a binding minted inside one tenant must not be spendable by another: a create
// binding carries no uid and no generation to distinguish it, so without the
// tenant the two tenants' bindings for one name in one space are the same
// digest, and a leaked digest would let a stranger spend it (spec/decisions/0028).
func prepareBindingPayload(
	specDigest string, ref FormRef, name string, scope resourceScope, uid, generation string,
) ([]byte, string, error) {
	payload := map[string]any{
		"plan":              "deterministic-reference-host-plan",
		"specDigest":        specDigest,
		"group":             ref.APIVersion,
		"kind":              ref.Kind,
		"definitionVersion": ref.DefinitionVersion,
		"schemaDigest":      ref.SchemaDigest,
		"name":              name,
		"tenant":            scope.Tenant,
		"space":             scope.Space,
		"uid":               uid,
		"generation":        generation,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	canonical, err := formpackage.Canonicalize(raw)
	if err != nil {
		return nil, "", err
	}
	return canonical, formpackage.DigestBytes(canonical), nil
}

func specCanonicalDigest(spec map[string]any) (string, error) {
	if spec == nil {
		spec = map[string]any{}
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return formpackage.DigestCanonicalJSON(raw)
}

func (h *ReferenceHost) handlePrepare(w http.ResponseWriter, request *http.Request) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body resourceWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	form, hostErr := h.resolveResourceWire(&body)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(diagnostics) != 0 {
		h.writeError(w, "invalid_argument", "prepare requires a valid desired spec")
		return
	}
	specDigest, err := specCanonicalDigest(body.Spec)
	if err != nil {
		h.writeError(w, "invalid_argument", "spec is not canonicalizable I-JSON")
		return
	}
	// The prepare precondition is "generation-fence-when-updating": an
	// existing target requires the exact update fence, and the CURRENT uid and
	// generation are bound into the digest so a later apply at any other
	// generation cannot reuse it. A create binds the create markers.
	fence := request.Header.Get(expectedGenerationHeader)
	// The target is resolved inside the CALLER'S tenant. A prepare that resolved
	// host-wide would mint a create binding against another tenant's live
	// resource, or refuse a create because a stranger holds the name.
	caller, _ := hostRequestAuth(request)
	scope := caller.scope(body.Metadata.Space)
	current := h.resourceUnderExactRef(scope, form.Ref, body.Metadata.Name)
	uid, generation := prepareCreateUID, prepareCreateGeneration
	if current != nil {
		if fence == "" {
			h.writeError(w, "invalid_argument", "prepare on an existing resource requires the "+expectedGenerationHeader+" fence")
			return
		}
		fenceValue, fenceErr := strconv.ParseInt(fence, 10, 64)
		if fenceErr != nil || fence[0] == '0' || fenceValue < 1 {
			h.writeError(w, "invalid_argument", "prepare generation fence must be a canonical positive decimal")
			return
		}
		if fenceValue != current.Generation {
			h.writeError(w, "generation_conflict", "prepare generation fence is stale")
			return
		}
		uid = current.UID
		generation = fence
	} else if fence != "" {
		h.writeError(w, "resource_not_found", "prepare generation fence names an absent resource")
		return
	}
	payload, prepareDigest, err := prepareBindingPayload(specDigest, form.Ref, body.Metadata.Name, scope, uid, generation)
	if err != nil {
		h.writeError(w, "internal_error", err.Error())
		return
	}
	h.prepares[prepareDigest] = payload
	echoed := map[string]any{
		"apiVersion": body.APIVersion,
		"kind":       body.Kind,
		"form":       wireFormReferenceJSON(body.Form),
		"metadata":   map[string]any{"name": body.Metadata.Name, "space": body.Metadata.Space},
		"spec":       specOrEmpty(body.Spec),
	}
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"resource": echoed,
		"review": map[string]any{
			"prepareDigest": prepareDigest,
			"specDigest":    specDigest,
		},
	})
}

func wireFormReferenceJSON(reference *wireFormReference) map[string]any {
	out := map[string]any{"formRef": refJSON(reference.FormRef)}
	if reference.PackageDigest != "" {
		out["packageDigest"] = reference.PackageDigest
	}
	return out
}

func specOrEmpty(spec map[string]any) map[string]any {
	if spec == nil {
		return map[string]any{}
	}
	return spec
}

// validateDesiredSemantics enforces the portable cross-resource rules that a
// desired-state schema cannot express, and returns the resolved relation
// records to store. Every rule runs before any mutation on both the apply and
// the import path, and is re-run at async COMMIT time so a 202 can never commit
// against a store that has since moved.
//
// Relation resolution is not a binding scan. Every reference a Form derives
// from its desired schema is resolved to a stored resource and pinned by that
// resource's UID, so a deployment's worker version, a version's bundle, and an
// attachment's worker are protected exactly as a typed binding is.
//
// The caller is carried in because every one of these rules is decided inside
// the caller's TENANT. Relation resolution addresses the caller's own scope, so
// a reference never reaches a resource another tenant holds under the same name
// (spec/decisions/0028); resolving a referenced artifact manifest asks the same
// per-tenant holding question the artifact read surfaces ask
// (spec/decisions/0018); and a hostname claim is unique per tenant across every
// space of that tenant (spec/decisions/0026). It is a value rather than a request
// so the async commit path re-derives all of it from the caller the mutation was
// accepted from, long after that request is gone.
func (h *ReferenceHost) validateDesiredSemantics(
	caller hostAuthContext,
	form *InstalledForm,
	scope resourceScope,
	name string,
	spec map[string]any,
) ([]storedRelation, *hostError) {
	// Two pure spec-shape rules first: neither needs another resource, so
	// neither should depend on one resolving.
	if hostErr := validateDeploymentWeightSum(form, spec); hostErr != nil {
		return nil, hostErr
	}
	if hostErr := validateEnvironmentNamespace(form, spec); hostErr != nil {
		return nil, hostErr
	}
	if hostErr := h.validateDeclaredHandlers(form, spec); hostErr != nil {
		return nil, hostErr
	}
	if hostErr := validateCronExpression(form, spec); hostErr != nil {
		return nil, hostErr
	}
	relations, hostErr := h.resolveRelations(form, scope, spec)
	if hostErr != nil {
		return nil, hostErr
	}
	if expectedKind, artifactBacked := artifactManifestKindForForm(form.Ref.Kind); artifactBacked {
		if _, hostErr := h.requireReferencedArtifactManifest(caller, spec, expectedKind); hostErr != nil {
			return nil, hostErr
		}
	}
	if hostErr := h.validateWorkerVersionAssets(caller, form, scope, spec, relations); hostErr != nil {
		return nil, hostErr
	}
	if hostErr := h.validateSQLiteMigrationApplication(caller, form, scope, relations); hostErr != nil {
		return nil, hostErr
	}
	// The Worker aggregate rules read the relations this apply just resolved,
	// never the names in the spec: every one of them is a statement about one
	// worker INCARNATION (spec/decisions/0016).
	if hostErr := h.validateWorkerAggregate(form, scope, name, spec, relations); hostErr != nil {
		return nil, hostErr
	}
	return relations, nil
}

// validateDeploymentWeightSum enforces the WorkerDeployment rule the Form
// description calls host-validated: traffic weights are basis points and must
// sum to exactly 10000, because a JSON Schema cannot add numbers. It is the
// weight half of the deployment integrity rule; the ownership, uniqueness, and
// availability halves need resolved relations and live in
// validateWorkerDeployment.
func validateDeploymentWeightSum(form *InstalledForm, spec map[string]any) *hostError {
	if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != workerDeploymentKind {
		return nil
	}
	versions, _ := spec["versions"].([]any)
	if len(versions) == 0 {
		return stableError("invalid_argument", "a WorkerDeployment requires at least one weighted version")
	}
	total := int64(0)
	for _, member := range versions {
		entry, _ := member.(map[string]any)
		weight, ok := entry["weight"].(json.Number)
		if !ok {
			return stableError("invalid_argument", "WorkerDeployment version weight must be an integer")
		}
		value, err := strconv.ParseInt(weight.String(), 10, 64)
		if err != nil {
			return stableError("invalid_argument", "WorkerDeployment version weight must be an integer")
		}
		total += value
	}
	if total != workerDeploymentWeightTotal {
		return stableError(
			"invalid_argument",
			"WorkerDeployment traffic weights must sum to exactly "+
				strconv.Itoa(workerDeploymentWeightTotal)+" basis points",
		)
	}
	return nil
}

// mutationFence carries the exact precondition headers of one mutation. The
// async path evaluates them again at commit time, so they are captured as
// values instead of read from a request that is long gone.
type mutationFence struct {
	IfMatch     string
	IfNoneMatch string
	Generation  string
}

func mutationFenceOf(request *http.Request) mutationFence {
	return mutationFence{
		IfMatch:     request.Header.Get("If-Match"),
		IfNoneMatch: request.Header.Get("If-None-Match"),
		Generation:  request.Header.Get(expectedGenerationHeader),
	}
}

// deleteFenceStale decides one delete's preconditions against the incarnation
// the delete resolved to. It is called twice for an asynchronous delete — once
// at accept and once at commit — because the store moves in between.
//
// A delete is a DESIRED-STATE mutation: it withdraws the desired state
// entirely. So it fences on the desired generation, exactly like every other
// desired-state mutation of this lane, and a stale fence is
// `generation_conflict` (spec/decisions/0011). The fence is REQUIRED; an
// unfenced delete is refused, because a client that names no incarnation has
// said nothing about which one it means to remove.
//
// `If-Match` on the representation revision is accepted and honored, and it is
// NOT required. A revision moves for reasons the deleting client did not cause
// and cannot see — a host-side status change, and above all the derived
// rendering of decision 0016 rule 9, where removing a dependent re-renders the
// resource that is about to be deleted next. Requiring it would make an
// ordinary teardown fail on a revision the teardown itself moved, so a host
// that demands it has broken `destroy` for every author. A client that means
// "remove exactly the representation I read" may still say so, and is answered
// `revision_conflict` when the representation moved.
func deleteFenceStale(fence mutationFence, resource *storedResource) *hostError {
	if fence.Generation == "" {
		return stableError(
			"invalid_argument",
			"delete requires the "+expectedGenerationHeader+" fence",
		)
	}
	if fence.Generation != strconv.FormatInt(resource.Generation, 10) {
		return stableError("generation_conflict", "delete generation fence is stale")
	}
	if fence.IfMatch != "" && fence.IfMatch != quotedRevision(resource.Revision) {
		return stableError("revision_conflict", "delete revision fence is stale")
	}
	return nil
}

// acceptedTarget is the exact resource incarnation ONE accepted mutation was
// admitted against, recorded on the Operation at accept time: the store key it
// was addressed through, the exact FormRef it is recorded under, the host-issued
// uid of that one incarnation, and the fence the acceptance was granted under.
//
// A commit closure resolves through this record and never re-derives a target
// from the name it was addressed by. The store key carries only space, group,
// kind, and name, because a name is unique per kind and a reference carries no
// definition version; a resource removed out of band and re-created under the
// same name therefore sits at exactly the same key — under the same contract or
// under another definition version of it, with a new uid, and at revision 1,
// which is a revision fence the original was very likely accepted under. Any
// commit that re-derived its target from that key would delete or rewrite a
// resource the operation was never accepted for, under a different exact
// FormRef and a different uid, reporting success. That is the substitution
// decision 0015 closed for relations — identity is pinned to what was actually
// resolved, never re-derived from a name later — stated about the operation's
// own target.
//
// A mutation accepted against no incarnation carries the zero value: a create
// is fenced against the free NAME rather than against an incarnation, so it has
// nothing to pin and its own fence decides at commit.
type acceptedTarget struct {
	Key   string
	Ref   FormRef
	UID   string
	Fence mutationFence
}

// acceptedIncarnation re-resolves the incarnation an accepted operation was
// admitted against, and refuses to hand back anything else.
//
// The two refusals name what actually happened, out of the closed taxonomy: the
// name is held by another incarnation — a different uid, whether or not it is
// recorded under another contract — so the incarnation this operation was
// accepted for moved (`uid_mismatch`, 409); or nothing holds the name at all
// (`resource_not_found`, 404). Neither is retryable, because no amount of
// waiting turns one incarnation back into another.
func (h *ReferenceHost) acceptedIncarnation(target acceptedTarget) (*storedResource, *hostError) {
	if target.Key == "" {
		return nil, nil
	}
	current := h.resources[target.Key]
	if current == nil {
		return nil, stableError(
			"resource_not_found",
			"the resource this operation was accepted for is absent",
		)
	}
	if current.UID != target.UID || current.Ref != target.Ref {
		return nil, stableError(
			"uid_mismatch",
			"the resource this operation was accepted for ("+target.UID+" under "+
				target.Ref.DefinitionVersion+"/"+target.Ref.SchemaDigest+") is gone; the name is now held by "+
				current.UID+" under "+current.Ref.DefinitionVersion+"/"+current.Ref.SchemaDigest,
		)
	}
	return current, nil
}

// mutationFences resolves the apply/import precondition surface. It returns
// the existing resource (nil on create intent) or a stable error.
func (h *ReferenceHost) mutationFences(
	fences mutationFence,
	form *InstalledForm,
	scope resourceScope,
	name string,
	bodyExpectedGeneration, expectedUID string,
) (existing *storedResource, create bool, hostErr *hostError) {
	ifNoneMatch := fences.IfNoneMatch
	if ifNoneMatch != "" && ifNoneMatch != "*" {
		return nil, false, stableError("invalid_argument", "If-None-Match only supports *")
	}
	headerGeneration := fences.Generation
	// The store key is per tenant, space, group, kind, and name; the RECORDED ref
	// then decides whether this request addresses the resource at all. A create
	// fence still sees a name that is taken under another contract, because a name
	// is unique per kind — but an update, an observe, or a delete under a ref the
	// resource was not applied under addresses nothing (decision 0022).
	//
	// The name is taken WITHIN THE CALLER'S TENANT and nowhere else: a create
	// fenced against a host-wide name would let one tenant deny another the use of
	// a name, and would be a membership oracle over the whole host
	// (decision 0028).
	occupant := h.resources[resourceKey(scope, form.Ref.APIVersion, form.Ref.Kind, name)]
	current := occupant
	if current != nil && current.Ref != form.Ref {
		current = nil
	}
	if ifNoneMatch == "*" {
		if headerGeneration != "" || bodyExpectedGeneration != "" {
			return nil, false, stableError("invalid_argument", "create must not carry a generation fence")
		}
		if expectedUID != "" {
			return nil, false, stableError("uid_mismatch", "create cannot pin an expected uid")
		}
		// A create is fenced against the NAME, not against the ref: a resource
		// name is unique within one space, group, and kind, because a reference
		// carries no definition version and could not choose between two.
		if occupant != nil {
			return nil, false, stableError("generation_conflict", "resource already exists under If-None-Match: *")
		}
		return nil, true, nil
	}
	fence := headerGeneration
	if fence == "" {
		fence = bodyExpectedGeneration
	} else if bodyExpectedGeneration != "" && bodyExpectedGeneration != fence {
		return nil, false, stableError("invalid_argument", "generation fences in header and body differ")
	}
	if fence == "" {
		return nil, false, stableError("invalid_argument", "mutation requires If-None-Match: * or a generation fence")
	}
	fenceValue, err := strconv.ParseInt(fence, 10, 64)
	if err != nil || fence[0] == '0' || fenceValue < 1 {
		return nil, false, stableError("invalid_argument", "generation fence must be a canonical positive decimal")
	}
	if current == nil {
		return nil, false, stableError("resource_not_found", "resource is absent")
	}
	if expectedUID != "" && expectedUID != current.UID {
		return nil, false, stableError("uid_mismatch", "expectedUid does not match the host-issued uid")
	}
	if fenceValue != current.Generation {
		return nil, false, stableError("generation_conflict", "generation fence is stale")
	}
	return current, false, nil
}

func (h *ReferenceHost) handleApply(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body applyWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	if h.tryReplay(w, request, raw, body.Metadata.Space) {
		return
	}
	if body.Metadata.Name != name || body.Kind != kind || body.APIVersion != group {
		h.writeError(w, "invalid_argument", "resource identity differs from the request target")
		return
	}
	form, hostErr := h.resolveResourceWire(&body.resourceWire)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	// Materialization happens HERE: before validation, before the spec digest,
	// before the applyOnce closure captures body.Spec, and before anything is
	// stored or echoed. Anywhere later and the prepare digest a client already
	// holds would not match what this apply computes.
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(diagnostics) != 0 {
		h.writeError(w, "invalid_argument", "desired spec violates the installed Form Definition")
		return
	}
	if body.Review == nil || !formpackage.ValidDigest(body.Review.PrepareDigest) {
		h.writeError(w, "invalid_argument", "apply requires review.prepareDigest")
		return
	}
	specDigest, err := specCanonicalDigest(body.Spec)
	if err != nil {
		h.writeError(w, "invalid_argument", "spec is not canonicalizable I-JSON")
		return
	}
	// Echoing the review object a prepare handed back is never itself a
	// refusal, so an ABSENT specDigest is fine; a PRESENT one that disagrees
	// with the spec actually being applied is the substitution this member
	// exists to catch.
	if body.Review.SpecDigest != "" && body.Review.SpecDigest != specDigest {
		h.writeError(w, "invalid_argument", "review.specDigest does not match the spec being applied")
		return
	}
	space := body.Metadata.Space
	fences := mutationFenceOf(request)
	prepareDigest := body.Review.PrepareDigest
	// The authenticated caller is captured with the fences and for the same
	// reason: this mutation addresses ONE tenant's resource plane — its own — and
	// the 202 path re-resolves every one of those lookups at commit time, when the
	// request no longer exists.
	caller, _ := hostRequestAuth(request)
	scope := caller.scope(space)
	// applyOnce is the complete pre-mutation gauntlet plus the commit. The
	// synchronous path runs it inline; the 202 path runs the SAME function at
	// poll time, so an accepted operation re-derives every precondition
	// against the store as it is when the mutation actually lands.
	applyOnce := func() (*storedResource, bool, *hostError) {
		relations, hostErr := h.validateDesiredSemantics(caller, form, scope, name, body.Spec)
		if hostErr != nil {
			return nil, false, hostErr
		}
		existing, create, hostErr := h.mutationFences(
			fences, form, scope, name, body.ExpectedGeneration, body.ExpectedUID,
		)
		if hostErr != nil {
			return nil, false, hostErr
		}
		// Capability, not role, decides what an apply to an EXISTING resource
		// may do. A Form Definition that omits update advertises no in-place
		// spec change, so accepting one here would let a host silently perform
		// an operation the Definition told every client was unavailable. The
		// refusal lands before any mutation, and before the prepare binding is
		// even consulted.
		if !create && !form.declaresUpdate() && specDigest != existing.SpecDigest {
			return nil, false, stableError(
				"invalid_argument",
				"the installed Form Definition declares no update capability; a spec-changing apply is not representable",
			)
		}
		if !create && form.Role == "revision" {
			return nil, false, stableError("invalid_argument", "update to a revision-role resource is not representable")
		}
		// The expected prepare binding is recomputed from the POST-fence
		// resolved state: a prepareDigest minted for another spec, another
		// resource, or another generation fails invalid_argument BEFORE any
		// mutation.
		expectedUID, expectedGeneration := prepareCreateUID, prepareCreateGeneration
		if !create {
			expectedUID = existing.UID
			expectedGeneration = strconv.FormatInt(existing.Generation, 10)
		}
		payload, _, err := prepareBindingPayload(specDigest, form.Ref, name, scope, expectedUID, expectedGeneration)
		if err != nil {
			return nil, false, stableError("internal_error", err.Error())
		}
		if !bytes.Equal(h.prepares[prepareDigest], payload) {
			return nil, false, stableError(
				"invalid_argument",
				"prepared review does not bind this exact resource at its current generation",
			)
		}
		if hostErr := h.applySQLiteMigrationSuffix(caller, form, scope, relations); hostErr != nil {
			return nil, false, hostErr
		}
		next := h.nextResource(form, existing, create, caller.Tenant, space, name, body.Spec, specDigest, false)
		repinRelations(next, existing, relations)
		h.storeResource(next)
		return next, create, nil
	}
	if request.Header.Get(ErrorProbeHeader) == ProbeAsync {
		// The incarnation this apply is accepted against, pinned HERE, while the
		// request that resolved it still exists. An apply fenced on
		// If-None-Match: * was accepted against a free NAME and pins nothing — its
		// fence is the name, and applyOnce re-evaluates it. An update was accepted
		// against exactly one incarnation, and rewriting any other would be the
		// same substitution the delete path refuses: the generation fence a
		// replacement satisfies says nothing about which resource it belongs to.
		accepted := acceptedTarget{Fence: fences}
		if fences.IfNoneMatch != "*" {
			if current := h.resourceUnderExactRef(scope, form.Ref, name); current != nil {
				accepted = acceptedTarget{Key: current.key(), Ref: current.Ref, UID: current.UID, Fence: fences}
			}
		}
		h.acceptOperation(w, request, raw, space, &hostOperation{
			Accepted: accepted,
			commit: func() (map[string]any, *hostError) {
				if _, hostErr := h.acceptedIncarnation(accepted); hostErr != nil {
					return nil, hostErr
				}
				next, _, hostErr := applyOnce()
				if hostErr != nil {
					return nil, hostErr
				}
				return map[string]any{"resource": h.renderResource(next)}, nil
			},
		})
		return
	}
	next, create, hostErr := applyOnce()
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	response := encodeJSONBody(h.renderResource(next))
	etag := quotedRevision(next.Revision)
	h.writeRaw(w, status, etag, response)
	h.recordReplay(request, raw, space, status, etag, response, replayBinding{UID: next.UID})
}

func artifactManifestKindForForm(formKind string) (string, bool) {
	switch formKind {
	case workerBundleKind:
		return workerBundleKind, true
	case staticAssetBundleKind:
		return staticAssetBundleKind, true
	case sqliteMigrationSetKind:
		return migrationBundleKind, true
	default:
		return "", false
	}
}

// requireReferencedArtifactManifest resolves the one thing an artifact-backed
// revision carries — manifestDigest — and holds the manifest it names to the
// expected closed kind before anything is mutated.
//
// Resolution is per TENANT, exactly as the read surfaces are: a digest names
// bytes and entitles nobody to them, so a manifest the caller's tenant does not
// hold is answered artifact_missing — the same answer, with the same message, an
// uncommitted digest gets (spec/decisions/0018). Distinguishing "exists but not
// yours" from "does not exist" would make USING a leaked digest an existence
// oracle over exactly what reading it already may not disclose.
func (h *ReferenceHost) requireReferencedArtifactManifest(
	caller hostAuthContext, spec map[string]any, expectedKind string,
) (artifactManifest, *hostError) {
	digest, _ := spec["manifestDigest"].(string)
	if !formpackage.ValidDigest(digest) {
		return artifactManifest{}, stableError("artifact_invalid", "manifestDigest must be a lowercase sha256:<hex> digest")
	}
	raw := h.manifests[digest]
	if raw == nil || !h.holdsManifest(caller.Tenant, digest) {
		return artifactManifest{}, stableError("artifact_missing", "manifestDigest names no committed artifact manifest")
	}
	// The content address is re-derived rather than trusted: a stored document
	// that no longer canonicalizes to the digest it is filed under is not the
	// manifest the client referenced.
	stored, err := formpackage.DigestCanonicalJSON(raw)
	if err != nil || stored != digest {
		return artifactManifest{}, stableError("artifact_invalid", "the stored manifest does not canonicalize to the referenced digest")
	}
	var manifest artifactManifest
	if err := formpackage.DecodeStrictIJSON(raw, &manifest); err != nil {
		return artifactManifest{}, stableError("artifact_invalid", "the stored manifest is not a decodable artifact manifest")
	}
	if manifest.Kind != expectedKind {
		return artifactManifest{}, stableError("artifact_invalid", "manifestDigest names a "+manifest.Kind+" manifest, not a "+expectedKind)
	}
	// The same closure the commit path enforces is re-proved here: a manifest
	// committed by an older or laxer path must not become executable state.
	if hostErr := validateArtifactManifest(manifest); hostErr != nil {
		return artifactManifest{}, hostErr
	}
	return manifest, nil
}

// nextResource computes the post-mutation identity: uid minted on create,
// generation advanced only on canonical spec change, revision advanced on
// every representation change.
//
// The creating tenant is recorded exactly like the exact ref: written once, at
// create, and copied forward by every update. Both answer the same class of
// question — under whose contract, and inside whose boundary, is this resource
// answered about — and neither is re-derived later from the request that
// happens to be in flight.
func (h *ReferenceHost) nextResource(
	form *InstalledForm,
	existing *storedResource,
	create bool,
	tenant, space, name string,
	spec map[string]any,
	specDigest string,
	imported bool,
) *storedResource {
	if create {
		h.uidCounter++
		return &storedResource{
			// The exact ref is recorded at CREATE and never rewritten. An update
			// copies it forward with the rest of the record, so the contract a
			// resource was applied under stays the contract it is answered about.
			Ref: form.Ref, Name: name, Tenant: tenant, Space: space,
			UID:        "uid-" + strconv.Itoa(h.uidCounter),
			Generation: 1, Revision: 1,
			Spec: specOrEmpty(spec), SpecDigest: specDigest,
			Imported:      imported,
			PackageDigest: form.PackageDigest,
		}
	}
	next := *existing
	next.Spec = specOrEmpty(spec)
	next.SpecDigest = specDigest
	if specDigest != existing.SpecDigest {
		next.Generation++
		next.Revision++
	}
	return &next
}

// storeResource installs one resource and keeps the UID reverse index exact:
// whatever the previous incarnation of this key held is unindexed first, so a
// stale relation can never keep a target undeletable.
//
// It is also one of the two places the store changes at all, so it is where the
// derived-rendering rule is applied: every resource the new state renders
// differently advances its revision (derived_rendering.go).
func (h *ReferenceHost) storeResource(resource *storedResource) {
	key := resource.key()
	previous := h.resources[key]
	if previous != nil {
		h.unindexRelations(previous)
	}
	h.resources[key] = resource
	h.indexRelations(resource)
	// The stored resource's own derived rendering is settled against the
	// POST-mutation store. A resource born here has no earlier representation to
	// differ from, so its first rendering simply IS revision 1; an existing one
	// moves its revision only when this mutation has not moved it already,
	// exactly like a relation re-pin — one representation change is one revision.
	if rendered := h.derivedRendering(resource); rendered != resource.DerivedRendering {
		if previous != nil &&
			resource.Generation == previous.Generation && resource.Revision == previous.Revision {
			resource.Revision++
		}
		resource.DerivedRendering = rendered
	}
	h.advanceDerivedRevisions(resource.scope(), key)
}

// removeResource deletes one resource and its reverse-index entries, and
// advances the revision of everything the removal renders differently — the
// out-of-band probe delete included, because a source whose target vanished is
// exactly the case that must not keep serving its old ETag.
func (h *ReferenceHost) removeResource(key string) {
	existing := h.resources[key]
	if existing == nil {
		return
	}
	h.unindexRelations(existing)
	delete(h.resources, key)
	h.advanceDerivedRevisions(existing.scope(), key)
}

// renderResource serves one resource under the exact ref it RECORDS, never
// under the ref of whatever definition of that kind the catalog happens to hold
// most recently. Rewriting an older resource's ref to a newer one would be the
// host asserting, in a response, that two contracts are the same
// (decision 0022).
func (h *ReferenceHost) renderResource(resource *storedResource) map[string]any {
	reference := map[string]any{"formRef": refJSON(resource.Ref)}
	if resource.PackageDigest != "" {
		reference["packageDigest"] = resource.PackageDigest
	}
	status := map[string]any{
		"observedGeneration": strconv.FormatInt(resource.Generation, 10),
		"conditions":         h.derivedConditions(resource),
	}
	if h.contract.lane.StatusCarriesObserved && resource.StatusTouches > 0 {
		status["observed"] = map[string]any{"statusTouches": resource.StatusTouches}
	}
	// `status.outputs` is present exactly when the installed Form Definition
	// declares an outputSchema, and absent otherwise — the rule the published
	// wire schema already states through x-takoform-requiredWhen and
	// x-takoform-omittedWhen. Reading the installed Definition rather than
	// switching on the kind is what makes it that rule rather than a coincidence.
	if form := h.catalog.exact(resource.Ref); form != nil && form.OutputSchema != nil {
		if outputs := h.resourceOutputs(resource); len(outputs) > 0 {
			status["outputs"] = outputs
		}
	}
	return map[string]any{
		"apiVersion": resource.group(),
		"kind":       resource.kind(),
		"form":       reference,
		"metadata": map[string]any{
			"name":       resource.Name,
			"space":      resource.Space,
			"uid":        resource.UID,
			"generation": strconv.FormatInt(resource.Generation, 10),
			"revision":   strconv.FormatInt(resource.Revision, 10),
		},
		"spec":   resource.Spec,
		"status": status,
	}
}

// resourceOutputs computes the host-owned `status.outputs` document of one
// stored resource. A kind this host publishes nothing for returns nil, which
// is how every Form in the family behaves except the one that declares an
// output contract.
func (h *ReferenceHost) resourceOutputs(resource *storedResource) map[string]any {
	if resource.group() != edgeFormsGroup {
		return nil
	}
	if resource.kind() == workerEndpointKind {
		return workerEndpointOutputs(resource)
	}
	return nil
}

func quotedRevision(revision int64) string {
	return `"` + strconv.FormatInt(revision, 10) + `"`
}

func (h *ReferenceHost) exactCurrentResource(
	w http.ResponseWriter,
	request *http.Request,
	group, kind, name string,
) (*storedResource, bool) {
	form, space, hostErr := h.exactQueryForm(request.URL.Query())
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return nil, false
	}
	if form.Ref.APIVersion != group || form.Ref.Kind != kind {
		h.writeError(w, "invalid_argument", "path and exact query identities differ")
		return nil, false
	}
	caller, _ := hostRequestAuth(request)
	resource := h.resourceUnderExactRef(caller.scope(space), form.Ref, name)
	if resource == nil {
		h.writeError(w, "resource_not_found", "resource is absent")
		return nil, false
	}
	return resource, true
}

// resourceUnderExactRef resolves one resource ADDRESSED BY the exact ref the
// request named, inside one scope. A resource stored under a different exact ref
// is absent from this query, not a near miss to be served anyway: state records
// one contract per resource, and answering under another would reinterpret it as
// something it was never applied under (decision 0022, decision 0017 rule 4).
//
// A resource of ANOTHER TENANT is absent for a stronger reason: it is not at this
// key at all, so there is nothing here to decide about. That is what makes the
// refusal `resource_not_found` (404) rather than `permission_denied` (403) — a
// foreign tenant's resource must be indistinguishable from one that was never
// created, or the 403 itself would answer "a resource of that name exists
// somewhere on this host" to anyone who asks (spec/decisions/0018, 0028).
func (h *ReferenceHost) resourceUnderExactRef(scope resourceScope, ref FormRef, name string) *storedResource {
	resource := h.resources[resourceKey(scope, ref.APIVersion, ref.Kind, name)]
	if resource == nil || resource.Ref != ref {
		return nil
	}
	return resource
}

func (h *ReferenceHost) handleRead(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	resource, ok := h.exactCurrentResource(w, request, group, kind, name)
	if !ok {
		return
	}
	h.writeRaw(w, http.StatusOK, quotedRevision(resource.Revision), encodeJSONBody(h.renderResource(resource)))
}

func (h *ReferenceHost) handleObserve(
	w http.ResponseWriter,
	request *http.Request,
	group, kind, name string,
) {
	const action = "observe"
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(raw) != 0 {
		h.writeError(w, "invalid_argument", action+" body must be empty")
		return
	}
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	if h.tryReplay(w, request, raw, request.URL.Query().Get("space")) {
		return
	}
	resource, ok := h.exactCurrentResource(w, request, group, kind, name)
	if !ok {
		return
	}
	fence := request.Header.Get(expectedGenerationHeader)
	if fence == "" {
		h.writeError(w, "invalid_argument", action+" requires the "+expectedGenerationHeader+" fence")
		return
	}
	if fence != strconv.FormatInt(resource.Generation, 10) {
		h.writeError(w, "generation_conflict", action+" generation fence is stale")
		return
	}
	if request.Header.Get(ErrorProbeHeader) == ProbeTouchStatus {
		// A host-side status touch: the representation changes, so the
		// revision advances while the desired generation does not. It touches
		// nothing else — a status counter is not an input to any other
		// resource's rendering — so this path needs no derived-rendering pass
		// (derived_rendering.go).
		resource.StatusTouches++
		resource.Revision++
	}
	response := encodeJSONBody(map[string]any{"resource": h.renderResource(resource)})
	etag := quotedRevision(resource.Revision)
	h.writeRaw(w, http.StatusOK, etag, response)
	h.recordReplay(
		request, raw, request.URL.Query().Get("space"),
		http.StatusOK, etag, response, replayBinding{UID: resource.UID},
	)
}

func (h *ReferenceHost) handleImport(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var body importWire
	if err := formpackage.DecodeStrictIJSON(raw, &body); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	if h.tryReplay(w, request, raw, body.Metadata.Space) {
		return
	}
	if body.Metadata.Name != name || body.Kind != kind || body.APIVersion != group {
		h.writeError(w, "invalid_argument", "resource identity differs from the request target")
		return
	}
	if strings.TrimSpace(body.NativeID) == "" {
		h.writeError(w, "invalid_argument", "nativeId is required")
		return
	}
	form, hostErr := h.resolveResourceWire(&body.resourceWire)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	body.Spec = form.materialize(body.Spec)
	diagnostics, hostErr := h.specDiagnostics(form, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(diagnostics) != 0 {
		h.writeError(w, "invalid_argument", "desired spec violates the installed Form Definition")
		return
	}
	specDigest, err := specCanonicalDigest(body.Spec)
	if err != nil {
		h.writeError(w, "invalid_argument", "spec is not canonicalizable I-JSON")
		return
	}
	// Adoption is a mutation, so it passes the SAME cross-resource gauntlet
	// as apply: an import may not mint a resource whose typed bindings, bundle
	// bytes, deployment weights, or handler gates a fresh apply would reject —
	// and it may not adopt its way to an artifact its tenant does not hold.
	importer, _ := hostRequestAuth(request)
	scope := importer.scope(body.Metadata.Space)
	relations, hostErr := h.validateDesiredSemantics(importer, form, scope, name, body.Spec)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	existing, create, hostErr := h.mutationFences(
		mutationFenceOf(request), form, scope, name, body.Metadata.Generation, "",
	)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	// The adoption CLAIM, decided before any mutation like every other
	// pre-mutation rule in this host (spec/decisions/0011).
	nativeID := strings.TrimSpace(body.NativeID)
	if hostErr := h.validateNativeIdentityClaim(scope, form, name, existing, nativeID); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if hostErr := h.applySQLiteMigrationSuffix(importer, form, scope, relations); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	next := h.nextResource(
		form, existing, create, importer.Tenant, body.Metadata.Space, name, body.Spec, specDigest, true,
	)
	next.NativeID = nativeID
	repinRelations(next, existing, relations)
	h.storeResource(next)
	status := http.StatusOK
	if create {
		status = http.StatusCreated
	}
	response := encodeJSONBody(h.renderResource(next))
	etag := quotedRevision(next.Revision)
	h.writeRaw(w, status, etag, response)
	h.recordReplay(request, raw, body.Metadata.Space, status, etag, response, replayBinding{UID: next.UID})
}

// validateNativeIdentityClaim decides the two halves of an adoption claim
// before anything is written (spec/decisions/0011).
//
// **One live resource per native identity.** A `nativeId` names a backend
// object that exists already; two resources adopting one object would leave the
// host managing one thing twice, with two desired states, two generations, and
// no rule saying which one the object follows. A practitioner reaches it by
// running `terraform import` a second time — after a state file is lost, or from
// a second workspace — and a host that answered by minting a resource has
// silently doubled the infrastructure they are paying for. The refusal is
// `import_conflict` (409): the request is well formed and its adoption claim
// collides with one this host already holds, which is exactly what that code in
// the published closed taxonomy is for.
//
// **The recorded claim is immutable.** An import naming an EXISTING resource of
// the caller under a different `nativeId` asks this host to move a managed
// resource onto another backend object, which orphans the first one just as
// surely from the other direction. A resource holding no claim yet — one this
// host created — takes the one it is handed, because that is its first claim
// rather than a change of one.
//
// The scope is the CALLER'S TENANT, both halves deliberately, exactly as the
// hostname claim of decision 0026 is scoped (edge_semantics.go).
//
// It spans every space of that tenant, because spaces partition one tenant's
// resources and a backend object does not partition with them: adopting one
// object into two spaces is the same duplication as adopting it twice in one.
//
// It spans every FORM KIND of that tenant, which is why the scan filters on the
// tenant and the identity and on nothing else. The tempting index is the one a
// real datastore reaches for — the claim column on the table the resource
// already lives in, keyed `(tenant, kind, nativeId)` — and it is wrong, because
// what is claimed is the object rather than the row: one object adopted once as
// a queue and once as a KV namespace is managed twice, with two desired states,
// exactly as it is when two queues adopt it.
//
// It stops at the tenant, because a host-wide scan would answer "somebody
// already holds this" to a caller who cannot see the holder — a membership
// oracle over every identifier a stranger can guess, which is precisely what
// spec/decisions/0028 refuses to let a refusal disclose. Two tenants naming one
// identifier is not this contract's problem to adjudicate: whose account the
// object lives in is authority this lane does not model. Within each tenant,
// including every tenant that is not the first one to call, it binds in full.
func (h *ReferenceHost) validateNativeIdentityClaim(
	scope resourceScope,
	form *InstalledForm,
	name string,
	existing *storedResource,
	nativeID string,
) *hostError {
	if existing != nil && existing.NativeID != "" && existing.NativeID != nativeID {
		return stableError(
			"import_conflict",
			"resource "+quoteText(name)+" was adopted onto native resource "+quoteText(existing.NativeID)+
				" and is answered for that one; an adoption never moves a managed resource to another native identity",
		)
	}
	selfKey := resourceKey(scope, form.Ref.APIVersion, form.Ref.Kind, name)
	for _, candidate := range h.sortedResources() {
		if candidate.Tenant != scope.Tenant || candidate.NativeID != nativeID {
			continue
		}
		if candidate.key() == selfKey {
			continue
		}
		return stableError(
			"import_conflict",
			"native resource "+quoteText(nativeID)+" is already under management as "+candidate.kind()+" "+
				candidate.Name+" in space "+quoteText(candidate.Space)+
				"; one native resource has one managed resource, so adopting it again would manage it twice",
		)
	}
	return nil
}

func (h *ReferenceHost) handleDelete(w http.ResponseWriter, request *http.Request, group, kind, name string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if len(raw) != 0 {
		h.writeError(w, "invalid_argument", "delete body must be empty")
		return
	}
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	if h.tryReplay(w, request, raw, request.URL.Query().Get("space")) {
		return
	}
	resource, ok := h.exactCurrentResource(w, request, group, kind, name)
	if !ok {
		return
	}
	fences := mutationFenceOf(request)
	if hostErr := deleteFenceStale(fences, resource); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	// An out-of-band backend deletion is the ONE path that bypasses relation
	// protection, and only a disposable conformance endpoint implements it.
	// Everything else — including the async commit re-check below — refuses to
	// remove a resource a live relation pins.
	externalChange := request.Header.Get(ErrorProbeHeader) == ProbeExternalChange
	if !externalChange {
		if hostErr := h.dependencyInUse(resource); hostErr != nil {
			h.writeHostError(w, hostErr)
			return
		}
	}
	// What this delete was accepted for: one incarnation, addressed under one
	// exact contract, at one desired generation.
	accepted := acceptedTarget{Key: resource.key(), Ref: resource.Ref, UID: resource.UID, Fence: fences}
	if request.Header.Get(ErrorProbeHeader) == ProbeAsync {
		h.acceptOperation(w, request, raw, request.URL.Query().Get("space"), &hostOperation{
			DeleteTarget: accepted.Key,
			Accepted:     accepted,
			commit: func() (map[string]any, *hostError) {
				// Identity first, then the fence, then the live-binding scan. The
				// order is the point: a generation fence cannot stand in for an
				// identity, because a replacement created under ANY contract starts
				// at generation 1 and satisfies the fence the original was accepted
				// under. Only after the incarnation is proved to be the one this
				// delete was accepted for does "has the desired state moved since"
				// mean anything.
				current, hostErr := h.acceptedIncarnation(accepted)
				if hostErr != nil {
					return nil, hostErr
				}
				if hostErr := deleteFenceStale(accepted.Fence, current); hostErr != nil {
					return nil, hostErr
				}
				if hostErr := h.dependencyInUse(current); hostErr != nil {
					return nil, hostErr
				}
				h.removeResource(accepted.Key)
				return map[string]any{"deleted": true}, nil
			},
		})
		return
	}
	h.removeResource(accepted.Key)
	h.writeRaw(w, http.StatusNoContent, "", nil)
	// A completed delete reports the incarnation GONE, so nothing retires this
	// record: the same delete retried after a lost response must keep being
	// answered 204 rather than executed against whatever holds the name now.
	h.recordReplay(
		request, raw, request.URL.Query().Get("space"),
		http.StatusNoContent, "", nil, replayBinding{},
	)
}

// acceptOperation registers a deferred mutation and answers 202 with a
// pending Operation envelope. The mutation runs when polling exhausts the
// deterministic wait; cancel before that point abandons it.
//
// The caller states what the operation was accepted AGAINST — the incarnation
// it may land on and what it is removing — because only the caller still holds
// the request that resolved it. This function owns the rest: the id, the owner
// the mutation was accepted from, and the polling bookkeeping.
func (h *ReferenceHost) acceptOperation(
	w http.ResponseWriter,
	request *http.Request,
	raw []byte,
	space string,
	operation *hostOperation,
) {
	h.opCounter++
	owner, _ := hostRequestAuth(request)
	operation.ID = "op_" + strconv.Itoa(h.opCounter)
	operation.Owner = owner
	operation.PollsRemaining = asyncOperationPolls
	h.operations[operation.ID] = operation
	response := encodeJSONBody(map[string]any{"operation": h.renderOperation(operation)})
	h.writeRaw(w, http.StatusAccepted, "", response)
	// The 202 names no incarnation yet, so the record follows the operation: it
	// replays while the mutation is pending, and it retires with whatever
	// incarnation the commit leaves live.
	h.recordReplay(request, raw, space, http.StatusAccepted, "", response, replayBinding{Operation: operation.ID})
}

func (h *ReferenceHost) renderOperation(operation *hostOperation) map[string]any {
	out := map[string]any{
		"apiVersion": operationAPIVersion,
		"kind":       "Operation",
		"id":         operation.ID,
		"done":       operation.Done,
	}
	return out
}

func (h *ReferenceHost) completeOperation(operation *hostOperation, result map[string]any, hostErr *hostError) {
	operation.Done = true
	document := h.renderOperation(operation)
	if hostErr != nil {
		document["error"] = map[string]any{
			"code":      hostErr.Code,
			"message":   hostErr.Message,
			"retryable": h.contract.lane.isAutomaticallyRetryable(hostErr.Code),
		}
	} else {
		document["result"] = result
		operation.CommittedUID = committedResourceUID(result)
	}
	operation.terminalBody = encodeJSONBody(document)
}

// committedResourceUID reads the incarnation an operation left live out of its
// own terminal result. An accepted apply or import commits a resource and names
// it; an accepted delete answers `{"deleted": true}` and names none, which is
// the whole difference between a record that must retire and one that must not.
func committedResourceUID(result map[string]any) string {
	resource, _ := result["resource"].(map[string]any)
	metadata, _ := resource["metadata"].(map[string]any)
	uid, _ := metadata["uid"].(string)
	return uid
}

// ownedOperation resolves one operation for the caller that created it. An
// operation belonging to another tenant or another principal is indistinguishable
// from an operation that never existed: both return nil, and the caller is told
// operation_not_found. Answering permission_denied instead would disclose that
// the id names a real operation, which is the fact a stranger holding a guessed
// or leaked id is trying to learn (spec/decisions/0018).
func (h *ReferenceHost) ownedOperation(request *http.Request, id string) *hostOperation {
	operation := h.operations[id]
	if operation == nil {
		return nil
	}
	caller, _ := hostRequestAuth(request)
	if operation.Owner != caller {
		return nil
	}
	return operation
}

func (h *ReferenceHost) handleOperationGet(w http.ResponseWriter, request *http.Request, id string) {
	operation := h.ownedOperation(request, id)
	if operation == nil {
		h.writeError(w, "operation_not_found", "operation is unknown")
		return
	}
	if !operation.Done {
		operation.PollsRemaining--
		if operation.PollsRemaining > 0 {
			w.Header().Set("Retry-After", "0")
			h.writeRaw(w, http.StatusOK, "", encodeJSONBody(h.renderOperation(operation)))
			return
		}
		result, hostErr := operation.commit()
		h.completeOperation(operation, result, hostErr)
	}
	h.writeRaw(w, http.StatusOK, "", operation.terminalBody)
}

func (h *ReferenceHost) handleOperationCancel(w http.ResponseWriter, request *http.Request, id string) {
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	operation := h.ownedOperation(request, id)
	if operation == nil {
		h.writeError(w, "operation_not_found", "operation is unknown")
		return
	}
	if !operation.Done {
		h.completeOperation(operation, nil, stableError("operation_cancelled", "operation was cancelled before completion"))
	}
	h.writeRaw(w, http.StatusOK, "", operation.terminalBody)
}

func (h *ReferenceHost) routeArtifacts(w http.ResponseWriter, request *http.Request, parts []string) {
	switch {
	case len(parts) == 2 && parts[1] == "uploads" && request.Method == http.MethodPost:
		h.handleArtifactUploadStart(w, request)
	case len(parts) == 3 && parts[1] == "uploads" && request.Method == http.MethodDelete:
		h.handleArtifactUploadAbandon(w, request, parts[2])
	case len(parts) == 4 && parts[1] == "uploads" && parts[3] == "commit" && request.Method == http.MethodPost:
		h.handleArtifactCommit(w, request, parts[2])
	case len(parts) == 5 && parts[1] == "uploads" && parts[3] == "blobs" && request.Method == http.MethodPut:
		h.handleArtifactBlobUpload(w, request, parts[2], parts[4])
	case len(parts) == 3 && parts[1] == "blobs" && request.Method == http.MethodHead:
		// A blob digest is a content address, not a bearer capability: a caller
		// whose tenant does not hold it is told 404 even when the bytes are
		// physically present for someone else (spec/decisions/0018).
		caller, _ := hostRequestAuth(request)
		if h.blobs[parts[2]] == nil || !h.holdsBlob(caller.Tenant, parts[2]) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	case len(parts) == 2 && request.Method == http.MethodGet:
		caller, _ := hostRequestAuth(request)
		manifestRaw := h.manifests[parts[1]]
		if manifestRaw == nil || !h.holdsManifest(caller.Tenant, parts[1]) {
			h.writeError(w, "artifact_missing", "manifest is unknown")
			return
		}
		h.writeRaw(w, http.StatusOK, "", manifestRaw)
	default:
		h.writeError(w, "resource_not_found", "unknown artifact operation")
	}
}

func validateArtifactManifest(manifest artifactManifest) *hostError {
	if manifest.APIVersion != artifactAPIVersion {
		return stableError("artifact_invalid", "manifest apiVersion must be "+artifactAPIVersion)
	}
	// Per-kind closure is enforced HERE, in code, and proved by a required
	// conformance check (spec/decisions/0014): the published manifest schema is
	// the structural minimum and declares mainModule, modules, and files as
	// properties for every kind, so nothing but the host stops a module bundle
	// from also shipping asset files, or an asset bundle from shipping modules.
	// A manifest that carries both shapes has two meanings, and two hosts would
	// be free to pick different ones.
	switch manifest.Kind {
	case "WorkerBundle":
		if manifest.MainModule == "" || len(manifest.Modules) == 0 {
			return stableError("artifact_invalid", "a WorkerBundle manifest requires mainModule and modules")
		}
		if len(manifest.Files) != 0 {
			return stableError("artifact_invalid", "a WorkerBundle manifest must not carry files")
		}
	case "StaticAssetBundle", "MigrationBundle":
		if len(manifest.Files) == 0 {
			return stableError("artifact_invalid", "a file bundle manifest requires files")
		}
		if manifest.MainModule != "" || len(manifest.Modules) != 0 {
			return stableError("artifact_invalid", "a "+manifest.Kind+" manifest must not carry mainModule or modules")
		}
	default:
		return stableError("artifact_invalid", "manifest kind is not a closed artifact kind")
	}
	entryCount := len(manifest.Modules)
	entryLimit := maximumWorkerBundleModules
	if len(manifest.Files) > 0 {
		entryCount = len(manifest.Files)
		entryLimit = maximumBundleFiles
	}
	if entryCount > entryLimit {
		return stableError("artifact_invalid", "manifest entry count overruns the host's published bundle file limit")
	}
	names := map[string]bool{}
	loadable := map[string]bool{}
	moduleBytes := int64(0)
	for _, module := range manifest.Modules {
		if !artifactPathPattern.MatchString(module.Name) || len(module.Name) > 240 {
			return stableError("artifact_invalid", "module path grammar is invalid")
		}
		if names[module.Name] {
			return stableError("artifact_invalid", "duplicate module path")
		}
		names[module.Name] = true
		if !artifactModuleMediaTypes[module.MediaType] {
			return stableError("artifact_invalid", "module mediaType is not closed")
		}
		loadable[module.Name] = currentformmodel.ModuleMediaTypeLoadable(module.MediaType)
		if err := validateArtifactSize(module.Size); err != nil {
			return err
		}
		if !formpackage.ValidDigest(module.Digest) {
			return stableError("artifact_invalid", "module digest grammar is invalid")
		}
		size, _ := strconv.ParseInt(module.Size.String(), 10, 64)
		moduleBytes += size
	}
	if manifest.Kind == "WorkerBundle" {
		if !names[manifest.MainModule] {
			return stableError("artifact_invalid", "mainModule must name one declared module")
		}
		// An AUXILIARY module — today, source-map evidence about another
		// module — may sit in the bundle and is never imported, so it can
		// never be the module the runtime instantiates first. The published
		// manifest schema admits it in `modules` alongside loadable code and
		// cannot tell the two apart, so this is a host rule proved by a
		// required conformance check (spec/decisions/0012, 0014, and 0019).
		if !loadable[manifest.MainModule] {
			return stableError(
				"artifact_invalid",
				"mainModule "+quoteText(manifest.MainModule)+
					" names a module the runtime never imports; mainModule must be a loadable module",
			)
		}
	}
	// A source map is evidence about another module. "<module>.map" is the
	// portable naming rule, so a source map whose target module is not
	// declared describes nothing and is rejected.
	for _, module := range manifest.Modules {
		if module.MediaType != sourceMapMediaType {
			continue
		}
		target := strings.TrimSuffix(module.Name, sourceMapSuffix)
		if target == module.Name || !names[target] {
			return stableError("artifact_invalid", "source map target module is absent from the manifest")
		}
	}
	// The host's published limit is the enforced ceiling: a manifest whose
	// declared module sizes overrun maximumBundleBytes is rejected before any
	// blob is accepted, not discovered after the bytes are already stored.
	if moduleBytes > maximumBundleBytes {
		return stableError("artifact_invalid", "manifest module sizes overrun the host's published bundle limit")
	}
	paths := map[string]bool{}
	fileBytes := int64(0)
	for _, file := range manifest.Files {
		if !artifactPathPattern.MatchString(file.Path) || len(file.Path) > 240 {
			return stableError("artifact_invalid", "file path grammar is invalid")
		}
		if paths[file.Path] {
			return stableError("artifact_invalid", "duplicate file path")
		}
		paths[file.Path] = true
		if err := validateArtifactSize(file.Size); err != nil {
			return err
		}
		if size, _ := strconv.ParseInt(file.Size.String(), 10, 64); size > maximumBundleBytes-fileBytes {
			return stableError("artifact_invalid", "manifest file sizes overrun the host's published bundle limit")
		} else {
			fileBytes += size
		}
		if !formpackage.ValidDigest(file.Digest) {
			return stableError("artifact_invalid", "file digest grammar is invalid")
		}
		if manifest.Kind == staticAssetBundleKind && !currentformmodel.ValidNormalizedMediaType(file.MediaType) {
			return stableError("artifact_invalid", "a StaticAssetBundle file must use a normalized v1alpha1 media type without parameters")
		}
		if manifest.Kind == migrationBundleKind && file.MediaType != "application/sql" {
			return stableError("artifact_invalid", "a MigrationBundle file must use application/sql")
		}
	}
	return nil
}

func validateArtifactSize(size json.Number) *hostError {
	value, err := strconv.ParseInt(size.String(), 10, 64)
	if err != nil || value < 0 || value > 268435456 {
		return stableError("artifact_invalid", "artifact size is out of range")
	}
	return nil
}

func (manifest artifactManifest) blobDigests() []string {
	seen := map[string]bool{}
	var out []string
	for _, module := range manifest.Modules {
		if !seen[module.Digest] {
			seen[module.Digest] = true
			out = append(out, module.Digest)
		}
	}
	for _, file := range manifest.Files {
		if !seen[file.Digest] {
			seen[file.Digest] = true
			out = append(out, file.Digest)
		}
	}
	sort.Strings(out)
	return out
}

func (h *ReferenceHost) handleArtifactUploadStart(w http.ResponseWriter, request *http.Request) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	var envelope struct {
		Manifest json.RawMessage `json:"manifest"`
	}
	if err := formpackage.DecodeStrictIJSON(raw, &envelope); err != nil {
		h.writeError(w, "invalid_argument", err.Error())
		return
	}
	if len(envelope.Manifest) == 0 {
		h.writeError(w, "invalid_argument", "manifest is required")
		return
	}
	if h.tryReplay(w, request, raw, "") {
		return
	}
	var manifest artifactManifest
	if err := formpackage.DecodeStrictIJSON(envelope.Manifest, &manifest); err != nil {
		h.writeError(w, "artifact_invalid", err.Error())
		return
	}
	if hostErr := validateArtifactManifest(manifest); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	manifestDigest, err := formpackage.DigestCanonicalJSON(envelope.Manifest)
	if err != nil {
		h.writeError(w, "artifact_invalid", "manifest is not canonicalizable I-JSON")
		return
	}
	h.uploadCounter++
	owner, _ := hostRequestAuth(request)
	upload := &artifactUpload{
		ID:             "up_" + strconv.Itoa(h.uploadCounter),
		Owner:          owner,
		ManifestRaw:    append([]byte(nil), envelope.Manifest...),
		Manifest:       manifest,
		ManifestDigest: manifestDigest,
	}
	h.uploads[upload.ID] = upload
	// missingBlobs is answered per TENANT, not per byte store. Reporting a blob
	// as already present because some other tenant uploaded it would hand this
	// tenant bytes it never had, using the digest as the entitlement — exactly
	// what a content address must not be. Dedup stays physical: the second
	// tenant's identical bytes overwrite one stored copy.
	missing := []string{}
	for _, digest := range manifest.blobDigests() {
		if !h.holdsBlob(owner.Tenant, digest) {
			missing = append(missing, digest)
		}
	}
	response := encodeJSONBody(map[string]any{"uploadId": upload.ID, "missingBlobs": missing})
	h.writeRaw(w, http.StatusCreated, "", response)
	// An artifact address is content, not an incarnation: nothing about a
	// resource's lifetime can make this answer wrong, so the record binds nothing.
	h.recordReplay(request, raw, "", http.StatusCreated, "", response, replayBinding{})
}

// ownedUpload resolves one upload session for the caller that started it. As
// with operations, a session belonging to another tenant or principal is
// answered as absent rather than forbidden (spec/decisions/0018).
func (h *ReferenceHost) ownedUpload(request *http.Request, uploadID string) *artifactUpload {
	upload := h.uploads[uploadID]
	if upload == nil {
		return nil
	}
	caller, _ := hostRequestAuth(request)
	if upload.Owner != caller {
		return nil
	}
	return upload
}

func (h *ReferenceHost) handleArtifactBlobUpload(w http.ResponseWriter, request *http.Request, uploadID, digest string) {
	upload := h.ownedUpload(request, uploadID)
	if upload == nil {
		h.writeError(w, "artifact_missing", "upload is unknown")
		return
	}
	declared := false
	var declaredSize int64
	for _, candidate := range upload.Manifest.blobDigests() {
		if candidate == digest {
			declared = true
		}
	}
	for _, module := range upload.Manifest.Modules {
		if module.Digest == digest {
			declaredSize, _ = strconv.ParseInt(module.Size.String(), 10, 64)
		}
	}
	for _, file := range upload.Manifest.Files {
		if file.Digest == digest {
			declaredSize, _ = strconv.ParseInt(file.Size.String(), 10, 64)
		}
	}
	if !declared {
		h.writeError(w, "artifact_invalid", "blob digest is not declared by the manifest")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(request.Body, 268435456+1))
	if err != nil {
		h.writeError(w, "invalid_argument", "invalid blob body")
		return
	}
	if formpackage.DigestBytes(raw) != digest || int64(len(raw)) != declaredSize {
		h.writeError(w, "artifact_invalid", "blob bytes do not match their declared digest and size")
		return
	}
	h.blobs[digest] = raw
	// The uploading tenant possessed these exact bytes, so it now holds this
	// content address; nobody else's holding changed.
	h.grantBlob(upload.Owner.Tenant, digest)
	h.writeRaw(w, http.StatusCreated, "", nil)
}

func (h *ReferenceHost) handleArtifactCommit(w http.ResponseWriter, request *http.Request, uploadID string) {
	raw, hostErr := h.readBody(request)
	if hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	if h.tryReplay(w, request, raw, "") {
		return
	}
	upload := h.ownedUpload(request, uploadID)
	if upload == nil {
		h.writeError(w, "artifact_missing", "upload is unknown")
		return
	}
	// The manifest is re-validated at commit, not only at upload start: commit
	// is the step that mints an immutable identity, so every closure rule has
	// to hold at exactly the moment the document becomes permanent.
	if hostErr := validateArtifactManifest(upload.Manifest); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	for _, digest := range upload.Manifest.blobDigests() {
		if h.blobs[digest] == nil || !h.holdsBlob(upload.Owner.Tenant, digest) {
			h.writeError(w, "artifact_missing", "manifest blob "+digest+" was not uploaded")
			return
		}
	}
	// Commit re-verifies size against the STORED bytes, not only against the
	// bytes of this upload. Blobs are content-addressed and shared, so a
	// second manifest can name an already-held digest under a size it never
	// had; that manifest must never become an immutable identity.
	if hostErr := h.verifyCommittedSizes(upload.Owner.Tenant, upload.Manifest); hostErr != nil {
		h.writeHostError(w, hostErr)
		return
	}
	status := http.StatusCreated
	if h.manifests[upload.ManifestDigest] != nil {
		status = http.StatusOK
	}
	h.manifests[upload.ManifestDigest] = append([]byte(nil), upload.ManifestRaw...)
	// Committing binds the manifest address to the committing tenant. Its blobs
	// are already held — commit refused otherwise — so nothing about the byte
	// store changes when a second tenant commits the same document.
	h.grantManifest(upload.Owner.Tenant, upload.ManifestDigest)
	response := encodeJSONBody(map[string]any{"manifestDigest": upload.ManifestDigest})
	h.writeRaw(w, status, "", response)
	h.recordReplay(request, raw, "", status, "", response, replayBinding{})
}

// verifyCommittedSizes binds every declared size to the stored byte length of
// the blob that digest resolves to. The lookup asks the committing tenant's
// holding, not the byte store: a blob is resolved on behalf of a caller here
// exactly as it is everywhere else, so this stays correct without depending on
// the order of the checks around it.
func (h *ReferenceHost) verifyCommittedSizes(tenant string, manifest artifactManifest) *hostError {
	declared := func(name, digest string, size json.Number) *hostError {
		want, err := strconv.ParseInt(size.String(), 10, 64)
		if err != nil {
			return stableError("artifact_invalid", "declared size of "+name+" is not an integer")
		}
		stored, ok := h.blobs[digest]
		if !ok || !h.holdsBlob(tenant, digest) {
			return stableError("artifact_missing", "manifest blob "+digest+" was not uploaded")
		}
		if int64(len(stored)) != want {
			return stableError("artifact_invalid", "stored bytes of "+name+" do not match the declared size")
		}
		return nil
	}
	for _, module := range manifest.Modules {
		if hostErr := declared(module.Name, module.Digest, module.Size); hostErr != nil {
			return hostErr
		}
	}
	for _, file := range manifest.Files {
		if hostErr := declared(file.Path, file.Digest, file.Size); hostErr != nil {
			return hostErr
		}
	}
	return nil
}

// handleArtifactUploadAbandon ends one upload session the caller owns. An id
// this caller does not hold — never issued, already abandoned, or another
// tenant's or principal's — is one answer, artifact_missing: replying 204 for
// an unknown id while replying 404 for a foreign one would make the difference
// between the two observable, which is the existence disclosure the ownership
// rule exists to prevent (spec/decisions/0018).
func (h *ReferenceHost) handleArtifactUploadAbandon(w http.ResponseWriter, request *http.Request, uploadID string) {
	if request.Header.Get("Idempotency-Key") == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return
	}
	upload := h.ownedUpload(request, uploadID)
	if upload == nil {
		h.writeError(w, "artifact_missing", "upload is unknown")
		return
	}
	delete(h.uploads, uploadID)
	h.collectStagedBlobs(upload)
	h.writeRaw(w, http.StatusNoContent, "", nil)
}

// collectStagedBlobs frees what an abandoned upload session staged. A blob any
// COMMITTED manifest the same tenant holds still names is retained: committed
// manifests are never collected, so a manifest a Resource references stays
// readable and its bytes stay resolvable no matter what unrelated upload
// sessions are abandoned around it. Blobs are content-addressed and therefore
// shared, which is exactly why this has to be a reachability question rather
// than a per-session one.
//
// Reachability is asked per tenant and answered in two steps: the abandoning
// tenant's HOLD on an unreachable address is dropped, and the shared BYTES are
// freed only once no tenant holds the address at all. One tenant abandoning a
// session can therefore never take bytes away from another.
func (h *ReferenceHost) collectStagedBlobs(upload *artifactUpload) {
	tenant := upload.Owner.Tenant
	retained := map[string]bool{}
	for digest, raw := range h.manifests {
		if !h.holdsManifest(tenant, digest) {
			continue
		}
		var committed artifactManifest
		if err := formpackage.DecodeStrictIJSON(raw, &committed); err != nil {
			continue
		}
		for _, blobDigest := range committed.blobDigests() {
			retained[blobDigest] = true
		}
	}
	for _, digest := range upload.Manifest.blobDigests() {
		if retained[digest] {
			continue
		}
		delete(h.blobTenants[digest], tenant)
		if len(h.blobTenants[digest]) == 0 {
			delete(h.blobTenants, digest)
			delete(h.blobs, digest)
		}
	}
}

// handleStandardServiceSupport answers whether this host can satisfy one
// external standard-service protocol (decision 0045).
//
// It exists so a client learns at PREPARE time that a slot cannot be filled,
// rather than at apply time from a resource that will never become Ready. The
// answer is a profile rather than a bare boolean because "can you speak
// postgresql" and "for whom, and under what identity" are the same question,
// and a bare boolean could not carry the second half.
func (h *ReferenceHost) handleStandardServiceSupport(w http.ResponseWriter, protocol string) {
	if !containsString(h.contract.RunnerInput.ExternalServices.Protocols, protocol) {
		h.writeError(w, "resource_not_found", "standard-service protocol is unknown")
		return
	}
	satisfiable := true
	h.writeJSON(w, http.StatusOK, "", map[string]any{
		"apiVersion": h.contract.APIVersion,
		"kind":       "StandardServiceSupport",
		"serviceRef": map[string]any{
			"apiVersion": h.contract.RunnerInput.ExternalServices.ServiceAPIVersion,
			"protocol":   protocol,
		},
		"satisfiable": satisfiable,
	})
}

func (h *ReferenceHost) routeSupport(w http.ResponseWriter, request *http.Request, parts []string) {
	if request.Method != http.MethodGet {
		h.writeError(w, "resource_not_found", "unknown support operation")
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "forms":
		// Every installed identity gets its own profile. Two definition versions
		// of one line are two entries, because a profile states what a host
		// supports for one exact Form.
		installed := h.catalog.sortedForms()
		profiles := make([]map[string]any, 0, len(installed))
		for _, form := range installed {
			profiles = append(profiles, h.formSupportProfile(form))
		}
		h.writeJSON(w, http.StatusOK, "", map[string]any{"profiles": profiles})
	case len(parts) == 6 && parts[1] == "forms":
		// The published path carries the definition version and no digest, so the
		// exact identity is resolved through the line index — which refuses to
		// hold two definitions that agree on the version and differ on the bytes.
		// The profile then answers about that one identity and echoes it.
		form := h.catalog.line(groupOf(parts[2], parts[3]), parts[4], parts[5])
		if form == nil {
			h.writeError(w, "form_unknown", "exact Form line is unknown")
			return
		}
		h.writeJSON(w, http.StatusOK, "", h.formSupportProfile(form))
	case len(parts) == 3 && parts[1] == "standard-services":
		h.handleStandardServiceSupport(w, parts[2])
	case len(parts) == 4 && parts[1] == "interfaces":
		reference, ok := h.catalog.interfaces[parts[2]+"@"+parts[3]]
		if !ok {
			h.writeError(w, "resource_not_found", "interface support profile is unknown")
			return
		}
		h.writeJSON(w, http.StatusOK, "", map[string]any{
			"apiVersion": supportAPIVersion,
			"kind":       "InterfaceSupport",
			"interfaceRef": map[string]any{
				"apiVersion":   "interfaces.takoform.com/v1alpha1",
				"name":         reference.Name,
				"version":      reference.Version,
				"schemaDigest": reference.SchemaDigest,
			},
		})
	case len(parts) == 4 && parts[1] == "bindings":
		reference, ok := h.catalog.bindings[parts[2]+"@"+parts[3]]
		if !ok {
			h.writeError(w, "resource_not_found", "binding support profile is unknown")
			return
		}
		h.writeJSON(w, http.StatusOK, "", map[string]any{
			"apiVersion": supportAPIVersion,
			"kind":       "BindingSupport",
			"bindingRef": map[string]any{
				"apiVersion":   "bindings.takoform.com/v1alpha1",
				"name":         reference.Name,
				"version":      reference.Version,
				"schemaDigest": reference.SchemaDigest,
			},
		})
	default:
		h.writeError(w, "resource_not_found", "unknown support operation")
	}
}

// formSupportProfile declares capability subsets, inclusive ranges, and
// numeric limits only. Price, SKU, region, quota, and commercial policy
// never appear in this surface.
func (h *ReferenceHost) formSupportProfile(form *InstalledForm) map[string]any {
	profile := map[string]any{
		"apiVersion": supportAPIVersion,
		"kind":       "FormSupport",
		"formRef":    refJSON(form.Ref),
		"operations": form.operations(),
	}
	if form.Ref.Kind == workerVersionKind {
		// The handler enum is the runtime ABI's vocabulary, read from the
		// installed contract. There is no compatibilityDate range and no
		// compatibilityFlags enum: a date is only meaningful against a registry
		// stating which behavior each date changes, this lane has none, and a
		// profile advertising one would promise portability it could not deliver
		// (spec/decisions/0019). Runtime behavior is advertised by implementing
		// the exact contract, which the interface support profile states.
		handlers := desiredSchemaEnum(form.DesiredSchema, "handlers")
		if contract, installed := h.runtimeContract(); installed {
			handlers = contract.Handlers
		}
		profile["supportedEnums"] = map[string]any{"handlers": handlers}
		profile["limits"] = map[string]any{"maximumBundleBytes": maximumBundleBytes}
	} else if artifactKind, artifactBacked := artifactManifestKindForForm(form.Ref.Kind); artifactBacked {
		entryLimit := maximumBundleFiles
		if artifactKind == workerBundleKind {
			entryLimit = maximumWorkerBundleModules
		}
		profile["limits"] = map[string]any{
			"maximumBundleBytes": maximumBundleBytes,
			"maximumBundleFiles": entryLimit,
		}
	}
	return profile
}

// Idempotency replay: keys are namespaced by authenticated tenant, principal,
// space, and key; the fingerprint binds method, request target,
// preconditions, and exact body bytes. A recorded success replays its exact
// status, ETag, and body; a fingerprint mismatch fails invalid_argument.
//
// A record whose incarnation is gone is RETIRED before any of that, and the
// request is executed as the new one it is (spec/decisions/0011, "A replay
// record does not outlive the incarnation it reports"). Without it a create
// never converges across a deletion: a create's prepare binds the create
// markers, so a byte-identical re-create derives a byte-identical key and
// fingerprint, and a destroy followed by an apply of an unchanged configuration
// would be answered the old 201 forever while nothing was created.
func (h *ReferenceHost) tryReplay(w http.ResponseWriter, request *http.Request, raw []byte, space string) bool {
	key := request.Header.Get("Idempotency-Key")
	if key == "" {
		h.writeError(w, "invalid_argument", "Idempotency-Key is required")
		return true
	}
	recordKey := h.replayKey(request, space, key)
	recorded, ok := h.replays[recordKey]
	if !ok {
		return false
	}
	if h.replayRetired(recorded.Binding) {
		delete(h.replays, recordKey)
		return false
	}
	if recorded.Fingerprint != requestFingerprint(request, raw) {
		h.writeError(w, "invalid_argument", "Idempotency-Key was reused for another request")
		return true
	}
	h.writeRaw(w, recorded.Status, recorded.ETag, recorded.Body)
	return true
}

// replayRetired reports whether a recorded answer has outlived the incarnation
// it reports. A record bound to nothing is never retired here — that is a
// delete's 204, a refusal, and an artifact commit, none of which claims a
// resource exists.
func (h *ReferenceHost) replayRetired(binding replayBinding) bool {
	uid := binding.UID
	if uid == "" && binding.Operation != "" {
		if operation := h.operations[binding.Operation]; operation != nil {
			uid = operation.CommittedUID
		}
	}
	if uid == "" {
		return false
	}
	return h.resourceByUID(uid) == nil
}

// resourceByUID resolves one host-issued incarnation wherever it lives. A uid
// names one resource across the whole store by construction, so this asks no
// tenant question and answers none: it is only ever used to ask whether an
// incarnation this host itself minted still exists.
func (h *ReferenceHost) resourceByUID(uid string) *storedResource {
	for _, resource := range h.resources {
		if resource.UID == uid {
			return resource
		}
	}
	return nil
}

func (h *ReferenceHost) recordReplay(
	request *http.Request,
	raw []byte,
	space string,
	status int,
	etag string,
	body []byte,
	binding replayBinding,
) {
	key := request.Header.Get("Idempotency-Key")
	if key == "" {
		return
	}
	h.replays[h.replayKey(request, space, key)] = recordedReplay{
		Fingerprint: requestFingerprint(request, raw),
		Status:      status,
		ETag:        etag,
		Body:        append([]byte(nil), body...),
		Binding:     binding,
	}
}

func (h *ReferenceHost) replayKey(request *http.Request, space, key string) string {
	auth, _ := hostRequestAuth(request)
	return strings.Join([]string{
		"tenant=" + auth.Tenant,
		"principal=" + auth.Principal,
		"space=" + space,
		"key=" + key,
	}, "\x00")
}

func requestFingerprint(request *http.Request, raw []byte) string {
	return formpackage.DigestBytes([]byte(strings.Join([]string{
		request.Method,
		request.URL.RequestURI(),
		request.Header.Get("If-Match"),
		request.Header.Get("If-None-Match"),
		request.Header.Get(expectedGenerationHeader),
		string(raw),
	}, "\x00")))
}

func encodeJSONBody(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(`{"error":{"code":"internal_error","message":` +
			strconv.Quote(err.Error()) + `,"requestId":"ref3-encode","retryable":false}}`)
	}
	return append(raw, '\n')
}

func (h *ReferenceHost) writeJSON(w http.ResponseWriter, status int, etag string, value any) {
	h.writeRaw(w, status, etag, encodeJSONBody(value))
}

func (h *ReferenceHost) writeRaw(w http.ResponseWriter, status int, etag string, raw []byte) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	w.WriteHeader(status)
	if len(raw) > 0 {
		_, _ = w.Write(raw)
	}
}

func (h *ReferenceHost) writeHostError(w http.ResponseWriter, hostErr *hostError) {
	h.writeError(w, hostErr.Code, hostErr.Message)
}

func (h *ReferenceHost) writeError(w http.ResponseWriter, code, message string) {
	status, known := h.contract.lane.ErrorHTTPStatus[code]
	if !known {
		status = http.StatusInternalServerError
		code = "internal_error"
	}
	if message == "" {
		message = "stable error"
	}
	h.requestCounter++
	h.writeJSON(w, status, "", map[string]any{
		"error": map[string]any{
			"code":      code,
			"message":   message,
			"requestId": "ref3-" + code + "-" + strconv.Itoa(h.requestCounter),
			"retryable": h.contract.lane.isAutomaticallyRetryable(code),
		},
	})
}
