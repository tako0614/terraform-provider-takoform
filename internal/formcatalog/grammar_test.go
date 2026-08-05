package formcatalog

import (
	"regexp"
	"testing"
)

// TestPortableGrammarsBindWhatTheyClaim pins the boundary of every declared
// grammar. A grammar that quietly accepts a bare hostname, an unbounded prefix
// length, or a plaintext URL would let non-portable input through the one
// place a Form says what a value may be.
func TestPortableGrammarsBindWhatTheyClaim(t *testing.T) {
	cases := map[Grammar]struct{ valid, invalid []string }{
		GrammarHostname: {[]string{"api.example.invalid", "a.b"}, []string{"localhost", "example.invalid.", "-bad.example.invalid", "UPPER..dot"}},
		GrammarCIDR: {[]string{"10.0.0.0/8", "2001:db8::/32"}, []string{
			"10.0.0.0", "10.0.0.0/33", "999.999.999.999/24",
			"2001:db8::/129", "2001:::1/64", "not/16",
		}},
		GrammarPath: {[]string{"/", "/api/v1"}, []string{"api", "", "/a b"}},
		GrammarCron: {[]string{"0 0 * * *", "59 23 31 12 6"}, []string{
			"0 0 * *", "0 0 * * * *", "0  0 * * *", "x x x x x",
			"60 24 32 13 7", "*/0 * * * *",
		}},
		GrammarMailbox: {[]string{"a@b.invalid"}, []string{"a@b", "no-at", "a b@c.invalid"}},
		GrammarHTTPSURL: {[]string{
			"https://a.invalid/x",
			"https://a.invalid/callback?mode=oidc",
			"https://a.invalid/callback#complete",
		}, []string{
			"http://a.invalid/x", "https:// a", "https://%", "https:///callback",
		}},
		GrammarCredentialFreeHTTPSURL: {[]string{
			"https://a.invalid/artifact.tar",
			"https://a.invalid:8443/releases/artifact.tar",
		}, []string{
			"http://a.invalid/artifact.tar",
			"https://user@a.invalid/artifact.tar",
			"https://a.invalid/artifact.tar?download=1",
			"https://a.invalid/artifact.tar#archive",
		}},
		GrammarDNSRelativeName: {
			[]string{"@", "api", "_service._tcp", "*.apps"},
			[]string{"", "api..internal", "/api", ".hidden"},
		},
		GrammarToken:   {[]string{"postgres", "a.b-c:d"}, []string{"1bad", "not a token", ""}},
		GrammarVersion: {[]string{"1", "1.2.3", "v2-beta"}, []string{"", "version 2", "/v2"}},
		GrammarCanonicalSHA256: {
			[]string{"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			[]string{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", "sha256:0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"},
		},
		GrammarCanonicalOCIDigest: {
			[]string{"registry.invalid/app@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
			[]string{"registry.invalid/app:latest", "registry.invalid/app@sha256:0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF0123456789ABCDEF"},
		},
	}
	for grammar, spec := range cases {
		pattern, ok := grammar.Pattern()
		if !ok {
			t.Fatalf("%s has no pattern", grammar)
		}
		re := regexp.MustCompile(pattern)
		for _, value := range spec.valid {
			if !re.MatchString(value) {
				t.Errorf("%s rejects valid %q", grammar, value)
			}
			if grammar == GrammarCredentialFreeHTTPSURL && !ValidCredentialFreeHTTPSURL(value) {
				t.Errorf("shared credential-free HTTPS validator rejects valid %q", value)
			}
		}
		for _, value := range spec.invalid {
			if re.MatchString(value) {
				t.Errorf("%s accepts invalid %q", grammar, value)
			}
			if grammar == GrammarCredentialFreeHTTPSURL && ValidCredentialFreeHTTPSURL(value) {
				t.Errorf("shared credential-free HTTPS validator accepts invalid %q", value)
			}
		}
	}
}

func TestCredentialFreeHTTPSValidatorAlsoEnforcesURISyntax(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"https://runtime.example.invalid/蛸/runtime",
		"https://runtime.example.invalid/%E8%9B%B8/runtime",
	} {
		if !ValidCredentialFreeHTTPSURL(value) {
			t.Errorf("shared credential-free HTTPS validator rejects valid URI %q", value)
		}
	}
	for _, value := range []string{
		"https://runtime.example.invalid/%ZZ",
		"https://runtime.example.invalid/has\x00control",
		"https://runtime.example.invalid/has\u00a0space",
	} {
		if ValidCredentialFreeHTTPSURL(value) {
			t.Errorf("shared credential-free HTTPS validator accepts invalid URI %q", value)
		}
	}
}

func TestPortableResourceNamesAndReferencesHaveOneCanonicalShape(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"app", "edge-worker", "a1"} {
		if !regexp.MustCompile(PatternName).MatchString(value) {
			t.Errorf("portable resource name rejects %q", value)
		}
	}
	for _, value := range []string{"", " leading", "trailing ", "Uppercase", "has_underscore", "two..dots"} {
		if regexp.MustCompile(PatternName).MatchString(value) {
			t.Errorf("portable resource name accepts %q", value)
		}
	}
	for _, value := range []string{"EdgeWorker/app", "ObjectBucket/static-assets"} {
		if !regexp.MustCompile(PatternResourceRef).MatchString(value) {
			t.Errorf("portable resource reference rejects %q", value)
		}
	}
	for _, value := range []string{"app", "edgeworker/app", "EdgeWorker/Uppercase", "EdgeWorker/has space", "EdgeWorker/app/extra"} {
		if regexp.MustCompile(PatternResourceRef).MatchString(value) {
			t.Errorf("portable resource reference accepts %q", value)
		}
	}
}
