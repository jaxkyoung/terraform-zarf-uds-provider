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
	"github.com/jackyoung/terraform-provider-zarf/internal/client"
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
	Retries    types.Int64  `tfsdk:"retries"`
	Resume     types.Bool   `tfsdk:"resume"`
}

// NewUDSBundleResource is the resource factory registered with the provider.
func NewUDSBundleResource() resource.Resource {
	return &udsBundleResource{}
}

func (r *udsBundleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_bundle"
}

func (r *udsBundleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deploys and manages a UDS bundle in the configured Kubernetes cluster. " +
			"A UDS bundle is a collection of Zarf packages deployed together. " +
			"Note: UDS does not provide a bundle list command, so Read reconciliation uses " +
			"'zarf package list' to check whether the listed packages are still deployed.",
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
				Description: "Subset of package names within the bundle to deploy. Deploys all packages when omitted. " +
					"Also used during Read to detect drift — if none of these packages are deployed, " +
					"Terraform will plan a new deployment.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"set_vars": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Variables to set during bundle deployment (KEY=value).",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
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

	opts := r.buildDeployOptions(ctx, plan)
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

	// Without a `uds bundle list` command, we approximate drift detection by checking
	// whether the constituent packages are still present in `zarf package list`.
	// If the user did not specify `packages`, we cannot detect drift and keep existing state.
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
		// None of the expected packages are deployed — the bundle is gone.
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

	opts := r.buildDeployOptions(ctx, plan)
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
	// The import ID is the logical bundle name (the `name` attribute).
	// After import, `bundle_path` and `packages` will be unknown and must be added to config.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildDeployOptions converts the Terraform model into CLI options.
func (r *udsBundleResource) buildDeployOptions(ctx context.Context, m udsBundleResourceModel) client.UDSDeployOptions {
	opts := client.UDSDeployOptions{
		BundlePath: m.BundlePath.ValueString(),
		Retries:    int(m.Retries.ValueInt64()),
		Resume:     m.Resume.ValueBool(),
	}
	if !m.Packages.IsNull() && !m.Packages.IsUnknown() {
		m.Packages.ElementsAs(ctx, &opts.Packages, false)
	}
	if !m.SetVars.IsNull() && !m.SetVars.IsUnknown() {
		m.SetVars.ElementsAs(ctx, &opts.SetVars, false)
	}
	return opts
}
