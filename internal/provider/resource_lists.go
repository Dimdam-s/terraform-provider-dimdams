package provider

import (
	"context"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func importHost(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host_id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

// ---------- SSH keys ----------

type sshKeysResource struct{ client *Client }

type sshKeyModel struct {
	Name      types.String `tfsdk:"name"`
	PublicKey types.String `tfsdk:"public_key"`
}

type sshKeysModel struct {
	ID     types.String  `tfsdk:"id"`
	HostID types.String  `tfsdk:"host_id"`
	Keys   []sshKeyModel `tfsdk:"key"`
}

type apiSSHKey struct {
	ID        string `json:"id,omitempty"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	CreatedAt int64  `json:"createdAt,omitempty"`
}

func NewSSHKeysResource() resource.Resource { return &sshKeysResource{} }

func (r *sshKeysResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ssh_keys"
}

func (r *sshKeysResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Clés SSH d'un VPS ajoutées depuis Dimdam's (20 au plus). Les clés ajoutées à la main dans le VPS ne sont pas touchées. " +
			"Demande la permission hosts:exec. Import : `<host_id>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute(),
			"host_id": hostIDAttribute(),
		},
		Blocks: map[string]schema.Block{
			"key": schema.ListNestedBlock{
				Validators: []validator.List{listvalidator.SizeAtMost(20)},
				NestedObject: schema.NestedBlockObject{Attributes: map[string]schema.Attribute{
					"name":       schema.StringAttribute{Description: "Nom de la clé (64 caractères au plus).", Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 64)}},
					"public_key": schema.StringAttribute{Description: "Clé publique OpenSSH sur une ligne (ssh-ed25519 AAAA… commentaire).", Required: true},
				}},
			},
		},
	}
}

func (r *sshKeysResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *sshKeysResource) put(ctx context.Context, m sshKeysModel) error {
	keys := make([]apiSSHKey, 0, len(m.Keys))
	for _, k := range m.Keys {
		keys = append(keys, apiSSHKey{Name: k.Name.ValueString(), PublicKey: k.PublicKey.ValueString()})
	}
	return r.client.Do(ctx, http.MethodPut, HostPath(m.HostID.ValueString(), "/ssh-keys"), map[string]any{"keys": keys}, nil)
}

func (r *sshKeysResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan sshKeysModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		apiErr(&resp.Diagnostics, "Enregistrement des clés SSH impossible", err)
		return
	}
	plan.ID = plan.HostID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *sshKeysResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state sshKeysModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var out struct {
		Keys []apiSSHKey `json:"keys"`
	}
	if err := r.client.Do(ctx, http.MethodGet, HostPath(state.HostID.ValueString(), "/ssh-keys"), nil, &out); err != nil {
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "Lecture des clés SSH impossible", err)
		return
	}
	state.ID = state.HostID
	state.Keys = nil
	for _, k := range out.Keys {
		state.Keys = append(state.Keys, sshKeyModel{Name: types.StringValue(k.Name), PublicKey: types.StringValue(k.PublicKey)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *sshKeysResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan sshKeysModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		apiErr(&resp.Diagnostics, "Enregistrement des clés SSH impossible", err)
		return
	}
	plan.ID = plan.HostID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *sshKeysResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state sshKeysModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Keys = nil
	if err := r.put(ctx, state); err != nil && !IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Suppression des clés SSH impossible", err)
	}
}

func (r *sshKeysResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHost(ctx, req, resp)
}

// ---------- Scheduled tasks ----------

type scheduledTasksResource struct{ client *Client }

type taskModel struct {
	Name    types.String `tfsdk:"name"`
	Action  types.String `tfsdk:"action"`
	Command types.String `tfsdk:"command"`
	Time    types.String `tfsdk:"time"`
	Enabled types.Bool   `tfsdk:"enabled"`
}

type scheduledTasksModel struct {
	ID     types.String `tfsdk:"id"`
	HostID types.String `tfsdk:"host_id"`
	Tasks  []taskModel  `tfsdk:"task"`
}

type apiTask struct {
	ID      string  `json:"id,omitempty"`
	Name    string  `json:"name"`
	Action  string  `json:"action"`
	Command *string `json:"command,omitempty"`
	Time    string  `json:"time"`
	Enabled bool    `json:"enabled"`
}

func NewScheduledTasksResource() resource.Resource { return &scheduledTasksResource{} }

func (r *scheduledTasksResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_scheduled_tasks"
}

func (r *scheduledTasksResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Toutes les tâches planifiées d'un service (10 au plus), chaque jour à l'heure de Paris. Une commande demande la permission hosts:exec. Import : `<host_id>`.",
		Attributes: map[string]schema.Attribute{
			"id":      idAttribute(),
			"host_id": hostIDAttribute(),
		},
		Blocks: map[string]schema.Block{
			"task": schema.ListNestedBlock{
				Validators: []validator.List{listvalidator.SizeAtMost(10)},
				NestedObject: schema.NestedBlockObject{Attributes: map[string]schema.Attribute{
					"name":    schema.StringAttribute{Description: "Nom, unique parmi les tâches du service.", Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 64)}},
					"action":  schema.StringAttribute{Description: "restart, backup ou command.", Required: true, Validators: []validator.String{stringvalidator.OneOf("restart", "backup", "command")}},
					"command": schema.StringAttribute{Description: "Commande pour action = command (une ligne, 512 caractères au plus).", Optional: true},
					"time":    schema.StringAttribute{Description: "Heure de Paris, HH:MM.", Required: true, Validators: []validator.String{stringvalidator.RegexMatches(timePattern, "format HH:MM")}},
					"enabled": schema.BoolAttribute{Description: "Tâche active (true par défaut).", Optional: true, Computed: true, Default: booldefault.StaticBool(true)},
				}},
			},
		},
	}
}

func (r *scheduledTasksResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = configureClient(req.ProviderData, &resp.Diagnostics)
}

func (r *scheduledTasksResource) get(ctx context.Context, host string) ([]apiTask, error) {
	var out struct {
		Tasks []apiTask `json:"tasks"`
	}
	err := r.client.Do(ctx, http.MethodGet, HostPath(host, "/scheduler"), nil, &out)
	return out.Tasks, err
}

// put sends the tasks; a task whose name already exists keeps its id, so the server sees it as kept
// (same author) rather than removed and re-created.
func (r *scheduledTasksResource) put(ctx context.Context, m scheduledTasksModel) error {
	host := m.HostID.ValueString()
	current, err := r.get(ctx, host)
	if err != nil && !IsNotFound(err) {
		return err
	}
	ids := map[string]string{}
	for _, t := range current {
		ids[t.Name] = t.ID
	}
	tasks := make([]apiTask, 0, len(m.Tasks))
	for _, t := range m.Tasks {
		task := apiTask{ID: ids[t.Name.ValueString()], Name: t.Name.ValueString(), Action: t.Action.ValueString(), Time: t.Time.ValueString(), Enabled: t.Enabled.ValueBool()}
		if !t.Command.IsNull() {
			cmd := t.Command.ValueString()
			task.Command = &cmd
		}
		tasks = append(tasks, task)
	}
	return r.client.Do(ctx, http.MethodPut, HostPath(host, "/scheduler"), map[string]any{"tasks": tasks}, nil)
}

func (r *scheduledTasksResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan scheduledTasksModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		apiErr(&resp.Diagnostics, "Enregistrement des tâches impossible", err)
		return
	}
	plan.ID = plan.HostID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scheduledTasksResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state scheduledTasksModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tasks, err := r.get(ctx, state.HostID.ValueString())
	if err != nil {
		if IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		apiErr(&resp.Diagnostics, "Lecture des tâches impossible", err)
		return
	}
	state.ID = state.HostID
	state.Tasks = nil
	for _, t := range tasks {
		state.Tasks = append(state.Tasks, taskModel{
			Name: types.StringValue(t.Name), Action: types.StringValue(t.Action), Command: types.StringPointerValue(t.Command),
			Time: types.StringValue(t.Time), Enabled: types.BoolValue(t.Enabled),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *scheduledTasksResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan scheduledTasksModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		apiErr(&resp.Diagnostics, "Enregistrement des tâches impossible", err)
		return
	}
	plan.ID = plan.HostID
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *scheduledTasksResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state scheduledTasksModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client.Do(ctx, http.MethodPut, HostPath(state.HostID.ValueString(), "/scheduler"), map[string]any{"tasks": []apiTask{}}, nil)
	if err != nil && !IsNotFound(err) {
		apiErr(&resp.Diagnostics, "Suppression des tâches impossible", err)
	}
}

func (r *scheduledTasksResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importHost(ctx, req, resp)
}
