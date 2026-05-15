package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*allocationSetResource)(nil)
	_ resource.ResourceWithConfigure   = (*allocationSetResource)(nil)
	_ resource.ResourceWithImportState = (*allocationSetResource)(nil)
)

func NewAllocationSetResource() resource.Resource {
	return &allocationSetResource{}
}

type allocationSetResource struct {
	clients *Clients
}

type allocationSetModel struct {
	ID            types.String      `tfsdk:"id"`
	FeatureFlagID types.String      `tfsdk:"feature_flag_id"`
	EnvironmentID types.String      `tfsdk:"environment_id"`
	Allocations   []allocationModel `tfsdk:"allocation"`
}

type allocationModel struct {
	ID               types.String           `tfsdk:"id"`
	Key              types.String           `tfsdk:"key"`
	Name             types.String           `tfsdk:"name"`
	Type             types.String           `tfsdk:"type"`
	OrderPosition    types.Int64            `tfsdk:"order_position"`
	TargetingRules   []targetingRuleModel   `tfsdk:"targeting_rule"`
	VariantWeights   []variantWeightModel   `tfsdk:"variant_weight"`
	ExposureSchedule *exposureScheduleModel `tfsdk:"exposure_schedule"`
}

// exposureScheduleModel is exposed as a `exposure_schedule` SingleNestedBlock.
// Block syntax (vs attribute) means a missing block decodes to a nil pointer
// rather than an "unknown" value, which the framework cannot represent in a
// Go struct field. As a consequence, drift detection is opt-in: a user who
// does not declare the block will not see UI-side changes surface in plan.
type exposureScheduleModel struct {
	ID                types.String         `tfsdk:"id"`
	AllocationID      types.String         `tfsdk:"allocation_id"`
	ControlVariantID  types.String         `tfsdk:"control_variant_id"`
	AbsoluteStartTime types.String         `tfsdk:"absolute_start_time"`
	RolloutOptions    *rolloutOptionsModel `tfsdk:"rollout_options"`
	RolloutSteps      []rolloutStepModel   `tfsdk:"rollout_step"`
}

type rolloutOptionsModel struct {
	Strategy            types.String `tfsdk:"strategy"`
	Autostart           types.Bool   `tfsdk:"autostart"`
	SelectionIntervalMs types.Int64  `tfsdk:"selection_interval_ms"`
}

type rolloutStepModel struct {
	ID               types.String  `tfsdk:"id"`
	OrderPosition    types.Int64   `tfsdk:"order_position"`
	ExposureRatio    types.Float64 `tfsdk:"exposure_ratio"`
	IntervalMs       types.Int64   `tfsdk:"interval_ms"`
	IsPauseRecord    types.Bool    `tfsdk:"is_pause_record"`
	GroupedStepIndex types.Int64   `tfsdk:"grouped_step_index"`
}

type targetingRuleModel struct {
	Conditions []conditionModel `tfsdk:"condition"`
}

type conditionModel struct {
	Attribute types.String `tfsdk:"attribute"`
	Operator  types.String `tfsdk:"operator"`
	Value     types.List   `tfsdk:"value"`
}

type variantWeightModel struct {
	VariantKey types.String  `tfsdk:"variant_key"`
	Value      types.Float64 `tfsdk:"value"`
}

func (r *allocationSetResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_allocation_set"
}

func (r *allocationSetResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the full set of targeting rules (allocations) for one feature flag in one environment. " +
			"\n\n" +
			"Allocations are evaluated in declaration order. The first allocation whose `targeting_rule` conditions match the " +
			"evaluation context determines the served variant via its `variant_weight` distribution; if no allocation matches, " +
			"the flag's `default_variant_key` is returned instead." +
			"\n\n" +
			"The Datadog API replaces the entire allocation list for a (flag, environment) pair on every write and exposes no " +
			"dedicated read endpoint. The provider recovers the current allocation list by parsing the parent flag's JSON, so " +
			"targeting rules, variant weights, allocation type, and (when declared) the `exposure_schedule` block all surface " +
			"as drift on `terraform plan`. The `exposure_schedule` block is opt-in: an allocation that does not declare it " +
			"will not surface UI-side changes to the schedule.",
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
		},
		Blocks: map[string]schema.Block{
			"allocation": schema.ListNestedBlock{
				MarkdownDescription: "One targeting rule plus the variant weight distribution served when it matches.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "UUID assigned by the API on create.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"key": schema.StringAttribute{
							MarkdownDescription: "Stable, human-readable key for this allocation. The Datadog API enforces uniqueness across the entire workspace (not just within the (flag, environment) set), so include enough scope — typically the environment name and/or the flag's product slug — to avoid `409 Conflict: allocation with this key already exists` when adding the same allocation to a second (flag, environment) pair.",
							Required:            true,
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Display name shown in the Datadog UI.",
							Required:            true,
						},
						"type": schema.StringAttribute{
							MarkdownDescription: "Allocation type. `FEATURE_GATE` for permanent / boolean-style rollouts, `CANARY` for time-bounded progressive rollouts. Defaults to `FEATURE_GATE`.",
							Optional:            true,
							Computed:            true,
							Default:             stringdefault.StaticString("FEATURE_GATE"),
							Validators: []validator.String{
								stringvalidator.OneOf("FEATURE_GATE", "CANARY"),
							},
						},
						"order_position": schema.Int64Attribute{
							MarkdownDescription: "Server-assigned evaluation order within the environment, derived from the declaration order in this block list.",
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								int64planmodifier.UseStateForUnknown(),
							},
						},
					},
					Blocks: map[string]schema.Block{
						"exposure_schedule": schema.SingleNestedBlock{
							MarkdownDescription: "Progressive (traffic exposure) rollout schedule attached to this allocation. Declare the block to manage rollout_options and rollout_steps via Terraform; omit it to leave drift detection off for this allocation. Server-assigned ids (schedule id, step ids) and computed fields are refreshed on every write.",
							Attributes: map[string]schema.Attribute{
								"id":                  schema.StringAttribute{Computed: true},
								"allocation_id":       schema.StringAttribute{Computed: true},
								"control_variant_id":  schema.StringAttribute{Optional: true, Computed: true},
								"absolute_start_time": schema.StringAttribute{Optional: true, Computed: true},
							},
							Blocks: map[string]schema.Block{
								"rollout_options": schema.SingleNestedBlock{
									Attributes: map[string]schema.Attribute{
										"strategy": schema.StringAttribute{
											Optional:            true,
											Computed:            true,
											MarkdownDescription: "Rollout cadence. One of `UNIFORM_INTERVALS` (all `rollout_step` blocks share `selection_interval_ms`) or `NO_ROLLOUT` (single step at 100 %). Required when the parent `exposure_schedule` block is declared.",
											Validators: []validator.String{
												stringvalidator.OneOf("UNIFORM_INTERVALS", "NO_ROLLOUT"),
											},
										},
										"autostart":             schema.BoolAttribute{Optional: true, Computed: true},
										"selection_interval_ms": schema.Int64Attribute{Optional: true, Computed: true},
									},
								},
								"rollout_step": schema.ListNestedBlock{
									NestedObject: schema.NestedBlockObject{
										Attributes: map[string]schema.Attribute{
											"id":                 schema.StringAttribute{Computed: true},
											"order_position":     schema.Int64Attribute{Computed: true},
											"exposure_ratio":     schema.Float64Attribute{Optional: true, Computed: true},
											"interval_ms":        schema.Int64Attribute{Optional: true, Computed: true},
											"is_pause_record":    schema.BoolAttribute{Optional: true, Computed: true},
											"grouped_step_index": schema.Int64Attribute{Optional: true, Computed: true},
										},
									},
								},
							},
						},
						"targeting_rule": schema.ListNestedBlock{
							MarkdownDescription: "Conjunction of conditions that determines whether this allocation applies to the evaluation context. All conditions inside one targeting_rule must match (AND); multiple targeting_rule blocks act as alternatives (OR).",
							NestedObject: schema.NestedBlockObject{
								Blocks: map[string]schema.Block{
									"condition": schema.ListNestedBlock{
										MarkdownDescription: "A single attribute comparison against a list of expected values.",
										NestedObject: schema.NestedBlockObject{
											Attributes: map[string]schema.Attribute{
												"attribute": schema.StringAttribute{
													MarkdownDescription: "Name of the evaluation-context attribute to compare against (for example `customer_tier`).",
													Required:            true,
												},
												"operator": schema.StringAttribute{
													MarkdownDescription: "Comparison operator. One of `EQUALS`, `ONE_OF`, `NOT_ONE_OF`, `MATCHES`, `NOT_MATCHES`, `LT`, `LTE`, `GT`, `GTE`, `IS_NULL`.",
													Required:            true,
													Validators: []validator.String{
														stringvalidator.OneOf("EQUALS", "ONE_OF", "NOT_ONE_OF", "MATCHES", "NOT_MATCHES", "LT", "LTE", "GT", "GTE", "IS_NULL"),
													},
												},
												"value": schema.ListAttribute{
													MarkdownDescription: "Values the attribute is compared against. The operator decides whether the list is exact-match, regex, comparison, or membership.",
													ElementType:         types.StringType,
													Required:            true,
													Validators: []validator.List{
														listvalidator.SizeAtLeast(1),
													},
												},
											},
										},
										Validators: []validator.List{
											listvalidator.SizeAtLeast(1),
										},
									},
								},
							},
						},
						"variant_weight": schema.ListNestedBlock{
							MarkdownDescription: "Probability distribution over variants when this allocation matches. The weights are interpreted as percentages and are expected to sum to 100.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"variant_key": schema.StringAttribute{
										MarkdownDescription: "Key of the variant declared on the parent `ddff_feature_flag` resource.",
										Required:            true,
									},
									"value": schema.Float64Attribute{
										MarkdownDescription: "Percentage weight in the range [0, 100].",
										Required:            true,
										Validators: []validator.Float64{
											float64validator.Between(0, 100),
										},
									},
								},
							},
							Validators: []validator.List{
								listvalidator.SizeAtLeast(1),
							},
						},
					},
				},
				Validators: []validator.List{
					listvalidator.SizeAtLeast(1),
				},
			},
		},
	}
}

func (r *allocationSetResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if clients := configureClients(req.ProviderData, &resp.Diagnostics); clients != nil {
		r.clients = clients
	}
}

func (r *allocationSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan allocationSetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.putAndReconcile(ctx, &plan, &resp.Diagnostics, &resp.State, false)
}

func (r *allocationSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state allocationSetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flagID, envID, ok := parseFlagEnvIDs(state.FeatureFlagID.ValueString(), state.EnvironmentID.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// Datadog does not expose a typed GET for allocations and the
	// generated SDK mis-types feature_flag_environments[].allocations
	// (declared map, actually array), so we hand-roll the parse from
	// the env entry's UnparsedObject map. On any failure we fall back
	// to preserving state so drift detection becomes best-effort.
	allocations, err := r.refreshAllocationsFromFlag(ctx, flagID, envID)
	if err != nil {
		if isNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddWarning(
			"Could not refresh allocations from API",
			"State was preserved. Underlying cause: "+err.Error(),
		)
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	mergeAllocationsRead(state.Allocations, allocations)
	state.Allocations = allocations
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// mergeAllocationsRead reconciles fresh server-side allocations with the
// prior state. exposure_schedule is opt-in (block syntax): if the user never
// declared it for an allocation, the prior state has ExposureSchedule == nil
// and we drop the value the API returned so the state continues to match the
// declared config. Allocations are matched by key; missing matches keep the
// fresh value.
func mergeAllocationsRead(prior, fresh []allocationModel) {
	priorByKey := make(map[string]*allocationModel, len(prior))
	for i := range prior {
		priorByKey[prior[i].Key.ValueString()] = &prior[i]
	}
	for i := range fresh {
		p, ok := priorByKey[fresh[i].Key.ValueString()]
		if !ok || p.ExposureSchedule == nil {
			fresh[i].ExposureSchedule = nil
		}
	}
}

// refreshAllocationsFromFlag fetches the parent flag, locates the env
// entry by environment_id via the shared raw-JSON lookup, and decodes
// the allocations array into the model shape this resource exposes.
func (r *allocationSetResource) refreshAllocationsFromFlag(ctx context.Context, flagID, envID uuid.UUID) ([]allocationModel, error) {
	raw, err := r.clients.findRawEnvEntry(ctx, flagID, envID)
	if err != nil {
		return nil, err
	}

	allocsRaw, ok := raw["allocations"].([]interface{})
	if !ok || len(allocsRaw) == 0 {
		return []allocationModel{}, nil
	}

	bytes, mErr := json.Marshal(allocsRaw)
	if mErr != nil {
		return nil, fmt.Errorf("re-marshal allocations: %w", mErr)
	}

	var typed []rawAllocation
	if uErr := json.Unmarshal(bytes, &typed); uErr != nil {
		return nil, fmt.Errorf("decode allocations: %w", uErr)
	}

	out := make([]allocationModel, 0, len(typed))
	for _, a := range typed {
		model, convErr := rawAllocationToModel(ctx, a)
		if convErr != nil {
			return nil, convErr
		}
		out = append(out, model)
	}
	return out, nil
}

// rawAllocation mirrors the JSON shape the API actually returns for an
// allocation, independent of the SDK's typed wrappers (which sometimes
// fail strict unmarshaling on the fields we care about).
type rawAllocation struct {
	ID               string               `json:"id"`
	Key              string               `json:"key"`
	Name             string               `json:"name"`
	Type             string               `json:"type"`
	OrderPosition    int64                `json:"order_position"`
	TargetingRules   []rawTargetingRule   `json:"targeting_rules"`
	VariantWeights   []rawVariantWeight   `json:"variant_weights"`
	ExposureSchedule *rawExposureSchedule `json:"exposure_schedule,omitempty"`
}

type rawTargetingRule struct {
	Conditions []rawCondition `json:"conditions"`
}

type rawCondition struct {
	Attribute string   `json:"attribute"`
	Operator  string   `json:"operator"`
	Value     []string `json:"value"`
}

type rawVariantWeight struct {
	Value   float64 `json:"value"`
	Variant struct {
		Key string `json:"key"`
	} `json:"variant"`
	VariantKey *string `json:"variant_key,omitempty"`
}

type rawExposureSchedule struct {
	ID                string             `json:"id"`
	AllocationID      string             `json:"allocation_id"`
	ControlVariantID  *string            `json:"control_variant_id"`
	AbsoluteStartTime *string            `json:"absolute_start_time"`
	RolloutOptions    *rawRolloutOptions `json:"rollout_options"`
	RolloutSteps      []rawRolloutStep   `json:"rollout_steps"`
}

type rawRolloutOptions struct {
	Strategy            string `json:"strategy"`
	Autostart           bool   `json:"autostart"`
	SelectionIntervalMs int64  `json:"selection_interval_ms"`
}

type rawRolloutStep struct {
	ID               string  `json:"id"`
	OrderPosition    int64   `json:"order_position"`
	ExposureRatio    float64 `json:"exposure_ratio"`
	IntervalMs       *int64  `json:"interval_ms"`
	IsPauseRecord    bool    `json:"is_pause_record"`
	GroupedStepIndex int64   `json:"grouped_step_index"`
}

func rawAllocationToModel(ctx context.Context, a rawAllocation) (allocationModel, error) {
	m := allocationModel{
		ID:            types.StringValue(a.ID),
		Key:           types.StringValue(a.Key),
		Name:          types.StringValue(a.Name),
		Type:          types.StringValue(a.Type),
		OrderPosition: types.Int64Value(a.OrderPosition),
	}

	rules := make([]targetingRuleModel, 0, len(a.TargetingRules))
	for _, r := range a.TargetingRules {
		conds := make([]conditionModel, 0, len(r.Conditions))
		for _, c := range r.Conditions {
			valueList, d := types.ListValueFrom(ctx, types.StringType, c.Value)
			if d.HasError() {
				return m, fmt.Errorf("convert condition.value: %v", d)
			}
			conds = append(conds, conditionModel{
				Attribute: types.StringValue(c.Attribute),
				Operator:  types.StringValue(c.Operator),
				Value:     valueList,
			})
		}
		rules = append(rules, targetingRuleModel{Conditions: conds})
	}
	m.TargetingRules = rules

	weights := make([]variantWeightModel, 0, len(a.VariantWeights))
	for _, w := range a.VariantWeights {
		key := w.Variant.Key
		if key == "" && w.VariantKey != nil {
			key = *w.VariantKey
		}
		weights = append(weights, variantWeightModel{
			VariantKey: types.StringValue(key),
			Value:      types.Float64Value(w.Value),
		})
	}
	m.VariantWeights = weights

	if a.ExposureSchedule != nil {
		m.ExposureSchedule = rawExposureScheduleToModel(*a.ExposureSchedule)
	}

	return m, nil
}

func rawExposureScheduleToModel(s rawExposureSchedule) *exposureScheduleModel {
	out := &exposureScheduleModel{
		ID:                types.StringValue(s.ID),
		AllocationID:      types.StringValue(s.AllocationID),
		ControlVariantID:  nullableString(s.ControlVariantID),
		AbsoluteStartTime: nullableString(s.AbsoluteStartTime),
	}
	if s.RolloutOptions != nil {
		out.RolloutOptions = &rolloutOptionsModel{
			Strategy:            types.StringValue(s.RolloutOptions.Strategy),
			Autostart:           types.BoolValue(s.RolloutOptions.Autostart),
			SelectionIntervalMs: types.Int64Value(s.RolloutOptions.SelectionIntervalMs),
		}
	}
	steps := make([]rolloutStepModel, 0, len(s.RolloutSteps))
	for _, st := range s.RolloutSteps {
		steps = append(steps, rolloutStepModel{
			ID:               types.StringValue(st.ID),
			OrderPosition:    types.Int64Value(st.OrderPosition),
			ExposureRatio:    types.Float64Value(st.ExposureRatio),
			IntervalMs:       nullableInt64(st.IntervalMs),
			IsPauseRecord:    types.BoolValue(st.IsPauseRecord),
			GroupedStepIndex: types.Int64Value(st.GroupedStepIndex),
		})
	}
	out.RolloutSteps = steps
	return out
}

func nullableString(p *string) types.String {
	if p == nil {
		return types.StringNull()
	}
	return types.StringValue(*p)
}

func nullableInt64(p *int64) types.Int64 {
	if p == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*p)
}

func (r *allocationSetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan allocationSetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.putAndReconcile(ctx, &plan, &resp.Diagnostics, &resp.State, true)
}

func (r *allocationSetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state allocationSetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	flagID, envID, ok := parseFlagEnvIDs(state.FeatureFlagID.ValueString(), state.EnvironmentID.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	unlock := r.clients.LockEnv(envID)
	defer unlock()

	body := datadogV2.OverwriteAllocationsRequest{Data: []datadogV2.AllocationDataRequest{}}
	_, httpResp, err := r.clients.FeatureFlags.UpdateAllocationsForFeatureFlagInEnvironment(r.clients.Context(ctx), flagID, envID, body)
	if err := notFoundIfHTTP404(err, httpResp); err != nil && !isNotFound(err) {
		resp.Diagnostics.AddError("Failed to delete allocations", apiErr(err, httpResp))
	}
}

func (r *allocationSetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// putAndReconcile performs the shared logic between Create and Update: build
// the PUT body from plan, call the API, map the typed response back into the
// plan struct (including server-assigned IDs / order positions), and write it
// to state. isUpdate is informational only.
func (r *allocationSetResource) putAndReconcile(ctx context.Context, plan *allocationSetModel, diags *diag.Diagnostics, state *tfsdk.State, isUpdate bool) {
	flagID, envID, ok := parseFlagEnvIDs(plan.FeatureFlagID.ValueString(), plan.EnvironmentID.ValueString(), diags)
	if !ok {
		return
	}

	// Serialise concurrent writes to the same environment: the
	// allocations endpoint returns a transient 409 when two parallel
	// writes hit the same env_id, even for distinct feature_flag_ids.
	unlock := r.clients.LockEnv(envID)
	defer unlock()

	// The Datadog API expects variant_weights to include the variant's
	// UUID, not just its key. Resolve the (key -> id) mapping by fetching
	// the parent flag once per write.
	variantKeyToID, lookupDiags := r.variantKeyToID(ctx, flagID)
	diags.Append(lookupDiags...)
	if diags.HasError() {
		return
	}

	body, buildDiags := buildOverwriteBody(ctx, plan.Allocations, variantKeyToID)
	diags.Append(buildDiags...)
	if diags.HasError() {
		return
	}

	res, httpResp, err := r.clients.FeatureFlags.UpdateAllocationsForFeatureFlagInEnvironment(r.clients.Context(ctx), flagID, envID, body)
	if err != nil {
		op := "create"
		if isUpdate {
			op = "update"
		}
		diags.AddError("Failed to "+op+" allocations", apiErr(err, httpResp))
		return
	}

	plan.ID = types.StringValue(plan.FeatureFlagID.ValueString() + ":" + plan.EnvironmentID.ValueString())
	mergeAllocationsResponse(plan, res.Data, diags)
	if diags.HasError() {
		return
	}

	// Refresh from raw JSON to capture fields not surfaced by the typed PUT
	// response (notably exposure_schedule). For allocations where the user
	// did not declare exposure_schedule in HCL, we drop the server-side
	// value to keep state aligned with config (block syntax is opt-in).
	fresh, refreshErr := r.refreshAllocationsFromFlag(ctx, flagID, envID)
	if refreshErr != nil {
		diags.AddWarning(
			"Could not refresh allocations after write",
			"State will reflect the PUT response only. Underlying cause: "+refreshErr.Error(),
		)
	} else {
		mergeAllocationsRead(plan.Allocations, fresh)
		plan.Allocations = fresh
	}

	diags.Append(state.Set(ctx, plan)...)
}

// variantKeyToID returns a (variant_key -> variant_id) map for the given
// feature flag.
func (r *allocationSetResource) variantKeyToID(ctx context.Context, flagID uuid.UUID) (map[string]uuid.UUID, diag.Diagnostics) {
	var diags diag.Diagnostics
	res, httpResp, err := r.clients.FeatureFlags.GetFeatureFlag(r.clients.Context(ctx), flagID)
	if err != nil {
		diags.AddError("Failed to resolve flag variants", apiErr(err, httpResp))
		return nil, diags
	}
	out := make(map[string]uuid.UUID, len(res.Data.Attributes.Variants))
	for _, v := range res.Data.Attributes.Variants {
		out[v.Key] = v.Id
	}
	return out, diags
}

// buildOverwriteBody converts the planned allocation list into the
// OverwriteAllocationsRequest expected by the SDK.
func buildOverwriteBody(ctx context.Context, allocs []allocationModel, variantKeyToID map[string]uuid.UUID) (datadogV2.OverwriteAllocationsRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	body := datadogV2.OverwriteAllocationsRequest{
		Data: make([]datadogV2.AllocationDataRequest, 0, len(allocs)),
	}
	for _, a := range allocs {
		targetingRules, d := buildTargetingRules(ctx, a.TargetingRules)
		diags.Append(d...)
		if diags.HasError() {
			return body, diags
		}

		variantWeights, d := buildVariantWeights(a.VariantWeights, variantKeyToID)
		diags.Append(d...)
		if diags.HasError() {
			return body, diags
		}

		attrs := datadogV2.UpsertAllocationRequest{
			Key:            a.Key.ValueString(),
			Name:           a.Name.ValueString(),
			Type:           datadogV2.AllocationType(a.Type.ValueString()),
			TargetingRules: targetingRules,
			VariantWeights: variantWeights,
		}
		if a.ExposureSchedule != nil {
			es, d := buildExposureSchedule(a.ExposureSchedule)
			diags.Append(d...)
			if diags.HasError() {
				return body, diags
			}
			attrs.ExposureSchedule = es
		}
		// Pass the existing allocation's UUID through when we know it
		// (i.e. on Update), so the server treats this as an update of an
		// existing record instead of creating a duplicate and rejecting
		// the conflicting key with 409.
		if !a.ID.IsNull() && !a.ID.IsUnknown() && a.ID.ValueString() != "" {
			if id, parseErr := uuid.Parse(a.ID.ValueString()); parseErr == nil {
				idCopy := id
				attrs.Id = &idCopy
			}
		}
		body.Data = append(body.Data, datadogV2.AllocationDataRequest{
			Type:       datadogV2.ALLOCATIONDATATYPE_ALLOCATIONS,
			Attributes: attrs,
		})
	}
	return body, diags
}

func buildTargetingRules(ctx context.Context, rules []targetingRuleModel) ([]datadogV2.TargetingRuleRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]datadogV2.TargetingRuleRequest, 0, len(rules))
	for _, r := range rules {
		conditions := make([]datadogV2.ConditionRequest, 0, len(r.Conditions))
		for _, c := range r.Conditions {
			values := make([]string, 0, len(c.Value.Elements()))
			d := c.Value.ElementsAs(ctx, &values, false)
			diags.Append(d...)
			if diags.HasError() {
				return out, diags
			}
			conditions = append(conditions, datadogV2.ConditionRequest{
				Attribute: c.Attribute.ValueString(),
				Operator:  datadogV2.ConditionOperator(c.Operator.ValueString()),
				Value:     values,
			})
		}
		out = append(out, datadogV2.TargetingRuleRequest{Conditions: conditions})
	}
	return out, diags
}

func buildExposureSchedule(s *exposureScheduleModel) (*datadogV2.ExposureScheduleRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	if s.RolloutOptions == nil {
		diags.AddError("Invalid exposure_schedule", "rollout_options is required when exposure_schedule is declared")
		return nil, diags
	}
	if s.RolloutOptions.Strategy.IsNull() || s.RolloutOptions.Strategy.IsUnknown() || s.RolloutOptions.Strategy.ValueString() == "" {
		diags.AddError("Invalid exposure_schedule", "rollout_options.strategy is required when exposure_schedule is declared")
		return nil, diags
	}

	out := &datadogV2.ExposureScheduleRequest{
		RolloutOptions: datadogV2.RolloutOptionsRequest{
			Strategy: datadogV2.RolloutStrategy(s.RolloutOptions.Strategy.ValueString()),
		},
	}
	if !s.RolloutOptions.Autostart.IsNull() && !s.RolloutOptions.Autostart.IsUnknown() {
		v := s.RolloutOptions.Autostart.ValueBool()
		out.RolloutOptions.Autostart = *datadog.NewNullableBool(&v)
	}
	if !s.RolloutOptions.SelectionIntervalMs.IsNull() && !s.RolloutOptions.SelectionIntervalMs.IsUnknown() {
		v := s.RolloutOptions.SelectionIntervalMs.ValueInt64()
		out.RolloutOptions.SelectionIntervalMs = &v
	}

	if !s.ControlVariantID.IsNull() && !s.ControlVariantID.IsUnknown() {
		v := s.ControlVariantID.ValueString()
		out.ControlVariantId = *datadog.NewNullableString(&v)
	}
	if !s.AbsoluteStartTime.IsNull() && !s.AbsoluteStartTime.IsUnknown() {
		raw := s.AbsoluteStartTime.ValueString()
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			diags.AddError("Invalid absolute_start_time", fmt.Sprintf("expected RFC3339 timestamp, got %q: %v", raw, err))
			return nil, diags
		}
		out.AbsoluteStartTime = *datadog.NewNullableTime(&t)
	}
	if !s.ID.IsNull() && !s.ID.IsUnknown() && s.ID.ValueString() != "" {
		if id, parseErr := uuid.Parse(s.ID.ValueString()); parseErr == nil {
			idCopy := id
			out.Id = &idCopy
		}
	}

	steps := make([]datadogV2.ExposureRolloutStepRequest, 0, len(s.RolloutSteps))
	for i, st := range s.RolloutSteps {
		isPause := false
		if !st.IsPauseRecord.IsNull() && !st.IsPauseRecord.IsUnknown() {
			isPause = st.IsPauseRecord.ValueBool()
		}
		gsi := int64(i)
		if !st.GroupedStepIndex.IsNull() && !st.GroupedStepIndex.IsUnknown() {
			gsi = st.GroupedStepIndex.ValueInt64()
		}
		step := datadogV2.ExposureRolloutStepRequest{
			ExposureRatio:    st.ExposureRatio.ValueFloat64(),
			GroupedStepIndex: gsi,
			IsPauseRecord:    isPause,
		}
		if !st.IntervalMs.IsNull() && !st.IntervalMs.IsUnknown() {
			v := st.IntervalMs.ValueInt64()
			step.IntervalMs = *datadog.NewNullableInt64(&v)
		}
		if !st.ID.IsNull() && !st.ID.IsUnknown() && st.ID.ValueString() != "" {
			if id, parseErr := uuid.Parse(st.ID.ValueString()); parseErr == nil {
				idCopy := id
				step.Id = &idCopy
			}
		}
		steps = append(steps, step)
	}
	out.RolloutSteps = steps
	return out, diags
}

func buildVariantWeights(weights []variantWeightModel, variantKeyToID map[string]uuid.UUID) ([]datadogV2.VariantWeightRequest, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]datadogV2.VariantWeightRequest, 0, len(weights))
	for _, w := range weights {
		key := w.VariantKey.ValueString()
		id, ok := variantKeyToID[key]
		if !ok {
			diags.AddError(
				"Unknown variant_key",
				fmt.Sprintf("variant_key %q does not match any variant on the parent feature flag; declared variants are: %s", key, variantKeyList(variantKeyToID)),
			)
			return out, diags
		}
		idCopy := id
		keyCopy := key
		out = append(out, datadogV2.VariantWeightRequest{
			VariantId:  &idCopy,
			VariantKey: &keyCopy,
			Value:      w.Value.ValueFloat64(),
		})
	}
	return out, diags
}

func variantKeyList(m map[string]uuid.UUID) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return fmt.Sprintf("%v", keys)
}

// mergeAllocationsResponse takes the typed list returned by the PUT response
// and fills in computed attributes (id, order_position, type when defaulted)
// on the matching plan allocations, looked up by their stable `key`.
//
// Mismatches between the plan and the response are surfaced as errors so
// we never silently drop server-side state: a plan key without a response
// indicates the API rejected or renamed the allocation, and an extra
// response key indicates the server kept an allocation we did not declare.
func mergeAllocationsResponse(plan *allocationSetModel, data []datadogV2.AllocationDataResponse, diags *diag.Diagnostics) {
	byKey := make(map[string]datadogV2.AllocationDataResponse, len(data))
	for _, a := range data {
		byKey[a.Attributes.Key] = a
	}

	seen := make(map[string]struct{}, len(plan.Allocations))
	for i := range plan.Allocations {
		key := plan.Allocations[i].Key.ValueString()
		resp, ok := byKey[key]
		if !ok {
			diags.AddError(
				"Allocation missing from API response",
				fmt.Sprintf("PUT returned no allocation with key %q for this (flag, environment); the API may have rejected or renamed it.", key),
			)
			return
		}
		seen[key] = struct{}{}
		plan.Allocations[i].ID = types.StringValue(resp.Id.String())
		plan.Allocations[i].OrderPosition = types.Int64Value(resp.Attributes.OrderPosition)
		if plan.Allocations[i].Type.IsNull() || plan.Allocations[i].Type.IsUnknown() {
			plan.Allocations[i].Type = types.StringValue(string(resp.Attributes.Type))
		}
	}

	extra := make([]string, 0)
	for key := range byKey {
		if _, ok := seen[key]; !ok {
			extra = append(extra, key)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		diags.AddError(
			"Server returned unexpected allocations",
			fmt.Sprintf("The API kept allocations not present in the plan: %v. The PUT body may not have been treated as a full replacement; investigate the (flag, environment) before re-running apply.", extra),
		)
	}
}
