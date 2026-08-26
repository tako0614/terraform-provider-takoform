package portableconformancev3

// runner_tenant_isolation_checks.go measures the one boundary this lane had
// documented and never driven: a resource's address begins with the
// authenticated TENANT (spec/decisions/0028).
//
// Every check here is BLACK BOX. Nothing below reaches inside a host: each one
// is two credentials, the same space, the same kind, the same resource name, and
// the ordinary lifecycle surface. That is possible only because the runner input
// already carries a second tenant's credential — the same one the operability
// checks use to prove an operation id, an upload session, and a content address
// are not capabilities (spec/decisions/0018). A runner that could authenticate as
// one tenant could not measure any of this, which is why the credential is
// required rather than optional (RunEndpoint).
//
// The set of checks here is enumerated by SURFACE, not by intent: every
// v1beta1 endpoint that takes a resource name is listed in
// nameAddressedResourceSurfaces (contract.go) beside the check that measures its
// boundary, and three tests bind that list to the reference host's routes, to
// the Lifecycle route block of the spec, and to requiredRunnerChecks. An
// enumeration by intent is what left `observe` and `import` out: both take a
// name, one hands back a whole representation and one mutates, and a host that
// scoped GET, PUT and DELETE while resolving either host-wide passed everything.
//
// What a black box cannot see is the SHAPE of the address — that the key carries
// the tenant rather than a filter being applied somewhere late — and it cannot
// construct a cross-tenant derived-rendering dependency at all, precisely because
// relations resolve inside one tenant. Those two facts are held by host-side
// tests in tenant_isolation_test.go, and the split is stated in
// spec/host-api/v1beta1.md.
//
// The refusal every ADDRESSING check expects is `resource_not_found` (404),
// never `permission_denied` (403). A foreign tenant's resource must be
// indistinguishable from one that was never created: a 403 would confirm that a
// resource of that name exists somewhere on the host, which is the exact fact the
// boundary exists to withhold, and it would leak it to anyone who can guess a
// name. Two refusals here are not 404, and neither is about addressing another
// tenant's resource: spending a review minted in another tenant is
// `invalid_argument` (400) against the caller's OWN resource, and an attachment
// whose worker exists but serves nothing is `unsupported_capability` (422) — the
// positive control that proves the relation resolved.

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// tenantIsolationNames are the probe resource names this file drives. They are
// invented rather than pinned for the same reason every other check's scratch
// names are: what the corpus pins is the Form identity and the desired shape, and
// these checks are about the ADDRESS a host files that shape under.
const (
	tenantSharedName      = "tenant-address-probe"
	tenantPrivateName     = "tenant-private-probe"
	tenantAbsentName      = "tenant-absent-probe"
	tenantOwnMutationName = "tenant-own-mutation-probe"
	tenantRelationWorker  = "tenant-relation-worker"
	tenantRelationBundle  = "tenant-relation-bundle"
	tenantRelationSource  = "tenant-relation-version"
	tenantPrepareName     = "tenant-prepare-probe"
	tenantIdempotencyName = "tenant-idempotency-probe"
	tenantImportHeldName  = "tenant-import-held-probe"
	tenantImportFreeName  = "tenant-import-free-probe"

	// The three names and the one native identity that measure the far edge of
	// the adoption claim: one identity, adopted once in each tenant, and held
	// inside each of them.
	tenantSharedNativeName        = "tenant-native-claim-probe"
	tenantSharedNativeForeignName = "tenant-native-claim-foreign-probe"
	tenantSharedNativeRivalName   = "tenant-native-claim-foreign-rival-probe"
	tenantSharedNativeID          = "native-tenant-shared-object"
)

// checkTenantIsolatedResourcePlane drives the whole V3-012 sequence. It runs
// LAST in the matrix and leaves the primary tenant's aggregate untouched: every
// resource it creates carries a name of its own, and every resource it creates in
// the second tenant is in a plane no other check reads.
func (r *v3Runner) checkTenantIsolatedResourcePlane() error {
	worker := r.target(r.contract.RunnerInput.ModuleWorker)
	for _, step := range []func(probeTarget) error{
		r.checkResourceAddressIsTenantScoped,
		r.checkResourceReadIsTenantIsolated,
		r.checkResourceObserveIsTenantIsolated,
		r.checkResourceUpdateIsTenantIsolated,
		r.checkResourceImportIsTenantIsolated,
		r.checkResourceDeleteIsTenantIsolated,
		r.checkRelationResolutionIsTenantScoped,
		r.checkPrepareIsTenantScoped,
		r.checkIdempotencyIsTenantScoped,
	} {
		if err := step(worker); err != nil {
			return err
		}
	}
	// And the half none of the nine states: the second tenant's plane is a plane,
	// not a waiting room. It runs last because it is the only one that needs a
	// Form declaring `update`, and because it must find both tenants' planes
	// already proved separate.
	return r.checkEachTenantMutatesItsOwnPlane()
}

// checkEachTenantMutatesItsOwnPlane proves the second tenant OWNS what it
// creates.
//
// Every other check in this file measures the boundary from one side: the
// alternate tenant creates, reads, and is refused everything belonging to the
// first. Across the whole corpus it never changed or removed anything of its
// own — so a host that is create-and-read-only for every tenant but the first,
// answering `resource_not_found` to each update and each delete of the second
// tenant's OWN resources, satisfied all nine while breaking decision 0028 rule 2
// outright. That host is not exotic: it is what a tenant column bolted onto a
// single-tenant store looks like when the write paths still resolve through the
// original owner, and the failure is silent, because a 404 for your own resource
// reads like a resource you never made.
//
// So both mutating surfaces are driven inside the second tenant's plane, against
// a name the FIRST tenant also holds, and the first tenant's resource is
// measured before and after each one. Two facts at once: the second tenant can
// really write, and writing does not reach across.
func (r *v3Runner) checkEachTenantMutatesItsOwnPlane() error {
	// The queue is the probe that declares `update`, so "the same operation the
	// holder was refused" is a real spec change here rather than a re-apply.
	subject := r.target(r.semanticSequenced())
	subject.Name = tenantOwnMutationName

	firstResponse, err := r.applyAs(r.token, subject, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-own-mutation-first",
	})
	if err != nil {
		return err
	}
	first, err := decodeResource(firstResponse, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the first tenant's create: %w", err)
	}
	secondResponse, err := r.applyAs(r.alternateTenantToken, subject, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-own-mutation-second",
	})
	if err != nil {
		return err
	}
	second, err := decodeResource(secondResponse, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the second tenant's create: %w", err)
	}

	changed := subject
	changed.Spec = r.desiredMutation(subject, 0,
		map[string]any{"deliveryDelaySeconds": 30, "messageRetentionSeconds": 600000})
	changed.Spec = r.materialize(changed.Ref, changed.Spec)
	updated, err := r.applyAs(r.alternateTenantToken, changed, applyOptions{
		ExpectedGeneration: second.Metadata.Generation,
		IdempotencyKey:     "key-tenant-own-mutation-update",
	})
	if err != nil {
		return err
	}
	moved, err := decodeResource(updated, http.StatusOK)
	if err != nil {
		return fmt.Errorf(
			"the second tenant updating its OWN %s: %w; a tenant that may create and read but not write is not "+
				"isolated, it is quarantined, and every tenant-boundary refusal in this file is satisfied by it",
			subject.Ref.Kind, err,
		)
	}
	if moved.Metadata.UID != second.Metadata.UID {
		return fmt.Errorf(
			"the second tenant's own update answered uid %s, want its own %s",
			moved.Metadata.UID, second.Metadata.UID,
		)
	}
	if moved.Metadata.Generation == second.Metadata.Generation {
		return fmt.Errorf(
			"the second tenant's own spec change left generation at %s; the update was answered but not performed",
			moved.Metadata.Generation,
		)
	}
	// It landed in the store, and only in that tenant's.
	readBack, err := r.readAs(r.alternateTenantToken, subject)
	if err != nil {
		return err
	}
	stored, err := decodeResource(readBack, http.StatusOK)
	if err != nil {
		return fmt.Errorf("the second tenant reading back what it updated: %w", err)
	}
	if stored.Metadata.Generation != moved.Metadata.Generation {
		return fmt.Errorf(
			"the second tenant's own update answered generation %s and reads back as %s",
			moved.Metadata.Generation, stored.Metadata.Generation,
		)
	}
	if err := r.requireUnmoved(first, "the second tenant's update of its own resource"); err != nil {
		return err
	}

	// The other mutating surface, in the tenant that holds the resource.
	if err := r.deleteExistingAs(r.alternateTenantToken, subject, "key-tenant-own-mutation-delete"); err != nil {
		return fmt.Errorf(
			"the second tenant deleting its OWN %s: %w; a delete that only ever refuses is a delete nobody has",
			subject.Ref.Kind, err,
		)
	}
	gone, err := r.readAs(r.alternateTenantToken, subject)
	if err != nil {
		return err
	}
	if err := r.expectStableError(gone, "resource_not_found"); err != nil {
		return fmt.Errorf("the second tenant reading what it deleted: %w", err)
	}
	if err := r.requireUnmoved(first, "the second tenant's delete of its own resource"); err != nil {
		return err
	}
	if err := r.deleteExisting(subject, "key-tenant-own-mutation-teardown"); err != nil {
		return err
	}
	r.complete("each-tenant-mutates-its-own-plane")
	return nil
}

// requireUnmoved re-reads one tenant's resource under the primary credential and
// holds its whole identity still.
func (r *v3Runner) requireUnmoved(before wireResource, subject string) error {
	target := probeTarget{
		Ref: before.Form.FormRef, Name: before.Metadata.Name, Space: before.Metadata.Space,
	}
	response, err := r.readAs(r.token, target)
	if err != nil {
		return err
	}
	after, err := decodeResource(response, http.StatusOK)
	if err != nil {
		return fmt.Errorf("%s left the other tenant's resource unreadable: %w", subject, err)
	}
	if after.Metadata.UID != before.Metadata.UID ||
		after.Metadata.Generation != before.Metadata.Generation ||
		after.Metadata.Revision != before.Metadata.Revision {
		return fmt.Errorf(
			"%s moved the other tenant's identically-named resource from uid %s generation %s revision %s to "+
				"uid %s generation %s revision %s",
			subject, before.Metadata.UID, before.Metadata.Generation, before.Metadata.Revision,
			after.Metadata.UID, after.Metadata.Generation, after.Metadata.Revision,
		)
	}
	return nil
}

// readAs reads one resource under a named credential without asserting anything
// about the representation. The tenant checks need the raw outcome — a 200 whose
// uid they compare, or a refusal they hold to the closed taxonomy — rather than
// the identity assertions r.read makes on the primary credential's behalf.
func (r *v3Runner) readAs(token string, target probeTarget) (wireResponse, error) {
	return r.requestWithToken(
		token, http.MethodGet,
		r.resourceURL(target.Ref, target.Name, "", r.exactQuery(target.Space, target.Ref)),
		nil, nil,
	)
}

// readUIDAs reads one resource under a named credential and returns the uid the
// host answered with.
func (r *v3Runner) readUIDAs(token string, target probeTarget, subject string) (string, error) {
	response, err := r.readAs(token, target)
	if err != nil {
		return "", err
	}
	resource, err := decodeResource(response, http.StatusOK)
	if err != nil {
		return "", fmt.Errorf("%s: %w", subject, err)
	}
	return resource.Metadata.UID, nil
}

// checkResourceAddressIsTenantScoped is the root fact: two tenants naming ONE
// resource — one space, one kind, one name — get two resources.
//
// A host whose internal key stops at space, kind and name cannot pass this. The
// second create addresses the record the first tenant already holds, so it either
// conflicts (`generation_conflict` under If-None-Match: *) or overwrites it; both
// are visible here, because the check requires two 201s and two different
// host-issued uids.
func (r *v3Runner) checkResourceAddressIsTenantScoped(worker probeTarget) error {
	shared := worker
	shared.Name = tenantSharedName

	primary, err := r.applyAs(r.token, shared, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-address-primary",
	})
	if err != nil {
		return err
	}
	primaryCreated, err := decodeResource(primary, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the first tenant's create: %w", err)
	}

	// The same space, the same kind, the same name, another tenant. This is a
	// CREATE, not a collision: the name is free in the second tenant's plane.
	alternate, err := r.applyAs(r.alternateTenantToken, shared, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-address-alternate",
	})
	if err != nil {
		return err
	}
	alternateCreated, err := decodeResource(alternate, http.StatusCreated)
	if err != nil {
		return fmt.Errorf(
			"a second tenant creating %s/%s/%s: %w; a resource address that omits the tenant makes one tenant's "+
				"name choice deny another's",
			shared.Space, shared.Ref.Kind, shared.Name, err,
		)
	}
	if primaryCreated.Metadata.UID == alternateCreated.Metadata.UID {
		return fmt.Errorf(
			"two tenants naming %s/%s/%s share the host-issued uid %s; they are one record",
			shared.Space, shared.Ref.Kind, shared.Name, primaryCreated.Metadata.UID,
		)
	}
	if alternateCreated.Metadata.Generation != "1" || alternateCreated.Metadata.Revision != "1" {
		return fmt.Errorf(
			"the second tenant's create answered generation/revision %s/%s, want 1/1; it adopted an existing record",
			alternateCreated.Metadata.Generation, alternateCreated.Metadata.Revision,
		)
	}

	// Each tenant keeps reading its own.
	primaryUID, err := r.readUIDAs(r.token, shared, "the first tenant reading its own resource")
	if err != nil {
		return err
	}
	alternateUID, err := r.readUIDAs(r.alternateTenantToken, shared, "the second tenant reading its own resource")
	if err != nil {
		return err
	}
	if primaryUID != primaryCreated.Metadata.UID || alternateUID != alternateCreated.Metadata.UID {
		return fmt.Errorf(
			"a read under one name answered uid %s to the first tenant and %s to the second, want %s and %s",
			primaryUID, alternateUID, primaryCreated.Metadata.UID, alternateCreated.Metadata.UID,
		)
	}
	r.complete("resource-address-is-tenant-scoped")
	return nil
}

// checkResourceReadIsTenantIsolated proves a read stops at the tenant, and that
// it stops INDISTINGUISHABLY.
//
// The second assertion is the one worth having. A host could answer 403 for a
// foreign resource and 404 for an absent one and still call itself isolated,
// while handing every caller a membership oracle over the whole host: guess a
// name, read the status code, learn whether somebody else owns it. So the check
// compares the foreign answer against the answer for a name nobody created and
// requires the same code — and the same HTTP status the closed taxonomy assigns
// it (spec/decisions/0018, 0028).
func (r *v3Runner) checkResourceReadIsTenantIsolated(worker probeTarget) error {
	private := worker
	private.Name = tenantPrivateName
	created, err := r.applyAs(r.token, private, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-private-create",
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(created, http.StatusCreated); err != nil {
		return fmt.Errorf("the holding tenant's create: %w", err)
	}

	foreign, err := r.readAs(r.alternateTenantToken, private)
	if err != nil {
		return err
	}
	if err := r.expectStableError(foreign, "resource_not_found"); err != nil {
		return fmt.Errorf("another tenant reading %s: %w", private.Name, err)
	}

	absent := worker
	absent.Name = tenantAbsentName
	nothing, err := r.readAs(r.alternateTenantToken, absent)
	if err != nil {
		return err
	}
	if err := r.expectStableError(nothing, "resource_not_found"); err != nil {
		return fmt.Errorf("reading a name nobody created: %w", err)
	}
	// The code and the status are equal by construction above. The MESSAGE is
	// where a host would give it away — "resource is absent" against "that
	// resource is not yours" is the same 404 and a complete oracle — so the two
	// are compared with the only two things that may legitimately differ removed:
	// the per-request id, and the resource name each request named.
	foreignMessage, err := comparableRefusal(foreign, private.Name)
	if err != nil {
		return err
	}
	absentMessage, err := comparableRefusal(nothing, absent.Name)
	if err != nil {
		return err
	}
	if foreignMessage != absentMessage {
		return fmt.Errorf(
			"another tenant's resource is refused with %q and an absent one with %q; the two must be "+
				"indistinguishable, or the difference is a membership oracle over every name on the host",
			foreignMessage, absentMessage,
		)
	}
	r.complete("resource-read-is-tenant-isolated")
	return nil
}

// checkResourceObserveIsTenantIsolated proves the lane's other READ surface
// stops at the tenant, to the same standard the plain read is held to.
//
// `observe` is a POST, it carries a generation fence and an Idempotency-Key, and
// it answers a `{resource: …}` envelope rather than a bare resource — so a host
// implementing it beside GET rather than through GET is the ordinary case, and
// scoping one says nothing about the other. What it hands back is the WHOLE
// representation: uid, generation, revision, the complete desired spec, and the
// rendered status. A host resolving it host-wide therefore discloses strictly
// more than a leaked GET would, and does it to any caller who can guess a name.
//
// The fence presented is the holder's REAL current generation, so nothing but
// the address can refuse it — a host-wide resolver would find the resource, find
// the fence exact, and answer 200. And the refusal is compared against the
// refusal for a name nobody created, with the requestId and the echoed name
// normalized out, because a 404 that reads differently is the same membership
// oracle a 403 would be.
func (r *v3Runner) checkResourceObserveIsTenantIsolated(worker probeTarget) error {
	private := worker
	private.Name = tenantPrivateName
	held, err := r.readAs(r.token, private)
	if err != nil {
		return err
	}
	holder, err := decodeResource(held, http.StatusOK)
	if err != nil {
		return fmt.Errorf("the holding tenant reading before the foreign observe: %w", err)
	}

	foreign, err := r.statusActionAs(
		r.alternateTenantToken, private, "observe",
		holder.Metadata.Generation, "key-tenant-foreign-observe", nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(foreign, "resource_not_found"); err != nil {
		return fmt.Errorf(
			"another tenant observing %s at its exact generation %s: %w",
			private.Name, holder.Metadata.Generation, err,
		)
	}

	absent := worker
	absent.Name = tenantAbsentName
	nothing, err := r.statusActionAs(
		r.alternateTenantToken, absent, "observe",
		holder.Metadata.Generation, "key-tenant-absent-observe", nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(nothing, "resource_not_found"); err != nil {
		return fmt.Errorf("observing a name nobody created: %w", err)
	}
	foreignMessage, err := comparableRefusal(foreign, private.Name)
	if err != nil {
		return err
	}
	absentMessage, err := comparableRefusal(nothing, absent.Name)
	if err != nil {
		return err
	}
	if foreignMessage != absentMessage {
		return fmt.Errorf(
			"an observe of another tenant's resource is refused with %q and of an absent one with %q; the two must "+
				"be indistinguishable, or the difference is a membership oracle over every name on the host",
			foreignMessage, absentMessage,
		)
	}

	// The holder observes the same name, with the same fence, and is answered
	// its own representation: the request shape and the fence were both valid,
	// so what refused the other tenant was the address and nothing else. Without
	// this leg a host that refused EVERY observe would pass the two above.
	own, err := r.statusActionAs(
		r.token, private, "observe", holder.Metadata.Generation, "key-tenant-holder-observe", nil,
	)
	if err != nil {
		return err
	}
	observed, err := decodeResourceEnvelope(own)
	if err != nil {
		return fmt.Errorf("the holding tenant observing its own resource: %w", err)
	}
	if observed.Metadata.UID != holder.Metadata.UID {
		return fmt.Errorf(
			"the holder's own observe answered uid %s, want %s",
			observed.Metadata.UID, holder.Metadata.UID,
		)
	}
	r.complete("resource-observe-is-tenant-isolated")
	return nil
}

// checkResourceImportIsTenantIsolated proves adoption addresses the caller's own
// tenant, and it is the one surface where "absent-equivalent" is not a refusal.
//
// Import has its own answer for a name the caller does not hold: under
// `If-None-Match: *` it ADOPTS, minting a fresh resource at generation 1. So the
// rule for a name that exists only in another tenant is not "refuse" — it is
// "behave exactly as though the name were free", and a check that accepted any
// refusal would accept the very defect it is looking for. A host that resolves
// import host-wide fails in one of two ways, and both are measured here:
//
//   - it sees the foreign record as an occupant and refuses the create with
//     `generation_conflict` — a "that name is taken" oracle over the host; or
//   - it takes the update path and adopts the foreign record, writing this
//     tenant's desired state over another tenant's live resource. Adoption
//     followed by a delete is the complete destruction of a stranger's state
//     through a name.
//
// Absent-equivalence is asserted as an equality rather than as a status code:
// the adoption of the foreign-held name and the adoption of a name nobody holds
// anywhere must produce the same status, the same ETag, and the same document
// once the two things that legitimately differ — the minted uid and the name —
// are normalized out.
//
// The fenced form of import is the other half. An import carrying an update
// generation fence names an EXISTING resource, so against a name only another
// tenant holds it must fail `resource_not_found` (404), indistinguishably from
// the same request against a name nobody created. That leg runs first, while the
// second tenant still holds nothing under the name.
func (r *v3Runner) checkResourceImportIsTenantIsolated(worker probeTarget) error {
	held := worker
	held.Name = tenantImportHeldName
	created, err := r.applyAs(r.token, held, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-import-held-create",
	})
	if err != nil {
		return err
	}
	holder, err := decodeResource(created, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the holding tenant's create: %w", err)
	}

	// An import fenced at the holder's exact live generation. Nothing but the
	// address refuses it.
	fenced, err := r.importResourceAs(r.alternateTenantToken, held, importOptions{
		NativeID:           "native-tenant-import-held",
		IdempotencyKey:     "key-tenant-import-foreign-fenced",
		ExpectedGeneration: holder.Metadata.Generation,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(fenced, "resource_not_found"); err != nil {
		return fmt.Errorf(
			"another tenant importing %s onto its exact generation %s: %w",
			held.Name, holder.Metadata.Generation, err,
		)
	}
	free := worker
	free.Name = tenantImportFreeName
	fencedAbsent, err := r.importResourceAs(r.alternateTenantToken, free, importOptions{
		NativeID:           "native-tenant-import-free",
		IdempotencyKey:     "key-tenant-import-absent-fenced",
		ExpectedGeneration: holder.Metadata.Generation,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(fencedAbsent, "resource_not_found"); err != nil {
		return fmt.Errorf("a fenced import of a name nobody created: %w", err)
	}
	foreignMessage, err := comparableRefusal(fenced, held.Name)
	if err != nil {
		return err
	}
	absentMessage, err := comparableRefusal(fencedAbsent, free.Name)
	if err != nil {
		return err
	}
	if foreignMessage != absentMessage {
		return fmt.Errorf(
			"a fenced import onto another tenant's resource is refused with %q and onto an absent one with %q; the "+
				"two must be indistinguishable, or the difference is a membership oracle over every name on the host",
			foreignMessage, absentMessage,
		)
	}

	// The create-intent adoption: the name is FREE in this tenant, so it lands.
	adopted, err := r.importResourceAs(r.alternateTenantToken, held, importOptions{
		NativeID: "native-tenant-import-held-adopt", IdempotencyKey: "key-tenant-import-foreign-adopt", Create: true,
	})
	if err != nil {
		return err
	}
	adoptedResource, err := decodeResource(adopted, http.StatusCreated)
	if err != nil {
		return fmt.Errorf(
			"a second tenant adopting the name %s: %w; the name is held by another tenant and is therefore free "+
				"here, so adoption must succeed exactly as it does against a name nobody holds",
			held.Name, err,
		)
	}
	if adoptedResource.Metadata.UID == holder.Metadata.UID {
		return fmt.Errorf(
			"the second tenant's adoption answered the holder's uid %s; it adopted another tenant's record",
			holder.Metadata.UID,
		)
	}
	control, err := r.importResourceAs(r.alternateTenantToken, free, importOptions{
		NativeID: "native-tenant-import-free-adopt", IdempotencyKey: "key-tenant-import-free-adopt", Create: true,
	})
	if err != nil {
		return err
	}
	controlResource, err := decodeResource(control, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("adopting a name nobody holds: %w", err)
	}
	adoptedShape, err := comparableCreation(adopted, held.Name, adoptedResource.Metadata.UID)
	if err != nil {
		return err
	}
	controlShape, err := comparableCreation(control, free.Name, controlResource.Metadata.UID)
	if err != nil {
		return err
	}
	if adoptedShape != controlShape {
		return fmt.Errorf(
			"adopting a name another tenant holds answered %q and adopting a free name answered %q; a name held "+
				"only in another tenant must be absent-equivalent, so the two must differ in nothing but uid and name",
			adoptedShape, controlShape,
		)
	}

	// The adoption landed in the second tenant's plane, and nowhere else.
	adoptedUID, err := r.readUIDAs(r.alternateTenantToken, held, "the second tenant reading what it adopted")
	if err != nil {
		return err
	}
	if adoptedUID != adoptedResource.Metadata.UID {
		return fmt.Errorf(
			"the second tenant reads uid %s under the name it adopted, want %s",
			adoptedUID, adoptedResource.Metadata.UID,
		)
	}
	unmoved, err := r.readAs(r.token, held)
	if err != nil {
		return err
	}
	still, err := decodeResource(unmoved, http.StatusOK)
	if err != nil {
		return fmt.Errorf("a foreign import removed %s: %w", held.Name, err)
	}
	if still.Metadata.UID != holder.Metadata.UID ||
		still.Metadata.Generation != holder.Metadata.Generation ||
		still.Metadata.Revision != holder.Metadata.Revision {
		return fmt.Errorf(
			"a foreign import moved %s from uid %s generation %s revision %s to uid %s generation %s revision %s",
			held.Name, holder.Metadata.UID, holder.Metadata.Generation, holder.Metadata.Revision,
			still.Metadata.UID, still.Metadata.Generation, still.Metadata.Revision,
		)
	}
	// And the adoption CLAIM stops at the tenant, exactly as the name does.
	//
	// One tenant holds a native identity; the other adopts the same identity and
	// MUST be answered a resource of its own. A host enforcing the claim of
	// spec/decisions/0011 host-wide would refuse here with `import_conflict`,
	// which reads to a caller who can see nothing else as "somebody on this host
	// already manages that object" — a membership oracle over every identifier a
	// stranger can guess, and the same disclosure every other leg of this check
	// exists to prevent. Whose account an object lives in is authority this lane
	// does not model, so it is not this refusal's question to answer.
	shared := worker
	shared.Name = tenantSharedNativeName
	firstAdoption, err := r.importResourceAs(r.token, shared, importOptions{
		NativeID: tenantSharedNativeID, IdempotencyKey: "key-tenant-shared-native-adopt", Create: true,
	})
	if err != nil {
		return err
	}
	firstHolder, err := decodeResource(firstAdoption, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the first tenant adopting %s: %w", tenantSharedNativeID, err)
	}
	crossTenant := worker
	crossTenant.Name = tenantSharedNativeForeignName
	secondAdoption, err := r.importResourceAs(r.alternateTenantToken, crossTenant, importOptions{
		NativeID: tenantSharedNativeID, IdempotencyKey: "key-tenant-shared-native-foreign", Create: true,
	})
	if err != nil {
		return err
	}
	secondHolder, err := decodeResource(secondAdoption, http.StatusCreated)
	if err != nil {
		return fmt.Errorf(
			"a second tenant adopting the native identity %s: %w; the claim is scoped to the caller's tenant, so a "+
				"refusal here reports what another tenant manages to a caller who cannot see it",
			tenantSharedNativeID, err,
		)
	}
	if secondHolder.Metadata.UID == firstHolder.Metadata.UID {
		return errors.New("two tenants adopting one native identity were answered one resource")
	}
	// And what the second tenant just took is a CLAIM, in its own plane.
	//
	// The leg above is satisfied by a host that enforces no claim at all outside
	// the first tenant — the permissive half this file already had to add once
	// for mutation (`each-tenant-mutates-its-own-plane`), and the same shape: a
	// boundary measured only from the first tenant's side is satisfied by a host
	// that simply does less for everybody else. Here that host would let one
	// backend object be adopted twice by every tenant but one, which is the
	// duplication decision 0011 exists to prevent, minus the only account it was
	// ever measured in.
	foreignRival := worker
	foreignRival.Name = tenantSharedNativeRivalName
	rivalAdoption, err := r.importResourceAs(r.alternateTenantToken, foreignRival, importOptions{
		NativeID: tenantSharedNativeID, IdempotencyKey: "key-tenant-shared-native-foreign-rival", Create: true,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(rivalAdoption, "import_conflict"); err != nil {
		return fmt.Errorf(
			"a second resource of the SECOND tenant adopting the native identity that tenant already manages as "+
				"%s: %w; the claim is one live resource per identity for the caller's tenant, and every tenant is "+
				"a caller",
			tenantSharedNativeForeignName, err,
		)
	}
	absent, err := r.readAs(r.alternateTenantToken, foreignRival)
	if err != nil {
		return err
	}
	if err := r.expectStableError(absent, "resource_not_found"); err != nil {
		return fmt.Errorf("the second tenant's refused adoption stored a resource: %w", err)
	}
	r.complete("resource-import-is-tenant-isolated")
	return nil
}

// comparableRefusal reduces one error envelope to what two refusals of the same
// class must agree on: the code, the retryable flag, and the message with the
// resource name it may echo replaced by a placeholder. The requestId is dropped,
// because it is per-request by contract.
func comparableRefusal(response wireResponse, name string) (string, error) {
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"requestId"`
			Retryable bool   `json:"retryable"`
			HostCode  string `json:"hostCode,omitempty"`
		} `json:"error"`
	}
	if err := decodeStrictResponse(response, &envelope); err != nil {
		return "", err
	}
	message := strings.ReplaceAll(envelope.Error.Message, name, "{name}")
	return fmt.Sprintf(
		"%d %s retryable=%t hostCode=%q %s",
		response.Status, envelope.Error.Code, envelope.Error.Retryable, envelope.Error.HostCode, message,
	), nil
}

// comparableCreation is comparableRefusal for the one absent-equivalent answer
// that is a SUCCESS: it reduces a 201 to its status, its ETag, and its exact
// served bytes with the two values that legitimately differ between two
// creations — the host-issued uid and the resource name — replaced by
// placeholders. A rendered status may quote either (a Module Worker's
// hostReason names both), so both are normalized everywhere they appear rather
// than only in `metadata`.
func comparableCreation(response wireResponse, name, uid string) (string, error) {
	if uid == "" {
		return "", errors.New("a creation with no host-issued uid cannot be compared")
	}
	body := strings.ReplaceAll(string(response.Body), uid, "{uid}")
	body = strings.ReplaceAll(body, name, "{name}")
	// A real host stamps each condition's lastTransitionTime from its own
	// clock, so two indistinguishable creations legitimately differ there the
	// way they differ in uid and name. The reference host pins a fixed
	// timestamp, which HID this from the self-test: comparing the raw member
	// made the check fail spuriously against every real deployment while
	// proving nothing extra against the fake. The value's grammar is still
	// validated where conditions are decoded; equality of the MOMENT is not
	// part of what this check claims.
	for _, match := range transitionTimePattern.FindAllStringSubmatch(body, -1) {
		if _, parseErr := time.Parse(time.RFC3339, match[1]); parseErr != nil {
			return "", fmt.Errorf("condition lastTransitionTime %q is not RFC 3339; normalization erases only the moment, never the grammar", match[1])
		}
	}
	body = transitionTimePattern.ReplaceAllString(body, `"lastTransitionTime":"{time}"`)
	etag := strings.ReplaceAll(response.Header.Get("ETag"), uid, "{uid}")
	return fmt.Sprintf("%d etag=%s %s", response.Status, etag, strings.TrimSpace(body)), nil
}

// transitionTimePattern matches the serialized condition member wherever it
// appears in a served body, tolerating JSON's optional whitespace after the
// colon.
var transitionTimePattern = regexp.MustCompile(`"lastTransitionTime"\s*:\s*"([^"]*)"`)

// checkResourceUpdateIsTenantIsolated proves a foreign tenant cannot move
// another tenant's desired state, on both surfaces an update passes through: the
// prepare that binds the fence, and the fenced apply itself.
//
// Both must refuse, and refuse as ABSENT. The prepare is checked separately
// because a host that scoped only its apply would still mint a review binding
// another tenant's live uid and generation — a document that says, to whoever
// holds it, that a resource of that name exists and is at generation 1.
//
// And the holder updates it AFTERWARDS, which is the half a refusal cannot
// state. Proving only that the foreign update did not land is satisfied by a
// host that quarantines the resource the moment a stranger touches it — refuse
// the foreigner, then refuse everyone, and nothing was destroyed. The lane's
// `observe` and `import` boundaries are already two-sided for this reason; this
// one is now too. The holder's apply is fenced at the same generation the
// foreign one presented, and is spec-identical, which is the update surface a
// Form declaring no in-place spec change still accepts
// (decision 0015 rule 9) — so what separates the two requests is the credential
// and nothing else.
func (r *v3Runner) checkResourceUpdateIsTenantIsolated(worker probeTarget) error {
	private := worker
	private.Name = tenantPrivateName
	held, err := r.readAs(r.token, private)
	if err != nil {
		return err
	}
	holder, err := decodeResource(held, http.StatusOK)
	if err != nil {
		return fmt.Errorf("the holding tenant reading before the foreign update: %w", err)
	}
	before := holder.Metadata.UID

	fenced, err := r.prepareRequestAs(
		r.alternateTenantToken, private, map[string]string{expectedGenerationHeader: "1"},
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(fenced, "resource_not_found"); err != nil {
		return fmt.Errorf("another tenant preparing an update fence against %s: %w", private.Name, err)
	}

	// The apply is driven under a review the foreign tenant can legitimately
	// mint — its own create-intent binding — so what refuses it is the update
	// fence finding nothing, and not the review.
	own, err := r.prepareWithFenceAs(r.alternateTenantToken, private, "")
	if err != nil {
		return err
	}
	update, err := r.applyAs(r.alternateTenantToken, private, applyOptions{
		ExpectedGeneration: "1",
		IdempotencyKey:     "key-tenant-foreign-update",
		PrepareDigest:      own.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(update, "resource_not_found"); err != nil {
		return fmt.Errorf("another tenant updating %s: %w", private.Name, err)
	}

	after, err := r.readUIDAs(r.token, private, "the holding tenant reading after the foreign update")
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("a foreign update replaced the holder's incarnation %s with %s", before, after)
	}

	// The permissive half: the SAME surface, the same fence, the same document,
	// the holder's own credential.
	ownUpdate, err := r.applyAs(r.token, private, applyOptions{
		ExpectedGeneration: holder.Metadata.Generation,
		IdempotencyKey:     "key-tenant-holder-update",
	})
	if err != nil {
		return err
	}
	reapplied, err := decodeResource(ownUpdate, http.StatusOK)
	if err != nil {
		return fmt.Errorf(
			"the holding tenant updating %s after a foreign attempt was refused: %w; a host that answers the "+
				"refusal by quarantining the resource passes the two legs above and has taken it from its owner",
			private.Name, err,
		)
	}
	if reapplied.Metadata.UID != before {
		return fmt.Errorf(
			"the holder's own update answered uid %s, want its own %s", reapplied.Metadata.UID, before,
		)
	}
	if reapplied.Metadata.Generation != holder.Metadata.Generation {
		return fmt.Errorf(
			"a spec-identical apply moved generation %s -> %s; no desired state changed",
			holder.Metadata.Generation, reapplied.Metadata.Generation,
		)
	}
	r.complete("resource-update-is-tenant-isolated")
	return nil
}

// checkResourceDeleteIsTenantIsolated proves the most damaging half: a foreign
// tenant cannot remove what it cannot read. Both of the holder's real fences are
// presented — the generation a delete requires and the revision it may carry —
// so nothing but the address stops it.
//
// Then the holder removes it, which is the half a refusal cannot state. "The
// resource survived a foreign delete" is also true of a host that answers a
// stranger's delete by locking the record — and an operator who cannot destroy
// what they created has lost the resource just as completely, only slower and
// with a bill attached. So the last exchange is the holder's own delete of the
// same name, under the same fences, succeeding.
func (r *v3Runner) checkResourceDeleteIsTenantIsolated(worker probeTarget) error {
	private := worker
	private.Name = tenantPrivateName
	held, err := r.readAs(r.token, private)
	if err != nil {
		return err
	}
	holder, err := decodeResource(held, http.StatusOK)
	if err != nil {
		return fmt.Errorf("the holding tenant reading before the foreign delete: %w", err)
	}

	foreign, err := r.requestWithToken(
		r.alternateTenantToken, http.MethodDelete,
		r.resourceURL(private.Ref, private.Name, "", r.exactQuery(private.Space, private.Ref)),
		map[string]string{
			expectedGenerationHeader: holder.Metadata.Generation,
			"If-Match":               `"` + holder.Metadata.Revision + `"`,
			"Idempotency-Key":        "key-tenant-foreign-delete",
		}, nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(foreign, "resource_not_found"); err != nil {
		return fmt.Errorf(
			"another tenant deleting %s at its exact generation %s and revision %s: %w",
			private.Name, holder.Metadata.Generation, holder.Metadata.Revision, err,
		)
	}

	survived, err := r.readAs(r.token, private)
	if err != nil {
		return err
	}
	still, err := decodeResource(survived, http.StatusOK)
	if err != nil {
		return fmt.Errorf("a foreign delete removed %s: %w", private.Name, err)
	}
	if still.Metadata.UID != holder.Metadata.UID || still.Metadata.Revision != holder.Metadata.Revision {
		return fmt.Errorf(
			"a foreign delete moved %s from uid %s revision %s to uid %s revision %s",
			private.Name, holder.Metadata.UID, holder.Metadata.Revision,
			still.Metadata.UID, still.Metadata.Revision,
		)
	}

	// The permissive half: the same request, the same fences, the holder's own
	// credential.
	own, err := r.requestWithToken(
		r.token, http.MethodDelete,
		r.resourceURL(private.Ref, private.Name, "", r.exactQuery(private.Space, private.Ref)),
		map[string]string{
			expectedGenerationHeader: still.Metadata.Generation,
			"If-Match":               `"` + still.Metadata.Revision + `"`,
			"Idempotency-Key":        "key-tenant-holder-delete",
		}, nil,
	)
	if err != nil {
		return err
	}
	if own.Status != http.StatusNoContent {
		return fmt.Errorf(
			"the holding tenant deleting %s after a foreign attempt was refused: HTTP %d, want 204; body=%s; "+
				"a host that quarantines a resource a stranger reached for passes the legs above and has taken it "+
				"from its owner",
			private.Name, own.Status, strings.TrimSpace(string(own.Body)),
		)
	}
	if err := r.expectResourceAbsent(private); err != nil {
		return fmt.Errorf("the holder's own delete answered 204 and removed nothing: %w", err)
	}
	r.complete("resource-delete-is-tenant-isolated")
	return nil
}

// checkRelationResolutionIsTenantScoped proves a reference resolves inside the
// referring resource's own tenant, when the name matches another tenant's
// resource EXACTLY — and that what the host STORED is the uid it resolved.
//
// A reference is `{apiVersion, kind, name}` and carries no tenant, so a host that
// resolved it against a store keyed without one would bind a stranger's desired
// state to this tenant's resource and pin its uid — the substitution decision
// 0015 closes for incarnations, committed across a security boundary instead of a
// naming one.
//
// The check used to stop at a refusal that MOVED: absent target first, attachment
// gate second, which says the name resolved to something. It never reached a
// successful apply, so no relation was ever stored and no pin was ever inspected
// — and a host that resolved correctly at the gate and then pinned the OTHER
// tenant's uid passed it. Pinning is the half that matters, because the pin is
// what deletion protection, drift, and every aggregate rule are computed from
// afterwards: a foreign pin means one tenant's resource is protected from its own
// owner by a stranger's reference, and the stranger's source silently follows a
// resource its author never named.
//
// So the source here is a `WorkerVersion`, which references a worker and a bundle
// and passes through no attachment gate, and the second tenant's apply SUCCEEDS.
// The pin is then read the only way a black box can read it — by what it protects:
//
//   - the first tenant deletes its identically-named worker and MUST succeed. A
//     host that pinned that uid refuses with `dependency_in_use`, which is the
//     foreign pin saying so out loud.
//   - the second tenant's source is still Ready afterwards. A host that pinned the
//     removed uid reports `DependencyMissing` or `ExternalChange` instead.
//   - the second tenant deletes its OWN worker and MUST be refused
//     `dependency_in_use`. Its version is that worker's only holder, so nothing
//     else can be producing that answer, and a host that pinned nothing at all
//     removes it.
func (r *v3Runner) checkRelationResolutionIsTenantScoped(worker probeTarget) error {
	if r.artifactManifestDigest == "" {
		return errors.New(
			"the second tenant's bundle needs the committed manifest digest; this check runs after the artifact sequence",
		)
	}
	holder := worker
	holder.Name = tenantRelationWorker
	created, err := r.applyAs(r.token, holder, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-relation-worker",
	})
	if err != nil {
		return err
	}
	holderCreated, err := decodeResource(created, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the first tenant's relation target: %w", err)
	}

	// The second tenant's own bundle, so the only relation left unresolved below
	// is the worker. Its manifest is one that tenant committed for itself: a
	// content address is not a capability, so the first tenant's would not resolve.
	bundle := r.target(r.contract.RunnerInput.WorkerBundle.ResourceProbe)
	bundle.Name = tenantRelationBundle
	bundle.Spec = map[string]any{"manifestDigest": r.artifactManifestDigest}
	bundleCreated, err := r.applyAs(r.alternateTenantToken, bundle, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-relation-bundle",
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(bundleCreated, http.StatusCreated); err != nil {
		return fmt.Errorf("the second tenant's own bundle: %w", err)
	}
	source := r.workerVersionOfBundle(tenantRelationSource, tenantRelationWorker, tenantRelationBundle, fetchHandler)

	absent, err := r.applyAs(r.alternateTenantToken, source, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-relation-foreign",
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(absent, "resource_not_found"); err != nil {
		return fmt.Errorf(
			"a reference from another tenant to %s %s: %w; the name matches exactly and the resource is not that "+
				"tenant's to bind",
			holder.Ref.Kind, tenantRelationWorker, err,
		)
	}
	if !bytes.Contains(absent.Body, []byte("/worker")) {
		return fmt.Errorf(
			"the refusal does not name the unresolved relation pointer: %s",
			strings.TrimSpace(string(absent.Body)),
		)
	}

	// The second tenant supplies its own target under the same name.
	own, err := r.applyAs(r.alternateTenantToken, holder, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-relation-worker-alternate",
	})
	if err != nil {
		return err
	}
	ownCreated, err := decodeResource(own, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the second tenant's own relation target: %w", err)
	}
	if ownCreated.Metadata.UID == holderCreated.Metadata.UID {
		return errors.New("the two tenants' relation targets share a uid")
	}

	resolved, err := r.applyAs(r.alternateTenantToken, source, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-relation-resolved",
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(resolved, http.StatusCreated); err != nil {
		return fmt.Errorf(
			"the same reference against the second tenant's OWN %s: %w; if it still reports an absent target the "+
				"relation resolves against nothing, and the first refusal proved no boundary",
			holder.Ref.Kind, err,
		)
	}

	// What was pinned. The first tenant's worker is nobody's target.
	if err := r.deleteExisting(holder, "key-tenant-relation-worker-delete"); err != nil {
		return fmt.Errorf(
			"deleting the FIRST tenant's %s %s while only the second tenant's source references that name: %w; "+
				"a `dependency_in_use` here is a foreign pin — one tenant's resource held hostage by a reference "+
				"it never appeared in",
			holder.Ref.Kind, tenantRelationWorker, err,
		)
	}
	stillBound, err := r.readAs(r.alternateTenantToken, source)
	if err != nil {
		return err
	}
	sourceAfter, err := decodeResource(stillBound, http.StatusOK)
	if err != nil {
		return fmt.Errorf("the second tenant reading its source after the other tenant released the name: %w", err)
	}
	if condition := readyCondition(sourceAfter); condition.Status != "True" {
		return fmt.Errorf(
			"removing the FIRST tenant's %s reported %s/%s on the second tenant's source; it is pinned to a uid "+
				"in another tenant",
			holder.Ref.Kind, condition.Status, condition.Reason,
		)
	}

	// And the second tenant's own worker is protected, which is the same fact
	// from the other side.
	ownWorker, err := r.readAs(r.alternateTenantToken, holder)
	if err != nil {
		return err
	}
	ownWorkerRead, err := decodeResource(ownWorker, http.StatusOK)
	if err != nil {
		return fmt.Errorf("the second tenant reading its own relation target: %w", err)
	}
	protected, err := r.requestWithToken(
		r.alternateTenantToken, http.MethodDelete,
		r.resourceURL(holder.Ref, holder.Name, "", r.exactQuery(holder.Space, holder.Ref)),
		map[string]string{
			expectedGenerationHeader: ownWorkerRead.Metadata.Generation,
			"Idempotency-Key":        "key-tenant-relation-worker-alternate-delete",
		}, nil,
	)
	if err != nil {
		return err
	}
	if err := r.expectStableError(protected, "dependency_in_use"); err != nil {
		return fmt.Errorf(
			"deleting the SECOND tenant's own %s while its own source references it: %w; its source is that "+
				"worker's only holder, so a host that permits this stored no pin at all",
			holder.Ref.Kind, err,
		)
	}

	for _, target := range []probeTarget{source, holder, bundle} {
		if err := r.deleteExistingAs(r.alternateTenantToken, target, "key-"+target.Name+"-tenant-delete"); err != nil {
			return err
		}
	}
	r.complete("relation-resolution-is-tenant-scoped")
	return nil
}

// checkPrepareIsTenantScoped proves a prepare binding is minted for one tenant
// and spendable by that tenant alone.
//
// A prepareDigest is a short-lived permission to mutate one exact resource, and
// a CREATE binding pins no uid and no generation — there is nothing in it to
// distinguish two tenants' bindings for one name in one space unless the tenant
// is in it. A host that left it out issues one digest that any caller holding it
// may spend, which turns a review — a value a client logs, prints in a plan, and
// passes between processes — into a bearer token over another tenant's plane.
//
// The refusal is `invalid_argument` (400) rather than 404: the resource the
// caller is addressing is its OWN, the request is well formed, and what is untrue
// is the review it presents about it. Nothing here is a statement about whether
// any other tenant's resource exists, so nothing here discloses one.
//
// The boundary is the TENANT, and the last leg is what says so. A review bound to
// the minting PRINCIPAL instead refuses the foreign tenant identically and passes
// every other leg here — while breaking the ordinary split decision 0018 assumes,
// where one identity plans and another applies. So a second principal of the
// MINTING tenant spends the same digest and is accepted: enforced tighter than
// written is still a divergence between two hosts, and this is the leg that
// catches it.
func (r *v3Runner) checkPrepareIsTenantScoped(worker probeTarget) error {
	subject := worker
	subject.Name = tenantPrepareName

	minted, err := r.prepareWithFenceAs(r.token, subject, "")
	if err != nil {
		return err
	}
	spent, err := r.applyAs(r.alternateTenantToken, subject, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-prepare-borrowed",
		PrepareDigest: minted.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if err := r.expectStableError(spent, "invalid_argument"); err != nil {
		return fmt.Errorf(
			"another tenant spending a review minted at %s: %w", minted.PrepareDigest, err,
		)
	}

	// Its own review, the same document, the same name: accepted. The refusal
	// above was the tenant and not the request.
	own, err := r.applyAs(r.alternateTenantToken, subject, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-prepare-own",
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(own, http.StatusCreated); err != nil {
		return fmt.Errorf("the same create under the second tenant's own review: %w", err)
	}
	// And the mint did not create anything in the tenant that minted it.
	if err := r.expectResourceAbsent(subject); err != nil {
		return fmt.Errorf("a prepare mutated the minting tenant's plane: %w", err)
	}

	// The same review, the same document, a second principal of the tenant that
	// minted it.
	shared, err := r.applyAs(r.alternateToken, subject, applyOptions{
		Create: true, IdempotencyKey: "key-tenant-prepare-shared",
		PrepareDigest: minted.PrepareDigest,
	})
	if err != nil {
		return err
	}
	if _, err := decodeResource(shared, http.StatusCreated); err != nil {
		return fmt.Errorf(
			"a second principal of the MINTING tenant spending the review minted at %s: %w; a review binds the "+
				"tenant, so a host binding it to the principal refuses the ordinary plan-here-apply-there split "+
				"and is indistinguishable from a correct one against the foreign leg above",
			minted.PrepareDigest, err,
		)
	}
	if err := r.deleteExisting(subject, "key-tenant-prepare-shared-delete"); err != nil {
		return err
	}
	if err := r.deleteExistingAs(r.alternateTenantToken, subject, "key-tenant-prepare-own-delete"); err != nil {
		return err
	}
	r.complete("prepare-is-tenant-scoped")
	return nil
}

// checkIdempotencyIsTenantScoped proves an Idempotency-Key names one operation
// of one tenant.
//
// Replay is a promise that repeating a request repeats its EFFECT, and the
// promise is made to the caller that made the request. A host whose replay record
// stopped at the key would answer a second tenant with the first tenant's
// recorded 201 — a response carrying another tenant's uid, generation, and ETag —
// and would do it while performing no mutation at all, so the second tenant would
// hold state for a resource it does not have.
//
// One key, two tenants, four assertions: two independent creates with two uids,
// the second tenant's resource actually THERE, a real replay for that tenant's
// own repeat, and the first tenant's resource unmoved.
//
// The read-back is not a formality. Without it the check asks only that the
// second tenant's 201 differ from the first tenant's, and a host that answers a
// key it has already seen from another tenant by minting a fresh-looking
// document — new uid, generation 1, a plausible ETag — and storing nothing
// satisfies every remaining assertion, including the byte-identical "replay",
// which is just the same synthesis twice. The client is told it owns a resource
// that does not exist, which is the same non-convergence a stale replay record
// produces, arrived at from the other end.
func (r *v3Runner) checkIdempotencyIsTenantScoped(worker probeTarget) error {
	const key = "key-tenant-idempotency-shared"
	subject := worker
	subject.Name = tenantIdempotencyName

	primary, err := r.applyAs(r.token, subject, applyOptions{
		Create: true, IdempotencyKey: key,
	})
	if err != nil {
		return err
	}
	primaryCreated, err := decodeResource(primary, http.StatusCreated)
	if err != nil {
		return fmt.Errorf("the first tenant's create: %w", err)
	}

	// The second tenant's own review, so the only thing shared with the request
	// above is the key.
	own, err := r.prepareWithFenceAs(r.alternateTenantToken, subject, "")
	if err != nil {
		return err
	}
	fullURL, headers, body, err := r.applyRequestParts(subject, applyOptions{
		Create: true, IdempotencyKey: key, PrepareDigest: own.PrepareDigest,
	})
	if err != nil {
		return err
	}
	alternate, err := r.requestWithToken(r.alternateTenantToken, http.MethodPut, fullURL, headers, body)
	if err != nil {
		return err
	}
	alternateCreated, err := decodeResource(alternate, http.StatusCreated)
	if err != nil {
		return fmt.Errorf(
			"the same Idempotency-Key from a second tenant: %w; it is a second operation, not a replay", err,
		)
	}
	if alternateCreated.Metadata.UID == primaryCreated.Metadata.UID {
		return fmt.Errorf(
			"the second tenant was answered the first tenant's record at uid %s", primaryCreated.Metadata.UID,
		)
	}
	// It EXECUTED. A 201 is a claim about the store, and this is the request that
	// would be answered from a record rather than performed.
	alternateStored, err := r.readUIDAs(
		r.alternateTenantToken, subject, "the second tenant reading what its own key created",
	)
	if err != nil {
		return err
	}
	if alternateStored != alternateCreated.Metadata.UID {
		return fmt.Errorf(
			"the second tenant's create answered uid %s and its plane holds %s; the key was answered rather "+
				"than executed",
			alternateCreated.Metadata.UID, alternateStored,
		)
	}

	// The second tenant DOES have a replay record of its own.
	replay, err := r.requestWithToken(r.alternateTenantToken, http.MethodPut, fullURL, headers, body)
	if err != nil {
		return err
	}
	if replay.Status != alternate.Status || !bytes.Equal(replay.Body, alternate.Body) ||
		replay.Header.Get("ETag") != alternate.Header.Get("ETag") {
		return errors.New("the second tenant's own repeat of its key was not a byte-identical replay")
	}

	unmoved, err := r.readUIDAs(r.token, subject, "the first tenant reading after the second tenant's key reuse")
	if err != nil {
		return err
	}
	if unmoved != primaryCreated.Metadata.UID {
		return fmt.Errorf(
			"the second tenant's use of the same key moved the first tenant's resource from %s to %s",
			primaryCreated.Metadata.UID, unmoved,
		)
	}
	r.complete("idempotency-is-tenant-scoped")
	return nil
}
