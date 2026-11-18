package provider

import (
	"context"
	"net/http"
	"sort"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// maskedValue is what the API returns for a secret the caller may not read in clear text. Sending it
// back keeps the real value on the server.
const maskedValue = "••••••••"

type environmentResource struct{ client *Client }

type environmentModel struct {
	ID        types.String `tfsdk:"id"`
	HostID    types.String `tfsdk:"host_id"`
	Variables types.Map    `tfsdk:"variables"`
	Secrets   types.Map    `tfsdk:"secrets"`
}

type apiEnvVar struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"isSecret"`
}

func NewEnvironmentResource() resource.Resource { return &environmentResource{} }

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Toutes les variables d'environnement d'un site ou d'un bot. La ressource fait autorité : une variable absente " +
			"de la configuration est supprimée. Redémarrez le service pour appliquer les changements. Import : `<host_id>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute(),
			"host_id": hostIDAttribute(),
			"variables": schema.MapAttribute{
				Description: "Variables ordinaires, visibles dans le tableau de bord.", ElementType: types.StringType, Optional: true,
			},
			"secrets": schema.MapAttribute{
				Description: "Variables secrètes : masquées dans le tableau de bord et dans le plan Terraform.", ElementType: types.StringType, Optional: true, Sensitive: true,
			},
		},
	}
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func stringMap(ctx context.Context, m types.Map) (map[string]string, bool) {
	out := map[string]string{}
	if m.IsNull() || m.IsUnknown() {
		return out, true
	}
	return out, !m.ElementsAs(ctx, &out, false).HasError()
}

// envList builds the list sent to the API, sorted for a stable order.
func envList(vars, secrets map[string]string) []apiEnvVar {
	list := make([]apiEnvVar, 0, len(vars)+len(secrets))
	for k, v := range vars {
		list = append(list, apiEnvVar{Key: k, Value: v})
	}
	for k, v := range secrets {
		list = append(list, apiEnvVar{Key: k, Value: v, IsSecret: true})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	return list
}

// splitEnv sorts what the API returned into the two maps. A key stays in the map where the
// configuration put it (the API flags names like *_TOKEN as secret on its own), and a masked value
// keeps the value already known.
func splitEnv(remote []apiEnvVar, prevVars, prevSecrets map[string]string) (map[string]string, map[string]string) {
	vars, secrets := map[string]string{}, map[string]string{}
	for _, item := range remote {
		_, wasVar := prevVars[item.Key]
		_, wasSecret := prevSecrets[item.Key]
		value := item.Value
		if value == maskedValue {
			if v, ok := prevSecrets[item.Key]; ok {
				value = v
			} else if v, ok := prevVars[item.Key]; ok {
				value = v
			}
		}
		switch {
		case wasVar:
			vars[item.Key] = value
		case wasSecret || item.IsSecret:
			secrets[item.Key] = value
		default:
			vars[item.Key] = value
		}
	}
	return vars, secrets
}

func mapValue(m map[string]string, keepNull bool) types.Map {
	if len(m) == 0 && keepNull {
		return types.MapNull(types.StringType)
	}
	values := map[string]types.String{}
	for k, v := range m {
		values[k] = types.StringValue(v)
	}
	out, _ := types.MapValueFrom(context.Background(), types.StringType, values)
	return out
}

func (r *environmentResource) write(ctx context.Context, plan *environmentModel) error {
	vars, _ := stringMap(ctx, plan.Variables)
	secrets, _ := stringMap(ctx, plan.Secrets)
	return r.client.Do(ctx, http.MethodPut, HostPath(plan.HostID.ValueString(), "/env"), map[string]any{"env": envList(vars, secrets)}, nil)
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan); err != nil {
		apiErr(&resp.Diagnostics, "Enregistrement des variables impossible", err)
		return
	}
	plan.ID = plan.HostID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if state.HostID.IsNull() {
		state.HostID = state.ID
	}
	var out struct {
		Env []apiEnvVar `json:"env"`
	}
	if err := r.client.Do(ctx, http.MethodGet, HostPath(state.HostID.ValueString(), "/env"), nil, &out); err != nil {
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "Lecture des variables impossible", err)
		return
	}
	prevVars, _ := stringMap(ctx, state.Variables)
	prevSecrets, _ := stringMap(ctx, state.Secrets)
	vars, secrets := splitEnv(out.Env, prevVars, prevSecrets)
	state.ID = state.HostID
	state.Variables = mapValue(vars, state.Variables.IsNull())
	state.Secrets = mapValue(secrets, state.Secrets.IsNull())
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.write(ctx, &plan); err != nil {
		apiErr(&resp.Diagnostics, "Enregistrement des variables impossible", err)
		return
	}
	plan.ID = plan.HostID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodPut, HostPath(state.HostID.ValueString(), "/env"), map[string]any{"env": []apiEnvVar{}}, nil)
	if err != nil && !IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Suppression des variables impossible", err)
	}
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHost(ctx, req, resp)
}
