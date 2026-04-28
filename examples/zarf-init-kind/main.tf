terraform {
  required_providers {
    zarf = {
      source  = "jackyoung/zarf"
      version = "0.1.0"
    }
  }
}

provider "zarf" {
  # Path to your kubeconfig. Defaults to ~/.kube/config or $KUBECONFIG.
  kubeconfig_path = "~/.kube/config"

  # Override if zarf is not on your PATH.
  # zarf_binary = "/usr/local/bin/zarf"
}

# Deploy the Zarf init package into the cluster.
# Download it first:
#   ZARF_VERSION=$(zarf version)
#   curl -sL "https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf-init-amd64-${ZARF_VERSION}.tar.zst" \
#     -o /tmp/zarf-init-amd64-${ZARF_VERSION}.tar.zst
resource "zarf_package" "init" {
  path    = "/tmp/zarf-init-amd64-v0.75.0.tar.zst"
  name    = "init"
  timeout = "20m"
  retries = 2

  skip_signature_validation = true
}

output "init_version" {
  description = "Version of the deployed init package."
  value       = zarf_package.init.version
}

output "init_connectivity" {
  description = "Connectivity mode (e.g. 'airgap')."
  value       = zarf_package.init.connectivity
}

output "init_deployed_components" {
  description = "Components deployed by the init package."
  value       = zarf_package.init.deployed_components
}
