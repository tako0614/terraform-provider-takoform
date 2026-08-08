package portableconformancev3

import (
	"strings"
	"testing"
)

// requireIdentity holds one stored resource to an exact generation/revision
// pair, which is the whole subject of these tests: what a mutation elsewhere is
// allowed to move.
func requireIdentity(t *testing.T, resource *storedResource, generation, revision int64, subject string) {
	t.Helper()
	if resource.Generation != generation || resource.Revision != revision {
		t.Fatalf(
			"%s: generation/revision = %d/%d, want %d/%d",
			subject, resource.Generation, resource.Revision, generation, revision,
		)
	}
}

// TestWorkerRevisionFollowsItsDeployment proves the derived-rendering rule for
// the Worker aggregate: the worker is never touched, yet what the host renders
// for it changes with its deployment, so its revision — and therefore its
// ETag — must move each time, and its generation must not move at all.
func TestWorkerRevisionFollowsItsDeployment(t *testing.T) {
	f := newAggregateFixture(t)
	f.store("WorkerBundle", "worker-bundle-probe", map[string]any{
		"manifestDigest": "sha256:" + strings.Repeat("a", 64),
	})
	worker := f.store(moduleWorkerKind, "worker", map[string]any{})
	requireIdentity(t, worker, 1, 1, "a freshly created worker")

	// A stored version is not a running one: it changes nothing about what the
	// worker serves, so it must not churn the worker's revision.
	f.store(workerVersionKind, "version", f.versionSpec("worker", fetchHandler))
	requireIdentity(t, worker, 1, 1, "a worker whose new version no deployment selects")

	f.store(workerDeploymentKind, "deployment", f.deploymentSpec("worker", map[string]int{"version": 10000}))
	requireIdentity(t, worker, 1, 2, "a worker whose deployment now serves fetch")

	// And back: removing the deployment returns the worker to Provisioning,
	// which is again a different representation.
	f.host.removeResource(resourceKey(f.space, f.group, workerDeploymentKind, "deployment"))
	requireIdentity(t, worker, 1, 3, "a worker whose deployment was removed")
}

// TestRelationDriftAdvancesTheSourceRevision proves the same rule from the
// other direction, on the path decision 0015 owns: the mutation is a delete of
// the TARGET, and the source's rendered Ready condition changes without the
// source being addressed at all.
func TestRelationDriftAdvancesTheSourceRevision(t *testing.T) {
	f := newAggregateFixture(t)
	f.store("WorkerBundle", "worker-bundle-probe", map[string]any{
		"manifestDigest": "sha256:" + strings.Repeat("a", 64),
	})
	f.store(moduleWorkerKind, "worker", map[string]any{})
	f.store("EdgeKVNamespace", "cache", map[string]any{})
	spec := f.versionSpec("worker", fetchHandler)
	spec["kvBindings"] = []any{map[string]any{
		"name":     "STORE",
		"resource": f.ref("EdgeKVNamespace", "cache"),
	}}
	source := f.store(workerVersionKind, "version", spec)
	requireIdentity(t, source, 1, 1, "a bound source")

	f.host.removeResource(resourceKey(f.space, f.group, "EdgeKVNamespace", "cache"))
	requireIdentity(t, source, 1, 2, "a source whose relation target vanished")
	if _, _, drifted := f.host.relationDrift(source); !drifted {
		t.Fatal("the source does not report the drift its new revision was issued for")
	}
}

// TestDerivedRevisionPassTerminatesAndDoesNotChurn is the evidence behind the
// one-pass claim. The derived rendering is a pure function of the stored specs,
// relations, and UIDs and reads no revision, so restoring the invariant once
// restores it for good: a second pass over a settled store moves nothing, and
// neither does a mutation in an unrelated corner of the space.
func TestDerivedRevisionPassTerminatesAndDoesNotChurn(t *testing.T) {
	f := newAggregateFixture(t)
	worker, version, deployment := f.baseAggregate()
	settled := map[*storedResource]int64{
		worker: worker.Revision, version: version.Revision, deployment: deployment.Revision,
	}

	f.host.advanceDerivedRevisions(f.space, "")
	f.host.advanceDerivedRevisions(f.space, "")
	for resource, revision := range settled {
		if resource.Revision != revision {
			t.Fatalf(
				"re-running the pass over a settled store moved %s %s from revision %d to %d",
				resource.Kind, resource.Name, revision, resource.Revision,
			)
		}
	}

	// An unrelated resource joining the space renders nothing differently.
	f.store("EdgeKVNamespace", "unrelated", map[string]any{})
	for resource, revision := range settled {
		if resource.Revision != revision {
			t.Fatalf(
				"an unrelated create moved %s %s from revision %d to %d",
				resource.Kind, resource.Name, revision, resource.Revision,
			)
		}
	}
}
