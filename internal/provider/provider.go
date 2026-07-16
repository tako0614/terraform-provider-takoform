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
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/client"
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
	capabilities      client.ProductCapabilities
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
		Description: "The Takoform provider exposes ten statically typed Service Form resources " +
			"through any conforming form host. It never selects a backend, target, credential, price, or operator policy.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional: true,
				Description: "Origin of a conforming Service Form host. " +
					"May also be set via the " + envEndpoint + " environment variable.",
			},
			"space": schema.StringAttribute{
				Optional: true,
				Description: "Default Space for resources that do not set their own. " +
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

	httpClient := newResourceAPIHTTPClient()

	c, err := configureClient(ctx, endpoint, token, httpClient)
	if err != nil {
		resp.Diagnostics.AddError("Takoform configuration failed", err.Error())
		return
	}

	data := &providerData{
		client:       c,
		defaultSpace: space,
		capabilities: c.Capabilities,
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func newResourceAPIHTTPClient() *http.Client {
	return &http.Client{Timeout: defaultResourceAPITimeout}
}

func (p *takoformProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewEdgeWorkerResource,
		NewObjectBucketResource,
		NewKVStoreResource,
		NewQueueResource,
		NewSQLDatabaseResource,
		NewContainerServiceResource,
		NewVectorIndexResource,
		NewDurableWorkflowResource,
		NewStatefulActorNamespaceResource,
		NewScheduleResource,
	}
}

func (p *takoformProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

// configureClient builds the client, discovers capabilities, and enforces the
// Service Form API gate. It is split out from Configure so it can be unit
// tested against an httptest server without driving the full framework.
func configureClient(ctx context.Context, endpoint, token string, httpClient *http.Client) (*client.Client, error) {
	c := client.New(endpoint, token, httpClient)

	disco, err := c.Discover(ctx)
	if err != nil {
		return nil, fmt.Errorf("discovering Takoform endpoint %q: %w", endpoint, err)
	}

	if !disco.SupportsServiceForms() {
		return nil, fmt.Errorf(
			"this endpoint does not expose the Takoform Service Form API "+
				"(features.service_forms is not true at %s/.well-known/takoform)",
			c.Endpoint(),
		)
	}
	if !supportsAPIVersion(disco.APIVersions, client.APIVersion) {
		return nil, fmt.Errorf(
			"this Takoform endpoint does not advertise API version %s (api_versions=%v)",
			client.APIVersion,
			disco.APIVersions,
		)
	}
	caps, err := c.GetCapabilities(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading Takoform capabilities from %q: %w", endpoint, err)
	}
	if caps.APIVersion != client.APIVersion {
		return nil, fmt.Errorf(
			"this Takoform endpoint returned unsupported capabilities apiVersion %q (expected %q)",
			caps.APIVersion,
			client.APIVersion,
		)
	}

	return c, nil
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
