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
		GrammarCIDR:     {[]string{"10.0.0.0/8", "2001:db8::/32"}, []string{"10.0.0.0", "10.0.0.0/999", "not/16"}},
		GrammarPath:     {[]string{"/", "/api/v1"}, []string{"api", "", "/a b"}},
		GrammarCron:     {[]string{"0 0 * * *"}, []string{"0 0 * *", "0 0 * * * *", "0  0 * * *"}},
		GrammarMailbox:  {[]string{"a@b.invalid"}, []string{"a@b", "no-at", "a b@c.invalid"}},
		GrammarHTTPSURL: {[]string{"https://a.invalid/x"}, []string{"http://a.invalid/x", "https:// a"}},
		GrammarToken:    {[]string{"postgres", "a.b-c:d"}, []string{"1bad", "not a token", ""}},
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
		}
		for _, value := range spec.invalid {
			if re.MatchString(value) {
				t.Errorf("%s accepts invalid %q", grammar, value)
			}
		}
	}
}
