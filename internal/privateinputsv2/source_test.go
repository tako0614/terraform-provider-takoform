package privateinputsv2

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestLoadSelectsOnlyExactSpaceResourceAndExpectedNames(t *testing.T) {
	t.Setenv(EnvironmentVariable, `{"prod":{"worker":{"API_TOKEN":"worker-secret","LOG_KEY":"second-secret"},"other":{"API_TOKEN":"other-resource-secret"}},"staging":{"worker":{"API_TOKEN":"staging-secret"}}}`)

	got, err := Load("prod", "worker", []string{"API_TOKEN", "LOG_KEY"})
	if err != nil {
		t.Fatal("Load() error:", err)
	}
	if len(got) != 2 || got["API_TOKEN"] != "worker-secret" || got["LOG_KEY"] != "second-secret" {
		t.Fatalf("Load() = %#v, want only the selected resource's declared values", got)
	}
	if _, ok := got["other-resource-secret"]; ok {
		t.Fatal("Load() returned another resource's value")
	}
}

func TestLoadAllowsAbsentOrPresentEmptySourceOnlyWithoutExpectedNames(t *testing.T) {
	if err := os.Unsetenv(EnvironmentVariable); err != nil {
		t.Fatal("Unsetenv:", err)
	}
	got, err := Load("prod", "worker", nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Load() with absent source and no expected names = %#v, %v; want empty map", got, err)
	}
	if _, err := Load("prod", "worker", []string{"API_TOKEN"}); err == nil {
		t.Fatal("Load() accepted absent source with required names")
	}

	t.Setenv(EnvironmentVariable, `{}`)
	got, err = Load("prod", "worker", nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("Load() with present empty source and no expected names = %#v, %v; want empty map", got, err)
	}
	if _, err := Load("prod", "worker", []string{"API_TOKEN"}); err == nil {
		t.Fatal("Load() accepted present empty source with required names")
	}
}

func TestLoadRejectsMalformedOrOutOfBoundsSourceWithoutEchoingValues(t *testing.T) {
	overlargeValue := strings.Repeat("s", MaxValueBytes+1)
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "invalid JSON", raw: `{"prod":`},
		{name: "invalid unrelated branch", raw: `{"prod":{"worker":{"API_TOKEN":"worker-secret"}},"staging":null}`, want: []string{"API_TOKEN"}},
		{name: "duplicate key", raw: `{"prod":{"worker":{"API_TOKEN":"first","API_TOKEN":"duplicate-secret"}}}`},
		{name: "non-string value", raw: `{"prod":{"worker":{"API_TOKEN":null}}}`},
		{name: "unknown name", raw: `{"prod":{"worker":{"UNKNOWN":"unknown-secret"}}}`, want: []string{"API_TOKEN"}},
		{name: "missing name", raw: `{"prod":{"worker":{"OTHER":"other-secret"}}}`, want: []string{"API_TOKEN"}},
		{name: "empty value", raw: `{"prod":{"worker":{"API_TOKEN":""}}}`, want: []string{"API_TOKEN"}},
		{name: "NUL value", raw: `{"prod":{"worker":{"API_TOKEN":"a\u0000b"}}}`, want: []string{"API_TOKEN"}},
		{name: "oversized value", raw: `{"prod":{"worker":{"API_TOKEN":"` + overlargeValue + `"}}}`, want: []string{"API_TOKEN"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvironmentVariable, tt.raw)
			_, err := Load("prod", "worker", tt.want)
			if err == nil {
				t.Fatal("Load() accepted invalid source")
			}
			for _, marker := range []string{"worker-secret", "duplicate-secret", "unknown-secret", "other-secret", "API_TOKEN"} {
				if strings.Contains(err.Error(), marker) {
					t.Fatalf("diagnostic leaked input content %q: %v", marker, err)
				}
			}
		})
	}
}

func TestLoadSelectsMoreThan64CallerExpectedNames(t *testing.T) {
	const count = 65
	want := make([]string, count)
	var raw strings.Builder
	raw.WriteString(`{"prod":{"worker":{`)
	for i := range want {
		if i > 0 {
			raw.WriteByte(',')
		}
		name := fmt.Sprintf("KEY_%02d", i)
		want[i] = name
		fmt.Fprintf(&raw, `"%s":"value"`, name)
	}
	raw.WriteString(`}}}`)
	t.Setenv(EnvironmentVariable, raw.String())

	got, err := Load("prod", "worker", want)
	if err != nil {
		t.Fatal("Load() rejected a generic caller selector with 65 names:", err)
	}
	if len(got) != count {
		t.Fatalf("Load() returned %d values, want %d", len(got), count)
	}
}

func TestLoadRejectsInvalidUnicodeAndOversizedDocument(t *testing.T) {
	t.Setenv(EnvironmentVariable, "{\"prod\":{\"worker\":{\"API_TOKEN\":\""+string([]byte{0xff})+"\"}}}")
	if _, err := Load("prod", "worker", []string{"API_TOKEN"}); err == nil {
		t.Fatal("Load() accepted invalid UTF-8")
	}

	t.Setenv(EnvironmentVariable, strings.Repeat(" ", MaxDocumentBytes+1))
	if _, err := Load("prod", "worker", nil); err == nil {
		t.Fatal("Load() accepted oversized input")
	}
}

func TestLoadRejectsUnpairedSurrogateEscapes(t *testing.T) {
	t.Setenv(EnvironmentVariable, `{"prod":{"worker":{"API_TOKEN":"\uD800"}}}`)
	if _, err := Load("prod", "worker", []string{"API_TOKEN"}); err == nil {
		t.Fatal("Load() accepted an unpaired surrogate")
	}
}

func TestLoadRejectsInvalidSelectorsAndExpectedNames(t *testing.T) {
	t.Setenv(EnvironmentVariable, `{"prod":{"worker":{"API_TOKEN":"worker-secret"}}}`)
	tests := []struct {
		space    string
		resource string
		want     []string
	}{
		{space: "../prod", resource: "worker", want: []string{"API_TOKEN"}},
		{space: "prod", resource: "worker/path", want: []string{"API_TOKEN"}},
		{space: "prod", resource: "worker", want: []string{"API_TOKEN", "API_TOKEN"}},
		{space: "prod", resource: "worker", want: []string{"api-token"}},
	}
	for _, tt := range tests {
		if _, err := Load(tt.space, tt.resource, tt.want); err == nil {
			t.Errorf("Load(%q, %q, %#v) accepted invalid selector/declaration", tt.space, tt.resource, tt.want)
		}
	}
}

func TestLoadOptionalOmitsAbsentUpdateSourceButRejectsPresentMismatch(t *testing.T) {
	if err := os.Unsetenv(EnvironmentVariable); err != nil {
		t.Fatal(err)
	}
	if got, present, err := LoadOptional("prod", "worker", []string{"API_TOKEN"}); err != nil || present || got != nil {
		t.Fatalf("absent optional map = %#v, present=%v, err=%v", got, present, err)
	}
	t.Setenv(EnvironmentVariable, `{"prod":{"other":{"API_TOKEN":"another-resource"}}}`)
	if got, present, err := LoadOptional("prod", "worker", []string{"API_TOKEN"}); err != nil || present || got != nil {
		t.Fatalf("unselected optional map = %#v, present=%v, err=%v", got, present, err)
	}
	t.Setenv(EnvironmentVariable, `{"prod":{"worker":{"API_TOKEN":"value"}}}`)
	if got, present, err := LoadOptional("prod", "worker", []string{"API_TOKEN"}); err != nil || !present || got["API_TOKEN"] != "value" {
		t.Fatalf("selected optional map = %#v, present=%v, err=%v", got, present, err)
	}
	for _, raw := range []string{
		`{"prod":{"worker":{}}}`,
		`{"prod":{"worker":{"OTHER":"wrong"}}}`,
		`{"prod":{"worker":{"API_TOKEN":null}}}`,
		`{"prod":{"worker":{"API_TOKEN":""}}}`,
	} {
		t.Setenv(EnvironmentVariable, raw)
		if _, _, err := LoadOptional("prod", "worker", []string{"API_TOKEN"}); err == nil || strings.Contains(err.Error(), "API_TOKEN") {
			t.Fatalf("malformed selected map accepted or leaked name: %v", err)
		}
	}
}
