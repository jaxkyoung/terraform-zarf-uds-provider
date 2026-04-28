# terraform-provider-zarf

A Terraform provider for deploying and managing [Zarf](https://docs.zarf.dev/) packages and [UDS](https://uds.defenseunicorns.com/) bundles in Kubernetes clusters.

## Resources

| Resource | Description |
|---|---|
| `zarf_package` | Deploy and manage a Zarf package |
| `uds_bundle` | Deploy and manage a UDS bundle |

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) >= 1.0
- [Go](https://go.dev/) >= 1.23 (to build from source)
- [`zarf`](https://github.com/zarf-dev/zarf/releases) CLI on `$PATH` (or set `zarf_binary`)
- [`uds`](https://github.com/defenseunicorns/uds-cli/releases) CLI on `$PATH` (or set `uds_binary`) — required only when using `uds_bundle`

## Installation

### From the Terraform Registry

```hcl
terraform {
  required_providers {
    zarf = {
      source  = "jaxkyoung/zarf"
      version = "~> 0.1"
    }
  }
}
```

### Build from source

```bash
git clone https://github.com/jaxkyoung/terraform-provider-zarf
cd terraform-provider-zarf
make install
```

This builds the binary and installs it into `~/.terraform.d/plugins/` for local use. Pass `TF_CLI_CONFIG_FILE=examples/zarf-init-kind/terraform.rc` to point Terraform at your local build.

## Provider configuration

```hcl
provider "zarf" {
  # Path to kubeconfig. Defaults to ~/.kube/config or $KUBECONFIG.
  kubeconfig_path = "~/.kube/config"

  # Kubernetes context to use. Defaults to current-context in kubeconfig.
  kube_context = "kind-zarf-test"

  # Absolute paths to CLI binaries if not on $PATH.
  # zarf_binary = "/usr/local/bin/zarf"
  # uds_binary  = "/usr/local/bin/uds"
}
```

## Resources

### `zarf_package`

Deploys a Zarf package from a local tarball or OCI reference.

```hcl
# Local tarball
resource "zarf_package" "nginx" {
  path    = "/packages/zarf-package-nginx-amd64-1.0.0.tar.zst"
  name    = "nginx"
  timeout = "10m"
  retries = 3

  set_variables = {
    NGINX_VERSION = "1.25"
  }
}

# OCI reference — `name` is required for OCI sources
resource "zarf_package" "init" {
  path = "oci://ghcr.io/zarf-dev/packages/init:v0.32.0"
  name = "init"
}

# Selective component deployment
resource "zarf_package" "monitoring" {
  path       = "/packages/zarf-package-monitoring-amd64-2.0.0.tar.zst"
  name       = "monitoring"
  components = ["prometheus", "grafana"]

  set_variables = {
    GRAFANA_ADMIN_PASSWORD = "changeme"
  }

  adopt_existing_resources = true
}
```

**Arguments**

| Argument | Type | Required | Description |
|---|---|---|---|
| `path` | string | yes | Path to the package tarball or OCI reference (`oci://...`) |
| `name` | string | no | Package name. Required for OCI references where the name cannot be inferred. |
| `components` | list(string) | no | Subset of components to deploy. Deploys all when omitted. |
| `set_variables` | map(string) | no | Zarf template variables (`###ZARF_VAR_*###`) to set during deploy |
| `namespace` | string | no | Override the deployment namespace |
| `timeout` | string | no | Deploy timeout, e.g. `"15m"` |
| `retries` | number | no | Number of retry attempts |
| `adopt_existing_resources` | bool | no | Adopt pre-existing Kubernetes resources into Helm charts |
| `skip_signature_validation` | bool | no | Skip package signature validation |

**Computed attributes**

| Attribute | Description |
|---|---|
| `id` | Deployed package name (same as `name`) |
| `version` | Package version as reported by `zarf package list` |
| `connectivity` | Connectivity mode, e.g. `"airgap"` |
| `deployed_components` | Component names currently deployed in the cluster |

**Import**

```bash
terraform import zarf_package.init init
```

The import ID is the package name shown by `zarf package list`. After import, add the `path` attribute to your config before the next `apply`.

---

### `uds_bundle`

Deploys a UDS bundle from a local tarball or OCI reference.

```hcl
resource "uds_bundle" "core" {
  name        = "uds-core"
  bundle_path = "/bundles/uds-bundle-k3d-core-amd64-0.25.0.tar.zst"
  packages    = ["init", "uds-core"]

  set_vars = {
    DOMAIN = "uds.dev"
  }

  retries = 3
}
```

With Helm value overrides via `config_yaml`:

```hcl
locals {
  bundle_config = {
    packages = [{
      name = "my-app"
      overrides = {
        my-component = {
          my-chart = {
            values = [
              { path = "replicaCount", value = 2 },
            ]
          }
        }
      }
    }]
  }
}

resource "uds_bundle" "app" {
  name        = "my-app-bundle"
  bundle_path = "oci://ghcr.io/myorg/bundles/my-app:v1.0.0"
  packages    = ["my-app"]

  config_yaml = yamlencode(local.bundle_config)
}
```

**Arguments**

| Argument | Type | Required | Description |
|---|---|---|---|
| `name` | string | yes | Logical name for the bundle. Used as the resource ID and must be unique. Changing this forces a new resource. |
| `bundle_path` | string | yes | Path to the bundle tarball or OCI reference (`oci://...`). Changing this forces a new resource. |
| `packages` | list(string) | no | Subset of Zarf package names within the bundle to deploy. Also used for drift detection during `refresh`. |
| `set_vars` | map(string) | no | Key=value variables passed as `--set` flags to `uds deploy` |
| `config_yaml` | string | no | UDS config YAML passed via `--config`. Use `yamlencode()` to generate from a structured map. Changes trigger an in-place redeploy. |
| `retries` | number | no | Number of retry attempts |
| `resume` | bool | no | Resume a previous partial deployment, skipping already-deployed packages |

**Computed attributes**

| Attribute | Description |
|---|---|
| `id` | Equals the `name` attribute |

**Drift detection**

UDS provides no bundle list command. When `packages` is set, the provider checks `zarf package list` for those package names. If none are found, Terraform plans a new deployment.

**Import**

```bash
terraform import uds_bundle.core uds-core
```

The import ID is the logical bundle `name`. After import, add `bundle_path` and optionally `packages` to your config.

## Configuration patterns

| Need | Use |
|---|---|
| Set a `###ZARF_VAR_*###` template variable | `set_variables` on `zarf_package` |
| Pass a simple key=value to a UDS bundle | `set_vars` on `uds_bundle` |
| Override a Helm chart value inside a UDS bundle | `config_yaml` on `uds_bundle` |

## Examples

- [`examples/provider/`](examples/provider/) — minimal provider configuration
- [`examples/zarf_package/`](examples/zarf_package/) — local tarball, OCI reference, component selection
- [`examples/uds_bundle/`](examples/uds_bundle/) — bundle deployment with drift detection
- [`examples/zarf-init-kind/`](examples/zarf-init-kind/) — end-to-end example on a local kind cluster

## Development

```bash
# Build
make build

# Build and install locally
make install

# Run tests
make test

# Lint
make lint

# Format
make fmt
```

## License

Apache 2.0
