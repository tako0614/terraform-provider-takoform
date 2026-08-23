package portableconformancev3

import (
	"fmt"
	"net/http"

	"github.com/tako0614/terraform-provider-takoform/formpackage"
)

func (r *v3Runner) checkStaticAssetSPAJourney() error {
	input := r.contract.RunnerInput
	body := []byte("<!doctype html>\n")
	blobDigest := formpackage.DigestBytes(body)
	manifest := map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       staticAssetBundleKind,
		"files": []any{map[string]any{
			"path": "index.html", "mediaType": "text/html", "size": len(body), "digest": blobDigest,
		}},
	}
	static := r.target(input.StaticAssetBundle)
	wantDigest, err := formpackage.DigestCanonicalJSON(mustRunnerJSON(manifest))
	if err != nil {
		return err
	}
	if staticDigest, _ := static.Spec["manifestDigest"].(string); staticDigest != wantDigest {
		return fmt.Errorf("static asset probe digest = %s, want manifest digest %s", staticDigest, wantDigest)
	}
	committed, err := r.uploadAndCommitManifest(manifest, map[string][]byte{blobDigest: body}, "key-static-asset-spa")
	if err != nil {
		return err
	}
	if committed != wantDigest {
		return fmt.Errorf("static asset commit returned %s, want %s", committed, wantDigest)
	}
	if _, _, err := r.applyResource(static, applyOptions{
		Create: true, IdempotencyKey: "key-static-asset-resource",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("StaticAssetBundle apply: %w", err)
	}

	version := r.target(input.WorkerVersion)
	version.Name = "static-assets-spa-version"
	version.Spec = cloneJSONMap(version.Spec)
	version.Spec["assets"] = map[string]any{
		"bundle":           exactReference(static, static.Name),
		"runWorkerFirst":   false,
		"notFoundHandling": "single_page_application",
	}
	if _, _, err := r.applyResource(version, applyOptions{
		Create: true, IdempotencyKey: "key-static-assets-spa-version",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("WorkerVersion SPA attachment: %w", err)
	}
	versionCurrent, _, err := r.read(version)
	if err != nil {
		return err
	}
	if condition := readyCondition(versionCurrent); condition.Status != "True" || condition.Reason != "Available" {
		return fmt.Errorf("WorkerVersion SPA attachment Ready = %s/%s, want True/Available", condition.Status, condition.Reason)
	}
	if response, err := r.deleteResource(version, versionCurrent.Metadata.Generation, "key-static-assets-spa-version-delete", nil); err != nil {
		return err
	} else if response.Status != http.StatusNoContent {
		return fmt.Errorf("WorkerVersion SPA attachment delete HTTP %d", response.Status)
	}
	staticCurrent, _, err := r.read(static)
	if err != nil {
		return err
	}
	if response, err := r.deleteResource(static, staticCurrent.Metadata.Generation, "key-static-asset-resource-delete", nil); err != nil {
		return err
	} else if response.Status != http.StatusNoContent {
		return fmt.Errorf("StaticAssetBundle delete HTTP %d", response.Status)
	}
	r.complete("static-asset-spa-paths")
	return nil
}

func (r *v3Runner) checkSQLiteMigrationLedgerReadiness() error {
	input := r.contract.RunnerInput
	database := r.target(input.SQLiteDatabase)
	set := r.target(input.SQLiteMigrationSet)
	application := r.target(input.SQLiteMigrationApplication)
	firstSQL := []byte("CREATE TABLE probe(id TEXT);\n")
	firstDigest := formpackage.DigestBytes(firstSQL)
	firstManifest := map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       migrationBundleKind,
		"files": []any{map[string]any{
			"path": "0001.sql", "mediaType": "application/sql", "size": len(firstSQL), "digest": firstDigest,
		}},
	}
	wantFirst, err := formpackage.DigestCanonicalJSON(mustRunnerJSON(firstManifest))
	if err != nil {
		return err
	}
	if digest, _ := set.Spec["manifestDigest"].(string); digest != wantFirst {
		return fmt.Errorf("migration set probe digest = %s, want manifest digest %s", digest, wantFirst)
	}
	committed, err := r.uploadAndCommitManifest(firstManifest, map[string][]byte{firstDigest: firstSQL}, "key-migration-first")
	if err != nil {
		return err
	}
	if committed != wantFirst {
		return fmt.Errorf("first migration commit returned %s, want %s", committed, wantFirst)
	}
	if _, _, err := r.applyResource(database, applyOptions{
		Create: true, IdempotencyKey: "key-migration-database",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("SQLiteDatabase apply: %w", err)
	}
	if _, _, err := r.applyResource(set, applyOptions{
		Create: true, IdempotencyKey: "key-migration-set-first",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("SQLiteMigrationSet apply: %w", err)
	}
	if _, _, err := r.applyResource(application, applyOptions{
		Create: true, IdempotencyKey: "key-migration-application-first",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("SQLiteMigrationApplication first apply: %w", err)
	}
	first, _, err := r.read(application)
	if err != nil {
		return err
	}
	if condition := readyCondition(first); condition.Status != "True" || condition.Reason != "Available" {
		return fmt.Errorf("first migration application Ready = %s/%s, want True/Available", condition.Status, condition.Reason)
	}

	secondSQL := []byte("ALTER TABLE probe ADD COLUMN body TEXT;\n")
	secondDigest := formpackage.DigestBytes(secondSQL)
	supersetManifest := map[string]any{
		"apiVersion": artifactAPIVersion,
		"kind":       migrationBundleKind,
		"files": []any{
			map[string]any{"path": "0001.sql", "mediaType": "application/sql", "size": len(firstSQL), "digest": firstDigest},
			map[string]any{"path": "0002.sql", "mediaType": "application/sql", "size": len(secondSQL), "digest": secondDigest},
		},
	}
	supersetDigest, err := r.uploadAndCommitManifest(supersetManifest, map[string][]byte{
		firstDigest: firstSQL, secondDigest: secondSQL,
	}, "key-migration-superset")
	if err != nil {
		return err
	}
	set2 := set
	set2.Name = "sqlite-migration-set-superset"
	set2.Spec = map[string]any{"manifestDigest": supersetDigest}
	if _, _, err := r.applyResource(set2, applyOptions{
		Create: true, IdempotencyKey: "key-migration-set-superset",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("SQLiteMigrationSet superset apply: %w", err)
	}
	// A database has AT MOST ONE live application, over its UID, exactly like
	// the deployment and consumer rules. A second one against the same
	// database is refused BEFORE any mutation: two live applications would
	// each declare the database's whole schema, and a reader would have no
	// rule for which of them the ledger is supposed to equal.
	application2 := application
	application2.Name = "sqlite-migration-application-superset"
	application2.Spec = map[string]any{
		"database":     exactReference(database, database.Name),
		"migrationSet": exactReference(set2, set2.Name),
	}
	if response, err := r.apply(application2, applyOptions{
		Create: true, IdempotencyKey: "key-migration-application-superset",
	}); err != nil {
		return err
	} else if err := r.expectStableError(response, "invalid_argument"); err != nil {
		return fmt.Errorf("a second migration application against one database: %w", err)
	}

	// Advancing the schema is a HANDOVER: delete the application and create
	// one referencing the longer set. The ledger check makes the new
	// application resume at the unapplied suffix, so the handover replays
	// nothing — which is the whole reason the one-live rule costs nothing.
	if err := r.deleteExisting(application, "key-migration-application-handover"); err != nil {
		return fmt.Errorf("removing the first application for the handover: %w", err)
	}
	if _, _, err := r.applyResource(application2, applyOptions{
		Create: true, IdempotencyKey: "key-migration-application-superset-after-handover",
	}, http.StatusCreated); err != nil {
		return fmt.Errorf("SQLiteMigrationApplication superset apply after handover: %w", err)
	}
	second, _, err := r.read(application2)
	if err != nil {
		return err
	}
	if condition := readyCondition(second); condition.Status != "True" || condition.Reason != "Available" {
		return fmt.Errorf("superset migration application Ready = %s/%s, want True/Available", condition.Status, condition.Reason)
	}

	for _, item := range []struct {
		target probeTarget
		key    string
	}{
		{application2, "key-migration-application-superset-delete"},
		{set2, "key-migration-set-superset-delete"},
		{set, "key-migration-set-first-delete"},
		{database, "key-migration-database-delete"},
	} {
		current, err := r.readRaw(item.target)
		if err != nil {
			return err
		}
		response, err := r.deleteResource(item.target, current.Metadata.Generation, item.key, nil)
		if err != nil {
			return err
		}
		if response.Status != http.StatusNoContent {
			return fmt.Errorf("deleting %s %s HTTP %d", item.target.Ref.Kind, item.target.Name, response.Status)
		}
	}
	r.complete("sqlite-migration-ledger-readiness")
	return nil
}

func mustRunnerJSON(value any) []byte {
	raw, err := encodeRunnerJSON(value)
	if err != nil {
		panic(err)
	}
	return raw
}
