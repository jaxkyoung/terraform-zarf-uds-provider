package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/jaxkyoung/terraform-provider-zarf/internal/client"
)

var _ resource.Resource = &udsBundleResource{}
var _ resource.ResourceWithImportState = &udsBundleResource{}
var _ resource.ResourceWithConfigure = &udsBundleResource{}

type udsBundleResource struct {
	client *client.Client
}

type udsBundleResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	BundlePath types.String `tfsdk:"bundle_path"`
	Packages   types.List   `tfsdk:"packages"`
	SetVars    types.Map    `tfsdk:"set_vars"`
	ConfigYAML types.String `tfsdk:"config_yaml"`
	Retries    types.Int64  `tfsdk:"retries"`
	Resume     types.Bool   `tfsdk:"resume"`
}

// NewUDSBundleResource is the resource factory registered with the provider.
func NewUDSBundleResource() resource.Resource {
	return &udsBundleResource{}
}

func (r *udsBundleResource) Metadata(_ context.Context, _ resource.MetadataRequest, resp *resource.MetadataResponse) {
	// Intentionally not prefixed with the provider name so the HCL resource
	// type matches the UDS tooling convention: resource "uds_bundle" "..."
	resp.TypeName = "uds_bundle"
}

func (r *udsBundleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deploys and manages a UDS bundle in the configured Kubernetes cluster. " +
			"A UDS bundle is a collection of Zarf packages deployed together via 'uds deploy'. " +
			"Read drift detection uses 'zarf package list' to check constituent packages, " +
			"since UDS provides no bundle list command.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "Equals the 'name' attribute. Used as the unique resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Logical name for this bundle deployment. Used as the resource ID and must be unique.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"bundle_path": schema.StringAttribute{
				Required:    true,
				Description: "Path to the UDS bundle tarball or OCI reference (oci://...).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"packages": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Subset of Zarf package names within the bundle to deploy. " +
					"Deploys all packages when omitted. Also used for drift detection during Read — " +
					"if none of these package names appear in 'zarf package list', Terraform plans a new deployment.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"set_vars": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Package-level variables passed as --set KEY=VALUE flags to 'uds deploy'. " +
					"For Helm chart value overrides, use config_yaml instead.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"config_yaml": schema.StringAttribute{
				Optional: true,
				Description: "UDS config file contents as a YAML string, passed to 'uds deploy --config'. " +
					"Supports per-package Helm chart value overrides and variable injection. " +
					"Use yamlencode() in Terraform to generate this from a structured map. " +
					"Changes to this attribute trigger an in-place redeploy (no destroy required).\n\n" +
					"Example structure:\n" +
					"  packages:\n" +
					"    - name: my-package\n" +
					"      overrides:\n" +
					"        my-component:\n" +
					"          my-chart:\n" +
					"            values:\n" +
					"              - path: replicaCount\n" +
					"                value: 2",
			},
			"retries": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of retry attempts for package deployments.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"resume": schema.BoolAttribute{
				Optional:    true,
				Description: "Resume a previous partial deployment, skipping already-deployed packages.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *udsBundleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Provider Data Type",
			fmt.Sprintf("Expected *client.Client, got %T", req.ProviderData),
		)
		return
	}
	if err := c.ValidateUDSBinary(ctx); err != nil {
		resp.Diagnostics.AddError("UDS Binary Validation Failed", err.Error())
		return
	}
	r.client = c
}

func (r *udsBundleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan udsBundleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	opts, err := r.buildDeployOptions(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Prepare Deploy Options", err.Error())
		return
	}
	if err := r.client.UDSDeployBundle(ctx, opts); err != nil {
		resp.Diagnostics.AddError("UDS Deploy Failed", err.Error())
		return
	}

	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *udsBundleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state udsBundleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var expectedPkgs []string
	if !state.Packages.IsNull() && !state.Packages.IsUnknown() {
		resp.Diagnostics.Append(state.Packages.ElementsAs(ctx, &expectedPkgs, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Without a `uds bundle list` command, drift detection checks constituent
	// package names in `zarf package list`. If packages is not set we can't
	// detect drift, so keep the existing state.
	if len(expectedPkgs) == 0 {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
		return
	}

	allDeployed, err := r.client.ZarfListPackages(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to List Packages for Bundle Drift Detection", err.Error())
		return
	}

	deployedNames := make(map[string]bool, len(allDeployed))
	for _, p := range allDeployed {
		deployedNames[p.Name] = true
	}

	anyFound := false
	for _, pkg := range expectedPkgs {
		if deployedNames[pkg] {
			anyFound = true
			break
		}
	}

	if !anyFound {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *udsBundleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan udsBundleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state udsBundleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID

	opts, err := r.buildDeployOptions(ctx, plan)
	if err != nil {
		resp.Diagnostics.AddError("Failed to Prepare Deploy Options", err.Error())
		return
	}
	if err := r.client.UDSDeployBundle(ctx, opts); err != nil {
		resp.Diagnostics.AddError("UDS Re-Deploy (Update) Failed", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *udsBundleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state udsBundleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var packages []string
	if !state.Packages.IsNull() && !state.Packages.IsUnknown() {
		resp.Diagnostics.Append(state.Packages.ElementsAs(ctx, &packages, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if err := r.client.UDSRemoveBundle(ctx, state.BundlePath.ValueString(), packages); err != nil {
		resp.Diagnostics.AddError("UDS Remove Failed", err.Error())
		return
	}
}

func (r *udsBundleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import ID is the logical bundle name (the `name` attribute).
	// After import, bundle_path, packages, and config_yaml are unknown — add them to config before next apply.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildDeployOptions converts the Terraform model into CLI options.
func (r *udsBundleResource) buildDeployOptions(ctx context.Context, m udsBundleResourceModel) (client.UDSDeployOptions, error) {
	opts := client.UDSDeployOptions{
		BundlePath: m.BundlePath.ValueString(),
		Retries:    int(m.Retries.ValueInt64()),
		Resume:     m.Resume.ValueBool(),
		ConfigYAML: m.ConfigYAML.ValueString(),
	}
	if !m.Packages.IsNull() && !m.Packages.IsUnknown() {
		m.Packages.ElementsAs(ctx, &opts.Packages, false)
	}
	if !m.SetVars.IsNull() && !m.SetVars.IsUnknown() {
		m.SetVars.ElementsAs(ctx, &opts.SetVars, false)
	}
	return opts, nil
}
