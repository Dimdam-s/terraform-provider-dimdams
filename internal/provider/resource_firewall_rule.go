package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type firewallRuleResource struct{ client *Client }

type firewallRuleModel struct {
	ID          types.String `tfsdk:"id"`
	HostID      types.String `tfsdk:"host_id"`
	Direction   types.String `tfsdk:"direction"`
	Action      types.String `tfsdk:"action"`
	Protocol    types.String `tfsdk:"protocol"`
	PortRange   types.String `tfsdk:"port_range"`
	SourceCIDR  types.String `tfsdk:"source_cidr"`
	Description types.String `tfsdk:"description"`
	Priority    types.Int64  `tfsdk:"priority"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

type apiFirewallRule struct {
	ID          string  `json:"id"`
	Direction   string  `json:"direction"`
	Action      string  `json:"action"`
	Protocol    string  `json:"protocol"`
	PortRange   *string `json:"portRange"`
	SourceCIDR  string  `json:"sourceCidr"`
	Description *string `json:"description"`
	Priority    int64   `json:"priority"`
	Enabled     bool    `json:"enabled"`
}

func NewFirewallRuleResource() resource.Resource { return &firewallRuleResource{} }

func (r *firewallRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firewall_rule"
}

func (r *firewallRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Règle du pare-feu réseau d'un VPS, d'un site ou d'un serveur de jeu. Import : `<host_id>/<rule_id>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute(),
			"host_id": hostIDAttribute(),
			"direction": schema.StringAttribute{
				Description: "INBOUND (par défaut) ou OUTBOUND.", Optional: true, Computed: true,
				Default:    stringdefault.StaticString("INBOUND"),
				Validators: []validator.String{stringvalidator.OneOf("INBOUND", "OUTBOUND")},
			},
			"action": schema.StringAttribute{
				Description: "ACCEPT (par défaut) ou DROP.", Optional: true, Computed: true,
				Default:    stringdefault.StaticString("ACCEPT"),
				Validators: []validator.String{stringvalidator.OneOf("ACCEPT", "DROP")},
			},
			"protocol": schema.StringAttribute{
				Description: "TCP (par défaut), UDP, ICMP ou ANY.", Optional: true, Computed: true,
				Default:    stringdefault.StaticString("TCP"),
				Validators: []validator.String{stringvalidator.OneOf("TCP", "UDP", "ICMP", "ANY")},
			},
			"port_range": schema.StringAttribute{
				Description: "Port (80) ou plage (3000-3050). Absent : tous les ports. Interdit avec ICMP et ANY.", Optional: true,
			},
			"source_cidr": schema.StringAttribute{
				Description: "Adresse ou réseau IPv4 d'origine (0.0.0.0/0 par défaut).", Optional: true, Computed: true,
				Default: stringdefault.StaticString("0.0.0.0/0"),
			},
			"description": schema.StringAttribute{Description: "Note libre (200 caractères au plus).", Optional: true},
			"priority": schema.Int64Attribute{
				Description: "1 à 1000 ; la plus petite s'applique en premier (100 par défaut).", Optional: true, Computed: true,
				Default:    int64default.StaticInt64(100),
				Validators: []validator.Int64{int64validator.Between(1, 1000)},
			},
			"enabled": schema.BoolAttribute{Description: "Règle active (true par défaut).", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		},
	}
}

func (r *firewallRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func firewallBody(m firewallRuleModel) map[string]any {
	body := map[string]any{
		"direction":   m.Direction.ValueString(),
		"action":      m.Action.ValueString(),
		"protocol":    m.Protocol.ValueString(),
		"sourceCidr":  m.SourceCIDR.ValueString(),
		"priority":    m.Priority.ValueInt64(),
		"enabled":     m.Enabled.ValueBool(),
		"portRange":   nil,
		"description": nil,
	}
	if !m.PortRange.IsNull() {
		body["portRange"] = m.PortRange.ValueString()
	}
	if !m.Description.IsNull() {
		body["description"] = m.Description.ValueString()
	}
	return body
}

func (m *firewallRuleModel) fromAPI(rule apiFirewallRule) {
	m.ID = types.StringValue(rule.ID)
	m.Direction = types.StringValue(rule.Direction)
	m.Action = types.StringValue(rule.Action)
	m.Protocol = types.StringValue(rule.Protocol)
	m.PortRange = types.StringPointerValue(rule.PortRange)
	m.SourceCIDR = types.StringValue(rule.SourceCIDR)
	m.Description = types.StringPointerValue(rule.Description)
	m.Priority = types.Int64Value(rule.Priority)
	m.Enabled = types.BoolValue(rule.Enabled)
}

func (r *firewallRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Rule apiFirewallRule `json:"rule"`
	}
	host := plan.HostID.ValueString()
	if err := r.client.Do(ctx, http.MethodPost, HostPath(host, "/firewall"), firewallBody(plan), &out); err != nil {
		apiErr(&resp.Diagnostics, "Création de la règle impossible", err)
		return
	}
	// The creation always enables the rule: apply "enabled = false" right after.
	if !plan.Enabled.ValueBool() {
		if err := r.client.Do(ctx, http.MethodPut, HostPath(host, "/firewall/"+out.Rule.ID), map[string]any{"enabled": false}, &out); err != nil {
			apiErr(&resp.Diagnostics, "Désactivation de la règle impossible", err)
			return
		}
	}
	plan.fromAPI(out.Rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *firewallRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Rules []apiFirewallRule `json:"rules"`
	}
	if err := r.client.Do(ctx, http.MethodGet, HostPath(state.HostID.ValueString(), "/firewall"), nil, &out); err != nil {
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "Lecture du pare-feu impossible", err)
		return
	}
	for _, rule := range out.Rules {
		if rule.ID == state.ID.ValueString() {
			state.fromAPI(rule)
			resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *firewallRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state firewallRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Rule apiFirewallRule `json:"rule"`
	}
	if err := r.client.Do(ctx, http.MethodPut, HostPath(plan.HostID.ValueString(), "/firewall/"+state.ID.ValueString()), firewallBody(plan), &out); err != nil {
		apiErr(&resp.Diagnostics, "Modification de la règle impossible", err)
		return
	}
	if out.Rule.ID == "" {
		out.Rule.ID = state.ID.ValueString()
	}
	plan.fromAPI(out.Rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *firewallRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firewallRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, HostPath(state.HostID.ValueString(), "/firewall/"+state.ID.ValueString()), nil, nil)
	if err != nil && !IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Suppression de la règle impossible", err)
	}
}

func (r *firewallRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHostChild(ctx, req, resp)
}

// importHostChild reads an import id of the form "<host_id>/<id>".
func importHostChild(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	host, id, ok := strings.Cut(req.ID, "/")
	if !ok || host == "" || id == "" {
		resp.Diagnostics.AddError("Identifiant d'import invalide", fmt.Sprintf("attendu <host_id>/<id>, reçu %q", req.ID))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_id"), host)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
