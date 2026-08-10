package portableconformancev3

import "testing"

// mustProbeRef is probeFormRef for a test that cannot proceed without it.
func mustProbeRef(t *testing.T, contract Contract, kind string) FormRef {
	t.Helper()
	ref, known := probeFormRef(contract, kind)
	if !known {
		t.Fatalf("the corpus pins no probe of kind %s", kind)
	}
	return ref
}

// probeFormRef resolves the exact FormRef the corpus pins for one probe kind.
//
// Unit tests address a Form the way a client does — by its whole identity —
// because the catalog is keyed by that identity and holds more than one
// contract for at least one kind. A helper that resolved by kind would be the
// very substitution these tests exist to keep out of the host.
func probeFormRef(contract Contract, kind string) (FormRef, bool) {
	input := contract.RunnerInput
	for _, probe := range []ResourceProbe{
		input.ModuleWorker, input.EdgeKvNamespace, input.AtLeastOnceQueue,
		input.WorkerVersion, input.WorkerBundle.ResourceProbe, input.StaticAssetBundle,
		input.SQLiteDatabase, input.SQLiteMigrationSet, input.SQLiteMigrationApplication,
		input.WorkerDeployment, input.WorkerCustomDomain, input.WorkerCronTrigger, input.QueueConsumer,
	} {
		if probe.Identity.FormRef.Kind == kind {
			return probe.Identity.FormRef, true
		}
	}
	return FormRef{}, false
}

// probeForm resolves the installed Form of one kind at the exact identity the
// corpus pins for it, falling back to the catalog for the family kinds the
// corpus drives no probe of. The fallback refuses an ambiguous kind rather than
// picking one, so a test can never silently exercise the wrong contract.
func (h *ReferenceHost) probeForm(kind string) *InstalledForm {
	if ref, known := probeFormRef(h.contract, kind); known {
		return h.catalog.exact(ref)
	}
	var found *InstalledForm
	for _, form := range h.catalog.sortedForms() {
		if form.Ref.APIVersion != edgeFormsGroup || form.Ref.Kind != kind {
			continue
		}
		if found != nil {
			return nil
		}
		found = form
	}
	return found
}
