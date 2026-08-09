package workerbundle

import (
	"errors"
	"reflect"
	"testing"
)

func TestDeriveExportedHandlersReadsABoundObjectLiteral(t *testing.T) {
	source := []byte(`
const handlers = {
  async fetch(request, env, ctx) { return new Response("{ not a brace }"); },
  async queue(batch) {},
};
export default handlers;
`)
	handlers, err := DeriveExportedHandlers(source, HandlerVocabulary)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if !reflect.DeepEqual(handlers, []string{"fetch", "queue"}) {
		t.Fatalf("handlers = %v", handlers)
	}
}

func TestDeriveExportedHandlersReadsAnInlineLiteralAndQuotedKeys(t *testing.T) {
	source := []byte(`
// export default { scheduled(){} } in a comment must not count
export default {
  "fetch": function (request) {},
  scheduled: async () => {},
  queue(batch) {},
};
`)
	handlers, err := DeriveExportedHandlers(source, HandlerVocabulary)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if !reflect.DeepEqual(handlers, []string{"fetch", "scheduled", "queue"}) {
		t.Fatalf("handlers = %v", handlers)
	}
}

func TestDeriveExportedHandlersRefusesWhatItCannotRead(t *testing.T) {
	for name, source := range map[string]string{
		"unclosed literal": "export default {\n  async fetch(request) {\n",
		"no default":       "export const fetch = 1;\n",
		"spread":           "const base = {};\nconst handlers = { ...base, fetch(){} };\nexport default handlers;\n",
		"computed key":     "const key = \"fetch\";\nconst handlers = { [key](){} };\nexport default handlers;\n",
		"call result":      "export default makeHandlers();\n",
	} {
		if _, err := DeriveExportedHandlers([]byte(source), HandlerVocabulary); err == nil {
			t.Fatalf("%s: expected a refusal", name)
		} else if !errors.Is(err, ErrModuleSyntax) {
			t.Fatalf("%s: expected ErrModuleSyntax, got %v", name, err)
		}
	}
}

func TestEnvironmentPropertyNamesFailClosedOnACollision(t *testing.T) {
	deployment := Deployment{
		Vars:     []string{"CACHE"},
		Bindings: []Binding{{Name: "CACHE", Interface: "edge.kv"}},
	}
	if _, err := deployment.EnvironmentPropertyNames(); err == nil {
		t.Fatalf("expected an environment_name_collision")
	}
	deployment = Deployment{
		Vars:          []string{"LOG_LEVEL"},
		SensitiveVars: []string{"TOKEN"},
		Bindings:      []Binding{{Name: "CACHE", Interface: "edge.kv"}},
	}
	names, err := deployment.EnvironmentPropertyNames()
	if err != nil {
		t.Fatalf("names: %v", err)
	}
	if !reflect.DeepEqual(names, []string{"CACHE", "LOG_LEVEL", "TOKEN"}) {
		t.Fatalf("names = %v, want the lexicographic union", names)
	}
}

func TestLoadableMediaTypesAreClosed(t *testing.T) {
	if IsLoadableMediaType("text/html") || IsLoadableMediaType("application/javascript") {
		t.Fatalf("the loadable media-type set must be exactly the ABI's importable set")
	}
	if IsLoadableMediaType("application/source-map+json") {
		t.Fatalf("an auxiliary media type is carried, never loaded")
	}
	for _, mediaType := range LoadableMediaTypes {
		if !IsLoadableMediaType(mediaType) {
			t.Fatalf("%s must load", mediaType)
		}
	}
}
