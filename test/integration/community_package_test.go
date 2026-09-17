package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/cli"
	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
	"github.com/SomeoneWithOptions/n8n-cli/test/integration"
)

const (
	envCommunityPackage        = "N8N_INTEGRATION_COMMUNITY_PACKAGE"
	envCommunityPackageInstall = "N8N_INTEGRATION_COMMUNITY_PACKAGE_INSTALL_VERSION"
	envCommunityPackageUpdate  = "N8N_INTEGRATION_COMMUNITY_PACKAGE_UPDATE_VERSION"
)

// TestListCommunityPackages is read-only and runs whenever the base integration
// credential is available. An empty instance is a valid result.
func TestListCommunityPackages(t *testing.T) {
	instance := integration.Require(t)
	packages, err := instance.Client(t).ListCommunityPackages(context.Background())
	if err != nil {
		t.Fatalf("ListCommunityPackages: %v", err)
	}
	for _, pkg := range packages {
		if pkg.PackageName == "" || pkg.InstalledVersion == "" {
			t.Errorf("package is missing its name or installed version: %+v", pkg)
		}
	}
	t.Logf("instance has %d community package(s)", len(packages))
}

// TestCommunityPackageLifecycle is opt-in twice: destructive integration must
// be enabled, and the caller must name a package plus two explicit versions.
// It refuses to touch a package already installed on the instance, then cleans
// up the package it created even when update assertions fail.
func TestCommunityPackageLifecycle(t *testing.T) {
	instance := integration.RequireDestructive(t)
	name := os.Getenv(envCommunityPackage)
	installVersion := os.Getenv(envCommunityPackageInstall)
	updateVersion := os.Getenv(envCommunityPackageUpdate)
	if name == "" || installVersion == "" || updateVersion == "" {
		t.Skipf("set %s, %s and %s to run community-package mutations", envCommunityPackage, envCommunityPackageInstall, envCommunityPackageUpdate)
	}
	if installVersion == updateVersion {
		t.Fatalf("%s and %s must differ so update is exercised", envCommunityPackageInstall, envCommunityPackageUpdate)
	}

	client := instance.Client(t)
	packages, err := client.ListCommunityPackages(context.Background())
	if err != nil {
		t.Fatalf("preflight ListCommunityPackages: %v", err)
	}
	for _, pkg := range packages {
		if pkg.PackageName == name {
			t.Fatalf("refusing to touch pre-existing package %q", name)
		}
	}

	installed := false
	t.Cleanup(func() {
		if installed {
			if err := client.UninstallCommunityPackage(context.Background(), name); err != nil {
				t.Errorf("cleanup UninstallCommunityPackage(%q): %v", name, err)
			}
		}
	})

	pkg, err := client.InstallCommunityPackage(context.Background(), n8n.InstallCommunityPackageRequest{
		Name: name, Version: installVersion, Verify: n8n.Bool(true),
	})
	if err != nil {
		t.Fatalf("InstallCommunityPackage: %v", err)
	}
	installed = true
	if pkg.PackageName != name || pkg.InstalledVersion != installVersion {
		t.Errorf("installed package = %+v, want %s@%s", pkg, name, installVersion)
	}

	pkg, err = client.UpdateCommunityPackage(context.Background(), name, n8n.UpdateCommunityPackageRequest{
		Version: updateVersion, Verify: n8n.Bool(true),
	})
	if err != nil {
		t.Fatalf("UpdateCommunityPackage: %v", err)
	}
	if pkg.PackageName != name || pkg.InstalledVersion != updateVersion {
		t.Errorf("updated package = %+v, want %s@%s", pkg, name, updateVersion)
	}

	if err := client.UninstallCommunityPackage(context.Background(), name); err != nil {
		t.Fatalf("UninstallCommunityPackage: %v", err)
	}
	installed = false
}

// TestCommunityPackageListCommand runs the CLI with an environment API key and
// a throwaway config directory. It is read-only and verifies JSON output.
func TestCommunityPackageListCommand(t *testing.T) {
	instance := integration.Require(t)
	env := map[string]string{config.EnvURL: instance.URL, config.EnvAPIKey: instance.APIKey.Reveal()}
	interactive := false
	var out, errOut strings.Builder
	code := cli.Run(context.Background(), []string{"community-package", "list", "--output", "json"}, cli.Options{
		Streams:     cli.Streams{In: strings.NewReader(""), Out: &out, Err: &errOut},
		Version:     version.Get(),
		ConfigDir:   t.TempDir(),
		Env:         func(name string) string { return env[name] },
		Keyring:     config.NewMemoryStore(config.StorageKeyring),
		Interactive: &interactive,
	})
	if code != cli.ExitSuccess {
		t.Fatalf("community-package list exit code = %d, want %d (stderr: %s)", code, cli.ExitSuccess, errOut.String())
	}
	var packages []n8n.CommunityPackage
	if err := json.Unmarshal([]byte(out.String()), &packages); err != nil {
		t.Fatalf("stdout is not package JSON: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), instance.APIKey.Reveal()) {
		t.Error("credential leaked into stdout")
	}
}
