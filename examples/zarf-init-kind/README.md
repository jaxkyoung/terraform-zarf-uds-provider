# Example: Zarf Init on a kind Cluster

This example deploys the [Zarf init package](https://docs.zarf.dev/ref/init-package/) into a local [kind](https://kind.sigs.k8s.io/) cluster using the Terraform provider. The init package bootstraps a cluster with Zarf's in-cluster container registry, image injector, and admission agent — the prerequisite for all subsequent Zarf package deployments.

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

This compiles the provider and copies the binary to `~/.terraform.d/plugins/`.

### 2. Create a kind cluster

```bash
kind create cluster --name zarf-test --wait 60s
```

Verify the cluster is up:

```bash
kubectl cluster-info --context kind-zarf-test
```

### 3. Download the Zarf init package

```bash
ZARF_VERSION=$(zarf version)
curl -L "https://github.com/zarf-dev/zarf/releases/download/${ZARF_VERSION}/zarf-init-amd64-${ZARF_VERSION}.tar.zst" \
  -o /tmp/zarf-init-amd64-${ZARF_VERSION}.tar.zst
```

Update `path` in `main.tf` if your version differs from `v0.75.0`.

### 4. Initialise Terraform

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform init
```

`terraform.rc` points Terraform at the locally installed provider binary instead of the public registry.

### 5. Preview the deployment

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform plan
```

You should see one resource to create: `zarf_package.init`.

### 6. Apply

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform apply
```

Terraform runs `zarf package deploy` and waits for it to complete (~60 s on a fresh cluster). On success, the outputs are printed:

```
init_connectivity        = "airgap"
init_deployed_components = ["zarf-injector", "zarf-seed-registry", "zarf-registry", "zarf-agent"]
init_version             = "v0.75.0"
```

### 7. Verify

```bash
zarf package list
```

```
Package | Version  | Connectivity | Components
init    | v0.75.0  | airgap       | [zarf-injector zarf-seed-registry zarf-registry zarf-agent]
```

### 8. Import an existing deployment (optional)

If `zarf init` was already run outside of Terraform, import it instead of redeploying:

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform import zarf_package.init init
TF_CLI_CONFIG_FILE=terraform.rc terraform plan   # should show no changes
```

### 9. Destroy

```bash
TF_CLI_CONFIG_FILE=terraform.rc terraform destroy
```

Terraform runs `zarf package remove init`. To also delete the kind cluster:

```bash
kind delete cluster --name zarf-test
```

## File reference

| File | Purpose |
|---|---|
| `main.tf` | Terraform config — provider block and `zarf_package.init` resource |
| `terraform.rc` | Local provider mirror config — passed via `TF_CLI_CONFIG_FILE` |

## Troubleshooting

**`zarf binary not found`** — ensure `zarf` is on your `PATH` or set `zarf_binary` in the provider block to its absolute path.

**`unable to connect to the cluster`** — check `KUBECONFIG` points to the kind cluster: `kubectl config current-context` should show `kind-zarf-test`. The provider reads `kubeconfig_path` and passes it as the `KUBECONFIG` environment variable to every zarf subprocess.

**Timeout during deploy** — increase `timeout` in `main.tf` (default is `"20m"`). On slow machines or networks, pulling images into kind can take longer.

**`Package Not Found After Deploy`** — set the `name` attribute explicitly (as done here with `name = "init"`). This is required for OCI-sourced packages and recommended for all packages.
