# Example: Zarf Init + Reloader on a kind Cluster

This example deploys two packages into a local [kind](https://kind.sigs.k8s.io/) cluster and demonstrates the two main ways to pass configuration into Zarf/UDS deployments from Terraform.

## What gets deployed

| Resource | Type | Package |
|---|---|---|
| `zarf_package.init` | Zarf init | Bootstraps in-cluster registry, image injector, and admission agent |
| `zarf_package.reloader` | Zarf package | [Stakater Reloader](https://github.com/stakater/Reloader) — watches ConfigMaps/Secrets and triggers rolling restarts |

`depends_on` ensures init always completes before reloader.

## Variable / config patterns

### Pattern 1 — `set_variables` on `zarf_package`

```hcl
resource "zarf_package" "example" {
  path = "oci://..."
  name = "example"

  set_variables = {
    MY_VAR = "my-value"
  }
}
```

`set_variables` maps to `--set-variables KEY=VALUE` flags passed to `zarf package deploy`. It sets **Zarf template variables** — the `###ZARF_VAR_*###` placeholders declared in a package's `zarf.yaml`. Use this for package-level configuration that the package author has explicitly exposed as a variable.

### Pattern 2 — `config_yaml` on `uds_bundle`

When you need to override **Helm chart values** (not just Zarf variables), the right tool is a UDS bundle with a [uds-config](https://uds.defenseunicorns.com/reference/cli/quickstart-and-usage/#configure-a-bundle) file. Use `yamlencode()` to generate the config inline in Terraform:

```hcl
locals {
  reloader_uds_config = {
    packages = [
      {
        name = "mtn-zarf-reloader"
        overrides = {
          reloader = {            # component name from zarf.yaml
            reloader = {          # chart name
              values = [
                { path = "reloader.logFormat", value = "json" },
                { path = "reloader.logLevel",  value = "info" },
              ]
            }
          }
        }
      }
    ]
  }
}

resource "uds_bundle" "reloader" {
  name        = "reloader-bundle"
  bundle_path = "oci://ghcr.io/myorg/bundles/reloader-bundle:1.0.0"
  packages    = ["mtn-zarf-reloader"]

  config_yaml = yamlencode(local.reloader_uds_config)

  # Simple key=value vars passed as --set flags (no Helm path needed)
  set_vars = {
    DOMAIN = "example.com"
  }

  depends_on = [zarf_package.init]
}
```

`config_yaml` is written to a temp file and passed as `uds deploy --config <file>`. Changes to `config_yaml` trigger an in-place redeploy without destroying the resource first.

The `uds_bundle` resource type name is intentionally `uds_bundle` (not `zarf_bundle`) to match UDS tooling conventions, even though it is registered in the `zarf` provider.

### When to use which

| Need | Use |
|---|---|
| Set a `###ZARF_VAR_*###` template variable | `set_variables` on `zarf_package` |
| Override a Helm chart value inside a Zarf package | `config_yaml` on `uds_bundle` |
| Pass a simple key=value to a UDS bundle | `set_vars` on `uds_bundle` |
| Override Helm values inside a UDS bundle | `config_yaml` → `overrides` block |

## Prerequisites

| Tool | Install |
|---|---|
| [Docker](https://docs.docker.com/get-docker/) | Required by kind |
| [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) | `brew install kind` |
| [zarf](https://docs.zarf.dev/getting-started/install/) | Download from [GitHub releases](https://github.com/zarf-dev/zarf/releases) |
| [Terraform](https://developer.hashicorp.com/terraform/install) | `brew install hashicorp/tap/terraform` |

## Steps

### 1. Build and install the provider

From the repository root:

```bash
make install
```

### 2. Create a kind cluster

```bash
kind create cluster --name zarf-test --wait 60s
kubectl cluster-info --context kind-zarf-test
```

### 3. Authenticate to GHCR

The reloader package is on GitHub Container Registry:

```bash
docker login ghcr.io -u <your-github-username>
# Enter a GitHub PAT with read:packages scope when prompted
```

### 4. Download the Zarf init package

```bash
ZARF_VERSION=$(zarf version)
curl -L "https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf-init-amd64-${ZARF_VERSION}.tar.zst" \
  -o /tmp/zarf-init-amd64-${ZARF_VERSION}.tar.zst
```

Update the `path` in `main.tf` if your Zarf version differs from `v0.75.0`.

### 5. Initialise Terraform

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform init
```

### 6. Preview

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform plan
```

Expected: 2 resources to create (`zarf_package.init`, `zarf_package.reloader`).

### 7. Apply

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform apply
```

Deployment order is enforced by `depends_on`:

1. `zarf_package.init` (~60 s) — bootstraps the in-cluster registry
2. `zarf_package.reloader` (~60 s) — images are pushed into the cluster registry then the Helm chart is installed

Expected outputs:

```
init_connectivity            = "airgap"
init_deployed_components     = ["zarf-injector", "zarf-seed-registry", "zarf-registry", "zarf-agent"]
init_version                 = "v0.75.0"
reloader_deployed_components = ["reloader"]
reloader_version             = "0.0.5"
```

### 8. Verify

```bash
zarf package list
kubectl get pods -n reloader
```

### 9. Import existing deployments

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform import zarf_package.init init
TF_CLI_CONFIG_FILE=terraform.rc terraform import zarf_package.reloader mtn-zarf-reloader
TF_CLI_CONFIG_FILE=terraform.rc terraform plan   # should show no changes
```

### 10. Destroy

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform destroy
kind delete cluster --name zarf-test
```

## File reference

| File | Purpose |
|---|---|
| `main.tf` | Provider, both `zarf_package` resources, commented `uds_bundle` example, and outputs |
| `terraform.rc` | Local provider mirror — passed via `TF_CLI_CONFIG_FILE` |

## Troubleshooting

**`zarf binary not found`** — set `zarf_binary` in the provider block to the absolute path.

**`unable to connect to the cluster`** — run `kubectl config current-context`; it should show `kind-zarf-test`. The provider passes `kubeconfig_path` as the `KUBECONFIG` env var to every subprocess.

**`unauthorized` pulling reloader OCI package** — run `docker login ghcr.io` first. Zarf uses the Docker credential store for OCI pulls.

**`Package Not Found After Deploy`** — the `name` attribute must match the package's internal metadata name exactly. Find it with `zarf package inspect definition <oci-ref>` under `metadata.name`.

**UDS config not applied** — check the temp file was written correctly by running `uds deploy --help` to confirm your UDS version supports `--config`. The config YAML structure must match the [UDS config schema](https://uds.defenseunicorns.com/reference/cli/quickstart-and-usage/#configure-a-bundle).
