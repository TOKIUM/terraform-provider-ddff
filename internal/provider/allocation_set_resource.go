package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
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
	ID             types.String         `tfsdk:"id"`
	Key            types.String         `tfsdk:"key"`
	Name           types.String         `tfsdk:"name"`
	Type           types.String         `tfsdk:"type"`
	OrderPosition  types.Int64          `tfsdk:"order_position"`
	TargetingRules []targetingRuleModel `tfsdk:"targeting_rule"`
	VariantWeights []variantWeightModel `tfsdk:"variant_weight"`
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
			"dedicated read endpoint, so this resource owns the complete list. Drift caused by edits made in the Datadog UI is " +
			"not detected by `terraform plan`; the next `terraform apply` overwrites whatever is on the server with the state " +
			"recorded by Terraform.",
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
						},
						"key": schema.StringAttribute{
							MarkdownDescription: "Stable, human-readable key for this allocation. Must be unique within the (flag, environment) set.",
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
						},
					},
					Blocks: map[string]schema.Block{
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

func (r *allocationSetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan allocationSetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.putAndReconcile(ctx, &plan, &resp.Diagnostics, &resp.State, false)
}

func (r *allocationSetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Datadog exposes no GET endpoint for allocations. Trust state; the
	// next Update / Delete will overwrite whatever is on the server. See
	// the resource-level documentation for the drift implication.
	var state allocationSetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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

	body := datadogV2.OverwriteAllocationsRequest{Data: []datadogV2.AllocationDataRequest{}}
	_, httpResp, err := r.clients.FeatureFlags.UpdateAllocationsForFeatureFlagInEnvironment(r.clients.Context(ctx), flagID, envID, body)
	if err != nil && (httpResp == nil || httpResp.StatusCode != http.StatusNotFound) {
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
	return fmt.Sprintf("%v", keys)
}

// mergeAllocationsResponse takes the typed list returned by the PUT response
// and fills in computed attributes (id, order_position, type when defaulted)
// on the matching plan allocations, looked up by their stable `key`.
func mergeAllocationsResponse(plan *allocationSetModel, data []datadogV2.AllocationDataResponse, diags *diag.Diagnostics) {
	byKey := make(map[string]datadogV2.AllocationDataResponse, len(data))
	for _, a := range data {
		byKey[a.Attributes.Key] = a
	}

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
		plan.Allocations[i].ID = types.StringValue(resp.Id.String())
		plan.Allocations[i].OrderPosition = types.Int64Value(resp.Attributes.OrderPosition)
		if plan.Allocations[i].Type.IsNull() || plan.Allocations[i].Type.IsUnknown() {
			plan.Allocations[i].Type = types.StringValue(string(resp.Attributes.Type))
		}
	}
}
