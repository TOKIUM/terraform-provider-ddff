package provider

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*featureFlagDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*featureFlagDataSource)(nil)
)

func NewFeatureFlagDataSource() datasource.DataSource {
	return &featureFlagDataSource{}
}

type featureFlagDataSource struct {
	clients *Clients
}

type featureFlagDataSourceModel struct {
	ID          types.String         `tfsdk:"id"`
	Key         types.String         `tfsdk:"key"`
	Name        types.String         `tfsdk:"name"`
	Description types.String         `tfsdk:"description"`
	ValueType   types.String         `tfsdk:"value_type"`
	JSONSchema  jsontypes.Normalized `tfsdk:"json_schema"`
	Variants    []variantModel       `tfsdk:"variants"`
	CreatedAt   types.String         `tfsdk:"created_at"`
	UpdatedAt   types.String         `tfsdk:"updated_at"`
}

func (d *featureFlagDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_feature_flag"
}

func (d *featureFlagDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Datadog feature flag by its stable key.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UUID of the feature flag.",
			},
			"key": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Stable, code-facing key of the feature flag to look up.",
			},
			"name": schema.StringAttribute{Computed: true},
			"description": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Description shown in the Datadog UI.",
			},
			"value_type": schema.StringAttribute{Computed: true},
			"json_schema": schema.StringAttribute{
				Computed:   true,
				CustomType: jsontypes.NormalizedType{},
			},
			"variants": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key":   schema.StringAttribute{Computed: true},
						"name":  schema.StringAttribute{Computed: true},
						"value": schema.StringAttribute{Computed: true},
					},
				},
			},
			"created_at": schema.StringAttribute{Computed: true},
			"updated_at": schema.StringAttribute{Computed: true},
		},
	}
}

func (d *featureFlagDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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
	d.clients = clients
}

func (d *featureFlagDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data featureFlagDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	key := data.Key.ValueString()
	params := datadogV2.NewListFeatureFlagsOptionalParameters().WithKey(key)
	res, httpResp, err := d.clients.FeatureFlags.ListFeatureFlags(d.clients.Context(ctx), *params)
	if err != nil {
		resp.Diagnostics.AddError("Failed to look up feature flag", apiErr(err, httpResp))
		return
	}
	if len(res.Data) == 0 {
		resp.Diagnostics.AddError("Feature flag not found", fmt.Sprintf("no feature flag exists with key %q", key))
		return
	}

	// The list endpoint filters by exact key, so a single result is expected.
	flag := res.Data[0]
	attrs := flag.Attributes

	data.ID = types.StringValue(flag.Id.String())
	data.Key = types.StringValue(attrs.Key)
	data.Name = types.StringValue(attrs.Name)
	data.Description = types.StringValue(attrs.Description)
	data.ValueType = types.StringValue(string(attrs.ValueType))

	if attrs.JsonSchema.IsSet() && attrs.JsonSchema.Get() != nil {
		data.JSONSchema = jsontypes.NewNormalizedValue(*attrs.JsonSchema.Get())
	} else {
		data.JSONSchema = jsontypes.NewNormalizedNull()
	}

	variants := make([]variantModel, 0, len(attrs.Variants))
	for _, v := range attrs.Variants {
		variants = append(variants, variantModel{
			Key:   types.StringValue(v.Key),
			Name:  types.StringValue(v.Name),
			Value: types.StringValue(v.Value),
		})
	}
	data.Variants = variants

	if attrs.CreatedAt != nil {
		data.CreatedAt = types.StringValue(attrs.CreatedAt.Format(timeFormat))
	}
	if attrs.UpdatedAt != nil {
		data.UpdatedAt = types.StringValue(attrs.UpdatedAt.Format(timeFormat))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
