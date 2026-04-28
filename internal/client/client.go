package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds provider-level configuration passed from provider.Configure().
type Config struct {
	KubeconfigPath string
	KubeContext    string
	ZarfBinary     string
	UDSBinary      string
}

// Client is the CLI executor. Resources hold a *Client.
type Client struct {
	cfg Config
}

// DeployedPackage mirrors the JSON struct emitted by
// `zarf package list --output-format json`.
// Field names match the actual CLI output (v0.75.0+).
type DeployedPackage struct {
	Name              string   `json:"package"`
	Version           string   `json:"version"`
	NamespaceOverride string   `json:"namespaceOverride"`
	Connectivity      string   `json:"connectivity"`
	Components        []string `json:"components"`
}

// ZarfDeployOptions configures a `zarf package deploy` invocation.
type ZarfDeployOptions struct {
	Path                    string
	Components              []string
	SetVariables            map[string]string
	Namespace               string
	Timeout                 string
	Retries                 int
	AdoptExistingResources  bool
	SkipSignatureValidation bool
}

// UDSDeployOptions configures a `uds deploy` invocation.
type UDSDeployOptions struct {
	BundlePath string
	Packages   []string
	SetVars    map[string]string
	Retries    int
	Resume     bool
}

// NewClient creates a Client with defaults applied.
func NewClient(cfg Config) *Client {
	if cfg.ZarfBinary == "" {
		cfg.ZarfBinary = "zarf"
	}
	if cfg.UDSBinary == "" {
		cfg.UDSBinary = "uds"
	}
	cfg.KubeconfigPath = expandHome(cfg.KubeconfigPath)
	return &Client{cfg: cfg}
}

// expandHome expands a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "" {
		return p
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[1:])
	}
	return p
}

// ValidateZarfBinary checks that the zarf CLI is reachable. Called from zarf_package Configure().
func (c *Client) ValidateZarfBinary(ctx context.Context) error {
	if _, err := c.run(ctx, c.cfg.ZarfBinary, "version"); err != nil {
		return fmt.Errorf("zarf binary %q not found or not executable: %w", c.cfg.ZarfBinary, err)
	}
	return nil
}

// ValidateUDSBinary checks that the uds CLI is reachable. Called from uds_bundle Configure().
func (c *Client) ValidateUDSBinary(ctx context.Context) error {
	if _, err := c.run(ctx, c.cfg.UDSBinary, "version"); err != nil {
		return fmt.Errorf("uds binary %q not found or not executable: %w", c.cfg.UDSBinary, err)
	}
	return nil
}

// run executes a CLI binary, merging stdout+stderr. Use for commands where we
// only care about success/failure and want the full output in error messages.
func (c *Client) run(ctx context.Context, binary string, args ...string) (string, error) {
	stdout, _, err := c.exec(ctx, binary, args...)
	return stdout, err
}

// runStdout executes a CLI binary and returns only stdout, keeping stderr
// separate (used only in error messages). Use for commands whose stdout
// is parsed as structured data (e.g. JSON).
func (c *Client) runStdout(ctx context.Context, binary string, args ...string) (string, error) {
	stdout, stderr, err := c.exec(ctx, binary, args...)
	if err != nil {
		return "", fmt.Errorf("%s %s: %w\n%s", binary, strings.Join(args, " "), err, stderr)
	}
	return stdout, nil
}

// exec is the core subprocess runner. Returns stdout, stderr, and any error.
func (c *Client) exec(ctx context.Context, binary string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, binary, args...)

	env := os.Environ()
	if c.cfg.KubeconfigPath != "" {
		env = setEnvVar(env, "KUBECONFIG", c.cfg.KubeconfigPath)
	}
	cmd.Env = env

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	runErr := cmd.Run()
	outStr := outBuf.String()
	errStr := errBuf.String()

	if runErr != nil {
		// Include both streams in the returned error for full context.
		combined := strings.TrimSpace(outStr + "\n" + errStr)
		return "", "", fmt.Errorf("%s %s: %w\n%s", binary, strings.Join(args, " "), runErr, combined)
	}
	return outStr, errStr, nil
}

// setEnvVar replaces the value of key in env, or appends it if absent.
func setEnvVar(env []string, key, value string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// ZarfDeployPackage runs `zarf package deploy`.
func (c *Client) ZarfDeployPackage(ctx context.Context, opts ZarfDeployOptions) error {
	args := []string{"package", "deploy", opts.Path, "--confirm"}

	if len(opts.Components) > 0 {
		args = append(args, "--components", strings.Join(opts.Components, ","))
	}
	for k, v := range opts.SetVariables {
		args = append(args, "--set-variables", k+"="+v)
	}
	if opts.Namespace != "" {
		args = append(args, "--namespace", opts.Namespace)
	}
	if opts.Timeout != "" {
		args = append(args, "--timeout", opts.Timeout)
	}
	if opts.Retries > 0 {
		args = append(args, "--retries", strconv.Itoa(opts.Retries))
	}
	if opts.AdoptExistingResources {
		args = append(args, "--adopt-existing-resources")
	}
	if opts.SkipSignatureValidation {
		args = append(args, "--skip-signature-validation")
	}

	_, err := c.run(ctx, c.cfg.ZarfBinary, args...)
	return err
}

// ZarfRemovePackage runs `zarf package remove`.
func (c *Client) ZarfRemovePackage(ctx context.Context, name string, components []string) error {
	args := []string{"package", "remove", name, "--confirm"}
	if len(components) > 0 {
		args = append(args, "--components", strings.Join(components, ","))
	}
	_, err := c.run(ctx, c.cfg.ZarfBinary, args...)
	return err
}

// ZarfListPackages runs `zarf package list --output-format json` and returns all deployed packages.
func (c *Client) ZarfListPackages(ctx context.Context) ([]DeployedPackage, error) {
	// --no-color prevents ANSI escape codes from polluting the JSON stdout stream.
	out, err := c.runStdout(ctx, c.cfg.ZarfBinary, "package", "list", "--output-format", "json", "--no-color")
	if err != nil {
		return nil, err
	}

	// zarf package list outputs an array; handle empty cluster gracefully.
	out = strings.TrimSpace(out)
	if out == "" || out == "null" {
		return nil, nil
	}

	var packages []DeployedPackage
	if err := json.Unmarshal([]byte(out), &packages); err != nil {
		return nil, fmt.Errorf("parsing zarf package list output: %w\nraw: %s", err, out)
	}
	return packages, nil
}

// ZarfFindPackage returns the deployed package with the given name, or nil if not found.
func (c *Client) ZarfFindPackage(ctx context.Context, name string) (*DeployedPackage, error) {
	packages, err := c.ZarfListPackages(ctx)
	if err != nil {
		return nil, err
	}
	for i := range packages {
		if packages[i].Name == name {
			return &packages[i], nil
		}
	}
	return nil, nil
}

// ZarfSnapshotNames returns a set of currently-deployed package names. Used to
// diff before/after a deploy when the package name is unknown in advance.
func (c *Client) ZarfSnapshotNames(ctx context.Context) (map[string]struct{}, error) {
	packages, err := c.ZarfListPackages(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[string]struct{}, len(packages))
	for _, p := range packages {
		names[p.Name] = struct{}{}
	}
	return names, nil
}

// UDSDeployBundle runs `uds deploy`.
func (c *Client) UDSDeployBundle(ctx context.Context, opts UDSDeployOptions) error {
	args := []string{"deploy", opts.BundlePath, "--confirm"}

	if len(opts.Packages) > 0 {
		args = append(args, "--packages", strings.Join(opts.Packages, ","))
	}
	for k, v := range opts.SetVars {
		args = append(args, "--set", k+"="+v)
	}
	if opts.Retries > 0 {
		args = append(args, "--retries", strconv.Itoa(opts.Retries))
	}
	if opts.Resume {
		args = append(args, "--resume")
	}

	_, err := c.run(ctx, c.cfg.UDSBinary, args...)
	return err
}

// UDSRemoveBundle runs `uds remove`.
func (c *Client) UDSRemoveBundle(ctx context.Context, bundlePath string, packages []string) error {
	args := []string{"remove", bundlePath, "--confirm"}
	if len(packages) > 0 {
		args = append(args, "--packages", strings.Join(packages, ","))
	}
	_, err := c.run(ctx, c.cfg.UDSBinary, args...)
	return err
}
