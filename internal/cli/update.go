package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SomeoneWithOptions/n8n-cli/internal/selfupdate"
)

type updateFlags struct {
	version, output         string
	check, force, assumeYes bool
}

func newUpdateCommand(opts Options) *cobra.Command {
	var f updateFlags
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update this CLI to the latest release",
		Long: "Replace this binary with a release published on GitHub.\n\n" +
			"Use --check first: it reports the installed and the latest version and writes\n" +
			"nothing. A plain 'n8n update' asks before replacing the binary; --yes skips the\n" +
			"question for scripts. Nothing else is touched: contexts and credentials belong\n" +
			"to 'n8n auth login' and survive every update.\n\n" +
			"The asset for this platform is downloaded next to the current binary, checked\n" +
			"against the release checksums.txt, and run once to confirm it reports the\n" +
			"expected version before it is moved into place. A failure at any of those steps\n" +
			"leaves the installed binary untouched. Those checksums prove the download\n" +
			"arrived intact, not who built it; releases are not signed yet.\n\n" +
			"Refused unless --force is passed: a development build, because its version\n" +
			"cannot be compared with a release, and a binary that a package manager owns\n" +
			"(Homebrew, Nix, Snap, 'go install', a system package), because that manager\n" +
			"should do the update instead. A binary installed by install.sh, install.ps1 or\n" +
			"a manual download updates in place. --version installs an exact tag, including\n" +
			"an older one.",
		Example: "  n8n update --check\n" +
			"  n8n update --check --output json\n" +
			"  n8n update\n" +
			"  n8n update --yes\n" +
			"  n8n update --version v1.4.0 --yes",
		Annotations: map[string]string{"cliOnly": "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), opts, f)
		},
	}
	cmd.Flags().BoolVar(&f.check, "check", false, "report the installed and latest versions without writing anything")
	cmd.Flags().StringVar(&f.version, "version", "", "install this exact release tag (for example v1.4.0) instead of the latest")
	cmd.Flags().BoolVar(&f.force, "force", false, "update a development build or a package-manager-owned binary, and reinstall the current version")
	cmd.Flags().BoolVarP(&f.assumeYes, "yes", "y", false, "do not ask before replacing the binary")
	cmd.Flags().StringVar(&f.output, "output", outputText, "output format: text or json")
	cmd.MarkFlagsMutuallyExclusive("check", "yes")
	return cmd
}

// updateReport is the machine-readable result of one run of `n8n update`.
type updateReport struct {
	SchemaVersion int `json:"schemaVersion"`
	// CurrentVersion is what this binary reports, release tag or not.
	CurrentVersion string `json:"currentVersion"`
	// CurrentIsRelease is false for a development build, whose version cannot
	// be compared with a release tag.
	CurrentIsRelease bool `json:"currentIsRelease"`
	// LatestVersion is the latest published tag, empty when --version pinned a
	// tag and no lookup was made.
	LatestVersion string `json:"latestVersion"`
	// TargetVersion is the tag this run would install or installed.
	TargetVersion string `json:"targetVersion"`
	Asset         string `json:"asset"`
	// Path is the binary that would be replaced or was replaced.
	Path string `json:"path"`
	// Manager names the package manager owning Path, empty when none does.
	Manager         string `json:"manager"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Installed       bool   `json:"installed"`
}

func runUpdate(ctx context.Context, opts Options, f updateFlags) error {
	if err := validateOutput(f.output); err != nil {
		return err
	}
	updater := opts.updater()
	asset, err := updater.Asset()
	if err != nil {
		return err
	}
	dest, err := updater.Destination()
	if err != nil {
		return err
	}

	current, currentErr := selfupdate.ParseLocalVersion(opts.Version.Version)
	target, latest, err := updateTarget(ctx, updater, f.version)
	if err != nil {
		return err
	}

	report := updateReport{
		SchemaVersion:    1,
		CurrentVersion:   opts.Version.Version,
		CurrentIsRelease: currentErr == nil,
		LatestVersion:    latest,
		TargetVersion:    target.String(),
		Asset:            asset,
		Path:             dest.Path,
		Manager:          dest.Manager,
		UpdateAvailable:  currentErr != nil || current.Compare(target) < 0,
	}
	if f.check {
		return writeUpdateReport(opts.Streams.Out, f.output, report)
	}

	// Nothing to do is not a failure: a scripted `n8n update` on an
	// up-to-date machine must not fail the script.
	if currentErr == nil && !f.force && f.version == "" {
		switch current.Compare(target) {
		case 0:
			return writeUpdateReport(opts.Streams.Out, f.output, report)
		case 1:
			fmt.Fprintf(opts.Streams.Err, "n8n %s is newer than the latest release %s; nothing to install.\n", current, target)
			return writeUpdateReport(opts.Streams.Out, f.output, report)
		}
	}
	if err := updateAllowed(opts, f, dest, current, currentErr, target); err != nil {
		return err
	}
	if err := dest.Writable(); err != nil {
		return fmt.Errorf("%w: reinstall into a writable directory with install.sh, or rerun with the privileges that own %s", err, dest.Path)
	}
	if err := opts.confirmer(f.assumeYes).Confirm(fmt.Sprintf("Replace %s (%s) with %s?", dest.Path, report.CurrentVersion, target)); err != nil {
		return err
	}

	fmt.Fprintf(opts.Streams.Err, "Downloading %s %s...\n", asset, target)
	staged, err := updater.Fetch(ctx, target.String(), dest)
	if err != nil {
		return err
	}
	if err := updater.VerifyBinary(ctx, staged, target.String()); err != nil {
		_ = os.Remove(staged)
		return err
	}
	if err := updater.Install(staged, dest); err != nil {
		_ = os.Remove(staged)
		return err
	}
	report.Installed = true
	report.UpdateAvailable = false
	return writeUpdateReport(opts.Streams.Out, f.output, report)
}

// updateTarget resolves the release to install: the pinned tag, or the latest
// published one. The second result is the latest tag, empty when pinning made
// the lookup unnecessary.
func updateTarget(ctx context.Context, updater *selfupdate.Updater, pinned string) (selfupdate.Release, string, error) {
	if strings.TrimSpace(pinned) != "" {
		target, err := selfupdate.ParseTag(pinned)
		if err != nil {
			return selfupdate.Release{}, "", fmt.Errorf("--version %q: %w: pass a published tag such as v1.4.0", pinned, err)
		}
		return target, "", nil
	}
	tag, err := updater.LatestTag(ctx)
	if err != nil {
		return selfupdate.Release{}, "", err
	}
	target, err := selfupdate.ParseTag(tag)
	if err != nil {
		return selfupdate.Release{}, "", fmt.Errorf("the latest release is tagged %q: %w: install a tag explicitly with --version", tag, err)
	}
	return target, target.String(), nil
}

// updateAllowed refuses the cases where overwriting the binary is more likely
// to break an install than to fix it. Every one of them is a --force away.
func updateAllowed(opts Options, f updateFlags, dest selfupdate.Destination, current selfupdate.Release, currentErr error, target selfupdate.Release) error {
	if f.force {
		return nil
	}
	if currentErr != nil {
		return fmt.Errorf("this binary reports version %q, which is not a published release (%w): reinstall with install.sh, or pass --force to overwrite it with %s", opts.Version.Version, currentErr, target)
	}
	if dest.Manager != "" {
		return fmt.Errorf("%s was installed by %s: update it there, or pass --force to overwrite the file in place", dest.Path, dest.Manager)
	}
	if f.version != "" && current.Compare(target) > 0 {
		fmt.Fprintf(opts.Streams.Err, "Downgrading n8n %s to %s.\n", current, target)
	}
	if f.version != "" && current.Compare(target) == 0 {
		return fmt.Errorf("n8n %s is already installed: pass --force to reinstall it", current)
	}
	return nil
}

func writeUpdateReport(out io.Writer, format string, report updateReport) error {
	if format == outputJSON {
		return writeJSON(out, report)
	}
	var text strings.Builder
	switch {
	case report.Installed:
		fmt.Fprintf(&text, "Updated n8n %s to %s.\n", report.CurrentVersion, report.TargetVersion)
		fmt.Fprintf(&text, "Installed at %s. Run 'n8n version' to confirm.\n", report.Path)
	case !report.CurrentIsRelease:
		fmt.Fprintf(&text, "n8n %s is not a published release; the latest release is %s.\n", report.CurrentVersion, report.TargetVersion)
		fmt.Fprintln(&text, "Run 'n8n update --force' to replace this build with it.")
	case report.UpdateAvailable:
		fmt.Fprintf(&text, "n8n %s is out of date; %s is available.\n", report.CurrentVersion, report.TargetVersion)
		fmt.Fprintln(&text, "Run 'n8n update' to install it.")
	default:
		fmt.Fprintf(&text, "n8n %s is up to date.\n", report.CurrentVersion)
	}
	fmt.Fprintf(&text, "Binary: %s\n", report.Path)
	if report.Manager != "" {
		fmt.Fprintf(&text, "Installed by %s: update it there rather than in place.\n", report.Manager)
	}
	_, err := io.WriteString(out, text.String())
	return err
}
