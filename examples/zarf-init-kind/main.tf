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

  # Override if zarf/uds are not on your PATH.
  # zarf_binary = "/usr/local/bin/zarf"
  # uds_binary  = "/usr/local/bin/uds"
}

# ---------------------------------------------------------------------------
# 1. Zarf init — bootstraps the in-cluster registry, image injector, and
#    admission agent. All subsequent Zarf packages depend on this.
#
# Download the init tarball first:
#   ZARF_VERSION=$(zarf version)
#   curl -L "https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf-init-amd64-${ZARF_VERSION}.tar.zst" \
#     -o /tmp/zarf-init-amd64-${ZARF_VERSION}.tar.zst
# ---------------------------------------------------------------------------
resource "zarf_package" "init" {
  path    = "/tmp/zarf-init-amd64-v0.75.0.tar.zst"
  name    = "init"
  timeout = "20m"
  retries = 2

  skip_signature_validation = true
}

# ---------------------------------------------------------------------------
# 2. Reloader — watches ConfigMaps/Secrets and triggers rolling restarts.
#    Deployed as a zarf_package directly from GHCR (requires docker login).
#
#    set_variables passes Zarf template variables (###ZARF_VAR_*###) defined
#    in the package's zarf.yaml. The reloader package doesn't declare any
#    Zarf variables, so this block is illustrative — remove it in practice.
#
#    For Helm chart value overrides on a bare zarf_package, use a uds_bundle
#    with config_yaml instead (see the commented block below).
# ---------------------------------------------------------------------------
resource "zarf_package" "reloader" {
  path    = "oci://ghcr.io/defencedigital/mtn-zarf-reloader:0.0.5"
  name    = "mtn-zarf-reloader"
  timeout = "10m"
  retries = 2

  skip_signature_validation = true

  # Zarf template variables — set any ###ZARF_VAR_*### defined in zarf.yaml.
  # set_variables = {
  #   MY_VAR = "my-value"
  # }

  depends_on = [zarf_package.init]
}

# ---------------------------------------------------------------------------
# 3. UDS bundle example — shows how to pass Helm chart value overrides via
#    a uds-config, yamlencode'd directly in Terraform.
#
#    This pattern is used when you have a uds-bundle.yaml artifact that wraps
#    one or more Zarf packages and you need to override Helm values per-chart.
#
#    The uds_bundle resource type is registered in the zarf provider but uses
#    the "uds_bundle" type name to match UDS tooling conventions.
#
#    Uncomment and adjust once you have a UDS bundle artifact to deploy.
# ---------------------------------------------------------------------------

locals {
  # UDS config structure — controls Helm value overrides per package/component/chart.
  # yamlencode() converts this map to the YAML string that uds deploy --config expects.
  reloader_uds_config = {
    packages = [
      {
        name = "mtn-zarf-reloader"
        overrides = {
          # structure: component-name -> chart-name -> override block
          reloader = {
            reloader = {
              values = [
                { path = "reloader.logFormat", value = "json" },
                { path = "reloader.logLevel", value = "info" },
                { path = "reloader.reloadOnCreate", value = true },
                { path = "reloader.syncAfterRestart", value = true },
              ]
              # variables lets UDS expose Helm values as named variables,
              # which can then be set via --set or the variables block here.
              variables = [
                { name = "RELOADER_LOG_FORMAT", path = "reloader.logFormat", value = "json" },
              ]
            }
          }
        }
      }
    ]
  }
}

# resource "uds_bundle" "reloader" {
#   name        = "reloader-bundle"
#   bundle_path = "oci://ghcr.io/myorg/bundles/reloader-bundle:1.0.0"
#   packages    = ["mtn-zarf-reloader"]
#   retries     = 2
#
#   # Pass the UDS config as YAML — yamlencode() handles serialisation.
#   config_yaml = yamlencode(local.reloader_uds_config)
#
#   # Simple key=value variables passed as --set flags (no Helm path needed).
#   set_vars = {
#     DOMAIN = "example.com"
#   }
#
#   depends_on = [zarf_package.init]
# }

# ---------------------------------------------------------------------------
# Outputs
# ---------------------------------------------------------------------------

output "init_version" {
  description = "Version of the deployed init package."
  value       = zarf_package.init.version
}

output "init_connectivity" {
  description = "Connectivity mode reported by Zarf (e.g. 'airgap')."
  value       = zarf_package.init.connectivity
}

output "init_deployed_components" {
  description = "Components deployed by the init package."
  value       = zarf_package.init.deployed_components
}

output "reloader_version" {
  description = "Version of the deployed reloader package."
  value       = zarf_package.reloader.version
}

output "reloader_deployed_components" {
  description = "Components deployed by the reloader package."
  value       = zarf_package.reloader.deployed_components
}
