package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SomeoneWithOptions/n8n-cli/internal/selfupdate"
	"github.com/SomeoneWithOptions/n8n-cli/internal/version"
)

// updateFixture is one machine with a binary on disk and a GitHub stand-in
// serving one release. Each run() is a separate CLI process against that state,
// so a test can assert what the installed file looks like afterwards.
type updateFixture struct {
	t *testing.T
	// exe is the file `n8n update` is pointed at.
	exe string
	// version is what the running binary reports about itself.
	version version.Info
	// tag is the release the lookup endpoint reports as latest.
	tag string
	// asset is the published binary. Empty publishes a script reporting tag,
	// which is what a real release asset does when run with `version`.
	asset string
	// manifest overrides the generated checksums.txt.
	manifest string
	// latestStatus overrides the release lookup status.
	latestStatus int
	interactive  bool
	stdin        string

	requests []string
	server   *httptest.Server
}

const installedContent = "the binary that was already installed"

func newUpdateFixture(t *testing.T) *updateFixture {
	t.Helper()
	f := &updateFixture{
		t:            t,
		exe:          filepath.Join(t.TempDir(), "n8n"),
		version:      testVersion,
		tag:          "v1.3.0",
		latestStatus: http.StatusOK,
		interactive:  true,
	}
	if err := os.WriteFile(f.exe, []byte(installedContent), 0o755); err != nil {
		t.Fatalf("write %s: %v", f.exe, err)
	}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests = append(f.requests, r.URL.Path)
		switch {
		case r.URL.Path == "/repos/"+selfupdate.Repo+"/releases/latest":
			if f.latestStatus != http.StatusOK {
				w.WriteHeader(f.latestStatus)
				_, _ = w.Write([]byte(`{"message":"no"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"tag_name":%q}`, f.tag)
		case strings.HasSuffix(r.URL.Path, "/"+selfupdate.ChecksumsAsset):
			_, _ = w.Write([]byte(f.checksums()))
		case strings.Contains(r.URL.Path, "/releases/download/"):
			_, _ = w.Write([]byte(f.assetBody()))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

// assetBody is the published binary: by default a program that reports the
// released version, so the post-download check passes.
func (f *updateFixture) assetBody() string {
	if f.asset != "" {
		return f.asset
	}
	return "#!/bin/sh\necho \"n8n " + f.tag + "\"\n"
}

func (f *updateFixture) checksums() string {
	if f.manifest != "" {
		return f.manifest
	}
	sum := sha256.Sum256([]byte(f.assetBody()))
	return fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), releaseAsset(f.t))
}

func (f *updateFixture) run(args ...string) result {
	f.t.Helper()
	var out, errOut strings.Builder
	interactive := f.interactive
	code := Run(context.Background(), args, Options{
		Streams:            Streams{In: strings.NewReader(f.stdin), Out: &out, Err: &errOut},
		Version:            f.version,
		ConfigDir:          filepath.Join(f.t.TempDir(), "config"),
		Env:                func(string) string { return "" },
		Interactive:        &interactive,
		ExecutablePath:     f.exe,
		ReleaseAPIURL:      f.server.URL,
		ReleaseDownloadURL: f.server.URL,
	})
	return result{code: code, stdout: out.String(), stderr: errOut.String()}
}

// installed is the current content of the binary under update.
func (f *updateFixture) installed() string {
	f.t.Helper()
	content, err := os.ReadFile(f.exe)
	if err != nil {
		f.t.Fatalf("read %s: %v", f.exe, err)
	}
	return string(content)
}

// unchanged fails the test when the installed binary was touched.
func (f *updateFixture) unchanged() {
	f.t.Helper()
	if got := f.installed(); got != installedContent {
		f.t.Errorf("installed binary changed to %q", got)
	}
	left, err := filepath.Glob(filepath.Join(filepath.Dir(f.exe), ".n8n-update-*"))
	if err != nil {
		f.t.Fatal(err)
	}
	if len(left) != 0 {
		f.t.Errorf("a failed update left %v behind", left)
	}
}

func (f *updateFixture) report(got result) updateReport {
	f.t.Helper()
	var report updateReport
	if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
		f.t.Fatalf("decode report: %v\n--- stdout ---\n%s", err, got.stdout)
	}
	return report
}

// releaseAsset is the asset name for the platform running the suite. A platform
// with no published asset cannot be tested here, only refused, which
// TestUpdateRefusesUnreleasedPlatform covers in the selfupdate package.
func releaseAsset(t *testing.T) string {
	t.Helper()
	asset, err := selfupdate.New(selfupdate.Config{}).Asset()
	if err != nil {
		t.Skipf("no release asset for this platform: %v", err)
	}
	return asset
}

// installable skips the tests that have to run the downloaded file: the stand-in
// asset is a shell script, which Windows cannot execute.
func installable(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in release asset is a shell script")
	}
}

func TestUpdateCheckReportsAnAvailableRelease(t *testing.T) {
	f := newUpdateFixture(t)

	got := f.run("update", "--check")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, want := range []string{"1.2.3", "v1.3.0", "out of date", "n8n update"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\n--- stdout ---\n%s", want, got.stdout)
		}
	}
	f.unchanged()
	for _, path := range f.requests {
		if strings.Contains(path, "/releases/download/") {
			t.Errorf("--check downloaded %s", path)
		}
	}
}

func TestUpdateCheckJSONContract(t *testing.T) {
	f := newUpdateFixture(t)

	got := f.run("update", "--check", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	report := f.report(got)
	want := updateReport{
		SchemaVersion: 1, CurrentVersion: "1.2.3", CurrentIsRelease: true,
		LatestVersion: "v1.3.0", TargetVersion: "v1.3.0", Asset: releaseAsset(t),
		Path: report.Path, UpdateAvailable: true,
	}
	if report != want {
		t.Errorf("report = %+v, want %+v", report, want)
	}
	if resolved, err := filepath.EvalSymlinks(f.exe); err != nil || report.Path != resolved {
		t.Errorf("path = %q, want %q (%v)", report.Path, resolved, err)
	}
	f.unchanged()
}

func TestUpdateCheckWhenUpToDate(t *testing.T) {
	f := newUpdateFixture(t)
	f.tag = "v1.2.3"

	got := f.run("update", "--check", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if report := f.report(got); report.UpdateAvailable || report.Installed {
		t.Errorf("report = %+v, want neither an update nor an install", report)
	}
	if text := f.run("update", "--check"); !strings.Contains(text.stdout, "up to date") {
		t.Errorf("stdout = %q, want it to report being up to date", text.stdout)
	}
	f.unchanged()
}

func TestUpdateCheckOnADevelopmentBuild(t *testing.T) {
	f := newUpdateFixture(t)
	f.version.Version = "v0.1.0-5-gabc1234"

	got := f.run("update", "--check", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	report := f.report(got)
	if report.CurrentIsRelease || !report.UpdateAvailable {
		t.Errorf("report = %+v, want a non-release build with an update available", report)
	}
	if text := f.run("update", "--check"); !strings.Contains(text.stdout, "--force") {
		t.Errorf("stdout = %q, want it to point at --force", text.stdout)
	}
	f.unchanged()
}

func TestUpdateInstallsAVerifiedRelease(t *testing.T) {
	installable(t)
	f := newUpdateFixture(t)

	got := f.run("update", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stdout, "Updated n8n 1.2.3 to v1.3.0") {
		t.Errorf("stdout = %q, want the installed versions", got.stdout)
	}
	if !strings.Contains(got.stderr, "Downloading") {
		t.Errorf("stderr = %q, want progress on the diagnostic stream", got.stderr)
	}
	if got.stdout == "" || strings.Contains(got.stdout, "Downloading") {
		t.Error("progress belongs on stderr, not stdout")
	}
	if installed := f.installed(); installed != f.assetBody() {
		t.Errorf("installed binary = %q, want the published asset", installed)
	}
	info, err := os.Stat(f.exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("installed mode = %#o, want 0755", info.Mode().Perm())
	}
	left, err := filepath.Glob(filepath.Join(filepath.Dir(f.exe), ".n8n-update-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("update left %v behind", left)
	}
}

func TestUpdateInstallReportsJSON(t *testing.T) {
	installable(t)
	f := newUpdateFixture(t)

	got := f.run("update", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	report := f.report(got)
	if !report.Installed || report.UpdateAvailable || report.TargetVersion != "v1.3.0" {
		t.Errorf("report = %+v, want an installed v1.3.0", report)
	}
}

func TestUpdateRefusesADevelopmentBuildWithoutForce(t *testing.T) {
	f := newUpdateFixture(t)
	f.version.Version = "dev"

	got := f.run("update", "--yes")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	for _, want := range []string{"dev", "--force", "install.sh"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q: %s", want, got.stderr)
		}
	}
	f.unchanged()
}

func TestUpdateForceOverwritesADevelopmentBuild(t *testing.T) {
	installable(t)
	f := newUpdateFixture(t)
	f.version.Version = "v0.1.0-dirty"

	got := f.run("update", "--force", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if installed := f.installed(); installed != f.assetBody() {
		t.Errorf("installed binary = %q, want the published asset", installed)
	}
}

func TestUpdateAsksBeforeReplacingTheBinary(t *testing.T) {
	tests := map[string]struct {
		interactive bool
		stdin       string
		want        string
	}{
		"no terminal":  {interactive: false, want: "--yes"},
		"answered no":  {interactive: true, stdin: "n\n", want: "aborted"},
		"answered eof": {interactive: true, stdin: "", want: "aborted"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f := newUpdateFixture(t)
			f.interactive, f.stdin = tt.interactive, tt.stdin

			got := f.run("update")
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitError, got.stderr)
			}
			if !strings.Contains(got.stderr, tt.want) {
				t.Errorf("stderr = %q, want it to mention %q", got.stderr, tt.want)
			}
			f.unchanged()
		})
	}
}

func TestUpdateConfirmsWithTheBinaryAndVersions(t *testing.T) {
	installable(t)
	f := newUpdateFixture(t)
	f.stdin = "y\n"

	got := f.run("update")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	// The prompt names the resolved binary, which is not the path handed in
	// wherever the temporary directory is itself a symlink, as on macOS.
	resolved, err := filepath.EvalSymlinks(f.exe)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Replace", resolved, "1.2.3", "v1.3.0"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("prompt missing %q: %s", want, got.stderr)
		}
	}
}

func TestUpdateRefusesATamperedDownload(t *testing.T) {
	f := newUpdateFixture(t)
	sum := sha256.Sum256([]byte("a different binary"))
	f.manifest = fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), releaseAsset(t))

	got := f.run("update", "--yes")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "checksum mismatch") {
		t.Errorf("stderr = %q, want a checksum mismatch", got.stderr)
	}
	f.unchanged()
}

func TestUpdateRefusesABinaryReportingTheWrongVersion(t *testing.T) {
	installable(t)
	f := newUpdateFixture(t)
	f.asset = "#!/bin/sh\necho \"n8n v9.9.9\"\n"

	got := f.run("update", "--yes")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "v9.9.9") || !strings.Contains(got.stderr, "nothing was installed") {
		t.Errorf("stderr = %q, want the reported version and no install", got.stderr)
	}
	f.unchanged()
}

func TestUpdatePinnedVersionSkipsTheLookup(t *testing.T) {
	installable(t)
	f := newUpdateFixture(t)
	// The pinned tag must be served without asking what the latest release is.
	f.latestStatus = http.StatusInternalServerError
	f.tag = "v1.0.0"

	got := f.run("update", "--version", "v1.0.0", "--yes", "--output", "json")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	for _, path := range f.requests {
		if strings.HasSuffix(path, "/releases/latest") {
			t.Error("a pinned --version still looked up the latest release")
		}
	}
	report := f.report(got)
	if report.LatestVersion != "" || report.TargetVersion != "v1.0.0" || !report.Installed {
		t.Errorf("report = %+v, want an installed v1.0.0 with no latest lookup", report)
	}
	if !strings.Contains(got.stderr, "Downgrading") {
		t.Errorf("stderr = %q, want a downgrade notice", got.stderr)
	}
}

func TestUpdateRejectsUnusableFlagValues(t *testing.T) {
	tests := map[string][]string{
		"unknown output":  {"update", "--check", "--output", "yaml"},
		"unusable tag":    {"update", "--version", "latest", "--yes"},
		"check with yes":  {"update", "--check", "--yes"},
		"positional args": {"update", "v1.3.0"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			f := newUpdateFixture(t)
			got := f.run(args...)
			if got.code != ExitError {
				t.Fatalf("exit code = %d, want %d (stdout: %s)", got.code, ExitError, got.stdout)
			}
			f.unchanged()
		})
	}
}

func TestUpdateRefusesReinstallingTheCurrentVersion(t *testing.T) {
	f := newUpdateFixture(t)

	got := f.run("update", "--version", "v1.2.3", "--yes")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "--force") {
		t.Errorf("stderr = %q, want it to point at --force", got.stderr)
	}
	f.unchanged()
}

func TestUpdateOnTheLatestReleaseDoesNothing(t *testing.T) {
	f := newUpdateFixture(t)
	f.tag = "v1.2.3"

	got := f.run("update", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stdout, "up to date") {
		t.Errorf("stdout = %q, want it to report being up to date", got.stdout)
	}
	for _, path := range f.requests {
		if strings.Contains(path, "/releases/download/") {
			t.Errorf("an up-to-date binary downloaded %s", path)
		}
	}
	f.unchanged()
}

func TestUpdateAheadOfTheLatestReleaseDoesNothing(t *testing.T) {
	f := newUpdateFixture(t)
	f.tag = "v1.0.0"

	got := f.run("update", "--yes")
	if got.code != ExitSuccess {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", got.code, ExitSuccess, got.stderr)
	}
	if !strings.Contains(got.stderr, "newer than the latest release") {
		t.Errorf("stderr = %q, want a notice that the binary is ahead", got.stderr)
	}
	f.unchanged()
}

func TestUpdateReportsARateLimitedLookup(t *testing.T) {
	f := newUpdateFixture(t)
	f.latestStatus = http.StatusForbidden

	got := f.run("update", "--check")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "rate-limited") || !strings.Contains(got.stderr, "--version") {
		t.Errorf("stderr = %q, want a rate-limit message naming --version", got.stderr)
	}
	f.unchanged()
}

func TestUpdateRefusesAPackageManagedBinary(t *testing.T) {
	f := newUpdateFixture(t)
	dir := filepath.Join(t.TempDir(), "go", "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f.exe = filepath.Join(dir, "n8n")
	if err := os.WriteFile(f.exe, []byte(installedContent), 0o755); err != nil {
		t.Fatal(err)
	}

	got := f.run("update", "--yes")
	if got.code != ExitError {
		t.Fatalf("exit code = %d, want %d", got.code, ExitError)
	}
	if !strings.Contains(got.stderr, "go install") || !strings.Contains(got.stderr, "--force") {
		t.Errorf("stderr = %q, want the owning manager and --force", got.stderr)
	}
	f.unchanged()
	if report := f.report(f.run("update", "--check", "--output", "json")); report.Manager != "go install" {
		t.Errorf("report manager = %q, want %q", report.Manager, "go install")
	}
}
