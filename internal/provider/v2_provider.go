package provider

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/tako0614/terraform-provider-takoform/internal/hostv2"
	"github.com/tako0614/terraform-provider-takoform/internal/intentv2"
)

// v2Provider is the fresh-install major. The retained takoformProvider above
// remains source for exact historical v1 release tests, but New never chooses
// it or negotiates v1 on a v2 failure.
type v2Provider struct{ version string }

type v2ProviderConfig struct {
	Endpoint    types.String `tfsdk:"endpoint"`
	HostAPIRoot types.String `tfsdk:"host_api_root"`
	Space       types.String `tfsdk:"space"`
	Token       types.String `tfsdk:"token"`
	StorePath   types.String `tfsdk:"store_path"`
	StoreSlot   types.String `tfsdk:"intent_store_slot"`
	StoreID     types.String `tfsdk:"store_id"`
}

type v2ProviderData struct {
	client      *hostv2.Client
	store       *intentv2.Store
	endpoint    string
	hostAPIRoot string
	storeID     string
	space       string
}

var _ provider.Provider = (*v2Provider)(nil)

func (p *v2Provider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "takoform"
	resp.Version = p.version
}

func (p *v2Provider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Takoform Host API v2 Provider for the thirteen exact published Edge Forms. Fresh v2 state only.",
		Attributes: map[string]schema.Attribute{
			"endpoint":          schema.StringAttribute{Required: true, Description: "HTTPS Host origin used only for v2 discovery."},
			"host_api_root":     schema.StringAttribute{Required: true, Description: "Exact v2 API root from Host discovery; must match the pre-initialized intent store binding."},
			"space":             schema.StringAttribute{Optional: true, Description: "Default exact Host Space ID."},
			"token":             schema.StringAttribute{Required: true, Sensitive: true, Description: "Bearer credential for this Host principal. Not persisted in the intent store."},
			"store_path":        schema.StringAttribute{Optional: true, Description: "Absolute path to an explicitly initialized private intent store. Mutually exclusive with intent_store_slot."},
			"intent_store_slot": schema.StringAttribute{Optional: true, Description: "Explicit opaque Runner private-file slot for a remote intent snapshot. Mutually exclusive with store_path."},
			"store_id":          schema.StringAttribute{Required: true, Description: "Fixed caller-scope identifier of that store. A label, not authentication proof."},
		},
	}
}

func (p *v2Provider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg v2ProviderConfig
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for name, value := range map[string]types.String{
		"endpoint": cfg.Endpoint, "host_api_root": cfg.HostAPIRoot,
		"token": cfg.Token, "store_id": cfg.StoreID,
	} {
		if value.IsUnknown() || value.IsNull() || value.ValueString() == "" {
			resp.Diagnostics.AddError("Invalid v2 provider configuration", name+" must be a known, nonempty value before intent store open")
		}
	}
	if cfg.StorePath.IsUnknown() || cfg.StoreSlot.IsUnknown() {
		resp.Diagnostics.AddError("Invalid v2 intent store selection", "store_path and intent_store_slot must be known at configure time")
	} else {
		pathSelected := !cfg.StorePath.IsNull() && cfg.StorePath.ValueString() != ""
		slotSelected := !cfg.StoreSlot.IsNull() && cfg.StoreSlot.ValueString() != ""
		if pathSelected == slotSelected {
			resp.Diagnostics.AddError("Invalid v2 intent store selection", "exactly one of store_path or intent_store_slot must be selected")
		}
	}
	if cfg.Space.IsUnknown() {
		resp.Diagnostics.AddError("Invalid v2 Space", "space must be known at configure time")
	}
	if resp.Diagnostics.HasError() {
		return
	}
	endpoint := cfg.Endpoint.ValueString()
	root := cfg.HostAPIRoot.ValueString()
	if err := validateV2HostURLs(endpoint, root); err != nil {
		resp.Diagnostics.AddError("Invalid v2 Host identity", err.Error())
		return
	}
	space := cfg.Space.ValueString()
	if space != "" && !v2IDPattern.MatchString(space) {
		resp.Diagnostics.AddError("Invalid v2 Space", "space must match the Host API v2 ID grammar")
		return
	}
	var store *intentv2.Store
	var err error
	if !cfg.StorePath.IsNull() && cfg.StorePath.ValueString() != "" {
		storePath := cfg.StorePath.ValueString()
		if !filepath.IsAbs(storePath) || filepath.Clean(storePath) != storePath {
			resp.Diagnostics.AddError("Invalid v2 intent store path", "store_path must be an absolute, clean path to an initialized private store")
			return
		}
		store, err = intentv2.Open(storePath, cfg.StoreID.ValueString())
	} else {
		store, err = intentv2.OpenRemote(ctx, cfg.StoreSlot.ValueString(), cfg.StoreID.ValueString(), intentv2.StoreBinding{HostAPI: root})
	}
	if err != nil {
		resp.Diagnostics.AddError("v2 intent store unavailable", "Open the selected intent store with the matching ID and Host binding; no Host request was sent")
		return
	}
	binding, err := store.Binding(ctx)
	if err != nil || binding.HostAPI != root {
		_ = store.Close()
		resp.Diagnostics.AddError("v2 intent store binding mismatch", "The store must already be bound to the exact configured host_api_root; no Host request was sent")
		return
	}
	data := &v2ProviderData{
		client: hostv2.New(endpoint, cfg.Token.ValueString(), newResourceAPIHTTPClient()),
		store:  store, endpoint: endpoint, hostAPIRoot: root, storeID: cfg.StoreID.ValueString(), space: space,
	}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func validateV2HostURLs(endpoint, root string) error {
	e, err := url.Parse(endpoint)
	if err != nil || e.Scheme != "https" || e.Host == "" || e.User != nil || (e.Path != "" && e.Path != "/") || e.RawQuery != "" || e.ForceQuery || e.Fragment != "" || strings.Contains(endpoint, "#") {
		return fmt.Errorf("endpoint must be an exact HTTPS origin")
	}
	r, err := url.Parse(root)
	if err != nil || r.Scheme != "https" || r.Host != e.Host || r.User != nil || r.RawQuery != "" || r.ForceQuery || r.Fragment != "" || strings.Contains(root, "#") || strings.HasSuffix(root, "/") || root == "" {
		return fmt.Errorf("host_api_root must be an exact same-origin HTTPS base URL without a trailing slash")
	}
	return nil
}

func (p *v2Provider) Resources(context.Context) []func() resource.Resource       { return newV2Resources() }
func (p *v2Provider) DataSources(context.Context) []func() datasource.DataSource { return nil }
