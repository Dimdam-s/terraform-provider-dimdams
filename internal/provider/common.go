package provider

import (
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
)

// configureClient extracts the API client given by the provider.
func configureClient(data any, diags *diag.Diagnostics) *Client {
	if data == nil {
		return nil // provider not configured yet (validation)
	}
	client, ok := data.(*Client)
	if !ok {
		diags.AddError("Configuration inattendue", fmt.Sprintf("client de type %T", data))
		return nil
	}
	return client
}

// hostIDAttribute is the service a resource belongs to; changing it recreates the resource.
func hostIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Description:   "Identifiant du service (par exemple vps-1793740523964-d3bw0r).",
		Required:      true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

func idAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		Computed:      true,
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
	}
}

func apiErr(diags *diag.Diagnostics, what string, err error) {
	diags.AddError(what, err.Error())
}

