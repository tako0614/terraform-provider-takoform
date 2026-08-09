package portableconformancev3

import (
	"strings"
	"testing"
)

func TestCanonicalStaticAssetPathClosedMapping(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
		ok   bool
	}{
		{name: "root", path: "/", want: "", ok: true},
		{name: "one leading slash", path: "/app/index.html", want: "app/index.html", ok: true},
		{name: "query ignored", path: "/app/index.html?cache=1", want: "app/index.html", ok: true},
		{name: "fragment ignored", path: "/app/index.html#section", want: "app/index.html", ok: true},
		{name: "percent decode once", path: "/app/%69ndex.html", want: "app/index.html", ok: true},
		{name: "no leading slash", path: "app/index.html", ok: false},
		{name: "encoded slash", path: "/app%2Findex.html", ok: false},
		{name: "encoded backslash", path: "/app%5Cindex.html", ok: false},
		{name: "repeated slash", path: "/app//index.html", ok: false},
		{name: "trailing slash", path: "/app/", ok: false},
		{name: "dot segment", path: "/app/./index.html", ok: false},
		{name: "dot dot segment", path: "/app/../index.html", ok: false},
		{name: "backslash", path: "/app\\index.html", ok: false},
		{name: "control", path: "/app/%00.html", ok: false},
		{name: "noncharacter", path: "/app/%EF%BF%BE.html", ok: false},
		{name: "invalid utf8", path: "/app/%FF.html", ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := CanonicalStaticAssetPath(test.path)
			if got != test.want || ok != test.ok {
				t.Fatalf("CanonicalStaticAssetPath(%q) = %q, %v; want %q, %v", test.path, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestStaticAssetResolutionNeverFallsBackForInvalidPath(t *testing.T) {
	manifest := artifactManifest{Kind: staticAssetBundleKind, Files: []artifactFile{
		{Path: "index.html", MediaType: "text/html", Size: "0", Digest: "sha256:" + strings.Repeat("0", 64)},
		{Path: "app.js", MediaType: "application/javascript", Size: "0", Digest: "sha256:" + strings.Repeat("1", 64)},
	}}
	if file, found, hostErr := resolveStaticAssetPath(manifest, "/missing%2Fpath", "single_page_application"); hostErr == nil || found || file.Path != "" {
		t.Fatalf("invalid path resolution = file=%+v found=%v err=%+v, want fail closed", file, found, hostErr)
	}
	if file, found, hostErr := resolveStaticAssetPath(manifest, "/missing", "single_page_application"); hostErr != nil || !found || file.Path != "index.html" {
		t.Fatalf("SPA missing resolution = file=%+v found=%v err=%+v, want index.html", file, found, hostErr)
	}
	if file, found, hostErr := resolveStaticAssetPath(manifest, "/missing", "none"); hostErr != nil || found || file.Path != "" {
		t.Fatalf("none missing resolution = file=%+v found=%v err=%+v, want miss", file, found, hostErr)
	}
}
