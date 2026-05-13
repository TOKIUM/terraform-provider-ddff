package provider

import (
	"context"
	"net/url"
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
		MarkdownDescription: "Manages Datadog Feature Flags resources. Credentials and the API URL are read from the provider configuration first, then from `DD_*` environment variables (e.g. `DD_API_KEY`), then from `DATADOG_*` environment variables (e.g. `DATADOG_API_KEY`). The naming and precedence match the official `DataDog/datadog` Terraform provider.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				MarkdownDescription: "Datadog API key. Falls back to `DD_API_KEY`, then `DATADOG_API_KEY`.",
				Optional:            true,
				Sensitive:           true,
			},
			"app_key": schema.StringAttribute{
				MarkdownDescription: "Datadog application key. Falls back to `DD_APP_KEY`, then `DATADOG_APP_KEY`.",
				Optional:            true,
				Sensitive:           true,
			},
			"api_url": schema.StringAttribute{
				MarkdownDescription: "Full Datadog API URL (e.g. `https://api.datadoghq.com`, `https://api.datadoghq.eu`, `https://api.us3.datadoghq.com`). Must include both protocol and host. Falls back to `DD_HOST`, then `DATADOG_HOST`. When unset the Datadog SDK default (`https://api.datadoghq.com`) is used.",
				Optional:            true,
			},
		},
	}
}

type providerModel struct {
	APIKey types.String `tfsdk:"api_key"`
	AppKey types.String `tfsdk:"app_key"`
	APIURL types.String `tfsdk:"api_url"`
}

// Clients holds the Datadog SDK clients and the credentials needed to
// build an authenticated context per request. We deliberately do NOT
// capture the configure-time context here: that context is canceled once
// Configure returns, so each CRUD call rebuilds the DD context from its
// own request-scoped ctx.
type Clients struct {
	APIKey       string
	AppKey       string
	APIURL       string
	FeatureFlags *datadogV2.FeatureFlagsApi
}

// Context returns ctx augmented with the Datadog API keys and (optionally)
// the alternate server URL so the generated SDK picks them up.
//
// When APIURL is set, we override the SDK's default server URL using the
// same ContextServerIndex + ContextServerVariables pattern that the
// official DataDog/terraform-provider-datadog uses. The URL must already
// have been validated at Configure time.
func (c *Clients) Context(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {Key: c.APIKey},
		"appKeyAuth": {Key: c.AppKey},
	})
	if c.APIURL != "" {
		parsed, err := url.Parse(c.APIURL)
		if err == nil && parsed.Host != "" && parsed.Scheme != "" {
			ctx = context.WithValue(ctx, datadog.ContextServerIndex, 1)
			ctx = context.WithValue(ctx, datadog.ContextServerVariables, map[string]string{
				"name":     parsed.Host,
				"protocol": parsed.Scheme,
			})
		}
	}
	return ctx
}

func (p *ddffProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	apiKey := stringOrEnv(cfg.APIKey, "DD_API_KEY", "DATADOG_API_KEY")
	appKey := stringOrEnv(cfg.AppKey, "DD_APP_KEY", "DATADOG_APP_KEY")
	apiURL := stringOrEnv(cfg.APIURL, "DD_HOST", "DATADOG_HOST")

	if apiKey == "" {
		resp.Diagnostics.AddError(
			"Missing Datadog API key",
			"Set the `api_key` provider attribute, the `DD_API_KEY` environment variable, or `DATADOG_API_KEY`.",
		)
	}
	if appKey == "" {
		resp.Diagnostics.AddError(
			"Missing Datadog application key",
			"Set the `app_key` provider attribute, the `DD_APP_KEY` environment variable, or `DATADOG_APP_KEY`.",
		)
	}
	if apiURL != "" {
		parsed, parseErr := url.Parse(apiURL)
		if parseErr != nil {
			resp.Diagnostics.AddError("Invalid api_url", parseErr.Error())
		} else if parsed.Host == "" || parsed.Scheme == "" {
			resp.Diagnostics.AddError("Invalid api_url", "missing protocol or host: "+apiURL)
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}

	configuration := datadog.NewConfiguration()
	configuration.UserAgent = "terraform-provider-ddff/" + p.version
	// DDFF_DEBUG=1 enables verbose HTTP request/response logging in the
	// underlying Datadog SDK. Useful for debugging schema mismatches but
	// noisy enough to default off.
	if os.Getenv("DDFF_DEBUG") != "" {
		configuration.Debug = true
	}
	apiClient := datadog.NewAPIClient(configuration)

	clients := &Clients{
		APIKey:       apiKey,
		AppKey:       appKey,
		APIURL:       apiURL,
		FeatureFlags: datadogV2.NewFeatureFlagsApi(apiClient),
	}
	resp.DataSourceData = clients
	resp.ResourceData = clients
}

func (p *ddffProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewFeatureFlagResource,
		NewEnvironmentResource,
		NewFeatureFlagEnvironmentResource,
		NewAllocationSetResource,
	}
}

func (p *ddffProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewFeatureFlagDataSource,
		NewEnvironmentDataSource,
	}
}

func stringOrEnv(v types.String, envs ...string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	for _, env := range envs {
		if val := os.Getenv(env); val != "" {
			return val
		}
	}
	return ""
}
