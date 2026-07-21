package composition

import (
	"os"
	"path/filepath"
	"testing"
)

const validManifest = `{
  "apiVersion": "compositions.takoform.com/v1alpha1",
  "kind": "CapsuleComposition",
  "metadata": {"name":"office-with-storage","version":"1.0.0","title":"Office with storage"},
  "components": [
    {"id":"office","title":"Office","source":{"url":"https://github.com/tako0614/takos-office.git","ref":"0123456789012345678901234567890123456789","path":"."}},
    {"id":"storage","title":"Storage","source":{"url":"https://github.com/tako0614/takos-storage.git","ref":"0123456789012345678901234567890123456789","path":"."}}
  ],
  "connections": [{"from":{"component":"storage","interface":"storage.object"},"to":{"component":"office","interface":"storage.object"}}]
}`

func TestVerify(t *testing.T) {
	manifest, digest, err := Verify([]byte(validManifest))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if manifest.Metadata.Name != "office-with-storage" || len(manifest.Components) != 2 {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if len(digest) != len("sha256:")+64 {
		t.Fatalf("unexpected digest %q", digest)
	}
}

func TestVerifyRejectsHostAuthority(t *testing.T) {
	_, _, err := Verify([]byte(`{
    "apiVersion":"compositions.takoform.com/v1alpha1",
    "kind":"CapsuleComposition",
    "metadata":{"name":"bad","version":"1.0.0","title":"Bad"},
    "components":[{"id":"app","title":"App","source":{"url":"https://example.com/app.git","ref":"main","path":"."},"providerConnectionId":"pc_secret"}]
  }`))
	if err == nil {
		t.Fatal("Verify() accepted host authority field")
	}
}

func TestInitialCompositionsVerify(t *testing.T) {
	for _, name := range []string{
		"yurucommu-standalone.json",
		"yurumeet-standalone.json",
		"takos-office-with-storage.json",
	} {
		raw, err := os.ReadFile(filepath.Join("..", "compositions", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, _, err := Verify(raw); err != nil {
			t.Fatalf("Verify(%s): %v", name, err)
		}
	}
}
