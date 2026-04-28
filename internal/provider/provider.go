package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jaxkyoung/terraform-provider-zarf/internal/client"
	"github.com/jaxkyoung/terraform-provider-zarf/internal/resources"
)

var _ provider.Provider = &zarfProvider{}

type zarfProvider struct {
	version string
}

type zarfProviderModel struct {
	KubeconfigPath types.String `tfsdk:"kubeconfig_path"`
	KubeContext    types.String `tfsdk:"kube_context"`
	ZarfBinary     types.String `tfsdk:"zarf_binary"`
	UDSBinary      types.String `tfsdk:"uds_binary"`
}

// New returns a provider factory function.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &zarfProvider{version: version}
	}
}

func (p *zarfProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "zarf"
	resp.Version = p.version
}

func (p *zarfProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Zarf packages and UDS bundles deployed into Kubernetes clusters.",
		Attributes: map[string]schema.Attribute{
			"kubeconfig_path": schema.StringAttribute{
				Optional:    true,
				Description: "Path to the kubeconfig file. Defaults to ~/.kube/config or the KUBECONFIG environment variable.",
			},
			"kube_context": schema.StringAttribute{
				Optional:    true,
				Description: "Kubernetes context name within the kubeconfig. Set the desired context as current-context in your kubeconfig before applying.",
			},
			"zarf_binary": schema.StringAttribute{
				Optional:    true,
				Description: "Path to the zarf binary. Defaults to 'zarf' found via PATH.",
			},
			"uds_binary": schema.StringAttribute{
				Optional:    true,
				Description: "Path to the uds binary. Defaults to 'uds' found via PATH.",
			},
		},
	}
}

func (p *zarfProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data zarfProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := client.Config{
		KubeconfigPath: data.KubeconfigPath.ValueString(),
		KubeContext:    data.KubeContext.ValueString(),
		ZarfBinary:     data.ZarfBinary.ValueString(),
		UDSBinary:      data.UDSBinary.ValueString(),
	}

	c := client.NewClient(cfg)

	resp.ResourceData = c
	resp.DataSourceData = c
}

func (p *zarfProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		resources.NewZarfPackageResource,
		resources.NewUDSBundleResource,
	}
}

func (p *zarfProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}
