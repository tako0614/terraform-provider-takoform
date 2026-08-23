package clientv3

// incarnation_key_test.go measures one property of the Idempotency-Key this
// client derives: it names the INCARNATION, not just the name and a counter.
//
// The measurements are driven against the lane's own deterministic reference
// host over real HTTP, because the defect only exists in the presence of a host
// that RETAINS replay records — and retaining them is exactly what the record
// is for. A hand-written stub could be written to agree with whatever the
// client did; the reference host is the implementation the whole v1beta1
// corpus is measured against, and it replays a recorded response before it
// resolves the resource the request addresses (ReferenceHost.tryReplay).

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/portableconformancev3"
)

// referenceHostToken is the primary credential the deterministic reference host
// accepts. It is a fixture of a disposable in-process host, never a secret.
const referenceHostToken = "reference-primary-token"

type incarnationProbe struct {
	client *Client
	ref    FormRef
	space  string
	name   string
}

func newIncarnationProbe(t *testing.T) incarnationProbe {
	t.Helper()
	// This client speaks v1beta2, so it is driven against the corpus of that
	// lane: pointing it at the retained corpus would test a client against a
	// host serving paths it no longer builds.
	contract, err := portableconformancev3.Verify("../../conformance/portable-host-v1beta2")
	if err != nil {
		t.Fatalf("verify portable host v3 contract: %v", err)
	}
	catalog, err := portableconformancev3.FallbackCatalog(contract)
	if err != nil {
		t.Fatalf("fallback catalog: %v", err)
	}
	server := httptest.NewServer(portableconformancev3.NewReferenceHost(contract, catalog))
	t.Cleanup(server.Close)
	client := NewWithOptions(server.URL, referenceHostToken, server.Client(), Options{})
	if _, err := client.Discover(context.Background()); err != nil {
		t.Fatalf("discover: %v", err)
	}
	probe := contract.RunnerInput.AtLeastOnceQueue
	return incarnationProbe{
		client: client,
		ref: FormRef{
			APIVersion:        probe.Identity.FormRef.APIVersion,
			Kind:              probe.Identity.FormRef.Kind,
			DefinitionVersion: probe.Identity.FormRef.DefinitionVersion,
			SchemaDigest:      probe.Identity.FormRef.SchemaDigest,
		},
		space: contract.RunnerInput.Space,
		name:  probe.Name,
	}
}

// create applies a fresh incarnation of the probe's one name. The retention is
// a parameter so two successive creations of the SAME name are two distinct
// requests: an apply names no incarnation (none exists yet), so a create whose
// every byte repeated would be answered from the first create's own record and
// there would be no second incarnation to measure a delete against.
func (p incarnationProbe) create(t *testing.T, retentionSeconds int) *Resource {
	t.Helper()
	res, err := p.client.ApplyResource(context.Background(), &Resource{
		APIVersion: p.ref.APIVersion,
		Kind:       p.ref.Kind,
		Form:       &FormReference{FormRef: p.ref},
		Metadata:   Metadata{Name: p.name, Space: p.space},
		// Every declared field is sent, defaults included: a prepare echoes the
		// materialized spec, and a client that omitted a defaulted field would
		// be told its own spec came back changed.
		Spec: map[string]any{
			"messageRetentionSeconds": retentionSeconds,
			"deliveryDelaySeconds":    0,
		},
	}, Fence{})
	if err != nil {
		t.Fatalf("create %s with retention %d: %v", p.name, retentionSeconds, err)
	}
	return res
}

func (p incarnationProbe) read(t *testing.T) (*Resource, error) {
	t.Helper()
	return p.client.GetResource(context.Background(), p.space, p.ref, p.name)
}

// TestDeleteKeyDistinguishesTwoIncarnationsOfOneName is the regression.
//
// A delete fences on the desired generation (decision 0011, amended
// 2026-08-09). A re-created resource starts at generation 1 exactly as it
// started at revision 1, so a delete key built from the name and the generation
// alone is the SAME key — and the same request fingerprint — for two resources
// that only ever shared a name. The host replays the first delete's 204 before
// it resolves the resource, and the second incarnation is never deleted while
// its client records it as gone.
func TestDeleteKeyDistinguishesTwoIncarnationsOfOneName(t *testing.T) {
	ctx := context.Background()
	probe := newIncarnationProbe(t)

	first := probe.create(t, 345600)
	if err := probe.client.DeleteResource(
		ctx, probe.space, probe.ref, probe.name, first.Metadata.UID, first.Metadata.Generation,
	); err != nil {
		t.Fatalf("delete the first incarnation: %v", err)
	}
	if _, err := probe.read(t); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after the first delete the resource must be gone, read = %v", err)
	}

	second := probe.create(t, 600000)
	if second.Metadata.UID == first.Metadata.UID {
		t.Fatalf("the host reused uid %s for a second incarnation; the probe measures nothing",
			second.Metadata.UID)
	}
	// The precondition that makes the collision reachable: both incarnations of
	// this name are at the same generation, so everything the old key was built
	// from is identical.
	if second.Metadata.Generation != first.Metadata.Generation {
		t.Fatalf("a re-created resource must start at generation %s, got %s",
			first.Metadata.Generation, second.Metadata.Generation)
	}

	if err := probe.client.DeleteResource(
		ctx, probe.space, probe.ref, probe.name, second.Metadata.UID, second.Metadata.Generation,
	); err != nil {
		t.Fatalf("delete the second incarnation: %v", err)
	}
	live, err := probe.read(t)
	if err == nil {
		t.Fatalf(
			"the second delete was answered from the first delete's replay record and never happened: "+
				"the host still holds uid %s at generation %s under the name %s, while the client was told 204",
			live.Metadata.UID, live.Metadata.Generation, probe.name,
		)
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("after the second delete the resource must be gone, read = %v", err)
	}
}

// TestDeleteKeyStillReplaysTheSameDeleteRetried is the other half: the key is
// still deterministic for the operation idempotency exists for. One incarnation
// deleted twice under the same uid and generation — a client that lost the
// first response and retried — is answered from the record with the same
// terminal 204, and does NOT become a second delete that finds nothing.
func TestDeleteKeyStillReplaysTheSameDeleteRetried(t *testing.T) {
	ctx := context.Background()
	probe := newIncarnationProbe(t)
	created := probe.create(t, 345600)

	for attempt := 1; attempt <= 2; attempt++ {
		err := probe.client.DeleteResource(
			ctx, probe.space, probe.ref, probe.name, created.Metadata.UID, created.Metadata.Generation,
		)
		if errors.Is(err, ErrNotFound) {
			t.Fatalf(
				"attempt %d of the SAME delete was executed a second time instead of replayed: "+
					"the host resolved the name, found nothing, and answered resource_not_found",
				attempt,
			)
		}
		if err != nil {
			t.Fatalf("attempt %d of the same delete: %v", attempt, err)
		}
	}
}

// TestObserveKeyDistinguishesTwoIncarnationsOfOneName measures the same defect
// on the lane's other keyed resource operation. observe is read-only, but it is
// a POST carrying an Idempotency-Key, so a stale record answers it too — and
// what it hands back is a whole representation. Every response check the client
// makes would pass: the name, space and exact FormRef are the ones asked for,
// and the fenced generation matches, because both incarnations are at 1.
func TestObserveKeyDistinguishesTwoIncarnationsOfOneName(t *testing.T) {
	ctx := context.Background()
	probe := newIncarnationProbe(t)

	first := probe.create(t, 345600)
	if _, err := probe.client.ObserveResource(
		ctx, probe.space, probe.ref, probe.name, first.Metadata.UID, first.Metadata.Generation,
	); err != nil {
		t.Fatalf("observe the first incarnation: %v", err)
	}
	if err := probe.client.DeleteResource(
		ctx, probe.space, probe.ref, probe.name, first.Metadata.UID, first.Metadata.Generation,
	); err != nil {
		t.Fatalf("delete the first incarnation: %v", err)
	}

	second := probe.create(t, 600000)
	observed, err := probe.client.ObserveResource(
		ctx, probe.space, probe.ref, probe.name, second.Metadata.UID, second.Metadata.Generation,
	)
	if err != nil {
		t.Fatalf("observe the second incarnation: %v", err)
	}
	if observed.Metadata.UID != second.Metadata.UID {
		t.Fatalf(
			"observe was answered from the first incarnation's replay record: reported uid %s for a resource "+
				"the host holds as uid %s",
			observed.Metadata.UID, second.Metadata.UID,
		)
	}
}

// TestIncarnationKeyComposition pins the derivation itself, so a future edit
// that drops a component from the key fails here by name rather than only in
// the host-driven probes above.
func TestIncarnationKeyComposition(t *testing.T) {
	ref := FormRef{
		APIVersion:        "edge.forms.takoform.com/v1beta1",
		Kind:              "AtLeastOnceQueue",
		DefinitionVersion: "0.1.0",
		SchemaDigest:      "sha256:" + "ab",
	}
	base := incarnationKey("delete", ref, "queue", "prod", "uid-1", "1", "")

	if got := incarnationKey("delete", ref, "queue", "prod", "uid-1", "1", ""); got != base {
		t.Fatalf("the same operation on the same incarnation must derive one key, got %s and %s", base, got)
	}
	differs := map[string]string{
		"incarnation": incarnationKey("delete", ref, "queue", "prod", "uid-2", "1", ""),
		"operation":   incarnationKey("observe", ref, "queue", "prod", "uid-1", "1", ""),
		"generation":  incarnationKey("delete", ref, "queue", "prod", "uid-1", "2", ""),
		"name":        incarnationKey("delete", ref, "other", "prod", "uid-1", "1", ""),
		"space":       incarnationKey("delete", ref, "queue", "staging", "uid-1", "1", ""),
		"body":        incarnationKey("delete", ref, "queue", "prod", "uid-1", "1", "digest"),
	}
	for component, key := range differs {
		if key == base {
			t.Errorf("a different %s must derive a different Idempotency-Key", component)
		}
	}
	// The key carries the addressable half of the Form identity — the group and
	// the kind — because that is what selects the resource. The rest of the
	// exact FormRef is not in the key and does not need to be: definitionVersion
	// and schemaDigest travel in the request target of a delete or observe and
	// in the body of an apply or import, both of which the host's own
	// fingerprint covers, so a key reused across them is refused rather than
	// answered wrongly.
	otherKind := ref
	otherKind.Kind = "EdgeKVNamespace"
	otherGroup := ref
	otherGroup.APIVersion = "data.forms.takoform.com/v9"
	for component, form := range map[string]FormRef{"kind": otherKind, "group": otherGroup} {
		if incarnationKey("delete", form, "queue", "prod", "uid-1", "1", "") == base {
			t.Errorf("a different Form %s must derive a different Idempotency-Key", component)
		}
	}
}

// TestDeleteRequiresTheIncarnationItRemoves records that the uid is not
// optional. A caller that can name the generation fence read it off a verified
// representation, and such a representation always carries a uid
// (verifyResourceResponse), so there is no legitimate caller with one and not
// the other.
func TestDeleteRequiresTheIncarnationItRemoves(t *testing.T) {
	probe := newIncarnationProbe(t)
	created := probe.create(t, 345600)
	err := probe.client.DeleteResource(
		context.Background(), probe.space, probe.ref, probe.name, "", created.Metadata.Generation,
	)
	if err == nil || !contains(err, "requires the recorded metadata.uid") {
		t.Fatalf("uid-less delete = %v, want a refusal naming the recorded metadata.uid", err)
	}
	if _, err := probe.read(t); err != nil {
		t.Fatalf("a refused delete must not touch the resource, read = %v", err)
	}
}
