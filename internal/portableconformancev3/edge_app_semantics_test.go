package portableconformancev3

import (
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func edgeAppReferenceHost(t *testing.T) (*ReferenceHost, *Catalog) {
	t.Helper()
	contract, err := Verify(corpusRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCatalog(filepath.Join("..", ".."), contract)
	if err != nil {
		t.Fatal(err)
	}
	return NewReferenceHost(contract, catalog), catalog
}

func edgeAppForm(t *testing.T, catalog *Catalog, kind string) *InstalledForm {
	t.Helper()
	// The definition version is the Form's own: members whose contract
	// diverged from the generation line do not sit at it (decision 0046), so
	// this resolves by kind and takes whatever single line the catalog
	// installs for it.
	form := catalog.onlyLine(edgeFormsGroup, kind)
	if form == nil {
		t.Fatalf("%s candidate is not installed", kind)
	}
	return form
}

func commitFileManifest(
	t *testing.T, host *ReferenceHost, kind string, files []map[string]string,
) (string, map[string]any) {
	t.Helper()
	entries := make([]any, 0, len(files))
	for _, file := range files {
		body := file["body"]
		digest := formpackage.DigestBytes([]byte(body))
		entries = append(entries, map[string]any{
			"path": file["path"], "mediaType": file["mediaType"], "size": len(body), "digest": digest,
		})
		host.blobs[digest] = []byte(body)
		host.grantBlob(referencePrimaryAuth.Tenant, digest)
	}
	manifest := map[string]any{"apiVersion": artifactAPIVersion, "kind": kind, "files": entries}
	digest, raw := canonicalManifestDigest(t, manifest)
	host.manifests[digest] = raw
	host.grantManifest(referencePrimaryAuth.Tenant, digest)
	return digest, manifest
}

func storeEdgeAppResource(host *ReferenceHost, form *InstalledForm, name, uid string, spec map[string]any) *storedResource {
	resource := &storedResource{
		Ref: form.Ref, Name: name, Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: uid, Generation: 1, Revision: 1, Spec: spec,
	}
	host.storeResource(resource)
	return resource
}

func TestEdgeAppArtifactFormsRequireTheirExactManifestKinds(t *testing.T) {
	host, catalog := edgeAppReferenceHost(t)
	staticForm := edgeAppForm(t, catalog, staticAssetBundleKind)
	migrationForm := edgeAppForm(t, catalog, sqliteMigrationSetKind)
	scope := referencePrimaryAuth.scope("conformance")

	staticDigest, _ := commitFileManifest(t, host, staticAssetBundleKind, []map[string]string{{
		"path": "index.html", "mediaType": "text/html", "body": "<!doctype html>\n",
	}})
	migrationDigest, _ := commitFileManifest(t, host, migrationBundleKind, []map[string]string{{
		"path": "0001.sql", "mediaType": "application/sql", "body": "CREATE TABLE messages(id TEXT);\n",
	}})
	nonSQLDigest, _ := commitFileManifest(t, host, migrationBundleKind, []map[string]string{{
		"path": "0001.sql", "mediaType": "text/plain", "body": "SELECT 1;\n",
	}})

	cases := []struct {
		name   string
		form   *InstalledForm
		digest string
		code   string
	}{
		{"static accepts StaticAssetBundle", staticForm, staticDigest, ""},
		{"static rejects MigrationBundle", staticForm, migrationDigest, "artifact_invalid"},
		{"migration accepts SQL MigrationBundle", migrationForm, migrationDigest, ""},
		{"migration rejects StaticAssetBundle", migrationForm, staticDigest, "artifact_invalid"},
		{"migration rejects non-SQL file", migrationForm, nonSQLDigest, "artifact_invalid"},
	}
	for _, test := range cases {
		_, hostErr := host.validateDesiredSemantics(
			referencePrimaryAuth, test.form, scope, "artifact-probe", map[string]any{"manifestDigest": test.digest},
		)
		if test.code == "" && hostErr != nil {
			t.Errorf("%s: %+v", test.name, hostErr)
		}
		if test.code != "" && (hostErr == nil || hostErr.Code != test.code) {
			t.Errorf("%s: %+v, want %s", test.name, hostErr, test.code)
		}
	}
}

func TestWorkerVersionSPAFallbackRequiresIndexHTML(t *testing.T) {
	host, catalog := edgeAppReferenceHost(t)
	versionForm := edgeAppForm(t, catalog, workerVersionKind)
	assetForm := edgeAppForm(t, catalog, staticAssetBundleKind)
	scope := referencePrimaryAuth.scope("conformance")

	digest, _ := commitFileManifest(t, host, staticAssetBundleKind, []map[string]string{{
		"path": "app.js", "mediaType": "application/javascript", "body": "export {};\n",
	}})
	bundle := storeEdgeAppResource(host, assetForm, "assets", "uid-assets", map[string]any{"manifestDigest": digest})
	relations := []storedRelation{{
		Pointer: assetBundleRelationPointer, TargetAPIVersion: assetForm.Ref.APIVersion,
		TargetKind: assetForm.Ref.Kind, TargetName: bundle.Name, TargetUID: bundle.UID, TargetRef: bundle.Ref,
	}}
	spec := map[string]any{"assets": map[string]any{"notFoundHandling": "single_page_application"}}
	if hostErr := host.validateWorkerVersionAssets(referencePrimaryAuth, versionForm, scope, spec, relations); hostErr == nil {
		t.Fatal("SPA fallback accepted a StaticAssetBundle without index.html")
	}

	digest, _ = commitFileManifest(t, host, staticAssetBundleKind, []map[string]string{{
		"path": "index.html", "mediaType": "text/html", "body": "<!doctype html>\n",
	}})
	bundle.Spec = map[string]any{"manifestDigest": digest}
	if hostErr := host.validateWorkerVersionAssets(referencePrimaryAuth, versionForm, scope, spec, relations); hostErr != nil {
		t.Fatalf("SPA fallback with index.html: %+v", hostErr)
	}
}

func TestSQLiteMigrationHistoryIsExactPrefixAndDeleteNeverRollsBack(t *testing.T) {
	host, catalog := edgeAppReferenceHost(t)
	databaseForm := edgeAppForm(t, catalog, "SQLiteDatabase")
	setForm := edgeAppForm(t, catalog, sqliteMigrationSetKind)
	applicationForm := edgeAppForm(t, catalog, sqliteMigrationApplicationKind)
	scope := referencePrimaryAuth.scope("conformance")
	database := storeEdgeAppResource(host, databaseForm, "database", "uid-database", map[string]any{})

	digest, _ := commitFileManifest(t, host, migrationBundleKind, []map[string]string{
		{"path": "0001.sql", "mediaType": "application/sql", "body": "CREATE TABLE messages(id TEXT);\n"},
		{"path": "0002.sql", "mediaType": "application/sql", "body": "ALTER TABLE messages ADD COLUMN body TEXT;\n"},
	})
	set := storeEdgeAppResource(host, setForm, "set", "uid-set", map[string]any{"manifestDigest": digest})
	relations := []storedRelation{
		{Pointer: migrationDatabasePointer, TargetAPIVersion: databaseForm.Ref.APIVersion, TargetKind: databaseForm.Ref.Kind, TargetName: database.Name, TargetUID: database.UID, TargetRef: database.Ref},
		{Pointer: migrationSetRelationPointer, TargetAPIVersion: setForm.Ref.APIVersion, TargetKind: setForm.Ref.Kind, TargetName: set.Name, TargetUID: set.UID, TargetRef: set.Ref},
	}
	if hostErr := host.applySQLiteMigrationSuffix(referencePrimaryAuth, applicationForm, scope, relations); hostErr != nil {
		t.Fatalf("initial migration: %+v", hostErr)
	}
	want := append([]migrationLedgerEntry(nil), host.migrationLedgers[database.UID]...)
	if len(want) != 2 {
		t.Fatalf("ledger = %+v, want two ordered entries", want)
	}
	application := &storedResource{
		Ref: applicationForm.Ref, Name: "application", Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-application", Generation: 1, Revision: 1, Spec: map[string]any{}, Relations: relations,
	}
	host.storeResource(application)
	if condition := hMigrationReadyCondition(host, application); condition["status"] != "True" {
		t.Fatalf("initial application Ready = %+v, want True", condition)
	}
	if hostErr := host.applySQLiteMigrationSuffix(referencePrimaryAuth, applicationForm, scope, relations); hostErr != nil {
		t.Fatalf("idempotent re-application: %+v", hostErr)
	}
	if got := host.migrationLedgers[database.UID]; len(got) != len(want) {
		t.Fatalf("re-application duplicated history: %+v", got)
	}

	rewritten, _ := commitFileManifest(t, host, migrationBundleKind, []map[string]string{
		{"path": "0001.sql", "mediaType": "application/sql", "body": "CREATE TABLE rewritten(id TEXT);\n"},
		{"path": "0002.sql", "mediaType": "application/sql", "body": "ALTER TABLE rewritten ADD COLUMN body TEXT;\n"},
	})
	set.Spec = map[string]any{"manifestDigest": rewritten}
	if hostErr := host.validateSQLiteMigrationApplication(referencePrimaryAuth, applicationForm, scope, relations); hostErr == nil || hostErr.Code != "migration_required" {
		t.Fatalf("rewritten history = %+v, want migration_required", hostErr)
	}
	if got := host.migrationLedgers[database.UID]; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("refusal changed ledger: %+v", got)
	}

	delete(host.resources, application.key())
	if got := host.migrationLedgers[database.UID]; len(got) != len(want) {
		t.Fatalf("deleting attachment rolled back database history: %+v", got)
	}
}

func hMigrationReadyCondition(host *ReferenceHost, application *storedResource) map[string]any {
	conditions := host.derivedConditions(application)
	return conditions[0]
}

func TestSQLiteMigrationApplicationReadinessTracksExactLedgerSet(t *testing.T) {
	host, catalog := edgeAppReferenceHost(t)
	databaseForm := edgeAppForm(t, catalog, "SQLiteDatabase")
	setForm := edgeAppForm(t, catalog, sqliteMigrationSetKind)
	applicationForm := edgeAppForm(t, catalog, sqliteMigrationApplicationKind)
	scope := referencePrimaryAuth.scope("conformance")
	database := storeEdgeAppResource(host, databaseForm, "readiness-database", "uid-readiness-database", map[string]any{})
	digest1, _ := commitFileManifest(t, host, migrationBundleKind, []map[string]string{{
		"path": "0001.sql", "mediaType": "application/sql", "body": "CREATE TABLE readiness(id TEXT);\n",
	}})
	set1 := storeEdgeAppResource(host, setForm, "readiness-set-1", "uid-readiness-set-1", map[string]any{"manifestDigest": digest1})
	relations1 := []storedRelation{
		{Pointer: migrationDatabasePointer, TargetAPIVersion: databaseForm.Ref.APIVersion, TargetKind: databaseForm.Ref.Kind, TargetName: database.Name, TargetUID: database.UID, TargetRef: database.Ref},
		{Pointer: migrationSetRelationPointer, TargetAPIVersion: setForm.Ref.APIVersion, TargetKind: setForm.Ref.Kind, TargetName: set1.Name, TargetUID: set1.UID, TargetRef: set1.Ref},
	}
	if hostErr := host.applySQLiteMigrationSuffix(referencePrimaryAuth, applicationForm, scope, relations1); hostErr != nil {
		t.Fatalf("apply first migration: %+v", hostErr)
	}
	app1 := &storedResource{
		Ref: applicationForm.Ref, Name: "readiness-application-1", Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-readiness-application-1", Generation: 1, Revision: 1, Spec: map[string]any{}, Relations: relations1,
	}
	host.storeResource(app1)
	if condition := hMigrationReadyCondition(host, app1); condition["status"] != "True" || condition["reason"] != "Available" {
		t.Fatalf("first application condition = %+v, want Ready=True/Available", condition)
	}

	digest2, _ := commitFileManifest(t, host, migrationBundleKind, []map[string]string{
		{"path": "0001.sql", "mediaType": "application/sql", "body": "CREATE TABLE readiness(id TEXT);\n"},
		{"path": "0002.sql", "mediaType": "application/sql", "body": "ALTER TABLE readiness ADD COLUMN body TEXT;\n"},
	})
	set2 := storeEdgeAppResource(host, setForm, "readiness-set-2", "uid-readiness-set-2", map[string]any{"manifestDigest": digest2})
	relations2 := []storedRelation{
		{Pointer: migrationDatabasePointer, TargetAPIVersion: databaseForm.Ref.APIVersion, TargetKind: databaseForm.Ref.Kind, TargetName: database.Name, TargetUID: database.UID, TargetRef: database.Ref},
		{Pointer: migrationSetRelationPointer, TargetAPIVersion: setForm.Ref.APIVersion, TargetKind: setForm.Ref.Kind, TargetName: set2.Name, TargetUID: set2.UID, TargetRef: set2.Ref},
	}
	if hostErr := host.applySQLiteMigrationSuffix(referencePrimaryAuth, applicationForm, scope, relations2); hostErr != nil {
		t.Fatalf("apply superset migration: %+v", hostErr)
	}
	app2 := &storedResource{
		Ref: applicationForm.Ref, Name: "readiness-application-2", Tenant: referencePrimaryAuth.Tenant, Space: "conformance",
		UID: "uid-readiness-application-2", Generation: 1, Revision: 1, Spec: map[string]any{}, Relations: relations2,
	}
	host.storeResource(app2)
	if condition := hMigrationReadyCondition(host, app2); condition["status"] != "True" || condition["reason"] != "Available" {
		t.Fatalf("second application condition = %+v, want Ready=True/Available", condition)
	}
	if condition := hMigrationReadyCondition(host, app1); condition["status"] != "False" || condition["reason"] != "Reconciling" {
		t.Fatalf("older application condition = %+v, want Ready=False/Reconciling", condition)
	}
	if host.resources[app1.key()] == nil || host.resources[app2.key()] == nil {
		t.Fatal("a later migration application removed the older application resource")
	}
}
