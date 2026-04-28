terraform {
  required_providers {
    zarf = {
      source  = "jackyoung/zarf"
      version = "~> 0.1"
    }
  }
}

provider "zarf" {
  kubeconfig_path = "~/.kube/config"
}

# Deploy a local Zarf package tarball.
resource "zarf_package" "nginx" {
  path    = "/packages/zarf-package-nginx-amd64-1.0.0.tar.zst"
  name    = "nginx"
  timeout = "10m"
  retries = 3

  set_variables = {
    NGINX_VERSION = "1.25"
  }
}

# Deploy a package from an OCI registry.
# The `name` attribute is required for OCI sources because the package's
# internal metadata name cannot be inferred from the reference alone.
resource "zarf_package" "init" {
  path = "oci://ghcr.io/zarf-dev/packages/init:v0.32.0"
  name = "init"

  skip_signature_validation = false
  retries                   = 3
}

# Deploy only specific components of a package.
resource "zarf_package" "monitoring" {
  path       = "/packages/zarf-package-monitoring-amd64-2.0.0.tar.zst"
  name       = "monitoring"
  components = ["prometheus", "grafana"]

  set_variables = {
    GRAFANA_ADMIN_PASSWORD = "changeme"
  }

  adopt_existing_resources = true
}

output "nginx_generation" {
  value       = zarf_package.nginx.generation
  description = "Incremented each time the package is redeployed."
}

output "nginx_deployed_components" {
  value = zarf_package.nginx.deployed_components
}
