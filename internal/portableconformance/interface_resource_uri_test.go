package portableconformance

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type interfaceResourceURITransport struct {
	base        http.RoundTripper
	resourceURI string
}

func (transport interfaceResourceURITransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.base.RoundTrip(request)
	if err != nil || response.StatusCode != http.StatusOK ||
		!strings.Contains(request.URL.Path, "/interfaces/") {
		return response, err
	}
	raw, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return nil, err
	}
	var declaration map[string]any
	if err := json.Unmarshal(raw, &declaration); err != nil {
		return nil, err
	}
	declaration["resourceUri"] = transport.resourceURI
	encoded, err := json.Marshal(declaration)
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(encoded))
	response.ContentLength = int64(len(encoded))
	response.Header.Del("Content-Length")
	return response, nil
}

func TestBlackBoxRunnerRejectsNonCredentialFreeInterfaceResourceURI(t *testing.T) {
	contract, err := Verify(filepath.Join("..", "..", "conformance", "portable-host-v2"))
	if err != nil {
		t.Fatal(err)
	}
	for _, resourceURI := range []string{
		"",
		"http://runtime.example.invalid/oauth/resource",
		"https://user@runtime.example.invalid/oauth/resource",
		"https://runtime.example.invalid/oauth/resource?audience=one",
		"https://runtime.example.invalid/oauth/resource#fragment",
		"https://例え.テスト/oauth/resource",
	} {
		resourceURI := resourceURI
		t.Run(resourceURI, func(t *testing.T) {
			host := newReferenceHost(contract)
			server := httptest.NewServer(host)
			defer server.Close()

			httpClient := *server.Client()
			httpClient.Transport = interfaceResourceURITransport{
				base:        httpClient.Transport,
				resourceURI: resourceURI,
			}
			_, err := RunEndpoint(context.Background(), contract, EndpointOptions{
				Endpoint:       server.URL,
				HTTPClient:     &httpClient,
				Classification: ReferenceHostSelfTest,
				Subject:        "reference-host-with-invalid-interface-resource-uri",
			})
			if err == nil || !strings.Contains(err.Error(), "credential-free HTTPS resourceUri") {
				t.Fatalf("runner error = %v, want invalid resourceUri rejection", err)
			}
		})
	}
}
