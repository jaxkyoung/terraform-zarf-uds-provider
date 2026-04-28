# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build the provider binary
make build

# Build and install into ~/.terraform.d/plugins/ for local use
make install

# Run all tests
make test

# Run a single test
go test ./internal/... -run TestFunctionName -v

# Lint
make lint      # requires golangci-lint

# Format
make fmt

# Tidy dependencies
make tidy
```

To use a locally installed build against an example, pass `TF_CLI_CONFIG_FILE`:
```bash
TF_CLI_CONFIG_FILE=examples/zarf-init-kind/terraform.rc terraform init
TF_CLI_CONFIG_FILE=examples/zarf-init-kind/terraform.rc terraform apply
```

## Versioning and releases

Versions are managed with [commitizen](https://commitizen-tools.github.io/commitizen/) (`pyproject.toml`). Commits must follow Conventional Commits. To cut a release:

```bash
cz bump          # bumps version in pyproject.toml + GNUmakefile, creates an annotated tag
git push --follow-tags
```

The `release` GitHub Actions workflow triggers on `v*` tags, runs GoReleaser, GPG-signs the checksums, and publishes to GitHub Releases. GoReleaser config is in `.goreleaser.yml`.

## Architecture

This is a [Terraform Plugin Framework](https://developer.hashicorp.com/terraform/plugin/framework) provider. There are no data sources — only resources. The provider is a thin orchestration layer over the `zarf` and `uds` CLIs; it never talks to Kubernetes directly.

### Layer overview

```
main.go                    providerserver entry point; injects version string
internal/provider/         provider schema + Configure(); builds the shared *client.Client
internal/client/client.go  CLI executor — all subprocess calls live here
internal/resources/        one file per resource type
```

### `internal/client/client.go`

The single `Client` struct holds provider config and exposes typed methods that map 1:1 to CLI commands:

- `ZarfDeployPackage` → `zarf package deploy --confirm`
- `ZarfRemovePackage` → `zarf package remove --confirm`
- `ZarfListPackages` / `ZarfFindPackage` / `ZarfSnapshotNames` → `zarf package list --output-format json`
- `UDSDeployBundle` → `uds deploy --confirm` (writes `ConfigYAML` to a temp file if set)
- `UDSRemoveBundle` → `uds remove --confirm`

`KUBECONFIG` is injected into every subprocess env; `kube_context` is documented as a manual step (set `current-context` before apply) because `zarf` and `uds` do not accept a `--context` flag.

### `internal/resources/zarf_package.go`

`zarf_package` resource. The main complexity is in `Create`: because `zarf package deploy` does not return the package name, the resource snapshots deployed names before the deploy and diffs afterwards to find the new package. If the user sets `name`, this diff is skipped. The `id` attribute equals the package name as shown by `zarf package list`.

All mutable attributes use `UseStateForUnknown`; `path` uses `RequiresReplace`.

### `internal/resources/uds_bundle.go`

`uds_bundle` resource. The resource type name is `uds_bundle` (not `zarf_bundle`) intentionally — it matches UDS tooling convention. `bundle_path` and `name` both use `RequiresReplace`.

Drift detection in `Read` is limited: UDS has no bundle list command, so the provider checks `zarf package list` for the names in `packages`. If none are found, the resource is removed from state. If `packages` is not set, drift cannot be detected and state is kept as-is.

`config_yaml` is the mechanism for Helm chart value overrides; it is written to a temp file and passed as `--config` to `uds deploy`. Changes do not force replace — they trigger an in-place update (redeploy).

## Key design constraints

- No Kubernetes API calls — all cluster interaction goes through `zarf` / `uds` CLI subprocesses.
- `kube_context` does not map to a `--context` flag. Users must set the desired context as `current-context` in their kubeconfig before running Terraform.
- OCI-sourced `zarf_package` resources require `name` to be set explicitly; the package's internal metadata name cannot be derived from the OCI reference alone.
- The `uds_bundle` type is registered without the provider prefix (`uds_bundle`, not `zarf_uds_bundle`) — this is intentional and must not be changed.
