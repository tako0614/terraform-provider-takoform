package clientv3

import (
	"context"
	"net/http"
	"testing"
)

func wireSupportProfile() map[string]any {
	return map[string]any{
		"apiVersion": SupportProfileAPIVersion,
		"kind":       "FormSupport",
		"formRef": map[string]any{
			"apiVersion":        testGroup,
			"kind":              testKind,
			"definitionVersion": "1.0.0",
			"schemaDigest":      testSchemaDigest,
		},
		"operations": []string{"create", "read", "update", "delete"},
		"supportedEnums": map[string]any{
			"placement": []string{"auto"},
		},
	}
}

func TestListFormSupport(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.Path == APIRootPath+"/support/forms" {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"profiles": []map[string]any{wireSupportProfile()},
			})
			return true
		}
		return false
	})
	profiles, err := client.ListFormSupport(context.Background())
	if err != nil {
		t.Fatalf("list support: %v", err)
	}
	if len(profiles) != 1 || profiles[0]["kind"] != "FormSupport" {
		t.Fatalf("profiles not surfaced: %v", profiles)
	}
}

func TestGetFormSupportSplitsTheGroupIntoTwoPathSegments(t *testing.T) {
	wantPath := APIRootPath + "/support/forms/" + groupPathSegments(testGroup) + "/" + testKind + "/1.0.0"
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.EscapedPath() == wantPath {
			writeJSON(t, w, http.StatusOK, wireSupportProfile())
			return true
		}
		return false
	})
	profile, err := client.GetFormSupport(context.Background(), testRef)
	if err != nil {
		t.Fatalf("get support: %v", err)
	}
	if profile["apiVersion"] != SupportProfileAPIVersion {
		t.Fatalf("profile identity not surfaced: %v", profile)
	}
}

func TestGetFormSupportRejectsSubstitutedLine(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet {
			profile := wireSupportProfile()
			profile["formRef"].(map[string]any)["definitionVersion"] = "2.0.0"
			writeJSON(t, w, http.StatusOK, profile)
			return true
		}
		return false
	})
	if _, err := client.GetFormSupport(context.Background(), testRef); err == nil ||
		!contains(err, "different FormRef line") {
		t.Fatalf("expected substituted line rejection, got %v", err)
	}
}
