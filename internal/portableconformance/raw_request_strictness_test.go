package portableconformance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
)

func TestReferenceHostRejectsRawUnknownRequestFieldsBeforeMutation(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newReferenceHost(contract))
	defer server.Close()

	tests := []struct {
		name      string
		resource  string
		mutate    func(map[string]any)
		checkName string
	}{
		{
			name:     "unknown top-level Resource and apply field",
			resource: "portable-raw-top-level",
			mutate: func(document map[string]any) {
				document["managerAuthority"] = map[string]any{
					"backend": "attacker-controlled",
				}
			},
			checkName: "unknown-top-level-request-field-rejected-before-mutation",
		},
		{
			name:     "unknown metadata field",
			resource: "portable-raw-metadata",
			mutate: func(document map[string]any) {
				document["metadata"].(map[string]any)["managerAuthority"] = "attacker-controlled"
			},
			checkName: "unknown-metadata-field-rejected-before-mutation",
		},
		{
			name:     "unknown desired authority field",
			resource: "portable-raw-desired-authority",
			mutate: func(document map[string]any) {
				document["spec"].(map[string]any)["managerAuthority"] = map[string]any{
					"backend": "attacker-controlled",
				}
			},
			checkName: "unknown-desired-authority-field-rejected-before-mutation",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := rawRequestResource(contract, test.resource)
			assertRawResourceAbsent(t, server, contract, test.resource)

			previewTarget := server.URL + contract.APIPath + "/resources/preview"
			mutatedPreview := rawMapClone(t, resource)
			test.mutate(mutatedPreview)
			response := rawRequest(
				t,
				server,
				http.MethodPost,
				previewTarget,
				map[string]string{"If-None-Match": "*"},
				rawJSON(t, mutatedPreview),
			)
			if err := expectStableError(response, "invalid_argument"); err != nil {
				t.Fatalf("%s at preview: %v", test.checkName, err)
			}
			assertRawResourceAbsent(t, server, contract, test.resource)

			preview := rawRequest(
				t,
				server,
				http.MethodPost,
				previewTarget,
				map[string]string{"If-None-Match": "*"},
				rawJSON(t, resource),
			)
			if preview.Status != http.StatusOK {
				t.Fatalf("valid preview returned HTTP %d: %s", preview.Status, preview.Body)
			}
			var result client.PreviewResourceResult
			if err := decodeStrictBytes(preview.Body, &result); err != nil {
				t.Fatal(err)
			}

			mutatedApply := rawMapClone(t, resource)
			mutatedApply["review"] = map[string]any{"planDigest": result.Review.PlanDigest}
			test.mutate(mutatedApply)
			applyTarget := server.URL + contract.APIPath + "/resources/" +
				url.PathEscape(contract.RunnerInput.Identity.FormRef.Kind) + "/" +
				url.PathEscape(test.resource)
			response = rawRequest(
				t,
				server,
				http.MethodPut,
				applyTarget,
				map[string]string{
					"If-None-Match":   "*",
					"Idempotency-Key": test.checkName,
				},
				rawJSON(t, mutatedApply),
			)
			if err := expectStableError(response, "invalid_argument"); err != nil {
				t.Fatalf("%s at apply: %v", test.checkName, err)
			}
			assertRawResourceAbsent(t, server, contract, test.resource)
		})
	}
}

func rawRequestResource(contract Contract, name string) map[string]any {
	desired := cloneMap(contract.RunnerInput.Desired)
	desired["name"] = name
	return map[string]any{
		"apiVersion": contract.APIVersion,
		"kind":       contract.RunnerInput.Identity.FormRef.Kind,
		"form": map[string]any{
			"formRef": map[string]any{
				"apiVersion":        contract.RunnerInput.Identity.FormRef.APIVersion,
				"kind":              contract.RunnerInput.Identity.FormRef.Kind,
				"definitionVersion": contract.RunnerInput.Identity.FormRef.DefinitionVersion,
				"schemaDigest":      contract.RunnerInput.Identity.FormRef.SchemaDigest,
			},
			"packageDigest": contract.RunnerInput.Identity.PackageDigest,
		},
		"metadata": map[string]any{
			"name":  name,
			"space": contract.RunnerInput.Space,
		},
		"spec": desired,
	}
}

func assertRawResourceAbsent(
	t *testing.T,
	server *httptest.Server,
	contract Contract,
	name string,
) {
	t.Helper()
	identity := contract.RunnerInput.Identity
	query := url.Values{
		"space":             []string{contract.RunnerInput.Space},
		"apiVersion":        []string{identity.FormRef.APIVersion},
		"kind":              []string{identity.FormRef.Kind},
		"definitionVersion": []string{identity.FormRef.DefinitionVersion},
		"schemaDigest":      []string{identity.FormRef.SchemaDigest},
		"packageDigest":     []string{identity.PackageDigest},
	}
	target := server.URL + contract.APIPath + "/resources/" +
		url.PathEscape(identity.FormRef.Kind) + "/" + url.PathEscape(name) +
		"?" + query.Encode()
	response := rawRequest(t, server, http.MethodGet, target, nil, nil)
	if err := expectStableError(response, "resource_not_found"); err != nil {
		t.Fatalf("Resource %q is not absent: %v", name, err)
	}
}

func rawRequest(
	t *testing.T,
	server *httptest.Server,
	method string,
	target string,
	headers map[string]string,
	body []byte,
) wireResponse {
	t.Helper()
	request, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+referencePrimaryToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return wireResponse{
		Status: response.StatusCode,
		Header: response.Header.Clone(),
		Body:   raw,
	}
}

func rawMapClone(t *testing.T, input map[string]any) map[string]any {
	t.Helper()
	var output map[string]any
	if err := json.Unmarshal(rawJSON(t, input), &output); err != nil {
		t.Fatal(err)
	}
	return output
}

func rawJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
