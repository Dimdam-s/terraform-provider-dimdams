package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type apiHost struct {
	ID       string          `json:"id"`
	UniqueID string          `json:"uniqueId"`
	Unique   string          `json:"unique_id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Subtype  string          `json:"subtype"`
	Status   string          `json:"status"`
	Domain   *string         `json:"domain"`
	Config   json.RawMessage `json:"config"`
}

func (h apiHost) id() string {
	for _, v := range []string{h.UniqueID, h.Unique, h.ID} {
		if v != "" {
			return v
		}
	}
	return ""
}

// name follows the CLI: name, then the name kept in the configuration, then the domain.
func (h apiHost) name() string {
	if h.Name != "" {
		return h.Name
	}
	var cfg map[string]any
	raw := h.Config
	var asString string
	if json.Unmarshal(raw, &asString) == nil {
		raw = json.RawMessage(asString)
	}
	if json.Unmarshal(raw, &cfg) == nil {
		for _, k := range []string{"name", "siteName", "botName"} {
			if v, ok := cfg[k].(string); ok && v != "" {
				return v
			}
		}
	}
	if h.Domain != nil && *h.Domain != "" {
		return *h.Domain
	}
	return h.id()
}

type serviceModel struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Type    types.String `tfsdk:"type"`
	Subtype types.String `tfsdk:"subtype"`
	Status  types.String `tfsdk:"status"`
	Domain  types.String `tfsdk:"domain"`
}

func toServiceModel(h apiHost) serviceModel {
	return serviceModel{
		ID: types.StringValue(h.id()), Name: types.StringValue(h.name()), Type: types.StringValue(h.Type),
		Subtype: types.StringValue(h.Subtype), Status: types.StringValue(h.Status), Domain: types.StringPointerValue(h.Domain),
	}
}

func listHosts(ctx context.Context, c *Client) ([]apiHost, error) {
	var out struct {
		Hosts []apiHost `json:"hosts"`
	}
	err := c.Do(ctx, http.MethodGet, "/api/hosts", nil, &out)
	return out.Hosts, err
}

func serviceAttributes(lookup bool) map[string]schema.Attribute {
	attr := func(desc string) schema.StringAttribute {
		return schema.StringAttribute{Description: desc, Computed: true}
	}
	attrs := map[string]schema.Attribute{
		"id": attr("Identifiant du service."), "name": attr("Nom."), "type": attr("Type (web, bot, vps, db, minecraft…)."),
		"subtype": attr("Sous-type (nginx, nodejs, ubuntu…)."), "status": attr("État (running, stopped…)."), "domain": attr("Adresse publique, s'il en a une."),
	}
	if lookup {
		attrs["id"] = schema.StringAttribute{Description: "Identifiant du service (ou name).", Optional: true, Computed: true}
		attrs["name"] = schema.StringAttribute{Description: "Nom du service (ou id), sans tenir compte de la casse.", Optional: true, Computed: true}
	}
	return attrs
}

// ---------- dimdams_services ----------

type servicesDataSource struct{ client *Client }

type servicesModel struct {
	Type     types.String   `tfsdk:"type"`
	Services []serviceModel `tfsdk:"services"`
}

func NewServicesDataSource() datasource.DataSource { return &servicesDataSource{} }

func (d *servicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_services"
}

func (d *servicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Services du compte (et ceux partagés avec lui).",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{Description: "Ne garder qu'un type (web, vps…).", Optional: true},
			"services": schema.ListNestedAttribute{
				Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: serviceAttributes(false)},
			},
		},
	}
}

func (d *servicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (d *servicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state servicesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	hosts, err := listHosts(ctx, d.client)
	if err != nil {
		apiErr(&resp.Diagnostics, "Lecture des services impossible", err)
		return
	}
	state.Services = []serviceModel{}
	for _, h := range hosts {
		if state.Type.IsNull() || h.Type == state.Type.ValueString() {
			state.Services = append(state.Services, toServiceModel(h))
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// ---------- dimdams_service ----------

type serviceDataSource struct{ client *Client }

func NewServiceDataSource() datasource.DataSource { return &serviceDataSource{} }

func (d *serviceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service"
}

func (d *serviceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Un service, trouvé par son identifiant ou son nom.", Attributes: serviceAttributes(true)}
}

func (d *serviceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

// FindService picks one service by id or (case-insensitive) name; an ambiguous name is refused.
func FindService(hosts []apiHost, id, name string) (apiHost, error) {
	var matches []apiHost
	for _, h := range hosts {
		if (id != "" && h.id() == id) || (id == "" && strings.EqualFold(h.name(), name)) {
			matches = append(matches, h)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return apiHost{}, fmt.Errorf("aucun service %q", id+name)
	default:
		return apiHost{}, fmt.Errorf("%d services s'appellent %q : utilisez id", len(matches), name)
	}
}

func (d *serviceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg serviceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.ID.IsNull() == cfg.Name.IsNull() {
		resp.Diagnostics.AddError("Service à préciser", "Renseignez id ou name (un seul des deux).")
		return
	}
	hosts, err := listHosts(ctx, d.client)
	if err != nil {
		apiErr(&resp.Diagnostics, "Lecture des services impossible", err)
		return
	}
	host, err := FindService(hosts, cfg.ID.ValueString(), cfg.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Service introuvable", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, toServiceModel(host))...)
}
