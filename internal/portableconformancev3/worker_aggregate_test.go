package portableconformancev3

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// aggregateFixture drives the Worker aggregate rules against the REAL installed
// candidate catalog: the rules are about WorkerDeployment and the three
// attachment Forms, which only the Edge Platform Family declares.
type aggregateFixture struct {
	t     *testing.T
	host  *ReferenceHost
	group string
	space string
}

func newAggregateFixture(t *testing.T) *aggregateFixture {
	t.Helper()
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	catalog, err := LoadCatalog(filepath.Join("..", ".."), contract)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return &aggregateFixture{
		t:     t,
		host:  NewReferenceHost(contract, catalog),
		group: edgeFormsGroup,
		space: contract.RunnerInput.Space,
	}
}

func (f *aggregateFixture) ref(kind, name string) map[string]any {
	return map[string]any{"apiVersion": f.group, "kind": kind, "name": name}
}

// store installs one resource exactly the way an accepted apply would: the spec
// is materialized, every relation is resolved and pinned, the creating tenant is
// recorded, and the reverse index is kept exact.
func (f *aggregateFixture) store(kind, name string, spec map[string]any) *storedResource {
	f.t.Helper()
	return f.storeAs(referencePrimaryAuth, kind, name, spec)
}

// storeAs is store under another authenticated caller, so a rule scoped to the
// tenant can be driven from both sides of that boundary.
func (f *aggregateFixture) storeAs(
	caller hostAuthContext,
	kind, name string,
	spec map[string]any,
) *storedResource {
	f.t.Helper()
	form := f.host.probeForm(kind)
	if form == nil {
		f.t.Fatalf("%s is not installed", kind)
	}
	materialized := form.materialize(spec)
	relations, hostErr := f.host.resolveRelations(form, f.space, materialized)
	if hostErr != nil {
		f.t.Fatalf("store %s %s: %+v", kind, name, hostErr)
	}
	digest, err := specCanonicalDigest(materialized)
	if err != nil {
		f.t.Fatal(err)
	}
	f.host.uidCounter++
	resource := &storedResource{
		Ref: form.Ref, Name: name, Tenant: caller.Tenant, Space: f.space,
		UID: "uid-" + strconv.Itoa(f.host.uidCounter), Generation: 1, Revision: 1,
		Spec: materialized, SpecDigest: digest, Relations: relations,
	}
	f.host.storeResource(resource)
	return resource
}

// inSpace returns the same fixture pointed at another space of the same host,
// so a rule that spans spaces can be driven with real resources on both sides
// rather than with a store written by hand.
func (f *aggregateFixture) inSpace(space string) *aggregateFixture {
	elsewhere := *f
	elsewhere.space = space
	return &elsewhere
}

// validate runs the complete pre-mutation gauntlet for one desired spec.
func (f *aggregateFixture) validate(kind, name string, spec map[string]any) *hostError {
	f.t.Helper()
	return f.validateAs(referencePrimaryAuth, kind, name, spec)
}

// validateAs runs the same gauntlet as another authenticated caller. The caller
// decides two of the rules it contains: which tenant's artifacts a bundle may
// reference, and which tenant's hostname claims a custom domain collides with.
func (f *aggregateFixture) validateAs(
	caller hostAuthContext,
	kind, name string,
	spec map[string]any,
) *hostError {
	f.t.Helper()
	form := f.host.probeForm(kind)
	if form == nil {
		f.t.Fatalf("%s is not installed", kind)
	}
	_, hostErr := f.host.validateDesiredSemantics(caller, form, f.space, name, form.materialize(spec))
	return hostErr
}

func (f *aggregateFixture) requireCode(hostErr *hostError, code, subject string) {
	f.t.Helper()
	if hostErr == nil {
		f.t.Fatalf("%s was accepted; want %s", subject, code)
	}
	if hostErr.Code != code {
		f.t.Fatalf("%s = %s (%s); want %s", subject, hostErr.Code, hostErr.Message, code)
	}
}

func (f *aggregateFixture) requireAccepted(hostErr *hostError, subject string) {
	f.t.Helper()
	if hostErr != nil {
		f.t.Fatalf("%s was refused: %s (%s)", subject, hostErr.Code, hostErr.Message)
	}
}

// versionSpec builds a Worker Version desired spec of one worker.
func (f *aggregateFixture) versionSpec(worker string, handlers ...string) map[string]any {
	declared := make([]any, 0, len(handlers))
	for _, handler := range handlers {
		declared = append(declared, handler)
	}
	return map[string]any{
		"worker":   f.ref(moduleWorkerKind, worker),
		"bundle":   f.ref("WorkerBundle", "worker-bundle-probe"),
		"handlers": declared,
	}
}

// deploymentSpec builds a Worker Deployment desired spec. Weights are declared
// as decoded JSON numbers, exactly as the strict I-JSON wire decoder produces.
func (f *aggregateFixture) deploymentSpec(worker string, weights map[string]int) map[string]any {
	entries := make([]any, 0, len(weights))
	names := make([]string, 0, len(weights))
	for name := range weights {
		names = append(names, name)
	}
	sortStrings(names)
	for _, name := range names {
		entries = append(entries, map[string]any{
			"workerVersion": f.ref(workerVersionKind, name),
			"weight":        json.Number(strconv.Itoa(weights[name])),
		})
	}
	return map[string]any{"worker": f.ref(moduleWorkerKind, worker), "versions": entries}
}

// deploymentSpecList builds a deployment whose entries are declared in order,
// so one version can be weighted twice under different shares.
func (f *aggregateFixture) deploymentSpecList(worker string, entries ...[2]any) map[string]any {
	versions := make([]any, 0, len(entries))
	for _, entry := range entries {
		name, _ := entry[0].(string)
		weight, _ := entry[1].(int)
		versions = append(versions, map[string]any{
			"workerVersion": f.ref(workerVersionKind, name),
			"weight":        json.Number(strconv.Itoa(weight)),
		})
	}
	return map[string]any{"worker": f.ref(moduleWorkerKind, worker), "versions": versions}
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

// baseAggregate installs the shared starting point: a bundle, a worker, one
// version exporting every handler, and the deployment that serves it.
func (f *aggregateFixture) baseAggregate() (worker, version, deployment *storedResource) {
	f.t.Helper()
	f.store("WorkerBundle", "worker-bundle-probe", map[string]any{
		"manifestDigest": "sha256:" + strings.Repeat("a", 64),
	})
	worker = f.store(moduleWorkerKind, "worker", map[string]any{})
	version = f.store(workerVersionKind, "version",
		f.versionSpec("worker", fetchHandler, scheduledHandler, queueHandler))
	deployment = f.store(workerDeploymentKind, "deployment",
		f.deploymentSpec("worker", map[string]int{"version": 10000}))
	return worker, version, deployment
}

// TestOneActiveDeploymentPerWorker proves rule A: a second WorkerDeployment of
// one worker is refused before mutation, and re-applying the SAME deployment is
// not mistaken for one.
func TestOneActiveDeploymentPerWorker(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()

	f.requireCode(
		f.validate(workerDeploymentKind, "second", f.deploymentSpec("worker", map[string]int{"version": 10000})),
		"invalid_argument", "a second deployment of one worker",
	)
	f.requireAccepted(
		f.validate(workerDeploymentKind, "deployment", f.deploymentSpec("worker", map[string]int{"version": 10000})),
		"re-applying the worker's own deployment",
	)

	// A different worker is a different aggregate.
	f.store(moduleWorkerKind, "other-worker", map[string]any{})
	f.store(workerVersionKind, "other-version", f.versionSpec("other-worker", fetchHandler))
	f.requireAccepted(
		f.validate(workerDeploymentKind, "other-deployment",
			f.deploymentSpec("other-worker", map[string]int{"other-version": 10000})),
		"the first deployment of a second worker",
	)
}

// TestDeploymentIntegrity proves rule B: ownership, per-version uniqueness, the
// exact weight sum, and versions a host can actually run.
func TestDeploymentIntegrity(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	f.store(moduleWorkerKind, "other-worker", map[string]any{})
	f.store(workerVersionKind, "other-version", f.versionSpec("other-worker", fetchHandler))

	f.requireCode(
		f.validate(workerDeploymentKind, "other-deployment",
			f.deploymentSpec("other-worker", map[string]int{"version": 10000})),
		"invalid_argument", "weighting a version of a different worker",
	)
	f.requireCode(
		f.validate(workerDeploymentKind, "other-deployment",
			f.deploymentSpecList("other-worker", [2]any{"other-version", 6000}, [2]any{"other-version", 4000})),
		"invalid_argument", "weighting one version twice",
	)
	f.requireCode(
		f.validate(workerDeploymentKind, "other-deployment",
			f.deploymentSpec("other-worker", map[string]int{"other-version": 9999})),
		"invalid_argument", "weights summing to 9999",
	)
	f.requireAccepted(
		f.validate(workerDeploymentKind, "other-deployment",
			f.deploymentSpec("other-worker", map[string]int{"other-version": 10000})),
		"an owned, unique, exactly weighted version",
	)
}

// TestDeploymentRefusesUnavailableVersion proves the two availability halves of
// rule B: a version whose own relations no longer resolve is not Ready, and a
// version an accepted delete is already removing is on its way out. Weighting
// either would make the deployment's claim about what runs false the moment it
// was stored.
func TestDeploymentRefusesUnavailableVersion(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	f.store("EdgeKVNamespace", "cache", map[string]any{})
	f.store(moduleWorkerKind, "other-worker", map[string]any{})
	spec := f.versionSpec("other-worker", fetchHandler)
	spec["kvBindings"] = []any{map[string]any{
		"name":     "CACHE",
		"resource": f.ref("EdgeKVNamespace", "cache"),
	}}
	f.store(workerVersionKind, "drifting-version", spec)

	// The bound namespace vanishes out of band, so the version is not Ready.
	f.host.removeResource(resourceKey(f.space, f.group, "EdgeKVNamespace", "cache"))
	hostErr := f.validate(workerDeploymentKind, "other-deployment",
		f.deploymentSpec("other-worker", map[string]int{"drifting-version": 10000}))
	f.requireCode(hostErr, "invalid_argument", "weighting a version that is not Ready")
	if !strings.Contains(hostErr.Message, "not Ready") {
		t.Fatalf("the refusal does not say the version is not Ready: %s", hostErr.Message)
	}

	// A second version, healthy, but with an accepted delete already running.
	healthy := f.store(workerVersionKind, "leaving-version", f.versionSpec("other-worker", fetchHandler))
	f.host.operations["op_delete"] = &hostOperation{
		ID: "op_delete",
		DeleteTarget: resourceKey(
			healthy.Space, healthy.group(), healthy.kind(), healthy.Name,
		),
	}
	hostErr = f.validate(workerDeploymentKind, "other-deployment",
		f.deploymentSpec("other-worker", map[string]int{"leaving-version": 10000}))
	f.requireCode(hostErr, "invalid_argument", "weighting a version an accepted delete is removing")

	// The completed operation releases it.
	f.host.operations["op_delete"].Done = true
	f.requireAccepted(
		f.validate(workerDeploymentKind, "other-deployment",
			f.deploymentSpec("other-worker", map[string]int{"leaving-version": 10000})),
		"weighting the same version once the delete operation is terminal",
	)
}

// TestAttachmentsGateOnTheActiveDeployment proves rule C: each attachment is
// admitted only when the worker's active deployment exists AND every version it
// weights exports the handler that attachment invokes.
//
// The pre-0016 rule — any stored version declaring the handler — would admit
// every case below, because the fixture always stores such a version.
func TestAttachmentsGateOnTheActiveDeployment(t *testing.T) {
	f := newAggregateFixture(t)
	f.store("WorkerBundle", "worker-bundle-probe", map[string]any{
		"manifestDigest": "sha256:" + strings.Repeat("a", 64),
	})
	f.store("AtLeastOnceQueue", "queue", map[string]any{"messageRetentionSeconds": json.Number("345600")})
	f.store(moduleWorkerKind, "worker", map[string]any{})
	f.store(workerVersionKind, "version", f.versionSpec("worker", fetchHandler, scheduledHandler, queueHandler))

	attachments := map[string]map[string]any{
		workerCustomDomainKind: {
			"worker": f.ref(moduleWorkerKind, "worker"), "hostname": "app.portable-conformance.invalid",
		},
		workerCronTriggerKind: {
			"worker": f.ref(moduleWorkerKind, "worker"), "cron": "0 3 * * *",
		},
		queueConsumerKind: {
			"worker": f.ref(moduleWorkerKind, "worker"), "queue": f.ref("AtLeastOnceQueue", "queue"),
			"maxBatchSize": json.Number("10"), "maxBatchTimeoutSeconds": json.Number("5"),
			"maxRetries": json.Number("3"), "retryDelaySeconds": json.Number("60"),
			"maxConcurrency": json.Number("4"),
		},
	}
	for kind, spec := range attachments {
		f.requireCode(
			f.validate(kind, "attachment", spec),
			"unsupported_capability", kind+" while the worker has no deployment",
		)
	}

	// A deployment that serves only fetch answers the custom domain and nothing
	// else, even though a stored version declares every handler.
	f.store(workerVersionKind, "fetch-version", f.versionSpec("worker", fetchHandler))
	f.store(workerDeploymentKind, "deployment", f.deploymentSpec("worker", map[string]int{"fetch-version": 10000}))
	f.requireAccepted(
		f.validate(workerCustomDomainKind, "attachment", attachments[workerCustomDomainKind]),
		"a custom domain on a deployment that serves fetch",
	)
	for _, kind := range []string{workerCronTriggerKind, queueConsumerKind} {
		f.requireCode(
			f.validate(kind, "attachment", attachments[kind]),
			"unsupported_capability", kind+" whose handler the deployment does not serve",
		)
	}

	// EVERY weighted version must export it, not some: adding a second weighted
	// version that exports only fetch closes the gates the first opened.
	f.store(workerDeploymentKind, "deployment", f.deploymentSpec("worker", map[string]int{"version": 10000}))
	for kind, spec := range attachments {
		f.requireAccepted(f.validate(kind, "attachment", spec), kind+" on a fully serving deployment")
	}
	f.store(workerDeploymentKind, "deployment",
		f.deploymentSpec("worker", map[string]int{"version": 5000, "fetch-version": 5000}))
	for _, kind := range []string{workerCronTriggerKind, queueConsumerKind} {
		f.requireCode(
			f.validate(kind, "attachment", attachments[kind]),
			"unsupported_capability", kind+" when only some weighted versions export its handler",
		)
	}
}

// TestDeploymentChangePreservesDependents proves rule D in both directions: a
// change that would leave any of the four dependents unserved is refused before
// mutation, and the same change is representable once nothing needs it.
func TestDeploymentChangePreservesDependents(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	f.store("AtLeastOnceQueue", "queue", map[string]any{"messageRetentionSeconds": json.Number("345600")})
	f.store(workerCustomDomainKind, "domain", map[string]any{
		"worker": f.ref(moduleWorkerKind, "worker"), "hostname": "app.portable-conformance.invalid",
	})
	f.store(workerCronTriggerKind, "cron", map[string]any{
		"worker": f.ref(moduleWorkerKind, "worker"), "cron": "0 3 * * *",
	})
	f.store(queueConsumerKind, "consumer", map[string]any{
		"worker": f.ref(moduleWorkerKind, "worker"), "queue": f.ref("AtLeastOnceQueue", "queue"),
		"maxBatchSize": json.Number("10"), "maxBatchTimeoutSeconds": json.Number("5"),
		"maxRetries": json.Number("3"), "retryDelaySeconds": json.Number("60"),
		"maxConcurrency": json.Number("4"),
	})
	f.store(moduleWorkerKind, "caller-worker", map[string]any{})
	callerSpec := f.versionSpec("caller-worker", fetchHandler)
	callerSpec["serviceBindings"] = []any{map[string]any{
		"name":     "UPSTREAM",
		"resource": f.ref(moduleWorkerKind, "worker"),
	}}
	caller := f.store(workerVersionKind, "caller-version", callerSpec)

	f.store(workerVersionKind, "fetch-only", f.versionSpec("worker", fetchHandler))
	f.store(workerVersionKind, "no-fetch", f.versionSpec("worker", scheduledHandler, queueHandler))

	f.requireCode(
		f.validate(workerDeploymentKind, "deployment",
			f.deploymentSpec("worker", map[string]int{"fetch-only": 10000})),
		"unsupported_capability", "dropping the scheduled handler a live cron trigger needs",
	)
	f.requireCode(
		f.validate(workerDeploymentKind, "deployment",
			f.deploymentSpec("worker", map[string]int{"no-fetch": 10000})),
		"unsupported_capability", "dropping the fetch handler a live custom domain needs",
	)

	// With every attachment gone the inbound service binding is the ONLY
	// remaining dependent, and it alone still refuses the same change.
	for kind, name := range map[string]string{
		workerCustomDomainKind: "domain", workerCronTriggerKind: "cron", queueConsumerKind: "consumer",
	} {
		f.host.removeResource(resourceKey(f.space, f.group, kind, name))
	}
	hostErr := f.validate(workerDeploymentKind, "deployment",
		f.deploymentSpec("worker", map[string]int{"no-fetch": 10000}))
	f.requireCode(hostErr, "unsupported_capability", "dropping fetch while an inbound service binding is live")
	if !strings.Contains(hostErr.Message, caller.Name) {
		t.Fatalf("the refusal does not name the inbound service binding holder: %s", hostErr.Message)
	}

	f.host.removeResource(resourceKey(f.space, f.group, workerVersionKind, caller.Name))
	f.requireAccepted(
		f.validate(workerDeploymentKind, "deployment",
			f.deploymentSpec("worker", map[string]int{"no-fetch": 10000})),
		"the same change once no dependent needs fetch",
	)
}

// TestDeploymentDeleteBlockedByDependent proves rule E: nothing references a
// deployment, yet it is what serves every attachment and inbound binding of its
// worker, so deleting it while one lives is `dependency_in_use`.
func TestDeploymentDeleteBlockedByDependent(t *testing.T) {
	f := newAggregateFixture(t)
	_, _, deployment := f.baseAggregate()
	f.requireAccepted(f.host.dependencyInUse(deployment), "deleting a deployment nothing needs")

	f.store(workerCronTriggerKind, "cron", map[string]any{
		"worker": f.ref(moduleWorkerKind, "worker"), "cron": "0 3 * * *",
	})
	hostErr := f.host.dependencyInUse(deployment)
	f.requireCode(hostErr, "dependency_in_use", "deleting a deployment a live cron trigger needs")
	if !strings.Contains(hostErr.Message, "cron") {
		t.Fatalf("the refusal does not name the dependent: %s", hostErr.Message)
	}
	f.host.removeResource(resourceKey(f.space, f.group, workerCronTriggerKind, "cron"))
	f.requireAccepted(f.host.dependencyInUse(deployment), "deleting the deployment once the trigger is gone")
}

// TestWorkerReadinessFollowsDeployment proves rule G: a Module Worker's Ready
// condition is a claim about SERVICE. It exists and serves nothing until a
// deployment does, and an inbound service binding to a worker in that state is
// refused at bind time rather than stored as a promise nothing keeps.
func TestWorkerReadinessFollowsDeployment(t *testing.T) {
	f := newAggregateFixture(t)
	f.store("WorkerBundle", "worker-bundle-probe", map[string]any{
		"manifestDigest": "sha256:" + strings.Repeat("a", 64),
	})
	worker := f.store(moduleWorkerKind, "worker", map[string]any{})
	f.store(moduleWorkerKind, "caller-worker", map[string]any{})
	callerSpec := f.versionSpec("caller-worker", fetchHandler)
	callerSpec["serviceBindings"] = []any{map[string]any{
		"name":     "UPSTREAM",
		"resource": f.ref(moduleWorkerKind, "worker"),
	}}

	requireReady := func(want, reason, subject string) {
		t.Helper()
		rendered := f.host.renderResource(worker)
		status, _ := rendered["status"].(map[string]any)
		conditions, _ := status["conditions"].([]map[string]any)
		if len(conditions) != 1 {
			t.Fatalf("%s: conditions = %+v", subject, conditions)
		}
		if conditions[0]["status"] != want || (reason != "" && conditions[0]["reason"] != reason) {
			t.Fatalf("%s: Ready = %v/%v, want %s/%s", subject, conditions[0]["status"], conditions[0]["reason"], want, reason)
		}
	}
	requireReady("False", "Provisioning", "a worker with no deployment")
	f.requireCode(
		f.validate(workerVersionKind, "caller-version", callerSpec),
		"unsupported_capability", "a service binding to a worker with no deployment",
	)

	f.store(workerVersionKind, "no-fetch", f.versionSpec("worker", scheduledHandler))
	f.store(workerDeploymentKind, "deployment", f.deploymentSpec("worker", map[string]int{"no-fetch": 10000}))
	requireReady("False", "UnsupportedCapability", "a worker whose deployment exports no fetch")
	f.requireCode(
		f.validate(workerVersionKind, "caller-version", callerSpec),
		"unsupported_capability", "a service binding to a worker that serves no fetch",
	)

	f.store(workerVersionKind, "fetch", f.versionSpec("worker", fetchHandler))
	f.store(workerDeploymentKind, "deployment", f.deploymentSpec("worker", map[string]int{"fetch": 10000}))
	requireReady("True", "Available", "a worker whose deployment serves fetch")
	f.requireAccepted(
		f.validate(workerVersionKind, "caller-version", callerSpec),
		"a service binding to a serving worker",
	)
}

// TestEnvironmentNamespaceIsSingle proves rule F: `vars` keys,
// `requiredSensitiveVars` entries, and every binding name occupy ONE namespace.
// None of it is expressible in the desired schema, which is why each case below
// is a document a schema-only host accepts.
func TestEnvironmentNamespaceIsSingle(t *testing.T) {
	f := newAggregateFixture(t)
	f.baseAggregate()
	f.store("EdgeKVNamespace", "cache", map[string]any{})
	f.store("ObjectBucket", "media", map[string]any{})
	kvBinding := []any{map[string]any{"name": "STORE", "resource": f.ref("EdgeKVNamespace", "cache")}}

	cases := []struct {
		name    string
		mutate  func(spec map[string]any)
		refused bool
	}{
		{"vars key against a binding name", func(spec map[string]any) {
			spec["kvBindings"] = kvBinding
			spec["vars"] = map[string]any{"STORE": "https://store.invalid"}
		}, true},
		{"sealed value name against a binding name", func(spec map[string]any) {
			spec["kvBindings"] = kvBinding
			spec["requiredSensitiveVars"] = []any{"STORE"}
		}, true},
		{"two binding lists on one name", func(spec map[string]any) {
			spec["kvBindings"] = kvBinding
			spec["bucketBindings"] = []any{map[string]any{
				"name": "STORE", "resource": f.ref("ObjectBucket", "media"),
			}}
		}, true},
		{"a vars key against a sealed value name", func(spec map[string]any) {
			spec["vars"] = map[string]any{"STORE_TOKEN": "x"}
			spec["requiredSensitiveVars"] = []any{"STORE_TOKEN"}
		}, true},
		{"distinct names across all four sources", func(spec map[string]any) {
			spec["kvBindings"] = kvBinding
			spec["bucketBindings"] = []any{map[string]any{
				"name": "MEDIA", "resource": f.ref("ObjectBucket", "media"),
			}}
			spec["vars"] = map[string]any{"STORE_URL": "https://store.invalid"}
			spec["requiredSensitiveVars"] = []any{"STORE_TOKEN_NAME"}
		}, false},
	}
	for _, testCase := range cases {
		spec := f.versionSpec("worker", fetchHandler)
		testCase.mutate(spec)
		hostErr := f.validate(workerVersionKind, "environment-version", spec)
		if testCase.refused {
			f.requireCode(hostErr, "invalid_argument", testCase.name)
			continue
		}
		f.requireAccepted(hostErr, testCase.name)
	}
}

// commitPinnedBundle plants one corpus-pinned artifact manifest on the host
// exactly as an accepted commit would, and installs the WorkerBundle resource
// that references it. It is the state every rule about what a version RUNS is
// decided against: the resource carries only a digest, and the manifest behind
// that digest is what names the module.
func (f *aggregateFixture) commitPinnedBundle(name string, manifest map[string]any, wantDigest string) {
	f.t.Helper()
	digest, raw := canonicalManifestDigest(f.t, manifest)
	if wantDigest != "" && digest != wantDigest {
		f.t.Fatalf("pinned manifest of %s canonicalizes to %s, want %s", name, digest, wantDigest)
	}
	f.host.manifests[digest] = raw
	f.host.grantManifest(referencePrimaryAuth.Tenant, digest)
	f.store("WorkerBundle", name, map[string]any{"manifestDigest": digest})
}

// versionSpecOfBundle builds a Worker Version desired spec bound to a NAMED
// bundle rather than the corpus probe's.
func (f *aggregateFixture) versionSpecOfBundle(worker, bundle string, handlers ...string) map[string]any {
	spec := f.versionSpec(worker, handlers...)
	spec["bundle"] = f.ref("WorkerBundle", bundle)
	return spec
}

// TestDeclaredHandlerMustBeExportedByTheReferencedModule proves the second half
// of the ES Module Worker ABI's handler rule (spec/decisions/0019): the ABI says
// which handlers EXIST, and the module a version references says which of them
// that version may declare.
//
// `loadModule` fails a declared-but-unexported handler with
// `handler_not_exported` before any traffic arrives, so storing such a version
// would leave the attachment gate admitting a cron trigger or a queue consumer
// against a handler that does not exist. The refusal therefore has to happen
// where the bundle relation is resolvable, and it must NOT refuse a version
// declaring only what the module does export — a host that refused everything
// would satisfy the rule while implementing nothing.
func TestDeclaredHandlerMustBeExportedByTheReferencedModule(t *testing.T) {
	f := newAggregateFixture(t)
	input := f.host.contract.RunnerInput
	f.store(moduleWorkerKind, "worker", map[string]any{})
	f.commitPinnedBundle(
		input.FetchOnlyBundle.Name, input.FetchOnlyBundle.Manifest, input.FetchOnlyBundle.ManifestDigest,
	)

	f.requireCode(
		f.validate(workerVersionKind, "version", f.versionSpecOfBundle(
			"worker", input.FetchOnlyBundle.Name, fetchHandler, scheduledHandler,
		)),
		"invalid_argument", "a version declaring a handler its module does not export",
	)
	f.requireAccepted(
		f.validate(workerVersionKind, "version", f.versionSpecOfBundle(
			"worker", input.FetchOnlyBundle.Name, fetchHandler,
		)),
		"a version declaring only what its module exports",
	)

	// The corpus probe bundle is the one the lane's positive controls are driven
	// against, and one of them declares the WHOLE vocabulary. If its module did
	// not export all of it, that control would be a version this very rule must
	// refuse, and no conforming host could complete the check that carries it.
	f.commitPinnedBundle(
		input.WorkerBundle.Name, input.WorkerBundle.Manifest, input.WorkerBundle.Desired["manifestDigest"].(string),
	)
	f.requireAccepted(
		f.validate(workerVersionKind, "full-version", f.versionSpecOfBundle(
			"worker", input.WorkerBundle.Name, input.SupportProbes.RuntimeContract.Handlers...,
		)),
		"a version declaring the whole runtime vocabulary against the corpus bundle",
	)
}

// TestUnknownModuleBytesAreNotGuessedAbout states the exact reach of the rule
// above in the reference host. What a module exports is a fact about
// JavaScript, and this host executes none: it answers only about modules whose
// bytes the corpus pinned. Code it was never told about is accepted rather than
// refused, because refusing would be a guess — a real host answers by loading
// the module. The lane proves the refusal for pinned bundles and documents the
// rest as a host obligation (spec/host-api/v1alpha3.md).
func TestUnknownModuleBytesAreNotGuessedAbout(t *testing.T) {
	f := newAggregateFixture(t)
	f.store(moduleWorkerKind, "worker", map[string]any{})
	f.commitPinnedBundle("unknown-bundle", workerBundleUnitManifest(), "")
	f.requireAccepted(
		f.validate(workerVersionKind, "version", f.versionSpecOfBundle(
			"worker", "unknown-bundle", fetchHandler, scheduledHandler, queueHandler,
		)),
		"a version bound to a module this host was never told about",
	)
}
