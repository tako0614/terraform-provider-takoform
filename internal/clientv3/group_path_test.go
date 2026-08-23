package clientv3

import (
	"strings"
	"testing"
)

// TestGroupPathRoundTripsEveryShape pins the encoder and the decoder to each
// other. They are separate functions used by separate parties — this client
// builds URLs, a host reads them — and the shape they disagree about is the
// one decision 0049 introduced: a group that carries no version. A decoder
// that counted segments read the versionless shape as a group plus a kind and
// answered 404, which is indistinguishable to a caller from a resource that
// does not exist.
func TestGroupPathRoundTripsEveryShape(t *testing.T) {
	t.Parallel()
	for _, group := range []string{
		"edge.forms.takoform.com",
		"edge.forms.takoform.com/v1beta1",
		"forms.example.com",
		"forms.example.com/v1alpha1",
		"a.b.c.d.example.com/v2beta3",
	} {
		group := group
		t.Run(group, func(t *testing.T) {
			t.Parallel()
			encoded := groupPathSegments(group) + "/ModuleWorker/app"
			decoded, tail, ok := SplitGroupPath(strings.Split(encoded, "/"))
			if !ok {
				t.Fatalf("SplitGroupPath refused the path this client built: %q", encoded)
			}
			if decoded != group {
				t.Fatalf("round trip of %q produced %q", group, decoded)
			}
			if len(tail) != 2 || tail[0] != "ModuleWorker" || tail[1] != "app" {
				t.Fatalf("tail after the group is %v, want [ModuleWorker app]", tail)
			}
		})
	}
}

// TestSplitGroupPathRefusesPathsWithNoGroup keeps the decoder from inventing an
// empty group for a path that never carried one.
func TestSplitGroupPathRefusesPathsWithNoGroup(t *testing.T) {
	t.Parallel()
	for name, parts := range map[string][]string{
		"kind first":       {"ModuleWorker", "app"},
		"no kind at all":   {"edge.forms.takoform.com", "v1beta1"},
		"nothing":          {},
		"only a separator": {""},
	} {
		parts := parts
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if group, _, ok := SplitGroupPath(parts); ok {
				t.Fatalf("SplitGroupPath(%v) accepted with group %q", parts, group)
			}
		})
	}
}
