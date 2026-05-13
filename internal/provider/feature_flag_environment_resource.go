package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
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
		MarkdownDescription: "Controls whether a feature flag is enabled or disabled in a specific environment. Targeting rules and variant weight distribution are managed separately via `ddff_allocation`.",
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
				MarkdownDescription: "UUID of the default variant for this environment. Read-only here; set the variant `default_variant_key` on the parent `ddff_feature_flag` resource.",
				Computed:            true,
			},
			"rollout_percentage": schema.Int64Attribute{
				MarkdownDescription: "Reported rollout percentage for the environment. Manage the rollout through `ddff_allocation` instead.",
				Computed:            true,
			},
		},
	}
}

func (r *featureFlagEnvironmentResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	clients, ok := req.ProviderData.(*Clients)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *Clients, got %T", req.ProviderData),
		)
		return
	}
	r.clients = clients
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
	// Read is intentionally a state-preserving no-op.
	//
	// The Datadog API embeds the per-environment binding inside the
	// flag's GET response (`feature_flag_environments[].status`), but
	// the generated SDK models `FeatureFlagEnvironment.Allocations` as
	// `map[string]interface{}` while the API actually returns it as an
	// array as soon as the (flag, environment) pair has any allocation.
	// Strict JSON unmarshaling fails on that mismatch and the whole
	// entry collapses into `UnparsedObject`, leaving `EnvironmentId`
	// zero-valued and making the env lookup falsely return "not found".
	// Trusting state is the least surprising behavior until the SDK
	// catches up; Update / Delete still reconcile by hitting the
	// enable / disable endpoints directly.
	var state featureFlagEnvironmentModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
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
		_, err := r.clients.FeatureFlags.EnableFeatureFlagEnvironment(r.clients.Context(ctx), flagID, envID)
		return err
	}
	_, err := r.clients.FeatureFlags.DisableFeatureFlagEnvironment(r.clients.Context(ctx), flagID, envID)
	return err
}

func (r *featureFlagEnvironmentResource) refreshFromFlag(ctx context.Context, flagID, envID uuid.UUID, m *featureFlagEnvironmentModel) error {
	res, httpResp, err := r.clients.FeatureFlags.GetFeatureFlag(r.clients.Context(ctx), flagID)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			return errNotFound
		}
		return err
	}

	for _, env := range res.Data.Attributes.FeatureFlagEnvironments {
		if env.EnvironmentId == envID {
			applyFlagEnvironmentToModel(env, m)
			return nil
		}
	}
	return errNotFound
}

func applyFlagEnvironmentToModel(env datadogV2.FeatureFlagEnvironment, m *featureFlagEnvironmentModel) {
	m.Status = types.StringValue(string(env.Status))
	m.Enabled = types.BoolValue(env.Status == datadogV2.FEATUREFLAGSTATUS_ENABLED)
	if env.DefaultVariantId.IsSet() && env.DefaultVariantId.Get() != nil {
		m.DefaultVariantID = types.StringValue(*env.DefaultVariantId.Get())
	} else {
		m.DefaultVariantID = types.StringNull()
	}
	if env.RolloutPercentage != nil {
		m.RolloutPercentage = types.Int64Value(*env.RolloutPercentage)
	} else {
		m.RolloutPercentage = types.Int64Null()
	}
}
