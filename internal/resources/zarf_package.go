package resources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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

var _ resource.Resource = &zarfPackageResource{}
var _ resource.ResourceWithImportState = &zarfPackageResource{}
var _ resource.ResourceWithConfigure = &zarfPackageResource{}

type zarfPackageResource struct {
	client *client.Client
}

type zarfPackageResourceModel struct {
	ID                      types.String `tfsdk:"id"`
	Path                    types.String `tfsdk:"path"`
	Name                    types.String `tfsdk:"name"`
	Version                 types.String `tfsdk:"version"`
	Connectivity            types.String `tfsdk:"connectivity"`
	Components              types.List   `tfsdk:"components"`
	SetVariables            types.Map    `tfsdk:"set_variables"`
	Namespace               types.String `tfsdk:"namespace"`
	Timeout                 types.String `tfsdk:"timeout"`
	Retries                 types.Int64  `tfsdk:"retries"`
	AdoptExistingResources  types.Bool   `tfsdk:"adopt_existing_resources"`
	SkipSignatureValidation types.Bool   `tfsdk:"skip_signature_validation"`
	DeployedComponents      types.List   `tfsdk:"deployed_components"`
}

// NewZarfPackageResource is the resource factory registered with the provider.
func NewZarfPackageResource() resource.Resource {
	return &zarfPackageResource{}
}

func (r *zarfPackageResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_package"
}

func (r *zarfPackageResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Deploys and manages a Zarf package in the configured Kubernetes cluster.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The deployed package name, used as the unique resource identifier.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"path": schema.StringAttribute{
				Required:    true,
				Description: "Path to the Zarf package tarball or OCI reference (oci://...).",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Package name as reported by zarf package list. Set explicitly for OCI-sourced packages where the name cannot be inferred.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				Computed:    true,
				Description: "Package version as reported by zarf package list.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"connectivity": schema.StringAttribute{
				Computed:    true,
				Description: "Connectivity mode reported by zarf package list (e.g. 'airgap').",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"components": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Subset of components to deploy. Deploys all components when omitted.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"set_variables": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Variables to set during package deployment.",
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Optional:    true,
				Description: "Override the deployment namespace.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"timeout": schema.StringAttribute{
				Optional:    true,
				Description: "Deployment timeout duration, e.g. '15m'. Defaults to Zarf's built-in timeout.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"retries": schema.Int64Attribute{
				Optional:    true,
				Description: "Number of retry attempts. Defaults to Zarf's built-in retry count.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"adopt_existing_resources": schema.BoolAttribute{
				Optional:    true,
				Description: "Adopt pre-existing Kubernetes resources into Helm charts during deployment.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"skip_signature_validation": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip package signature validation.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"deployed_components": schema.ListAttribute{
				Computed:    true,
				ElementType: types.StringType,
				Description: "Component names that are currently deployed, as reported by the cluster.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *zarfPackageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	if err := c.ValidateZarfBinary(ctx); err != nil {
		resp.Diagnostics.AddError("Zarf Binary Validation Failed", err.Error())
		return
	}
	r.client = c
}

func (r *zarfPackageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan zarfPackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Snapshot existing package names so we can identify the newly deployed one
	// if the user did not specify `name` (needed for OCI refs).
	preSnap, err := r.client.ZarfSnapshotNames(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list packages before deploy", err.Error())
		return
	}

	opts := r.buildDeployOptions(ctx, plan)
	if err := r.client.ZarfDeployPackage(ctx, opts); err != nil {
		resp.Diagnostics.AddError("Zarf Deploy Failed", err.Error())
		return
	}

	deployed, err := r.resolveDeployedPackage(ctx, plan, preSnap)
	if err != nil {
		resp.Diagnostics.AddError("State Reconciliation Failed", err.Error())
		return
	}
	if deployed == nil {
		resp.Diagnostics.AddError(
			"Package Not Found After Deploy",
			"zarf package list did not return the package after a successful deploy. "+
				"Set the 'name' attribute explicitly to help identify the package.",
		)
		return
	}

	r.populateModelFromDeployed(ctx, &plan, deployed)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *zarfPackageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state zarfPackageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	deployed, err := r.client.ZarfFindPackage(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Failed to List Zarf Packages", err.Error())
		return
	}
	if deployed == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.populateModelFromDeployed(ctx, &state, deployed)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *zarfPackageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan zarfPackageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the ID from current state so we can look the package up after redeploy.
	var state zarfPackageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	plan.Name = state.Name

	opts := r.buildDeployOptions(ctx, plan)
	if err := r.client.ZarfDeployPackage(ctx, opts); err != nil {
		resp.Diagnostics.AddError("Zarf Re-Deploy (Update) Failed", err.Error())
		return
	}

	deployed, err := r.client.ZarfFindPackage(ctx, state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Post-Update State Read Failed", err.Error())
		return
	}
	if deployed == nil {
		resp.Diagnostics.AddError(
			"Package Not Found After Update",
			fmt.Sprintf("Package %q not found in zarf package list after redeploy.", state.ID.ValueString()),
		)
		return
	}

	r.populateModelFromDeployed(ctx, &plan, deployed)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *zarfPackageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state zarfPackageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var components []string
	if !state.Components.IsNull() && !state.Components.IsUnknown() {
		resp.Diagnostics.Append(state.Components.ElementsAs(ctx, &components, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	if err := r.client.ZarfRemovePackage(ctx, state.ID.ValueString(), components); err != nil {
		resp.Diagnostics.AddError("Zarf Remove Failed", err.Error())
		return
	}
}

func (r *zarfPackageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// The import ID is the package name as shown by `zarf package list`.
	// After import, `path` will be unknown — the user must add it to their config.
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// buildDeployOptions converts the Terraform model into CLI options.
func (r *zarfPackageResource) buildDeployOptions(ctx context.Context, m zarfPackageResourceModel) client.ZarfDeployOptions {
	opts := client.ZarfDeployOptions{
		Path:                    m.Path.ValueString(),
		Namespace:               m.Namespace.ValueString(),
		Timeout:                 m.Timeout.ValueString(),
		Retries:                 int(m.Retries.ValueInt64()),
		AdoptExistingResources:  m.AdoptExistingResources.ValueBool(),
		SkipSignatureValidation: m.SkipSignatureValidation.ValueBool(),
	}
	if !m.Components.IsNull() && !m.Components.IsUnknown() {
		m.Components.ElementsAs(ctx, &opts.Components, false)
	}
	if !m.SetVariables.IsNull() && !m.SetVariables.IsUnknown() {
		m.SetVariables.ElementsAs(ctx, &opts.SetVariables, false)
	}
	return opts
}

// populateModelFromDeployed fills computed attributes from cluster state.
func (r *zarfPackageResource) populateModelFromDeployed(_ context.Context, m *zarfPackageResourceModel, d *client.DeployedPackage) {
	m.ID = types.StringValue(d.Name)
	m.Name = types.StringValue(d.Name)
	m.Version = types.StringValue(d.Version)
	m.Connectivity = types.StringValue(d.Connectivity)

	if d.NamespaceOverride != "" && m.Namespace.IsNull() {
		m.Namespace = types.StringValue(d.NamespaceOverride)
	}

	componentVals := make([]attr.Value, len(d.Components))
	for i, c := range d.Components {
		componentVals[i] = types.StringValue(c)
	}
	listVal, diags := types.ListValue(types.StringType, componentVals)
	if diags.HasError() {
		m.DeployedComponents = types.ListValueMust(types.StringType, []attr.Value{})
	} else {
		m.DeployedComponents = listVal
	}
}

// resolveDeployedPackage finds the newly deployed package. If the user specified
// `name`, it queries directly. Otherwise it diffs the pre-deploy snapshot against
// the current list to identify the new package.
func (r *zarfPackageResource) resolveDeployedPackage(
	ctx context.Context,
	plan zarfPackageResourceModel,
	preSnap map[string]struct{},
) (*client.DeployedPackage, error) {
	// Fast path: user specified the package name.
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() && plan.Name.ValueString() != "" {
		return r.client.ZarfFindPackage(ctx, plan.Name.ValueString())
	}

	// Slow path: diff pre vs post to find the new package.
	postPackages, err := r.client.ZarfListPackages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing packages after deploy: %w", err)
	}

	var newPackages []*client.DeployedPackage
	for i := range postPackages {
		if _, existed := preSnap[postPackages[i].Name]; !existed {
			newPackages = append(newPackages, &postPackages[i])
		}
	}

	if len(newPackages) == 1 {
		return newPackages[0], nil
	}
	if len(newPackages) > 1 {
		return nil, fmt.Errorf(
			"multiple new packages appeared after deploy — set the 'name' attribute to disambiguate: %v",
			packageNames(newPackages),
		)
	}
	return nil, nil
}

func packageNames(pkgs []*client.DeployedPackage) []string {
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	return names
}
