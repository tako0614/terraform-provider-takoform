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
			"/placement": []string{"auto"},
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

func TestGetStandardServiceSupportPreservesOpaqueProtocolDecision(t *testing.T) {
	protocol := "org.example.unknown"
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && r.URL.EscapedPath() == APIRootPath+"/support/standard-services/"+protocol {
			writeJSON(t, w, http.StatusOK, map[string]any{
				"apiVersion": SupportProfileAPIVersion,
				"kind":       "StandardServiceSupport",
				"serviceRef": map[string]any{
					"apiVersion": "standards.takoform.com/v1",
					"protocol":   protocol,
				},
				"satisfiable": false,
			})
			return true
		}
		return false
	})
	profile, err := client.GetStandardServiceSupport(context.Background(), protocol)
	if err != nil {
		t.Fatalf("get standard-service support: %v", err)
	}
	if got, ok := profile["satisfiable"].(bool); !ok || got {
		t.Fatalf("support decision = %#v, want false", profile["satisfiable"])
	}
}

func TestValidateSupportProfileRequiresFormIdentityAndOperations(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"formRef", func(profile map[string]any) { delete(profile, "formRef") }, "omits formRef"},
		{"operations", func(profile map[string]any) { delete(profile, "operations") }, "omits operations"},
		{"unknown", func(profile map[string]any) { profile["future"] = true }, "unknown field"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			profile := wireSupportProfile()
			test.mutate(profile)
			if err := validateSupportProfile(profile); err == nil || !contains(err, test.want) {
				t.Fatalf("profile validation error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestListFormSupportRejectsMoreThan1024Profiles(t *testing.T) {
	client := newTestClient(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method != http.MethodGet || r.URL.Path != APIRootPath+"/support/forms" {
			return false
		}
		profiles := make([]map[string]any, 1025)
		for index := range profiles {
			profiles[index] = wireSupportProfile()
		}
		writeJSON(t, w, http.StatusOK, map[string]any{"profiles": profiles})
		return true
	})
	if _, err := client.ListFormSupport(context.Background()); err == nil || !contains(err, "more than 1024") {
		t.Fatalf("oversized support profile list was accepted: %v", err)
	}
}

func TestValidateSupportProfileUsesJSONPointerCapabilityKeys(t *testing.T) {
	valid := wireSupportProfile()
	valid["operations"] = []any{"create", "read", "update", "delete"}
	valid["supportedEnums"].(map[string]any)["/handlers"] = []any{"fetch"}
	delete(valid["supportedEnums"].(map[string]any), "/placement")
	if err := validateSupportProfile(valid); err != nil {
		t.Fatalf("valid pointer capability key rejected: %v", err)
	}
	for _, pointer := range []string{"handlers", "/bad\x00pointer"} {
		invalid := wireSupportProfile()
		invalid["operations"] = []any{"create"}
		invalid["supportedEnums"] = map[string]any{pointer: []any{"fetch"}}
		if err := validateSupportProfile(invalid); err == nil {
			t.Fatalf("invalid pointer capability key %q was accepted", pointer)
		}
	}
}
