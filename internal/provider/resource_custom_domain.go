package provider

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var timePattern = regexp.MustCompile(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`)

type customDomainResource struct{ client *Client }

type customDomainModel struct {
	ID         types.String `tfsdk:"id"`
	HostID     types.String `tfsdk:"host_id"`
	Domain     types.String `tfsdk:"domain"`
	TargetPort types.Int64  `tfsdk:"target_port"`
	SSLActive  types.Bool   `tfsdk:"ssl_active"`
}

func NewCustomDomainResource() resource.Resource { return &customDomainResource{} }

func (r *customDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_domain"
}

func (r *customDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Domaine personnel d'un site (un par service). Le domaine doit déjà pointer vers Dimdam's : le panel vérifie le DNS avant de l'accepter. Import : `<host_id>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute(),
			"host_id": hostIDAttribute(),
			"domain": schema.StringAttribute{
				Description: "Nom de domaine, par exemple www.example.com.", Required: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"target_port": schema.Int64Attribute{
				Description: "Port du service visé (port principal par défaut).", Optional: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"ssl_active": schema.BoolAttribute{Description: "Certificat HTTPS en place.", Computed: true},
		},
	}
}

func (r *customDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *customDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"domain": plan.Domain.ValueString()}
	if !plan.TargetPort.IsNull() {
		body["targetPort"] = plan.TargetPort.ValueInt64()
	}
	if err := r.client.Do(ctx, http.MethodPost, HostPath(plan.HostID.ValueString(), "/custom-domain"), body, nil); err != nil {
		apiErr(&resp.Diagnostics, "Ajout du domaine impossible", err)
		return
	}
	plan.ID = plan.HostID
	plan.SSLActive = types.BoolValue(false)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *customDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state customDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		CustomDomain *string `json:"customDomain"`
		SSLActive    bool    `json:"sslActive"`
	}
	if err := r.client.Do(ctx, http.MethodGet, HostPath(state.HostID.ValueString(), "/custom-domain"), nil, &out); err != nil {
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "Lecture du domaine impossible", err)
		return
	}
	if out.CustomDomain == nil || *out.CustomDomain == "" {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = state.HostID
	if !strings.EqualFold(state.Domain.ValueString(), *out.CustomDomain) {
		state.Domain = types.StringValue(*out.CustomDomain)
	}
	state.SSLActive = types.BoolValue(out.SSLActive)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is never called: every attribute recreates the resource.
func (r *customDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan customDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *customDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state customDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, HostPath(state.HostID.ValueString(), "/custom-domain"), nil, nil)
	if err != nil && !IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Suppression du domaine impossible", err)
	}
}

func (r *customDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHost(ctx, req, resp)
}
