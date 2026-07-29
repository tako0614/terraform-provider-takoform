package formcatalog

import (
	"reflect"
	"regexp"
	"testing"
)

func TestPortableConnectionReferenceCannotCarrySpace(t *testing.T) {
	t.Parallel()

	definition := connectionDefinitions(Kind{Connections: ConnectionsRequired})
	connection, ok := definition["connection"].(map[string]any)
	if !ok {
		t.Fatalf("connection schema = %#v", definition["connection"])
	}
	properties, ok := connection["properties"].(map[string]any)
	if !ok {
		t.Fatalf("connection properties = %#v", connection["properties"])
	}
	wantFields := []string{"permissions", "projection", "resource"}
	gotFields := make([]string, 0, len(properties))
	for field := range properties {
		gotFields = append(gotFields, field)
	}
	// The generator inserts these fields in no semantic order. Compare them as
	// a set through the fixed field count and explicit membership checks.
	if len(gotFields) != len(wantFields) {
		t.Fatalf("connection fields = %v, want exactly %v", gotFields, wantFields)
	}
	for _, field := range wantFields {
		if _, found := properties[field]; !found {
			t.Errorf("connection schema omits %q: %#v", field, properties)
		}
	}
	if connection["additionalProperties"] != false {
		t.Fatalf("connection schema permits unreviewed selectors: %#v", connection)
	}
	if !reflect.DeepEqual(connection["required"], wantFields) {
		t.Fatalf("connection required fields = %#v, want %#v", connection["required"], wantFields)
	}

	reference := regexp.MustCompile(PatternResourceRef)
	for _, crossSpace := range []string{
		"other-space/ObjectBucket/assets",
		"ObjectBucket/other-space/assets",
		"other-space:ObjectBucket/assets",
	} {
		if reference.MatchString(crossSpace) {
			t.Errorf("portable Resource reference accepts cross-Space encoding %q", crossSpace)
		}
	}
}
