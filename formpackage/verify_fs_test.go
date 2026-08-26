package formpackage

import (
	"io/fs"
	"os"
	"strings"
	"testing"
	"testing/fstest"
)

func TestVerifyFSIssuesCompletePackageCapabilityWithoutRepositoryPaths(t *testing.T) {
	t.Parallel()

	root := makeValidPackage(t, nil)
	report, err := VerifyFS(os.DirFS(root), ".")
	if err != nil {
		t.Fatal(err)
	}
	verified, ok := report.VerifiedPackage()
	if !ok || !verified.Valid() {
		t.Fatalf("VerifyFS issued no complete package capability: %#v, %v", verified, ok)
	}
	if verified.PackageDigest() != report.PackageDigest || verified.FormRef() != report.FormRef {
		t.Fatalf("VerifyFS capability drifted from report: package=%#v report=%#v", verified, report)
	}
}

func TestVerifyFSRejectsUnsafeOrChangingClosuresWithoutIssuingCapability(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]fs.FS{
		"symlink": fstest.MapFS{
			PackageIndexFilename: &fstest.MapFile{Data: []byte(`{}`)},
			"alias.json":         &fstest.MapFile{Data: []byte(`{}`), Mode: fs.ModeSymlink},
		},
		"executable": fstest.MapFS{
			PackageIndexFilename: &fstest.MapFile{Data: []byte(`{}`)},
			"payload.txt":        &fstest.MapFile{Data: []byte("data"), Mode: 0o755},
		},
		"code extension": fstest.MapFS{
			PackageIndexFilename: &fstest.MapFile{Data: []byte(`{}`)},
			"payload.js":         &fstest.MapFile{Data: []byte("data")},
		},
	} {
		name, source := name, source
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			report, err := VerifyFS(source, ".")
			if err == nil {
				t.Fatalf("unsafe embedded closure was accepted: %#v", report)
			}
			if verified, ok := report.VerifiedPackage(); ok || verified.Valid() {
				t.Fatalf("failed VerifyFS issued a package capability: %#v, %v", verified, ok)
			}
		})
	}

	for _, root := range []string{"", "/absolute", "../escape", "packages/../escape"} {
		report, err := VerifyFS(fstest.MapFS{}, root)
		if err == nil || !strings.Contains(err.Error(), "root") {
			t.Fatalf("VerifyFS root %q error = %v, want root refusal", root, err)
		}
		if verified, ok := report.VerifiedPackage(); ok || verified.Valid() {
			t.Fatalf("invalid root %q issued a package capability", root)
		}
	}
}
