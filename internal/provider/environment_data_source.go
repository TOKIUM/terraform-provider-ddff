package provider

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*environmentDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*environmentDataSource)(nil)
)

func NewEnvironmentDataSource() datasource.DataSource {
	return &environmentDataSource{}
}

type environmentDataSource struct {
	clients *Clients
}

type environmentDataSourceModel struct {
	ID                         types.String `tfsdk:"id"`
	Name                       types.String `tfsdk:"name"`
	Queries                    types.List   `tfsdk:"queries"`
	IsProduction               types.Bool   `tfsdk:"is_production"`
	RequireFeatureFlagApproval types.Bool   `tfsdk:"require_feature_flag_approval"`
	Description                types.String `tfsdk:"description"`
	CreatedAt                  types.String `tfsdk:"created_at"`
	UpdatedAt                  types.String `tfsdk:"updated_at"`
}

func (d *environmentDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_environment"
}

func (d *environmentDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Looks up a Datadog feature flag environment by its display name. The Datadog API does not expose a stable per-environment key, so lookup is by name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "UUID of the environment.",
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Display name of the environment to look up. Must match exactly.",
			},
			"queries":                       schema.ListAttribute{ElementType: types.StringType, Computed: true},
			"is_production":                 schema.BoolAttribute{Computed: true},
			"require_feature_flag_approval": schema.BoolAttribute{Computed: true},
			"description":                   schema.StringAttribute{Computed: true},
			"created_at":                    schema.StringAttribute{Computed: true},
			"updated_at":                    schema.StringAttribute{Computed: true},
		},
	}
}

func (d *environmentDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *environmentDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data environmentDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	res, httpResp, err := d.clients.FeatureFlags.ListFeatureFlagsEnvironments(d.clients.Context(ctx), *datadogV2.NewListFeatureFlagsEnvironmentsOptionalParameters().WithName(name))
	if err != nil {
		resp.Diagnostics.AddError("Failed to look up environment", apiErr(err, httpResp))
		return
	}

	// The list endpoint accepts a name filter but in practice ignores it,
	// so we do a client-side exact-match scan.
	var match *datadogV2.Environment
	for i := range res.Data {
		if res.Data[i].Attributes.Name == name {
			match = &res.Data[i]
			break
		}
	}
	if match == nil {
		resp.Diagnostics.AddError("Environment not found", fmt.Sprintf("no environment exists with name %q", name))
		return
	}

	attrs := match.Attributes
	data.ID = types.StringValue(match.Id.String())
	data.Name = types.StringValue(attrs.Name)
	if attrs.IsProduction != nil {
		data.IsProduction = types.BoolValue(*attrs.IsProduction)
	} else {
		data.IsProduction = types.BoolValue(false)
	}
	if attrs.RequireFeatureFlagApproval != nil {
		data.RequireFeatureFlagApproval = types.BoolValue(*attrs.RequireFeatureFlagApproval)
	} else {
		data.RequireFeatureFlagApproval = types.BoolValue(false)
	}
	if attrs.Description.IsSet() && attrs.Description.Get() != nil {
		data.Description = types.StringValue(*attrs.Description.Get())
	} else {
		data.Description = types.StringNull()
	}

	queries, qDiag := types.ListValueFrom(ctx, types.StringType, attrs.Queries)
	resp.Diagnostics.Append(qDiag...)
	data.Queries = queries

	if attrs.CreatedAt != nil {
		data.CreatedAt = types.StringValue(attrs.CreatedAt.Format(timeFormat))
	} else {
		data.CreatedAt = types.StringNull()
	}
	if attrs.UpdatedAt != nil {
		data.UpdatedAt = types.StringValue(attrs.UpdatedAt.Format(timeFormat))
	} else {
		data.UpdatedAt = types.StringNull()
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
