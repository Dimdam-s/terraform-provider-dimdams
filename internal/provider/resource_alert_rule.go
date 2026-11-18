package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type alertRuleResource struct{ client *Client }

type alertRuleModel struct {
	ID             types.String  `tfsdk:"id"`
	HostID         types.String  `tfsdk:"host_id"`
	MetricType     types.String  `tfsdk:"metric_type"`
	ThresholdValue types.Float64 `tfsdk:"threshold_value"`
	DurationSec    types.Int64   `tfsdk:"duration_sec"`
	ChannelType    types.String  `tfsdk:"channel_type"`
	ChannelTarget  types.String  `tfsdk:"channel_target"`
	CooldownMin    types.Int64   `tfsdk:"cooldown_min"`
	Enabled        types.Bool    `tfsdk:"enabled"`
}

type apiAlertRule struct {
	ID             string  `json:"id"`
	MetricType     string  `json:"metricType"`
	ThresholdValue float64 `json:"thresholdValue"`
	DurationSec    int64   `json:"durationSec"`
	ChannelType    string  `json:"channelType"`
	ChannelTarget  string  `json:"channelTarget"`
	CooldownMin    int64   `json:"cooldownMin"`
	Enabled        bool    `json:"enabled"`
}

func NewAlertRuleResource() resource.Resource { return &alertRuleResource{} }

func (r *alertRuleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_rule"
}

func (r *alertRuleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Alerte sur une mesure d'un service (5 au plus par service). Import : `<host_id>/<rule_id>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute(),
			"host_id": hostIDAttribute(),
			"metric_type": schema.StringAttribute{
				Description: "CPU, RAM, DISK ou STATUS (service arrêté).", Required: true,
				Validators: []validator.String{stringvalidator.OneOf("CPU", "RAM", "DISK", "STATUS")},
			},
			"threshold_value": schema.Float64Attribute{Description: "Seuil en % (90 par défaut).", Optional: true, Computed: true, Default: float64default.StaticFloat64(90)},
			"duration_sec":    schema.Int64Attribute{Description: "Durée au-dessus du seuil avant l'alerte, en secondes (600 par défaut).", Optional: true, Computed: true, Default: int64default.StaticInt64(600)},
			"channel_type": schema.StringAttribute{
				Description: "DISCORD_WEBHOOK, EMAIL, IN_APP ou WEB_PUSH.", Required: true,
				Validators: []validator.String{stringvalidator.OneOf("DISCORD_WEBHOOK", "EMAIL", "IN_APP", "WEB_PUSH")},
			},
			"channel_target": schema.StringAttribute{
				Description: "URL du webhook Discord ou adresse e-mail ; inutile pour IN_APP et WEB_PUSH.", Optional: true, Computed: true, Sensitive: true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"cooldown_min": schema.Int64Attribute{Description: "Délai minimal entre deux alertes, en minutes (30 par défaut).", Optional: true, Computed: true, Default: int64default.StaticInt64(30)},
			"enabled":      schema.BoolAttribute{Description: "Alerte active (true par défaut).", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
		},
	}
}

func (r *alertRuleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func alertBody(m alertRuleModel) map[string]any {
	body := map[string]any{
		"metricType":     m.MetricType.ValueString(),
		"thresholdValue": m.ThresholdValue.ValueFloat64(),
		"durationSec":    m.DurationSec.ValueInt64(),
		"channelType":    m.ChannelType.ValueString(),
		"cooldownMin":    m.CooldownMin.ValueInt64(),
		"enabled":        m.Enabled.ValueBool(),
	}
	if !m.ChannelTarget.IsNull() && !m.ChannelTarget.IsUnknown() {
		body["channelTarget"] = m.ChannelTarget.ValueString()
	}
	return body
}

func (m *alertRuleModel) fromAPI(rule apiAlertRule) {
	m.ID = types.StringValue(rule.ID)
	m.MetricType = types.StringValue(rule.MetricType)
	m.ThresholdValue = types.Float64Value(rule.ThresholdValue)
	m.DurationSec = types.Int64Value(rule.DurationSec)
	m.ChannelType = types.StringValue(rule.ChannelType)
	m.ChannelTarget = types.StringValue(rule.ChannelTarget)
	m.CooldownMin = types.Int64Value(rule.CooldownMin)
	m.Enabled = types.BoolValue(rule.Enabled)
}

func (r *alertRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Rule apiAlertRule `json:"rule"`
	}
	host := plan.HostID.ValueString()
	if err := r.client.Do(ctx, http.MethodPost, HostPath(host, "/alerts"), alertBody(plan), &out); err != nil {
		apiErr(&resp.Diagnostics, "Création de l'alerte impossible", err)
		return
	}
	if !plan.Enabled.ValueBool() {
		if err := r.client.Do(ctx, http.MethodPut, HostPath(host, "/alerts/"+out.Rule.ID), map[string]any{"enabled": false}, &out); err != nil {
			apiErr(&resp.Diagnostics, "Désactivation de l'alerte impossible", err)
			return
		}
	}
	plan.fromAPI(out.Rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Rules []apiAlertRule `json:"rules"`
	}
	if err := r.client.Do(ctx, http.MethodGet, HostPath(state.HostID.ValueString(), "/alerts"), nil, &out); err != nil {
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "Lecture des alertes impossible", err)
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

func (r *alertRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Rule apiAlertRule `json:"rule"`
	}
	if err := r.client.Do(ctx, http.MethodPut, HostPath(plan.HostID.ValueString(), "/alerts/"+state.ID.ValueString()), alertBody(plan), &out); err != nil {
		apiErr(&resp.Diagnostics, "Modification de l'alerte impossible", err)
		return
	}
	if out.Rule.ID == "" {
		out.Rule.ID = state.ID.ValueString()
	}
	plan.fromAPI(out.Rule)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodDelete, HostPath(state.HostID.ValueString(), "/alerts/"+state.ID.ValueString()), nil, nil)
	if err != nil && !IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Suppression de l'alerte impossible", err)
	}
}

func (r *alertRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHostChild(ctx, req, resp)
}
