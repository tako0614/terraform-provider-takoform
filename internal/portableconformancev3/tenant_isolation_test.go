package portableconformancev3

// tenant_isolation_test.go holds the reference host to the parts of
// spec/decisions/0028 the black-box corpus cannot reach.
//
// The corpus drives nine required checks over real HTTP with two credentials,
// and those cover everything a third-party host can be HELD to: two tenants
// naming one resource, reads, observes, updates, imports, deletes, relation
// resolution, prepare bindings, and idempotency. The nine are enumerated by
// SURFACE, and the three enumeration tests at the end of this file are what
// keeps that true. What it cannot see is the SHAPE of the address —
// whether the tenant is in the key or in a filter someone remembered to write —
// and it cannot construct a cross-tenant rendering dependency at all, precisely
// because relation resolution is scoped. Those are host obligations, so they are
// proved here against the host itself.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// surfaceKey is one name-addressed surface reduced to what three independent
// records of it must agree on.
func surfaceKey(method, route string) string { return method + " " + route }

func declaredNameAddressedSurfaces() []string {
	keys := make([]string, 0, len(nameAddressedResourceSurfaces))
	for _, surface := range nameAddressedResourceSurfaces {
		keys = append(keys, surfaceKey(surface.Method, surface.Route))
	}
	sort.Strings(keys)
	return keys
}

// TestEveryNameAddressedSurfaceIsMeasured is the gate the tenant matrix was
// missing. The matrix was enumerated by intent, so `observe` and `import` — two
// surfaces that take a resource name — were simply never thought of. The
// enumeration is now by surface, and this holds it to that: a surface either
// names the required checks that measure its boundary, or states why it owes
// none.
func TestEveryNameAddressedSurfaceIsMeasured(t *testing.T) {
	required := map[string]bool{}
	for _, check := range requiredRunnerChecks {
		required[check] = true
	}
	seen := map[string]bool{}
	for _, surface := range nameAddressedResourceSurfaces {
		key := surfaceKey(surface.Method, surface.Route)
		if seen[key] {
			t.Fatalf("%s is enumerated twice", key)
		}
		seen[key] = true
		if !strings.HasPrefix(surface.Route, "{api}/resources") {
			t.Fatalf("%s is not a resource-plane route", key)
		}
		if len(surface.TenantChecks) == 0 {
			if strings.TrimSpace(surface.Unresolved) == "" {
				t.Fatalf(
					"%s takes a resource name, names no tenant check, and states no reason; a name-addressed "+
						"surface is measured against spec/decisions/0028 or says why it cannot be",
					key,
				)
			}
			continue
		}
		if surface.Unresolved != "" {
			t.Fatalf("%s both names tenant checks and claims to resolve nothing", key)
		}
		for _, check := range surface.TenantChecks {
			if !required[check] {
				t.Fatalf("%s names tenant check %q, which is not in requiredRunnerChecks", key, check)
			}
		}
	}
}

// TestEveryNameAddressedSurfaceIsRouted binds the enumeration to the routes the
// reference host actually serves. Adding a name-addressed endpoint means adding
// a resourcePlaneHandlers entry, and an entry with no enumerated surface fails
// here — so the endpoint cannot reach a client without passing through
// TestEveryNameAddressedSurfaceIsMeasured first.
func TestEveryNameAddressedSurfaceIsRouted(t *testing.T) {
	routed := make([]string, 0, len(resourcePlaneHandlers))
	for route, handler := range resourcePlaneHandlers {
		if handler == nil {
			t.Fatalf("%s %q routes to no handler", route.Method, route.Action)
		}
		routed = append(routed, route.Method+" "+route.Action)
	}
	sort.Strings(routed)

	declared := make([]string, 0, len(nameAddressedResourceSurfaces))
	for _, surface := range nameAddressedResourceSurfaces {
		if surface.NameInBody {
			continue
		}
		declared = append(declared, surface.Method+" "+surface.Action)
	}
	sort.Strings(declared)

	if strings.Join(routed, ", ") != strings.Join(declared, ", ") {
		t.Fatalf(
			"the reference host routes [%s] under a resource name, and the enumeration declares [%s]; "+
				"every name-addressed route is enumerated and measured, or neither",
			strings.Join(routed, ", "), strings.Join(declared, ", "),
		)
	}
}

// TestEveryNameAddressedSurfaceIsPublished binds the enumeration to the
// PUBLISHED contract, which is what a third-party host implements. A surface
// that appears in the Lifecycle route block and nowhere else is one this
// repository is asking someone to build without measuring its tenant boundary.
func TestEveryNameAddressedSurfaceIsPublished(t *testing.T) {
	published, err := publishedResourceRoutes(filepath.Join("..", "..", "spec", "host-api", "v1alpha3.md"))
	if err != nil {
		t.Fatal(err)
	}
	declared := declaredNameAddressedSurfaces()
	if strings.Join(published, "\n") != strings.Join(declared, "\n") {
		t.Fatalf(
			"spec/host-api/v1alpha3.md publishes the resource routes\n  %s\nand the enumeration declares\n  %s",
			strings.Join(published, "\n  "), strings.Join(declared, "\n  "),
		)
	}
}

// publishedResourceRoutes reads the `{api}/resources` lines of the one fenced
// route block under the spec's Lifecycle heading.
func publishedResourceRoutes(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == "## Lifecycle" {
			start = index
			break
		}
	}
	if start < 0 {
		return nil, fmt.Errorf("%s states no Lifecycle section", path)
	}
	inBlock := false
	var routes []string
	for _, line := range lines[start:] {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inBlock {
				break
			}
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || !strings.HasPrefix(fields[1], "{api}/resources") {
			continue
		}
		routes = append(routes, surfaceKey(fields[0], fields[1]))
	}
	if len(routes) == 0 {
		return nil, fmt.Errorf("%s publishes no resource routes under Lifecycle", path)
	}
	sort.Strings(routes)
	return routes, nil
}

// TestResourceAddressCarriesTheTenant is the structural fact underneath every
// black-box check: the store key is tenant, space, apiVersion, kind, and name,
// in that order, and two scopes differing only in tenant are two addresses.
//
// A black-box runner can only observe the consequence — two records, two uids.
// This states the cause, so a host that reintroduced a tenant-blind key and
// papered over it with filters fails here as well as there.
func TestResourceAddressCarriesTheTenant(t *testing.T) {
	primary := referencePrimaryAuth.scope("conformance")
	foreign := referenceOtherTenantAuth.scope("conformance")

	key := resourceKey(primary, edgeFormsGroup, moduleWorkerKind, "api")
	members := strings.Split(key, "\x00")
	want := []string{
		referencePrimaryAuth.Tenant, "conformance", edgeFormsGroup, moduleWorkerKind, "api",
	}
	if len(members) != len(want) {
		t.Fatalf("resource address has %d members, want %d: %q", len(members), len(want), members)
	}
	for index, member := range want {
		if members[index] != member {
			t.Fatalf("address member %d = %q, want %q", index, members[index], member)
		}
	}
	// Two principals of one tenant address ONE resource; two tenants do not.
	if resourceKey(referenceAlternateAuth.scope("conformance"), edgeFormsGroup, moduleWorkerKind, "api") != key {
		t.Fatal("two principals of one tenant address two resources; the boundary is the tenant, not the credential")
	}
	if resourceKey(foreign, edgeFormsGroup, moduleWorkerKind, "api") == key {
		t.Fatal("two tenants naming one resource in one space address one record")
	}
	// And a resource's own key is derived from its own scope, so a record cannot
	// be stored under an address that is not its own.
	resource := &storedResource{
		Ref:  FormRef{APIVersion: edgeFormsGroup, Kind: moduleWorkerKind},
		Name: "api", Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
	}
	if resource.key() != key {
		t.Fatalf("stored resource key = %q, want %q", resource.key(), key)
	}
}

// TestScopedScansStopAtTheTenant proves the two store-wide passes obey the
// boundary: the scan that feeds every cross-resource rule, and the
// derived-rendering pass, which WRITES.
//
// The rendering pass is the one that cannot be measured black box. A mutation in
// one tenant recomputes the rendering of everything the mutation can affect and
// advances the revision of whatever changed; if that pass ran host-wide it would
// move another tenant's ETag, computed from that tenant's own relations and
// readiness. A runner cannot set that up, because it cannot make one tenant's
// resource render from another's — relation resolution refuses. So the stale
// rendering is planted here directly, which is the only way to ask the question.
func TestScopedScansStopAtTheTenant(t *testing.T) {
	host, contract := fallbackHost(t)
	space := contract.RunnerInput.Space
	ref := mustProbeRef(t, contract, "EdgeKVNamespace")

	store := func(tenant, name string, uid string) *storedResource {
		resource := &storedResource{
			Ref: ref, Name: name, Tenant: tenant, Space: space,
			UID: uid, Generation: 1, Revision: 1, Spec: map[string]any{},
		}
		host.storeResource(resource)
		return resource
	}
	mine := store(referencePrimaryAuth.Tenant, "mine", "uid-mine")
	sibling := store(referencePrimaryAuth.Tenant, "sibling", "uid-sibling")
	theirs := store(referenceOtherTenantAuth.Tenant, "mine", "uid-theirs")

	scope := referencePrimaryAuth.scope(space)
	seen := map[string]bool{}
	for _, candidate := range host.scopedResources(scope) {
		seen[candidate.UID] = true
	}
	if !seen["uid-mine"] || !seen["uid-sibling"] {
		t.Fatalf("the scoped view lost this tenant's own resources: %v", seen)
	}
	if seen["uid-theirs"] {
		t.Fatal("the scoped view of one tenant returned another tenant's resource")
	}

	// Both other records are left with a rendering that no longer matches what
	// the store renders, so a pass that reaches them WILL advance them.
	sibling.DerivedRendering = "stale"
	theirs.DerivedRendering = "stale"
	host.advanceDerivedRevisions(scope, mine.key())
	if sibling.Revision != 2 {
		t.Fatalf("the rendering pass left this tenant's sibling at revision %d, want 2", sibling.Revision)
	}
	if theirs.Revision != 1 || theirs.DerivedRendering != "stale" {
		t.Fatalf(
			"one tenant's mutation advanced another tenant's revision to %d; the pass writes, so it must not cross",
			theirs.Revision,
		)
	}
}

// TestRelationScopeSurvivesAnIdenticalNameInAnotherTenant proves the two facts
// the stored relation set decides, when a second tenant holds a resource of
// exactly the target's name: drift is answered from this tenant's plane, and
// deletion protection does not reach out of it.
//
// Both are consequences of the address rather than of a comparison, which is why
// they belong here: no request a runner can send would resolve to the foreign
// record in the first place, so the runner cannot tell a host that looks in the
// right place from one that would look in the wrong place if it could.
func TestRelationScopeSurvivesAnIdenticalNameInAnotherTenant(t *testing.T) {
	host, _, kv, version := relationFixture(t)

	// The other tenant holds an EdgeKVNamespace of the SAME name in the SAME
	// space. Nothing about the holder changes.
	foreign := &storedResource{
		Ref: kv.Ref, Name: kv.Name, Tenant: referenceOtherTenantAuth.Tenant, Space: kv.Space,
		UID: "uid-kv-foreign", Generation: 1, Revision: 1, Spec: map[string]any{},
	}
	host.storeResource(foreign)
	if _, _, drifted := host.relationDrift(version); drifted {
		t.Fatal("another tenant's identically-named resource changed what this tenant's relation resolves to")
	}
	if hostErr := host.dependencyInUse(foreign); hostErr != nil {
		t.Fatalf("one tenant's holder protected another tenant's resource: %+v", hostErr)
	}

	// Removing the foreign record is invisible to the holder; removing its own
	// target is not.
	host.removeResource(foreign.key())
	if _, _, drifted := host.relationDrift(version); drifted {
		t.Fatal("deleting another tenant's identically-named resource drifted this tenant's relation")
	}
	host.removeResource(kv.key())
	reason, _, drifted := host.relationDrift(version)
	if !drifted || reason != "DependencyMissing" {
		t.Fatalf("removing the real target reported %q/%v, want DependencyMissing", reason, drifted)
	}
}

// TestPrepareBindingIsMintedForOneTenant proves the binding payload carries the
// tenant, which is what makes a review unspendable outside the tenant that minted
// it. The corpus proves the REFUSAL over HTTP; this proves the mechanism, so a
// host that refused by some other means would still have to state which.
func TestPrepareBindingIsMintedForOneTenant(t *testing.T) {
	ref := FormRef{APIVersion: edgeFormsGroup, Kind: moduleWorkerKind, DefinitionVersion: "0.1.0"}
	specDigest, err := specCanonicalDigest(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	_, mine, err := prepareBindingPayload(
		specDigest, ref, "api", referencePrimaryAuth.scope("conformance"),
		prepareCreateUID, prepareCreateGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, sameTenant, err := prepareBindingPayload(
		specDigest, ref, "api", referenceAlternateAuth.scope("conformance"),
		prepareCreateUID, prepareCreateGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, theirs, err := prepareBindingPayload(
		specDigest, ref, "api", referenceOtherTenantAuth.scope("conformance"),
		prepareCreateUID, prepareCreateGeneration,
	)
	if err != nil {
		t.Fatal(err)
	}
	// A review is the tenant's, not the principal's: two principals of one tenant
	// plan the same mutation and mint the same binding.
	if mine != sameTenant {
		t.Fatal("two principals of one tenant minted two different reviews for one identical mutation")
	}
	if mine == theirs {
		t.Fatalf(
			"two tenants preparing one create in one space minted the same review %s; "+
				"a create binding pins no uid and no generation, so nothing else tells them apart",
			mine,
		)
	}
}
