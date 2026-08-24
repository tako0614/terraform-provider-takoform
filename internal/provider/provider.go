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
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/clientv3"
)

// Environment variable fallbacks for provider configuration.
const (
	envEndpoint = "TAKOFORM_ENDPOINT"
	envSpace    = "TAKOFORM_SPACE"
	envToken    = "TAKOFORM_TOKEN"

	defaultResourceAPITimeout = 12 * time.Minute

	// planPreviewTimeout bounds the read-only host preview performed during
	// `terraform plan` so a slow host cannot stall planning; apply performs
	// the authoritative review under defaultResourceAPITimeout.
	planPreviewTimeout = 30 * time.Second

	// discoveryTimeout bounds ONE lane negotiation. It is deliberately short and
	// deliberately separate from defaultResourceAPITimeout: discovery is a single
	// small read of a static document, while a resource operation may legitimately
	// take minutes. Sharing the long timeout meant an unresponsive lane held the
	// whole provider — and therefore the OTHER lane's resources — for twelve
	// minutes before anything could run (spec/decisions/0018).
	discoveryTimeout = 15 * time.Second

	// Host resource mutations acknowledge work before the provider polls the
	// resulting operation. Bound the time spent waiting for response headers so
	// a stalled host cannot hold an OpenTofu apply for the full
	// resource-operation timeout.
	resourceAPIResponseHeaderTimeout = 30 * time.Second
)

// Ensure takoformProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*takoformProvider)(nil)

// takoformProvider is the provider implementation.
type takoformProvider struct {
	// version is set at build time and surfaced to Terraform.
	version string
}

// providerData is shared with every resource via Configure. It carries the one
// negotiated Host API lane this build speaks, forms.takoform.com/v1. The
// retained v1alpha2 lane rode beside it through provider v2.x and was
// withdrawn with its epoch; existing v2.x installations keep working because a
// released provider is self-contained (decision 0037).
type providerData struct {
	clientV3     *clientv3.Client
	v3Err        error
	defaultSpace string
	// support caches the stable v1 Host Support Profiles the plan decides
	// against. It is per provider configuration because a profile is a static
	// statement about one host, and a plan asks the same questions once per
	// resource (v3_host_support.go).
	support *v3SupportCache
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
		if err := clientv3.ValidateSpaceID(space); err != nil {
			resp.Diagnostics.AddAttributeError(
				path.Root("space"),
				"Invalid Takoform SpaceID",
				fmt.Sprintf("The configured default Space is invalid: %v", err),
			)
			return
		}
	}

	httpClient := newResourceAPIHTTPClient()

	// Negotiate the one Host API lane this build speaks, under a short
	// dedicated discovery deadline so an unresponsive endpoint cannot hold the
	// provider for the resource-operation timeout (spec/decisions/0018). The
	// error is recorded rather than fatal here: each resource asserts the lane
	// and reports the recorded negotiation error with its own diagnostics.
	v3Client, v3Err := negotiateLane(ctx, endpoint, token, httpClient, discoveryTimeout)

	data := &providerData{
		clientV3:     v3Client,
		v3Err:        v3Err,
		defaultSpace: space,
		support:      newV3SupportCache(),
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func newResourceAPIHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = resourceAPIResponseHeaderTimeout

	return &http.Client{
		Transport: transport,
		Timeout:   defaultResourceAPITimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// Discovery and Resource API endpoints are exact protocol identities.
			// Do not forward a provider bearer token through an HTTP redirect.
			return http.ErrUseLastResponse
		},
	}
}

func (p *takoformProvider) Resources(_ context.Context) []func() resource.Resource {
	// Exactly the 31 typed current-family resources, one for each Form in the
	// generated current-family index. There is no generic carrier, and the
	// retained Provider 2.1.1 identities are not registered in this major line.
	return newV3FormResources()
}

func (p *takoformProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	// None. The takoform_interface data source spoke the withdrawn v1alpha2
	// lane and was retired with it.
	return nil
}

// configureClientV3 negotiates the stable Host API v1 lane against the same
// endpoint and token. The stable discovery contract is strict (closed
// api_versions, required features, same-origin endpoints), so a successful
// Discover is the whole gate.
func negotiateLane(
	ctx context.Context,
	endpoint, token string,
	httpClient *http.Client,
	timeout time.Duration,
) (*clientv3.Client, error) {
	discoveryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return configureClientV3(discoveryCtx, endpoint, token, httpClient)
}

func configureClientV3(ctx context.Context, endpoint, token string, httpClient *http.Client) (*clientv3.Client, error) {
	c := clientv3.NewWithOptions(endpoint, token, httpClient, clientv3.Options{})
	if _, err := c.Discover(ctx); err != nil {
		return nil, fmt.Errorf("discovering Takoform v1 endpoint %q: %w", endpoint, err)
	}
	return c, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
