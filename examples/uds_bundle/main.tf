terraform {
  required_providers {
    zarf = {
      source  = "jaxkyoung/zarf"
      version = "~> 0.1"
    }
  }
}

provider "zarf" {
  kubeconfig_path = "~/.kube/config"
}

# Deploy a UDS bundle from a local tarball.
resource "zarf_bundle" "core" {
  name        = "uds-core"
  bundle_path = "/bundles/uds-bundle-k3d-core-amd64-0.25.0.tar.zst"

  # Specifying packages enables drift detection: if none of these appear
  # in `zarf package list`, Terraform will plan a new deployment.
  packages = ["init", "uds-core"]

  set_vars = {
    DOMAIN = "uds.dev"
  }

  retries = 3
}

# Deploy a UDS bundle from an OCI registry, resuming a partial deployment.
resource "zarf_bundle" "app" {
  name        = "my-app-bundle"
  bundle_path = "oci://ghcr.io/myorg/bundles/my-app:v1.0.0"
  packages    = ["my-app-api", "my-app-frontend"]
  resume      = false

  set_vars = {
    APP_ENV    = "production"
    REPLICAS   = "3"
  }
}

output "core_bundle_name" {
  value = zarf_bundle.core.name
}
