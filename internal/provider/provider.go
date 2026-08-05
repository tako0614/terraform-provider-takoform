// Package provider implements the thin Takoform OpenTofu/Terraform provider.
//
// The provider is intentionally thin: it carries typed Service Form HCL
// schemas, validation, and a portable form-host HTTP client.
// It does not call AWS / Cloudflare / Kubernetes SDKs, does not select a
// backend, and does not manage credentials. Placement and implementation
// selection remain host responsibilities. The provider is capability-driven:
// on configure it discovers form support and never branches on an edition
// string.
package provider

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformcatalog"
	"github.com/tako0614/terraform-provider-takoform/internal/currentformregistry"
)

// Environment variable fallbacks for provider configuration.
const (
	envEndpoint = "TAKOFORM_ENDPOINT"
	envSpace    = "TAKOFORM_SPACE"
	envToken    = "TAKOFORM_TOKEN"

	defaultResourceAPITimeout = 12 * time.Minute
)

// Ensure takoformProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*takoformProvider)(nil)

// takoformProvider is the provider implementation.
type takoformProvider struct {
	// version is set at build time and surfaced to Terraform.
	version string
}

// providerData is shared with every resource via Configure.
type providerData struct {
	client            *client.Client
	defaultSpace      string
	forms             map[string]client.InstalledFormReference
	serviceFormMutate sync.Mutex
}

// takoformProviderModel maps the provider configuration schema.
type takoformProviderModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Space    types.String `tfsdk:"space"`
	Token    types.String `tfsdk:"token"`
}

// New returns a provider factory bound to a build version.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &takoformProvider{version: version}
	}
}

func (p *takoformProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "takoform"
	resp.Version = p.version
}

func (p *takoformProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Takoform provider exposes statically typed portable Service Form resources " +
			"through any conforming form host. It never selects a backend, target, credential, price, or operator policy.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional: true,
				Description: "Origin of a conforming Service Form host. " +
					"May also be set via the " + envEndpoint + " environment variable.",
			},
			"space": schema.StringAttribute{
				Optional: true,
				Validators: []validator.String{
					StringSpaceID(),
				},
				Description: "Default opaque SpaceID for resources that do not set their own. The exact value is preserved. " +
					"May also be set via the " + envSpace + " environment variable.",
			},
			"token": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "Bearer token sent as `Authorization: Bearer <token>`. " +
					"May also be set via the " + envToken + " environment variable.",
			},
		},
	}
}

func (p *takoformProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg takoformProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if cfg.Endpoint.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Unknown Takoform endpoint",
			"The endpoint cannot be determined at configuration time. Set it to a static value "+
				"or via the "+envEndpoint+" environment variable.",
		)
		return
	}
	if cfg.Token.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("token"),
			"Unknown Takoform token",
			"The token cannot be determined at configuration time. Set it to a static value "+
				"or omit it to use the "+envToken+" environment variable.",
		)
	}
	if cfg.Space.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("space"),
			"Unknown Takoform space",
			"The default Space cannot be determined at configuration time. Set it to a static value "+
				"or omit it to use the "+envSpace+" environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	endpoint := firstNonEmpty(cfg.Endpoint.ValueString(), os.Getenv(envEndpoint))
	if endpoint == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("endpoint"),
			"Missing Takoform endpoint",
			"Set the provider `endpoint` attribute or the "+envEndpoint+" environment variable.",
		)
		return
	}

	token := firstNonEmpty(cfg.Token.ValueString(), os.Getenv(envToken))
	space := firstNonEmpty(cfg.Space.ValueString(), os.Getenv(envSpace))
	if space != "" {
		if err := client.ValidateSpaceID(space); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("space"),
				"Invalid Takoform SpaceID",
				fmt.Sprintf("The configured default Space is invalid: %v", err),
			)
			return
		}
	}

	httpClient := newResourceAPIHTTPClient()

	c, err := configureClient(ctx, endpoint, token, httpClient)
	if err != nil {
		resp.Diagnostics.AddError("Takoform configuration failed", err.Error())
		return
	}

	data := &providerData{
		client:       c,
		defaultSpace: space,
		forms:        providerCandidateForms(),
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func newResourceAPIHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultResourceAPITimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Discovery and Resource API endpoints are exact protocol identities.
			// Do not forward a provider bearer token through an HTTP redirect.
			return http.ErrUseLastResponse
		},
	}
}

func (p *takoformProvider) Resources(_ context.Context) []func() resource.Resource {
	resources := make([]func() resource.Resource, 0, len(currentformcatalog.Kinds))
	for _, kind := range currentformcatalog.Kinds {
		resources = append(resources, NewFormResource(kind))
	}
	return resources
}

func (p *takoformProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewInterfaceDataSource,
	}
}

// configureClient builds the client, discovers the versioned Service Form API,
// and enforces the API gate. It is split out from Configure so it can be unit
// tested against an httptest server without driving the full framework.
func configureClient(ctx context.Context, endpoint, token string, httpClient *http.Client) (*client.Client, error) {
	c := client.NewWithOptions(endpoint, token, httpClient, client.Options{})

	disco, err := c.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovering Takoform endpoint %q: %w", endpoint, err)
	}

	if !disco.SupportsServiceForms() {
		return nil, fmt.Errorf(
			"this endpoint does not expose the Takoform Service Form API "+
				"(features.service_forms is not true at %s%s)",
			c.Endpoint(), client.DiscoveryPath,
		)
	}
	if !supportsAPIVersion(disco.APIVersions, client.APIVersion) {
		return nil, fmt.Errorf(
			"this Takoform endpoint does not advertise API version %s (api_versions=%v)",
			client.APIVersion,
			disco.APIVersions,
		)
	}
	return c, nil
}

func providerCandidateForms() map[string]client.InstalledFormReference {
	refs := currentformregistry.All()
	out := make(map[string]client.InstalledFormReference, len(refs))
	for kind, ref := range refs {
		out[kind] = client.InstalledFormReference{
			FormRef: client.FormRef{
				APIVersion: ref.APIVersion, Kind: ref.Kind,
				DefinitionVersion: ref.DefinitionVersion, SchemaDigest: ref.SchemaDigest,
			},
			PackageDigest: ref.PackageDigest,
		}
	}
	return out
}

func supportsAPIVersion(versions []string, want string) bool {
	for _, version := range versions {
		if version == want {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
