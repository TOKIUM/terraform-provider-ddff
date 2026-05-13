package provider

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
)

var (
	_ resource.Resource                = (*featureFlagResource)(nil)
	_ resource.ResourceWithConfigure   = (*featureFlagResource)(nil)
	_ resource.ResourceWithImportState = (*featureFlagResource)(nil)
)

func NewFeatureFlagResource() resource.Resource {
	return &featureFlagResource{}
}

type featureFlagResource struct {
	clients *Clients
}

type featureFlagModel struct {
	ID                types.String `tfsdk:"id"`
	Key               types.String `tfsdk:"key"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	ValueType         types.String `tfsdk:"value_type"`
	DefaultVariantKey types.String `tfsdk:"default_variant_key"`
	JSONSchema        types.String `tfsdk:"json_schema"`
	Variants          []variantModel `tfsdk:"variants"`
	CreatedAt         types.String `tfsdk:"created_at"`
	UpdatedAt         types.String `tfsdk:"updated_at"`
}

type variantModel struct {
	Key   types.String `tfsdk:"key"`
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (r *featureFlagResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_feature_flag"
}

func (r *featureFlagResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Datadog feature flag.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "UUID assigned by Datadog.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"key": schema.StringAttribute{
				MarkdownDescription: "Stable unique key referenced from code. Immutable.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Human-readable name shown in the UI.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description shown in the UI.",
				Required:            true,
			},
			"value_type": schema.StringAttribute{
				MarkdownDescription: "Type of values the variants resolve to. One of `BOOLEAN`, `INTEGER`, `NUMERIC`, `STRING`, `JSON`. Immutable.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("BOOLEAN", "INTEGER", "NUMERIC", "STRING", "JSON"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"default_variant_key": schema.StringAttribute{
				MarkdownDescription: "Variant key returned when no rule matches. Must match one of the declared variants.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"json_schema": schema.StringAttribute{
				MarkdownDescription: "JSON schema used to validate variant values when `value_type` is `JSON`.",
				Optional:            true,
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
		Blocks: map[string]schema.Block{
			"variants": schema.ListNestedBlock{
				MarkdownDescription: "Variants the flag can resolve to. The order is preserved.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Required: true,
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"value": schema.StringAttribute{
							MarkdownDescription: "Value of the variant encoded as a string (e.g. `\"true\"`, `\"42\"`, JSON literal).",
							Required:            true,
						},
					},
				},
			},
		},
	}
}

func (r *featureFlagResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *featureFlagResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan featureFlagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	variants := make([]datadogV2.CreateVariant, 0, len(plan.Variants))
	for _, v := range plan.Variants {
		variants = append(variants, datadogV2.CreateVariant{
			Key:   v.Key.ValueString(),
			Name:  v.Name.ValueString(),
			Value: v.Value.ValueString(),
		})
	}

	attrs := datadogV2.CreateFeatureFlagAttributes{
		Description: plan.Description.ValueString(),
		Key:         plan.Key.ValueString(),
		Name:        plan.Name.ValueString(),
		ValueType:   datadogV2.ValueType(plan.ValueType.ValueString()),
		Variants:    variants,
	}
	if !plan.DefaultVariantKey.IsNull() && !plan.DefaultVariantKey.IsUnknown() {
		attrs.DefaultVariantKey = *datadog.NewNullableString(datadog.PtrString(plan.DefaultVariantKey.ValueString()))
	}
	if !plan.JSONSchema.IsNull() && !plan.JSONSchema.IsUnknown() {
		attrs.JsonSchema = *datadog.NewNullableString(datadog.PtrString(plan.JSONSchema.ValueString()))
	}

	body := datadogV2.CreateFeatureFlagRequest{
		Data: datadogV2.CreateFeatureFlagData{
			Type:       datadogV2.CREATEFEATUREFLAGDATATYPE_FEATURE_FLAGS,
			Attributes: attrs,
		},
	}

	res, httpResp, err := r.clients.FeatureFlags.CreateFeatureFlag(r.clients.Context(ctx), body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create feature flag", apiErr(err, httpResp))
		return
	}

	flagToModel(&res, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *featureFlagResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state featureFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := uuid.Parse(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid feature flag ID", err.Error())
		return
	}

	res, httpResp, err := r.clients.FeatureFlags.GetFeatureFlag(r.clients.Context(ctx), id)
	if err != nil {
		if httpResp != nil && httpResp.StatusCode == http.StatusNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read feature flag", apiErr(err, httpResp))
		return
	}

	flagToModel(&res, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *featureFlagResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan featureFlagModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := uuid.Parse(plan.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid feature flag ID", err.Error())
		return
	}

	attrs := datadogV2.UpdateFeatureFlagAttributes{
		Name:        datadog.PtrString(plan.Name.ValueString()),
		Description: datadog.PtrString(plan.Description.ValueString()),
	}
	if !plan.JSONSchema.IsNull() && !plan.JSONSchema.IsUnknown() {
		attrs.JsonSchema = *datadog.NewNullableString(datadog.PtrString(plan.JSONSchema.ValueString()))
	} else {
		attrs.JsonSchema.Set(nil)
	}

	body := datadogV2.UpdateFeatureFlagRequest{
		Data: datadogV2.UpdateFeatureFlagData{
			Type:       datadogV2.UPDATEFEATUREFLAGDATATYPE_FEATURE_FLAGS,
			Attributes: attrs,
		},
	}

	res, httpResp, err := r.clients.FeatureFlags.UpdateFeatureFlag(r.clients.Context(ctx), id, body)
	if err != nil {
		resp.Diagnostics.AddError("Failed to update feature flag", apiErr(err, httpResp))
		return
	}

	flagToModel(&res, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *featureFlagResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state featureFlagModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := uuid.Parse(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid feature flag ID", err.Error())
		return
	}

	// The Datadog API does not expose a physical delete. Archive instead.
	_, httpResp, err := r.clients.FeatureFlags.ArchiveFeatureFlag(r.clients.Context(ctx), id)
	if err != nil && (httpResp == nil || httpResp.StatusCode != http.StatusNotFound) {
		resp.Diagnostics.AddError("Failed to archive feature flag", apiErr(err, httpResp))
		return
	}
}

func (r *featureFlagResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func flagToModel(res *datadogV2.FeatureFlagResponse, m *featureFlagModel) {
	data := res.Data
	attrs := data.Attributes

	m.ID = types.StringValue(data.Id.String())
	m.Key = types.StringValue(attrs.Key)
	m.Name = types.StringValue(attrs.Name)
	m.Description = types.StringValue(attrs.Description)
	m.ValueType = types.StringValue(string(attrs.ValueType))

	// default_variant_key is write-only at the flag level (the API returns it
	// inside per-environment settings instead). We preserve whatever is already
	// in the model so subsequent Reads don't show drift.

	// json_schema is preserved from the model. The Datadog API re-serializes
	// the JSON before returning it, which can change key order and whitespace
	// without changing meaning. Trusting the model value avoids
	// "inconsistent result after apply" failures at the cost of not detecting
	// drift if the schema is edited from the Datadog UI. A future patch can
	// add semantic-equality handling to restore drift detection.

	variants := make([]variantModel, 0, len(attrs.Variants))
	for _, v := range attrs.Variants {
		variants = append(variants, variantModel{
			Key:   types.StringValue(v.Key),
			Name:  types.StringValue(v.Name),
			Value: types.StringValue(v.Value),
		})
	}
	m.Variants = variants

	if attrs.CreatedAt != nil {
		m.CreatedAt = types.StringValue(attrs.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	if attrs.UpdatedAt != nil {
		m.UpdatedAt = types.StringValue(attrs.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
}

func apiErr(err error, httpResp *http.Response) string {
	if httpResp == nil {
		return err.Error()
	}
	return fmt.Sprintf("%s (status %d)", err.Error(), httpResp.StatusCode)
}
