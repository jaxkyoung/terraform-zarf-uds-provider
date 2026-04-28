terraform {
  required_providers {
    zarf = {
      source  = "jaxkyoung/zarf"
      version = "~> 0.1"
    }
  }
}

provider "zarf" {
  # Path to kubeconfig. Defaults to ~/.kube/config or $KUBECONFIG env var.
  kubeconfig_path = "~/.kube/config"

  # Optional: absolute paths to CLI binaries if not on PATH.
  # zarf_binary = "/usr/local/bin/zarf"
  # uds_binary  = "/usr/local/bin/uds"
}
