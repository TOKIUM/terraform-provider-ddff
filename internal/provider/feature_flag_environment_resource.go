package provider

import (
	"context"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*featureFlagEnvironmentResource)(nil)
	_ resource.ResourceWithConfigure   = (*featureFlagEnvironmentResource)(nil)
	_ resource.ResourceWithImportState = (*featureFlagEnvironmentResource)(nil)
)

func NewFeatureFlagEnvironmentResource() resource.Resource {
	return &featureFlagEnvironmentResource{}
}

type featureFlagEnvironmentResource struct {
	clients *Clients
}

type featureFlagEnvironmentModel struct {
	ID                types.String `tfsdk:"id"`
	FeatureFlagID     types.String `tfsdk:"feature_flag_id"`
	EnvironmentID     types.String `tfsdk:"environment_id"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	Status            types.String `tfsdk:"status"`
	DefaultVariantID  types.String `tfsdk:"default_variant_id"`
	RolloutPercentage types.Int64  `tfsdk:"rollout_percentage"`
}

func (r *featureFlagEnvironmentResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_feature_flag_environment"
}

func (r *featureFlagEnvironmentResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Controls whether a feature flag is enabled or disabled in a specific environment. Targeting rules and variant weight distribution are managed separately via `ddff_allocation_set`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Composite ID `<feature_flag_id>:<environment_id>` used for state addressing.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"feature_flag_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the feature flag.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"environment_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the environment.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the flag is enabled in this environment. Toggling this attribute calls the Datadog enable / disable endpoints.",
				Required:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Reported status from the API (`ENABLED` or `DISABLED`).",
				Computed:            true,
			},
			"default_variant_id": schema.StringAttribute{
				MarkdownDescription: "UUID of the default variant served when no allocation in this environment matches. Set this through the Datadog UI for now; the provider only surfaces the current value for drift detection.",
				Computed:            true,
			},
			"rollout_percentage": schema.Int64Attribute{
				MarkdownDescription: "Reported rollout percentage for the environment. Manage the rollout through `ddff_allocation_set` instead.",
				Computed:            true,
			},
		},
	}
}

func (r *featureFlagEnvironmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if clients := configureClients(req.ProviderData, &resp.Diagnostics); clients != nil {
		r.clients = clients
	}
}

func (r *featureFlagEnvironmentResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan featureFlagEnvironmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flagID, envID, ok := parseFlagEnvIDs(plan.FeatureFlagID.ValueString(), plan.EnvironmentID.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	if err := r.setEnabled(ctx, flagID, envID, plan.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Failed to set feature flag environment state", err.Error())
		return
	}

	plan.ID = types.StringValue(plan.FeatureFlagID.ValueString() + ":" + plan.EnvironmentID.ValueString())
	if err := r.refreshFromFlag(ctx, flagID, envID, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to refresh feature flag environment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *featureFlagEnvironmentResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state featureFlagEnvironmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flagID, envID, ok := parseFlagEnvIDs(state.FeatureFlagID.ValueString(), state.EnvironmentID.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// Pattern D: hand-roll the env entry parse via UnparsedObject because
	// the generated SDK mis-types feature_flag_environments[].allocations
	// (declared map, actually array) and collapses the entire entry into
	// UnparsedObject once any allocation exists. On any failure we fall
	// back to preserving state so drift detection becomes best-effort
	// rather than blocking.
	if err := r.refreshFromFlag(ctx, flagID, envID, &state); err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddWarning(
			"Could not refresh feature flag environment from API",
			"State was preserved. Underlying cause: "+err.Error(),
		)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *featureFlagEnvironmentResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan featureFlagEnvironmentModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flagID, envID, ok := parseFlagEnvIDs(plan.FeatureFlagID.ValueString(), plan.EnvironmentID.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	if err := r.setEnabled(ctx, flagID, envID, plan.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Failed to update feature flag environment", err.Error())
		return
	}
	if err := r.refreshFromFlag(ctx, flagID, envID, &plan); err != nil {
		resp.Diagnostics.AddError("Failed to refresh feature flag environment", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *featureFlagEnvironmentResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state featureFlagEnvironmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flagID, envID, ok := parseFlagEnvIDs(state.FeatureFlagID.ValueString(), state.EnvironmentID.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// "Deleting" the binding means returning the flag-in-environment to its
	// disabled state. The underlying flag and environment outlive this
	// resource and are managed elsewhere.
	if err := r.setEnabled(ctx, flagID, envID, false); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Failed to disable feature flag environment", err.Error())
		return
	}
}

func (r *featureFlagEnvironmentResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expect "<flag_id>:<env_id>" so users can import without a separate lookup.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *featureFlagEnvironmentResource) setEnabled(ctx context.Context, flagID, envID uuid.UUID, enabled bool) error {
	if enabled {
		httpResp, err := r.clients.FeatureFlags.EnableFeatureFlagEnvironment(r.clients.Context(ctx), flagID, envID)
		return notFoundIfHTTP404(err, httpResp)
	}
	httpResp, err := r.clients.FeatureFlags.DisableFeatureFlagEnvironment(r.clients.Context(ctx), flagID, envID)
	return notFoundIfHTTP404(err, httpResp)
}

// refreshFromFlag locates the (env_id) entry inside the parent flag's
// feature_flag_environments[] and populates the model's computed fields.
func (r *featureFlagEnvironmentResource) refreshFromFlag(ctx context.Context, flagID, envID uuid.UUID, m *featureFlagEnvironmentModel) error {
	raw, err := r.clients.findRawEnvEntry(ctx, flagID, envID)
	if err != nil {
		return err
	}
	applyRawFlagEnvironmentToModel(raw, m)
	return nil
}

// applyRawFlagEnvironmentToModel writes the status / default_variant_id /
// rollout_percentage / derived enabled bit from the raw env entry into the
// Plugin Framework model. Each field is read defensively and falls back
// to null when the JSON shape is unexpected.
func applyRawFlagEnvironmentToModel(raw map[string]interface{}, m *featureFlagEnvironmentModel) {
	status, _ := raw["status"].(string)
	m.Status = types.StringValue(status)
	m.Enabled = types.BoolValue(status == "ENABLED")

	switch v := raw["default_variant_id"].(type) {
	case string:
		m.DefaultVariantID = types.StringValue(v)
	default:
		m.DefaultVariantID = types.StringNull()
	}

	switch v := raw["rollout_percentage"].(type) {
	case float64:
		m.RolloutPercentage = types.Int64Value(int64(v))
	case int64:
		m.RolloutPercentage = types.Int64Value(v)
	default:
		m.RolloutPercentage = types.Int64Null()
	}
}
