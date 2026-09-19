// Package selfupdate replaces the running binary with a published release.
//
// The release workflow is the contract this package reads: `make dist` builds
// one plain binary per target named n8n-<os>-<arch> (with .exe on Windows) plus
// a checksums.txt over all of them, and the workflow publishes exactly those
// names under a v* tag. install.sh and install.ps1 fetch the same names, so a
// binary installed by either script and one installed here are the same file.
//
// Nothing here trusts the download: the asset is written to a temporary file
// next to the binary it will replace, its SHA-256 is checked against the
// release manifest, and the staged file must report the expected version when
// run before it is moved into place. The checksums travel over the same TLS
// connection as the asset, so they prove integrity, not authorship: a
// compromised release would still install. Signature verification would be the
// fix and does not exist yet.
package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// Repo is the GitHub repository releases are published to.
	Repo = "SomeoneWithOptions/n8n-cli"
	// DefaultAPIBaseURL is the GitHub REST API root.
	DefaultAPIBaseURL = "https://api.github.com"
	// DefaultDownloadBaseURL is the host serving release assets.
	DefaultDownloadBaseURL = "https://github.com"
	// ChecksumsAsset is the manifest `make dist` publishes with the binaries.
	ChecksumsAsset = "checksums.txt"
	// DefaultTimeout bounds one HTTP request.
	DefaultTimeout = 60 * time.Second

	// verifyTimeout bounds the single `version` run of the staged binary.
	verifyTimeout = 30 * time.Second
	// maxAssetBytes and maxManifestBytes bound both downloads, so a wrong or
	// hostile endpoint cannot fill the disk of the machine being updated.
	maxAssetBytes    = 256 << 20
	maxManifestBytes = 1 << 20
	// stagedPerm is the mode of the staged binary, matching what install.sh
	// sets. Rename carries it to the installed path.
	stagedPerm = 0o755
)

// Doer is the subset of *http.Client this package needs. Tests inject their own.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Config are the dependencies of an [Updater]. Every zero value resolves to the
// production default, so New(Config{}) updates this binary from GitHub.
type Config struct {
	// HTTP replaces the HTTP client. Nil uses a default one, which follows the
	// redirect GitHub answers asset downloads with.
	HTTP Doer
	// APIBaseURL overrides [DefaultAPIBaseURL].
	APIBaseURL string
	// DownloadBaseURL overrides [DefaultDownloadBaseURL].
	DownloadBaseURL string
	// UserAgent is sent on every request. GitHub rejects requests without one.
	UserAgent string
	// ExecPath overrides the binary to replace. Empty resolves through
	// [os.Executable].
	ExecPath string
	// GOOS and GOARCH override the platform the asset is chosen for. Empty
	// uses the platform this binary was built for.
	GOOS, GOARCH string
	// Timeout overrides [DefaultTimeout].
	Timeout time.Duration
}

// Updater downloads one release and installs it over the running binary.
type Updater struct{ cfg Config }

// New fills the defaults of cfg and returns the updater for it.
func New(cfg Config) *Updater {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{}
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = DefaultAPIBaseURL
	}
	if cfg.DownloadBaseURL == "" {
		cfg.DownloadBaseURL = DefaultDownloadBaseURL
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "n8n-cli"
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.GOARCH == "" {
		cfg.GOARCH = runtime.GOARCH
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	return &Updater{cfg: cfg}
}

// released lists the platforms the release workflow builds, keyed as
// GOOS/GOARCH. A platform absent here has no asset to install.
var released = map[string]bool{
	"linux/amd64":   true,
	"linux/arm64":   true,
	"darwin/amd64":  true,
	"darwin/arm64":  true,
	"windows/amd64": true,
}

// Asset is the release asset name for this platform.
func (u *Updater) Asset() (string, error) {
	platform := u.cfg.GOOS + "/" + u.cfg.GOARCH
	if !released[platform] {
		return "", fmt.Errorf("no release asset for %s: releases ship linux/amd64, linux/arm64, darwin/amd64, darwin/arm64 and windows/amd64; build from source or download from https://github.com/%s/releases", platform, Repo)
	}
	name := "n8n-" + u.cfg.GOOS + "-" + u.cfg.GOARCH
	if u.cfg.GOOS == "windows" {
		name += ".exe"
	}
	return name, nil
}

// LatestTag returns the tag of the latest published release. GitHub excludes
// drafts and prereleases from this endpoint, so a prerelease never installs
// itself over a stable binary.
func (u *Updater) LatestTag(ctx context.Context) (string, error) {
	url := u.cfg.APIBaseURL + "/repos/" + Repo + "/releases/latest"
	resp, err := u.get(ctx, url, http.Header{
		"Accept":               []string{"application/vnd.github+json"},
		"X-GitHub-Api-Version": []string{"2022-11-28"},
	})
	if err != nil {
		return "", err
	}
	defer closeBody(resp)

	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		return "", fmt.Errorf("GitHub rate-limited the release lookup (%s): retry later, or install a known tag with --version vX.Y.Z", resp.Status)
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("no published release found for %s", Repo)
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("release lookup failed: %s: %s", resp.Status, snippet(resp.Body))
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxManifestBytes)).Decode(&release); err != nil {
		return "", fmt.Errorf("decode release lookup: %w", err)
	}
	if strings.TrimSpace(release.TagName) == "" {
		return "", fmt.Errorf("release lookup returned no tag name")
	}
	return strings.TrimSpace(release.TagName), nil
}

// Destination is the binary an update would replace.
type Destination struct {
	// Path is the resolved binary, with symlinks followed: replacing the
	// symlink instead of its target would silently detach the install.
	Path string
	// Dir holds Path. The download is staged here, so the final move is a
	// rename within one filesystem and cannot leave a half-written binary.
	Dir string
	// Manager names the package manager that appears to own Path, empty when
	// the file looks self-installed. Overwriting a managed file works but the
	// manager will undo or fight it, so the CLI asks before doing that.
	Manager string
}

// Destination resolves the binary to replace.
func (u *Updater) Destination() (Destination, error) {
	path := u.cfg.ExecPath
	if path == "" {
		var err error
		path, err = os.Executable()
		if err != nil {
			return Destination{}, fmt.Errorf("locate the running binary: %w", err)
		}
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return Destination{}, fmt.Errorf("resolve %s: %w", path, err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return Destination{}, fmt.Errorf("resolve %s: %w", path, err)
	}
	return Destination{Path: resolved, Dir: filepath.Dir(resolved), Manager: managerFor(resolved)}, nil
}

// Writable reports whether a new binary can be staged next to the current one,
// by creating and removing a file the way an update would.
func (d Destination) Writable() error {
	probe, err := os.CreateTemp(d.Dir, ".n8n-update-probe-*")
	if err != nil {
		return fmt.Errorf("cannot write to %s: %w", d.Dir, err)
	}
	name := probe.Name()
	if err := probe.Close(); err != nil {
		return fmt.Errorf("cannot write to %s: %w", d.Dir, err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("cannot write to %s: %w", d.Dir, err)
	}
	return nil
}

// managerFor reports the package manager that owns path, empty when none does.
// The test is the path alone: asking each manager would mean running them.
func managerFor(path string) string {
	p := filepath.ToSlash(path)
	switch {
	case strings.Contains(p, "/nix/store/"):
		return "Nix"
	case strings.Contains(p, "/Cellar/"), strings.Contains(p, "/homebrew/"), strings.Contains(p, "/linuxbrew/"):
		return "Homebrew"
	case strings.Contains(p, "/snap/"):
		return "Snap"
	case strings.Contains(p, "/go/bin/"), strings.Contains(p, "/gopath/bin/"):
		return "go install"
	case strings.Contains(p, "/usr/lib/"), strings.Contains(p, "/usr/share/"):
		return "the system package manager"
	default:
		return ""
	}
}

// Fetch downloads the release asset for tag, verifies it against the release
// checksums, and returns the path of the staged file inside dest.Dir. The
// caller owns that file: install it with [Updater.Install] or remove it.
// Fetch removes it itself on every error of its own.
func (u *Updater) Fetch(ctx context.Context, tag string, dest Destination) (string, error) {
	asset, err := u.Asset()
	if err != nil {
		return "", err
	}
	want, err := u.checksum(ctx, tag, asset)
	if err != nil {
		return "", err
	}

	pattern := ".n8n-update-*"
	if u.cfg.GOOS == "windows" {
		// Windows will not execute a file without the extension, and the
		// staged binary is run once before it is installed.
		pattern += ".exe"
	}
	staged, err := os.CreateTemp(dest.Dir, pattern)
	if err != nil {
		return "", fmt.Errorf("stage the download in %s: %w", dest.Dir, err)
	}
	path := staged.Name()
	discard := func() {
		_ = staged.Close()
		_ = os.Remove(path)
	}

	resp, err := u.get(ctx, u.assetURL(tag, asset), nil)
	if err != nil {
		discard()
		return "", err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		discard()
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("release %s has no asset %s: check the tag on https://github.com/%s/releases", tag, asset, Repo)
		}
		return "", fmt.Errorf("download %s: %s: %s", asset, resp.Status, snippet(resp.Body))
	}

	digest := sha256.New()
	written, err := io.Copy(io.MultiWriter(staged, digest), io.LimitReader(resp.Body, maxAssetBytes+1))
	if err != nil {
		discard()
		return "", fmt.Errorf("download %s: %w", asset, err)
	}
	if written > maxAssetBytes {
		discard()
		return "", fmt.Errorf("download %s: larger than the %d-byte limit", asset, int64(maxAssetBytes))
	}
	if err := staged.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	if got := hex.EncodeToString(digest.Sum(nil)); got != want {
		_ = os.Remove(path)
		return "", fmt.Errorf("checksum mismatch for %s: %s has %s, the download has %s; nothing was installed", asset, ChecksumsAsset, want, got)
	}
	if err := os.Chmod(path, stagedPerm); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("make %s executable: %w", path, err)
	}
	return path, nil
}

// checksum reads the release manifest and returns the expected digest of asset.
func (u *Updater) checksum(ctx context.Context, tag, asset string) (string, error) {
	resp, err := u.get(ctx, u.assetURL(tag, ChecksumsAsset), nil)
	if err != nil {
		return "", err
	}
	defer closeBody(resp)
	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("release %s publishes no %s, so the download cannot be verified: install it manually from https://github.com/%s/releases", tag, ChecksumsAsset, Repo)
		}
		return "", fmt.Errorf("download %s for %s: %s: %s", ChecksumsAsset, tag, resp.Status, snippet(resp.Body))
	}
	manifest, err := io.ReadAll(io.LimitReader(resp.Body, maxManifestBytes))
	if err != nil {
		return "", fmt.Errorf("read %s for %s: %w", ChecksumsAsset, tag, err)
	}
	return checksumFor(manifest, asset)
}

// checksumFor finds the digest of asset in a sha256sum-format manifest. Both
// sha256sum and shasum are used by `make dist`, so the binary-mode "*" prefix
// on the name has to be accepted too.
func checksumFor(manifest []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(manifest), "\n") {
		digest, name, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		if strings.TrimPrefix(strings.TrimSpace(name), "*") != asset {
			continue
		}
		digest = strings.ToLower(digest)
		if raw, err := hex.DecodeString(digest); err != nil || len(raw) != sha256.Size {
			return "", fmt.Errorf("%s lists an invalid digest for %s", ChecksumsAsset, asset)
		}
		return digest, nil
	}
	return "", fmt.Errorf("%s does not list %s", ChecksumsAsset, asset)
}

// VerifyBinary runs the staged binary once and fails unless it reports tag.
// This is the same check the release workflow makes after building, and it
// catches a truncated download, a wrong-platform asset and a mislabelled
// release before any of them become the installed binary.
func (u *Updater) VerifyBinary(ctx context.Context, path, tag string) error {
	ctx, cancel := context.WithTimeout(ctx, verifyTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, path, "version").Output()
	if err != nil {
		return fmt.Errorf("the downloaded binary did not run: %w", err)
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if want := "n8n " + tag; strings.TrimSpace(first) != want {
		return fmt.Errorf("the downloaded binary reports %q, not %q; nothing was installed", strings.TrimSpace(first), want)
	}
	return nil
}

// Install moves the staged binary over dest.Path.
func (u *Updater) Install(staged string, dest Destination) error {
	cleanupLeftovers(dest.Path)
	return replaceExecutable(staged, dest.Path)
}

// assetURL is the download URL of one asset of one release, the same URL
// install.sh builds.
func (u *Updater) assetURL(tag, asset string) string {
	return u.cfg.DownloadBaseURL + "/" + Repo + "/releases/download/" + tag + "/" + asset
}

// get performs one bounded GET. Redirects are left to the HTTP client, because
// GitHub answers every asset download with one.
func (u *Updater) get(ctx context.Context, url string, header http.Header) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, u.cfg.Timeout)
	// The body is read by the caller, so the timeout must outlive this
	// function; the response body cancels it on close.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build request for %s: %w", url, err)
	}
	for name, values := range header {
		req.Header[name] = values
	}
	req.Header.Set("User-Agent", u.cfg.UserAgent)

	resp, err := u.cfg.HTTP.Do(req)
	if err != nil {
		cancel()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

// cancelBody releases the request context when the body is closed.
type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

func closeBody(resp *http.Response) { _ = resp.Body.Close() }

// snippet renders the start of an error response, so a proxy or an error page
// is reported instead of being hidden behind a bare status code.
func snippet(r io.Reader) string {
	body, _ := io.ReadAll(io.LimitReader(r, 512))
	text := strings.TrimSpace(strings.ReplaceAll(string(body), "\n", " "))
	if text == "" {
		return "(no response body)"
	}
	return text
}
