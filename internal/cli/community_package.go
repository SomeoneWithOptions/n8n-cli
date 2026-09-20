package cli

import (
	"context"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/config"
	"github.com/SomeoneWithOptions/n8n-cli/internal/n8n"
)

const communityPackageResource = "communitypackage"

func newCommunityPackageCommand(opts Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "community-package",
		Short: "Manage community node packages",
		Long: "Manage npm community node packages installed on an n8n instance.\n\n" +
			"Start with 'n8n community-package list'. Install, update and uninstall change\n" +
			"code loaded by the instance and therefore require confirmation, or --yes in a\n" +
			"non-interactive run. Workflows are not deleted, but changing or removing a\n" +
			"package can break workflows that use its nodes.\n\n" +
			"This API group accepts API-key authentication only. Bearer tokens and browser\n" +
			"cookies cannot be used. Typical workflow: login with --type api-key, list\n" +
			"installed packages, then install, update or uninstall one package.",
		Example: "  n8n community-package list\n" +
			"  n8n community-package install n8n-nodes-example\n" +
			"  n8n community-package update n8n-nodes-example --yes\n" +
			"  n8n community-package uninstall n8n-nodes-example --yes",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(
		newCommunityPackageListCommand(opts),
		newCommunityPackageInstallCommand(opts),
		newCommunityPackageUpdateCommand(opts),
		newCommunityPackageUninstallCommand(opts),
	)
	return cmd
}

type communityPackageListFlags struct {
	instance instanceFlags
	output   string
}

func newCommunityPackageListCommand(opts Options) *cobra.Command {
	var f communityPackageListFlags
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed community packages",
		Long: "List community node packages installed on the selected n8n instance.\n\n" +
			"Text output shows installed and available versions, load status, and node\n" +
			"count. --output json emits the API array with package authors, nodes and\n" +
			"timestamps for scripts. This command changes nothing and needs no confirmation.\n\n" +
			"Requires API-key authentication and the communityPackage:list scope; check\n" +
			"availability with 'n8n discover --resource communitypackage'.",
		Example: "  n8n community-package list\n" +
			"  n8n community-package list --output json\n" +
			"  n8n community-package list --context production",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCommunityPackageList(cmd.Context(), opts, f)
		},
	}
	f.instance.register(cmd)
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text; json is stable for scripting)")
	return cmd
}

func runCommunityPackageList(ctx context.Context, opts Options, f communityPackageListFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := requireAPIKey(resolution, "community-package"); err != nil {
		return err
	}
	packages, err := client.ListCommunityPackages(ctx)
	if err != nil {
		return communityPackageAPIError(err, resolution, "list")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, packages)
	}
	return writeCommunityPackageListText(opts, resolution, packages)
}

func writeCommunityPackageListText(opts Options, resolution config.Resolution, packages []n8n.CommunityPackage) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Packages:\t%d\n", len(packages))
	if len(packages) > 0 {
		fmt.Fprintln(tw, "\nNAME\tINSTALLED\tUPDATE\tSTATUS\tNODES")
		for _, pkg := range packages {
			update := pkg.UpdateAvailable
			if update == "" {
				update = "-"
			}
			status := "loaded"
			if pkg.FailedLoading {
				status = "failed"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n", pkg.PackageName, pkg.InstalledVersion, update, status, len(pkg.InstalledNodes))
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if len(packages) == 0 {
		fmt.Fprintln(opts.Streams.Err, "No community packages are installed on this instance.")
	}
	return nil
}

type communityPackageMutationFlags struct {
	instance instanceFlags
	version  string
	verify   bool
	yes      bool
	output   string
}

func newCommunityPackageInstallCommand(opts Options) *cobra.Command {
	var f communityPackageMutationFlags
	cmd := &cobra.Command{
		Use:   "install <name>",
		Short: "Install a community package",
		Long: "Install an npm community node package on the selected n8n instance.\n\n" +
			"The package name must start with n8n-nodes-. Omit --version to let npm select\n" +
			"the version. Verification against n8n's vetted package list is enabled by\n" +
			"default; --verify=false explicitly permits unverified code when the instance\n" +
			"allows it. Installed package code runs inside n8n.\n\n" +
			"Interactive runs ask before installing. Non-interactive runs fail unless --yes\n" +
			"is passed. Requires API-key authentication and communityPackage:install.\n" +
			"Next step: 'n8n community-package list'.",
		Example: "  n8n community-package install n8n-nodes-example\n" +
			"  n8n community-package install n8n-nodes-example --version 1.2.3 --yes\n" +
			"  n8n community-package install n8n-nodes-example --verify=false --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommunityPackageInstall(cmd.Context(), opts, args[0], f)
		},
	}
	registerCommunityPackageMutationFlags(cmd, &f, true)
	return cmd
}

func runCommunityPackageInstall(ctx context.Context, opts Options, name string, f communityPackageMutationFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	request := n8n.InstallCommunityPackageRequest{Name: name, Version: f.version, Verify: n8n.Bool(f.verify)}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := requireAPIKey(resolution, "community-package"); err != nil {
		return err
	}
	target := "the version selected by npm"
	if f.version != "" {
		target = "version " + f.version
	}
	question := fmt.Sprintf("Install community package %q (%s) on %s? Its code will run inside n8n.", name, target, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	installed, err := client.InstallCommunityPackage(ctx, request)
	if err != nil {
		return communityPackageAPIError(err, resolution, "install")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, installed)
	}
	return writeCommunityPackageText(opts, resolution, "Installed", installed)
}

func newCommunityPackageUpdateCommand(opts Options) *cobra.Command {
	var f communityPackageMutationFlags
	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an installed community package",
		Long: "Update one installed community node package on the selected n8n instance.\n\n" +
			"Omit --version to update to the version selected by n8n. Verification against\n" +
			"n8n's vetted package list is enabled by default; --verify=false explicitly\n" +
			"permits unverified code when the instance allows it. Updating code can change\n" +
			"node behavior in existing workflows, though it does not delete those workflows.\n\n" +
			"Interactive runs ask before updating. Non-interactive runs fail unless --yes is\n" +
			"passed. Requires API-key authentication and communityPackage:update.\n" +
			"Next step: run 'n8n community-package list' and test affected workflows.",
		Example: "  n8n community-package update n8n-nodes-example\n" +
			"  n8n community-package update n8n-nodes-example --version 2.0.0 --yes\n" +
			"  n8n community-package update '@scope/n8n-nodes-example' --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommunityPackageUpdate(cmd.Context(), opts, args[0], f)
		},
	}
	registerCommunityPackageMutationFlags(cmd, &f, true)
	return cmd
}

func runCommunityPackageUpdate(ctx context.Context, opts Options, name string, f communityPackageMutationFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	request := n8n.UpdateCommunityPackageRequest{Version: f.version, Verify: n8n.Bool(f.verify)}
	if err := validateCommunityPackageArgument(name); err != nil {
		return err
	}
	if err := request.Validate(); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := requireAPIKey(resolution, "community-package"); err != nil {
		return err
	}
	target := "the version selected by n8n"
	if f.version != "" {
		target = "version " + f.version
	}
	question := fmt.Sprintf("Update community package %q to %s on %s? Existing node behavior may change.", name, target, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	updated, err := client.UpdateCommunityPackage(ctx, name, request)
	if err != nil {
		return communityPackageAPIError(err, resolution, "update")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, updated)
	}
	return writeCommunityPackageText(opts, resolution, "Updated", updated)
}

func newCommunityPackageUninstallCommand(opts Options) *cobra.Command {
	var f communityPackageMutationFlags
	cmd := &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Uninstall a community package",
		Long: "Uninstall one community node package from the selected n8n instance.\n\n" +
			"This removes the package's code and nodes from the instance. It does not delete\n" +
			"workflows, credentials or execution data, but workflows that reference removed\n" +
			"nodes may stop loading or running. The API returns no package document.\n\n" +
			"Interactive runs ask before uninstalling. Non-interactive runs fail unless --yes\n" +
			"is passed. Requires API-key authentication and communityPackage:uninstall.\n" +
			"Next step: run 'n8n community-package list' and inspect affected workflows.",
		Example: "  n8n community-package uninstall n8n-nodes-example\n" +
			"  n8n community-package uninstall n8n-nodes-example --yes\n" +
			"  n8n community-package uninstall '@scope/n8n-nodes-example' --yes --output json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommunityPackageUninstall(cmd.Context(), opts, args[0], f)
		},
	}
	registerCommunityPackageMutationFlags(cmd, &f, false)
	return cmd
}

func runCommunityPackageUninstall(ctx context.Context, opts Options, name string, f communityPackageMutationFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	if err := validateCommunityPackageArgument(name); err != nil {
		return err
	}
	client, resolution, err := opts.apiClient(f.instance)
	if err != nil {
		return err
	}
	if err := requireAPIKey(resolution, "community-package"); err != nil {
		return err
	}
	question := fmt.Sprintf("Uninstall community package %q from %s? Workflows using its nodes may stop working.", name, resolution.URL)
	if err := opts.confirmer(f.yes).Confirm(question); err != nil {
		return err
	}
	if err := client.UninstallCommunityPackage(ctx, name); err != nil {
		return communityPackageAPIError(err, resolution, "uninstall")
	}
	if f.output == outputJSON {
		return writeJSON(opts.Streams.Out, struct {
			Name        string `json:"name"`
			Uninstalled bool   `json:"uninstalled"`
		}{Name: name, Uninstalled: true})
	}
	fmt.Fprintf(opts.Streams.Out, "Uninstalled %s from %s.\n", name, resolution.URL)
	return nil
}

func registerCommunityPackageMutationFlags(cmd *cobra.Command, f *communityPackageMutationFlags, withPackageOptions bool) {
	f.instance.register(cmd)
	if withPackageOptions {
		cmd.Flags().StringVar(&f.version, "version", "", "npm package version, e.g. 1.2.3 (default: let n8n select the version)")
		cmd.Flags().BoolVar(&f.verify, "verify", true, "verify against n8n's vetted package list (default true; --verify=false permits unverified code when allowed)")
	}
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm the instance-changing action without prompting (required when stdin is not interactive)")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json (default text; json is stable for scripting)")
}

func writeCommunityPackageText(opts Options, resolution config.Resolution, action string, pkg *n8n.CommunityPackage) error {
	tw := tabwriter.NewWriter(opts.Streams.Out, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "%s:\t%s\n", action, pkg.PackageName)
	fmt.Fprintf(tw, "Version:\t%s\n", pkg.InstalledVersion)
	fmt.Fprintf(tw, "Instance:\t%s\n", resolution.URL)
	fmt.Fprintf(tw, "Nodes:\t%d\n", len(pkg.InstalledNodes))
	if pkg.UpdateAvailable != "" {
		fmt.Fprintf(tw, "Update available:\t%s\n", pkg.UpdateAvailable)
	}
	if pkg.FailedLoading {
		fmt.Fprintln(tw, "Status:\tfailed to load")
	}
	return tw.Flush()
}

func validateCommunityPackageArgument(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("community package name is required")
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("community package name must not start or end with whitespace")
	}
	return nil
}

// requireAPIKey rejects API-key-only resource groups before prompting or
// sending a request. Environment credentials need different remediation from a
// saved context because they override it for the current process.
func requireAPIKey(resolution config.Resolution, command string) error {
	if resolution.AuthType == n8n.AuthAPIKey {
		return nil
	}
	if resolution.Source == config.SourceEnv {
		return fmt.Errorf("%s commands require API-key authentication, but the environment selected %s authentication: unset %s or %s and set %s", command, resolution.AuthType, config.EnvBearerToken, config.EnvAuthCookie, config.EnvAPIKey)
	}
	hint := "'n8n auth login --type api-key'"
	if resolution.ContextName != "" && resolution.ContextName != DefaultContextName {
		hint = fmt.Sprintf("'n8n auth login --context %s --type api-key'", resolution.ContextName)
	}
	return fmt.Errorf("%s commands require API-key authentication, but context %q uses %s: run %s", command, resolution.ContextName, resolution.AuthType, hint)
}

// communityPackageAPIError names the scope the denied action needs. Community
// nodes are an instance-level feature, so a 403 can also mean the instance has
// them switched off.
func communityPackageAPIError(err error, resolution config.Resolution, action string) error {
	if n8n.IsForbidden(err) {
		need := communityPackageScope(action)
		return forbiddenScopeError(err, resolution, "community-package "+action, need,
			"a 403 can also mean the instance does not allow community packages.", communityPackageResource)
	}
	return apiError(err, resolution, communityPackageResource)
}

// communityPackageScope maps a command action to the scope the API requires.
func communityPackageScope(action string) scopeNeed {
	switch action {
	case "list":
		return allOf("communityPackage:list")
	case "install":
		return allOf("communityPackage:install")
	case "update":
		return allOf("communityPackage:update")
	case "uninstall":
		return allOf("communityPackage:uninstall")
	}
	return scopeNeed{}
}
