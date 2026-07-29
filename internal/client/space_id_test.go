package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestSpaceIDContract(t *testing.T) {
	t.Parallel()

	valid := []string{
		"a",
		"Prod",
		"Prod North",
		"prod\u00a0north",
		"prod\u2028north",
		"prod\ufeffnorth",
		strings.Repeat("界", SpaceIDMaxLength),
		strings.Repeat("🐙", SpaceIDMaxLength),
	}
	for _, value := range valid {
		value := value
		t.Run("valid "+value, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSpaceID(value); err != nil {
				t.Fatalf("ValidateSpaceID(%q): %v", value, err)
			}
		})
	}

	invalid := []string{
		"",
		strings.Repeat("界", SpaceIDMaxLength+1),
		strings.Repeat("🐙", SpaceIDMaxLength+1),
		" leading",
		"trailing ",
		"\u00a0leading",
		"trailing\u3000",
		"\u0085leading",
		"\u2028leading",
		"trailing\u2029",
		"\ufeffleading",
		"trailing\ufeff",
		"has/slash",
		"has\x00control",
		"has\tcontrol",
		string([]byte{0xff}),
	}
	for _, value := range invalid {
		value := value
		t.Run("invalid "+value, func(t *testing.T) {
			t.Parallel()
			if err := ValidateSpaceID(value); err == nil {
				t.Fatalf("ValidateSpaceID(%q) unexpectedly succeeded", value)
			}
		})
	}
	for _, controlRange := range [][2]rune{{0x00, 0x1f}, {0x7f, 0x9f}} {
		for candidate := controlRange[0]; candidate <= controlRange[1]; candidate++ {
			if err := ValidateSpaceID("a" + string(candidate) + "b"); err == nil {
				t.Errorf("ValidateSpaceID accepted embedded control U+%04X", candidate)
			}
		}
	}
}

func TestSpaceIDIsPreservedExactlyInResourceAndInterfaceQueries(t *testing.T) {
	t.Parallel()

	const space = "Prod North"
	var formsSpace, interfacesSpace string
	server := interfaceHost(t, true, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/apis/forms.takoform.com/v1alpha1/forms":
			formsSpace = r.URL.Query().Get("space")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"forms": []any{map[string]any{
					"identity":             exactObjectBucketFixture,
					"definitionKnown":      true,
					"installed":            true,
					"executable":           true,
					"activated":            true,
					"availableToPrincipal": true,
					"operations":           []string{"read"},
				}},
			})
		case "/apis/forms.takoform.com/v1alpha1/interfaces":
			interfacesSpace = r.URL.Query().Get("space")
			_ = json.NewEncoder(w).Encode(map[string]any{"interfaces": []any{}})
		default:
			http.NotFound(w, r)
		}
	})
	c := discoveredClient(t, server)
	if err := c.EnsureFormAvailable(
		context.Background(),
		space,
		exactObjectBucketFixture,
		"read",
	); err != nil {
		t.Fatalf("EnsureFormAvailable: %v", err)
	}
	if _, err := c.ListInterfaces(context.Background(), space); err != nil {
		t.Fatalf("ListInterfaces: %v", err)
	}
	if formsSpace != space || interfacesSpace != space {
		t.Fatalf(
			"SpaceID changed in query: forms=%q interfaces=%q want=%q",
			formsSpace,
			interfacesSpace,
			space,
		)
	}
}

func TestInvalidSpaceIDFailsBeforeAnyHostOperation(t *testing.T) {
	t.Parallel()

	type operation struct {
		name string
		run  func(context.Context, *Client, string) error
	}
	operations := []operation{
		{
			name: "availability",
			run: func(ctx context.Context, c *Client, space string) error {
				return c.EnsureFormAvailable(ctx, space, exactObjectBucketFixture, "read")
			},
		},
		{
			name: "resource read",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.GetResource(ctx, "ObjectBucket", "assets", space, exactObjectBucketFixture)
				return err
			},
		},
		{
			name: "observe",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.ObserveResource(ctx, "ObjectBucket", "assets", space, MutationFence{
					ResourceVersion: "1",
					Form:            exactObjectBucketFixture,
				})
				return err
			},
		},
		{
			name: "refresh",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.RefreshResource(ctx, "ObjectBucket", "assets", space, MutationFence{
					ResourceVersion: "1",
					Form:            exactObjectBucketFixture,
				})
				return err
			},
		},
		{
			name: "delete",
			run: func(ctx context.Context, c *Client, space string) error {
				return c.DeleteResource(ctx, "ObjectBucket", "assets", space, MutationFence{
					ResourceVersion: "1",
					Form:            exactObjectBucketFixture,
				})
			},
		},
		{
			name: "preview",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.PreviewResource(ctx, spaceIDTestResource(space))
				return err
			},
		},
		{
			name: "apply",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.PutResource(ctx, "ObjectBucket", "assets", spaceIDTestResource(space))
				return err
			},
		},
		{
			name: "import",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.ImportResource(
					ctx,
					"ObjectBucket",
					"assets",
					"native-assets",
					spaceIDTestResource(space),
				)
				return err
			},
		},
		{
			name: "interface list",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.ListInterfaces(ctx, space)
				return err
			},
		},
		{
			name: "interface get",
			run: func(ctx context.Context, c *Client, space string) error {
				_, err := c.GetInterface(ctx, space, InterfaceSelector{
					Name: "object.storage", Version: "1",
					ResourceKind: "ObjectBucket", ResourceName: "assets",
				})
				return err
			},
		},
	}

	for _, test := range operations {
		test := test
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := interfaceHost(t, true, func(w http.ResponseWriter, _ *http.Request) {
				requests++
				http.Error(w, "unexpected host request", http.StatusInternalServerError)
			})
			err := test.run(context.Background(), discoveredClient(t, server), " invalid")
			if err == nil {
				t.Fatal("invalid SpaceID unexpectedly succeeded")
			}
			if requests != 0 {
				t.Fatalf("invalid SpaceID made %d host request(s)", requests)
			}
		})
	}
}

func spaceIDTestResource(space string) *Resource {
	return &Resource{
		APIVersion: APIVersion,
		Kind:       "ObjectBucket",
		Form:       &exactObjectBucketFixture,
		Metadata: Metadata{
			Name:  "assets",
			Space: space,
		},
		Spec: map[string]any{"name": "assets"},
	}
}
