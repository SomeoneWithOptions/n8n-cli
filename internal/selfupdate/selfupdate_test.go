package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// releaseServer is a GitHub stand-in: the release lookup endpoint plus the
// asset download paths, named exactly as the release workflow publishes them.
type releaseServer struct {
	t   *testing.T
	tag string
	// assets maps "<tag>/<asset>" to its bytes. checksums.txt is generated
	// from the other entries unless one is registered explicitly.
	assets map[string][]byte
	// latestStatus and latestBody override the release lookup response.
	latestStatus int
	latestBody   string
	// requests records every path served, so a test can prove a lookup was
	// skipped.
	requests []string
	server   *httptest.Server
}

func newReleaseServer(t *testing.T, tag string, asset string, content []byte) *releaseServer {
	t.Helper()
	s := &releaseServer{t: t, tag: tag, assets: map[string][]byte{}, latestStatus: http.StatusOK}
	if asset != "" {
		s.publish(tag, asset, content)
	}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests = append(s.requests, r.URL.Path)
		switch {
		case r.URL.Path == "/repos/"+Repo+"/releases/latest":
			if s.latestStatus != http.StatusOK {
				w.WriteHeader(s.latestStatus)
				_, _ = w.Write([]byte(`{"message":"nope"}`))
				return
			}
			body := s.latestBody
			if body == "" {
				body = fmt.Sprintf(`{"tag_name":%q}`, s.tag)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		case strings.HasPrefix(r.URL.Path, "/"+Repo+"/releases/download/"):
			key := strings.TrimPrefix(r.URL.Path, "/"+Repo+"/releases/download/")
			content, ok := s.assets[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("Not Found"))
				return
			}
			_, _ = w.Write(content)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

// publish registers one asset and rebuilds the checksum manifest over every
// asset of that tag, the way `make dist` does.
func (s *releaseServer) publish(tag, asset string, content []byte) {
	s.assets[tag+"/"+asset] = content
	var manifest strings.Builder
	for key, body := range s.assets {
		keyTag, name, _ := strings.Cut(key, "/")
		if keyTag != tag || name == ChecksumsAsset {
			continue
		}
		fmt.Fprintf(&manifest, "%s  %s\n", hex.EncodeToString(hashOf(body)), name)
	}
	s.assets[tag+"/"+ChecksumsAsset] = []byte(manifest.String())
}

func hashOf(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

// updater builds an updater aimed at this server, pinned to linux/amd64 so the
// asset name is the same on every machine running the suite.
func (s *releaseServer) updater(execPath string) *Updater {
	return New(Config{
		APIBaseURL:      s.server.URL,
		DownloadBaseURL: s.server.URL,
		ExecPath:        execPath,
		GOOS:            "linux",
		GOARCH:          "amd64",
		UserAgent:       "n8n-cli/test",
	})
}

// installedBinary writes a file standing in for the binary under update.
func installedBinary(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "n8n")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestAssetNamesEveryReleasedPlatform(t *testing.T) {
	for platform, want := range map[string]string{
		"linux/amd64":   "n8n-linux-amd64",
		"linux/arm64":   "n8n-linux-arm64",
		"darwin/amd64":  "n8n-darwin-amd64",
		"darwin/arm64":  "n8n-darwin-arm64",
		"windows/amd64": "n8n-windows-amd64.exe",
	} {
		goos, goarch, _ := strings.Cut(platform, "/")
		got, err := New(Config{GOOS: goos, GOARCH: goarch}).Asset()
		if err != nil {
			t.Errorf("%s: %v", platform, err)
			continue
		}
		if got != want {
			t.Errorf("%s asset = %q, want %q", platform, got, want)
		}
	}
	for _, platform := range []string{"windows/arm64", "linux/386", "freebsd/amd64"} {
		goos, goarch, _ := strings.Cut(platform, "/")
		if _, err := New(Config{GOOS: goos, GOARCH: goarch}).Asset(); err == nil {
			t.Errorf("%s: want an error, got an asset name", platform)
		} else if !strings.Contains(err.Error(), platform) {
			t.Errorf("%s: error does not name the platform: %v", platform, err)
		}
	}
}

func TestLatestTagReadsTheTag(t *testing.T) {
	server := newReleaseServer(t, "v1.3.0", "", nil)
	var header http.Header
	base := server.server.Config.Handler
	server.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		base.ServeHTTP(w, r)
	})

	got, err := server.updater("").LatestTag(context.Background())
	if err != nil {
		t.Fatalf("LatestTag: %v", err)
	}
	if got != "v1.3.0" {
		t.Errorf("LatestTag = %q, want %q", got, "v1.3.0")
	}
	if header.Get("User-Agent") != "n8n-cli/test" {
		t.Errorf("User-Agent = %q, want the configured one", header.Get("User-Agent"))
	}
	if header.Get("Accept") != "application/vnd.github+json" || header.Get("X-GitHub-Api-Version") == "" {
		t.Errorf("GitHub headers missing: %v", header)
	}
}

func TestLatestTagExplainsFailures(t *testing.T) {
	tests := map[string]struct {
		status int
		body   string
		want   string
	}{
		"rate limited": {status: http.StatusForbidden, want: "--version"},
		"too many":     {status: http.StatusTooManyRequests, want: "--version"},
		"no release":   {status: http.StatusNotFound, want: "no published release"},
		"server error": {status: http.StatusInternalServerError, want: "500"},
		"not json":     {body: "<html>error</html>", want: "decode"},
		"tag missing":  {body: `{"name":"1.3.0"}`, want: "no tag name"},
		"tag empty":    {body: `{"tag_name":"  "}`, want: "no tag name"},
		"tag unusable": {body: `{"tag_name":"nightly"}`, want: "nightly"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			server := newReleaseServer(t, "v1.3.0", "", nil)
			if tt.status != 0 {
				server.latestStatus = tt.status
			}
			server.latestBody = tt.body

			tag, err := server.updater("").LatestTag(context.Background())
			if err == nil {
				// An unusable tag parses further up, in the command.
				if _, perr := ParseTag(tag); perr == nil {
					t.Fatalf("LatestTag = %q, want a failure", tag)
				}
				return
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestChecksumForParsesBothManifestFormats(t *testing.T) {
	digest := strings.Repeat("ab", sha256.Size)
	manifest := []byte("" +
		digest + "  n8n-linux-amd64\n" +
		strings.Repeat("cd", sha256.Size) + " *n8n-windows-amd64.exe\n" +
		"\n" +
		"garbage\n")
	got, err := checksumFor(manifest, "n8n-linux-amd64")
	if err != nil || got != digest {
		t.Errorf("checksumFor = %q, %v, want %q", got, err, digest)
	}
	if _, err := checksumFor(manifest, "n8n-windows-amd64.exe"); err != nil {
		t.Errorf("binary-mode entry: %v", err)
	}
	if _, err := checksumFor(manifest, "n8n-darwin-arm64"); err == nil {
		t.Error("missing asset: want an error")
	}
	if _, err := checksumFor([]byte("nothex  n8n-linux-amd64\n"), "n8n-linux-amd64"); err == nil {
		t.Error("invalid digest: want an error")
	}
}

func TestFetchVerifiesAndStagesTheAsset(t *testing.T) {
	const body = "#!/bin/sh\necho binary\n"
	server := newReleaseServer(t, "v1.3.0", "n8n-linux-amd64", []byte(body))
	exe := installedBinary(t, "old binary")
	updater := server.updater(exe)
	dest, err := updater.Destination()
	if err != nil {
		t.Fatalf("Destination: %v", err)
	}

	staged, err := updater.Fetch(context.Background(), "v1.3.0", dest)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if filepath.Dir(staged) != dest.Dir {
		t.Errorf("staged at %s, want a file in %s", staged, dest.Dir)
	}
	got, err := os.ReadFile(staged)
	if err != nil {
		t.Fatalf("read staged: %v", err)
	}
	if string(got) != body {
		t.Errorf("staged content = %q, want %q", got, body)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(staged)
		if err != nil {
			t.Fatalf("stat staged: %v", err)
		}
		if info.Mode().Perm() != stagedPerm {
			t.Errorf("staged mode = %#o, want %#o", info.Mode().Perm(), stagedPerm)
		}
	}
	if installed, err := os.ReadFile(exe); err != nil || string(installed) != "old binary" {
		t.Errorf("Fetch touched the installed binary: %q, %v", installed, err)
	}

	if err := updater.Install(staged, dest); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if installed, err := os.ReadFile(exe); err != nil || string(installed) != body {
		t.Errorf("installed binary = %q, %v, want the downloaded one", installed, err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Errorf("staged file still present after Install: %v", err)
	}
}

func TestFetchRefusesAMismatchedChecksum(t *testing.T) {
	server := newReleaseServer(t, "v1.3.0", "n8n-linux-amd64", []byte("good"))
	// Replace the asset without rebuilding the manifest: the download no
	// longer matches what the release claims.
	server.assets["v1.3.0/n8n-linux-amd64"] = []byte("tampered")
	exe := installedBinary(t, "old binary")
	updater := server.updater(exe)
	dest, err := updater.Destination()
	if err != nil {
		t.Fatalf("Destination: %v", err)
	}

	staged, err := updater.Fetch(context.Background(), "v1.3.0", dest)
	if err == nil {
		t.Fatalf("Fetch staged %s, want a checksum failure", staged)
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want a checksum mismatch", err)
	}
	if installed, _ := os.ReadFile(exe); string(installed) != "old binary" {
		t.Errorf("installed binary changed: %q", installed)
	}
	left, err := filepath.Glob(filepath.Join(dest.Dir, ".n8n-update-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("failed download left %v behind", left)
	}
}

func TestFetchReportsMissingAssets(t *testing.T) {
	tests := map[string]func(*releaseServer){
		"no asset":     func(s *releaseServer) { delete(s.assets, "v1.3.0/n8n-linux-amd64") },
		"no checksums": func(s *releaseServer) { delete(s.assets, "v1.3.0/"+ChecksumsAsset) },
		"wrong tag":    func(s *releaseServer) { s.assets = map[string][]byte{} },
	}
	for name, breakIt := range tests {
		t.Run(name, func(t *testing.T) {
			server := newReleaseServer(t, "v1.3.0", "n8n-linux-amd64", []byte("binary"))
			breakIt(server)
			updater := server.updater(installedBinary(t, "old binary"))
			dest, err := updater.Destination()
			if err != nil {
				t.Fatalf("Destination: %v", err)
			}
			if _, err := updater.Fetch(context.Background(), "v1.3.0", dest); err == nil {
				t.Fatal("Fetch succeeded, want an error")
			} else if !strings.Contains(err.Error(), "v1.3.0") {
				t.Errorf("error = %v, want it to name the release", err)
			}
		})
	}
}

func TestDestinationReportsPathAndDirectory(t *testing.T) {
	exe := installedBinary(t, "binary")
	dest, err := New(Config{ExecPath: exe}).Destination()
	if err != nil {
		t.Fatalf("Destination: %v", err)
	}
	// EvalSymlinks resolves the temporary directory itself on macOS, so
	// compare the resolved form rather than the path handed in.
	want, err := filepath.EvalSymlinks(exe)
	if err != nil {
		t.Fatal(err)
	}
	if dest.Path != want {
		t.Errorf("Path = %q, want %q", dest.Path, want)
	}
	if dest.Dir != filepath.Dir(want) {
		t.Errorf("Dir = %q, want %q", dest.Dir, filepath.Dir(want))
	}
	if dest.Manager != "" {
		t.Errorf("Manager = %q, want empty for a plain file", dest.Manager)
	}
	if err := dest.Writable(); err != nil {
		t.Errorf("Writable: %v", err)
	}

	if _, err := New(Config{ExecPath: filepath.Join(filepath.Dir(exe), "absent")}).Destination(); err == nil {
		t.Error("Destination of a missing binary: want an error")
	}
	missing := Destination{Path: filepath.Join(exe, "nested", "n8n"), Dir: filepath.Join(exe, "nested")}
	if err := missing.Writable(); err == nil {
		t.Error("Writable on a missing directory: want an error")
	}
}

func TestManagerForNamesTheOwner(t *testing.T) {
	for path, want := range map[string]string{
		"/home/u/.local/bin/n8n":                 "",
		"/usr/local/bin/n8n":                     "",
		"/nix/store/abc-n8n-1.2.3/bin/n8n":       "Nix",
		"/opt/homebrew/Cellar/n8n/1.2.3/bin/n8n": "Homebrew",
		"/home/linuxbrew/.linuxbrew/bin/n8n":     "Homebrew",
		"/snap/n8n/current/bin/n8n":              "Snap",
		"/home/u/go/bin/n8n":                     "go install",
		"/usr/lib/n8n/n8n":                       "the system package manager",
	} {
		if got := managerFor(path); got != want {
			t.Errorf("managerFor(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestReplaceExecutableSwapsContent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "n8n")
	if err := os.WriteFile(target, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(dir, ".n8n-update-new")
	if err := os.WriteFile(staged, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(staged, target); err != nil {
		t.Fatalf("replaceExecutable: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "new" {
		t.Errorf("target = %q, %v, want %q", got, err, "new")
	}
	cleanupLeftovers(target)
	if _, err := os.Stat(target); err != nil {
		t.Errorf("cleanup removed the installed binary: %v", err)
	}
}
