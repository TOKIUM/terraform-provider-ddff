package provider

import (
	"context"
	"os"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*ddffProvider)(nil)

type ddffProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &ddffProvider{version: version}
	}
}

func (p *ddffProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "ddff"
	resp.Version = p.version
}

func (p *ddffProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages Datadog Feature Flags resources.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Datadog API key. Falls back to the `DD_API_KEY` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"app_key": schema.StringAttribute{
				MarkdownDescription: "Datadog application key. Falls back to the `DD_APP_KEY` environment variable.",
				Optional:            true,
				Sensitive:           true,
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Datadog site (e.g. `datadoghq.com`, `datadoghq.eu`, `us3.datadoghq.com`). Falls back to the `DD_SITE` environment variable, then `datadoghq.com`.",
				Optional:            true,
			},
		},
	}
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	AppKey types.String `tfsdk:"app_key"`
	Site   types.String `tfsdk:"site"`
}

// Clients holds the Datadog SDK clients passed to resources.
type Clients struct {
	Ctx          context.Context
	FeatureFlags *datadogV2.FeatureFlagsApi
}

func (p *ddffProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := stringOrEnv(cfg.APIKey, "DD_API_KEY")
	appKey := stringOrEnv(cfg.AppKey, "DD_APP_KEY")
	site := stringOrEnv(cfg.Site, "DD_SITE")
	if site == "" {
		site = "datadoghq.com"
	}

	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing Datadog API key",
			"Set the `api_key` provider attribute or the `DD_API_KEY` environment variable.",
		)
	}
	if appKey == "" {
		resp.Diagnostics.AddError(
			"Missing Datadog application key",
			"Set the `app_key` provider attribute or the `DD_APP_KEY` environment variable.",
		)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	ddCtx := datadog.NewDefaultContext(ctx)
	ddCtx = context.WithValue(ddCtx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: apiKey},
		"appKeyAuth": {Key: appKey},
	})
	ddCtx = context.WithValue(ddCtx, datadog.ContextServerVariables, map[string]string{
		"site": site,
	})

	configuration := datadog.NewConfiguration()
	configuration.UserAgent = "terraform-provider-datadog-feature-flags/" + p.version
	apiClient := datadog.NewAPIClient(configuration)

	clients := &Clients{
		Ctx:          ddCtx,
		FeatureFlags: datadogV2.NewFeatureFlagsApi(apiClient),
	}
	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *ddffProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFeatureFlagResource,
	}
}

func (p *ddffProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func stringOrEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}
