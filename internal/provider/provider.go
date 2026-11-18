package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const defaultAPIURL = "https://www.dimdams.com"

type dimdamsProvider struct{ version string }

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	APIURL types.String `tfsdk:"api_url"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider { return &dimdamsProvider{version: version} }
}

func (p *dimdamsProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "dimdams"
	resp.Version = p.version
}

func (p *dimdamsProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Gère la configuration des services Dimdam's existants avec une clé d'API.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Description: "Clé d'API (dim_live_…). Par défaut : variable d'environnement DIMDAMS_API_KEY.",
				Optional:    true,
				Sensitive:   true,
			},
			"api_url": schema.StringAttribute{
				Description: "Adresse du panel. Par défaut : DIMDAMS_API_URL, sinon " + defaultAPIURL + ".",
				Optional:    true,
			},
		},
	}
}

func (p *dimdamsProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key := os.Getenv("DIMDAMS_API_KEY")
	if !cfg.APIKey.IsNull() && !cfg.APIKey.IsUnknown() {
		key = cfg.APIKey.ValueString()
	}
	apiURL := os.Getenv("DIMDAMS_API_URL")
	if !cfg.APIURL.IsNull() && !cfg.APIURL.IsUnknown() {
		apiURL = cfg.APIURL.ValueString()
	}
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	if key == "" {
		resp.Diagnostics.AddAttributeError(path.Root("api_key"), "Clé d'API manquante",
			"Renseignez api_key ou la variable d'environnement DIMDAMS_API_KEY (clé créée dans Paramètres > Clés API).")
		return
	}
	client, err := NewClient(apiURL, key, p.version)
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("api_url"), "Adresse du panel invalide", err.Error())
		return
	}
	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *dimdamsProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFirewallRuleResource,
		NewAlertRuleResource,
		NewEnvironmentResource,
		NewSSHKeysResource,
		NewScheduledTasksResource,
		NewCustomDomainResource,
	}
}

func (p *dimdamsProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewServicesDataSource,
		NewServiceDataSource,
	}
}
