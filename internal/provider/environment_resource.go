package provider

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*environmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*environmentResource)(nil)
	_ resource.ResourceWithImportState = (*environmentResource)(nil)
)

func NewEnvironmentResource() resource.Resource {
	return &environmentResource{}
}

type environmentResource struct {
	clients *Clients
}

type environmentModel struct {
	ID                         types.String `tfsdk:"id"`
	Name                       types.String `tfsdk:"name"`
	Queries                    types.List   `tfsdk:"queries"`
	IsProduction               types.Bool   `tfsdk:"is_production"`
	RequireFeatureFlagApproval types.Bool   `tfsdk:"require_feature_flag_approval"`
	Description                types.String `tfsdk:"description"`
	CreatedAt                  types.String `tfsdk:"created_at"`
	UpdatedAt                  types.String `tfsdk:"updated_at"`
}

func (r *environmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (r *environmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Datadog feature flag environment.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID assigned by Datadog.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name shown in the Datadog UI.",
				Required:            true,
			},
			"queries": schema.ListAttribute{
				MarkdownDescription: "Datadog tag queries that define which traffic this environment captures. At least one entry is required.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"is_production": schema.BoolAttribute{
				MarkdownDescription: "Marks the environment as production. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"require_feature_flag_approval": schema.BoolAttribute{
				MarkdownDescription: "Requires a reviewer to approve every feature flag change in this environment. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description shown alongside the environment in the Datadog UI. Read-only via this API.",
				Computed:            true,
			},
			"created_at": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"updated_at": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (r *environmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if clients := configureClients(req.ProviderData, &resp.Diagnostics); clients != nil {
		r.clients = clients
	}
}

func (r *environmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	queries := make([]string, 0, len(plan.Queries.Elements()))
	resp.Diagnostics.Append(plan.Queries.ElementsAs(ctx, &queries, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := datadogV2.CreateEnvironmentRequest{
		Data: datadogV2.CreateEnvironmentData{
			Type: datadogV2.CREATEENVIRONMENTDATATYPE_ENVIRONMENTS,
			Attributes: datadogV2.CreateEnvironmentAttributes{
				Name:                       plan.Name.ValueString(),
				Queries:                    queries,
				IsProduction:               datadog.PtrBool(plan.IsProduction.ValueBool()),
				RequireFeatureFlagApproval: datadog.PtrBool(plan.RequireFeatureFlagApproval.ValueBool()),
			},
		},
	}

	created, httpResp, err := r.clients.FeatureFlags.CreateFeatureFlagsEnvironment(r.clients.Context(ctx), body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create environment", apiErr(err, httpResp))
		return
	}

	// Datadog's POST /environments response can omit server-derived fields
	// like `key`. Do an immediate GET so the model has every computed
	// attribute populated before we save state.
	res, httpResp, err := r.clients.FeatureFlags.GetFeatureFlagsEnvironment(r.clients.Context(ctx), created.Data.Id)
	if err != nil {
		resp.Diagnostics.AddError("Failed to refresh environment after create", apiErr(err, httpResp))
		return
	}

	envToModel(ctx, &res, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := parseUUID(state.ID.ValueString(), "environment ID", &resp.Diagnostics)
	if !ok {
		return
	}

	res, httpResp, err := r.clients.FeatureFlags.GetFeatureFlagsEnvironment(r.clients.Context(ctx), id)
	if err := notFoundIfHTTP404(err, httpResp); err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read environment", apiErr(err, httpResp))
		return
	}

	envToModel(ctx, &res, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *environmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan environmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := parseUUID(plan.ID.ValueString(), "environment ID", &resp.Diagnostics)
	if !ok {
		return
	}

	queries := make([]string, 0, len(plan.Queries.Elements()))
	resp.Diagnostics.Append(plan.Queries.ElementsAs(ctx, &queries, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	body := datadogV2.UpdateEnvironmentRequest{
		Data: datadogV2.UpdateEnvironmentData{
			Type: datadogV2.UPDATEENVIRONMENTDATATYPE_ENVIRONMENTS,
			Attributes: datadogV2.UpdateEnvironmentAttributes{
				Name:                       datadog.PtrString(plan.Name.ValueString()),
				Queries:                    queries,
				IsProduction:               datadog.PtrBool(plan.IsProduction.ValueBool()),
				RequireFeatureFlagApproval: datadog.PtrBool(plan.RequireFeatureFlagApproval.ValueBool()),
			},
		},
	}

	res, httpResp, err := r.clients.FeatureFlags.UpdateFeatureFlagsEnvironment(r.clients.Context(ctx), id, body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update environment", apiErr(err, httpResp))
		return
	}

	envToModel(ctx, &res, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *environmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state environmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := parseUUID(state.ID.ValueString(), "environment ID", &resp.Diagnostics)
	if !ok {
		return
	}

	httpResp, err := r.clients.FeatureFlags.DeleteFeatureFlagsEnvironment(r.clients.Context(ctx), id)
	if err := notFoundIfHTTP404(err, httpResp); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete environment", apiErr(err, httpResp))
		return
	}
}

func (r *environmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func envToModel(ctx context.Context, res *datadogV2.EnvironmentResponse, m *environmentModel, diags *diag.Diagnostics) {
	data := res.Data
	attrs := data.Attributes

	m.ID = types.StringValue(data.Id.String())
	m.Name = types.StringValue(attrs.Name)
	if attrs.IsProduction != nil {
		m.IsProduction = types.BoolValue(*attrs.IsProduction)
	} else {
		m.IsProduction = types.BoolValue(false)
	}
	if attrs.RequireFeatureFlagApproval != nil {
		m.RequireFeatureFlagApproval = types.BoolValue(*attrs.RequireFeatureFlagApproval)
	} else {
		m.RequireFeatureFlagApproval = types.BoolValue(false)
	}
	m.Description = nullableStringToTF(attrs.Description)

	queries, d := types.ListValueFrom(ctx, types.StringType, attrs.Queries)
	diags.Append(d...)
	m.Queries = queries

	m.CreatedAt = nullableTimeToTF(attrs.CreatedAt)
	m.UpdatedAt = nullableTimeToTF(attrs.UpdatedAt)
}
