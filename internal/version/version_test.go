package version

import (
	"strings"
	"testing"
)

// withBuildVars sets the linker-injected variables for one test and restores
// them afterwards, so tests exercise the same code path a stamped build takes.
func withBuildVars(t *testing.T, v, c, d string) {
	t.Helper()
	oldV, oldC, oldD := version, commit, date
	version, commit, date = v, c, d
	t.Cleanup(func() { version, commit, date = oldV, oldC, oldD })
}

func TestGetPrefersInjectedValues(t *testing.T) {
	withBuildVars(t, "1.4.0", "deadbeef", "2026-05-01T00:00:00Z")

	got := Get()
	if got.Version != "1.4.0" {
		t.Errorf("Version = %q, want %q", got.Version, "1.4.0")
	}
	if got.Commit != "deadbeef" {
		t.Errorf("Commit = %q, want %q", got.Commit, "deadbeef")
	}
	if got.Date != "2026-05-01T00:00:00Z" {
		t.Errorf("Date = %q, want %q", got.Date, "2026-05-01T00:00:00Z")
	}
	if got.GoVersion == "" || got.Platform == "" {
		t.Errorf("runtime fields not populated: %+v", got)
	}
}

func TestGetFallsBackWhenUnstamped(t *testing.T) {
	withBuildVars(t, "", "", "")

	got := Get()
	if got.Version == "" {
		t.Error("Version is empty; want a fallback")
	}
	if got.Commit == "" || got.Date == "" {
		t.Errorf("Commit/Date empty; want %q fallback: %+v", Unknown, got)
	}
	if !strings.Contains(got.Platform, "/") {
		t.Errorf("Platform = %q, want os/arch", got.Platform)
	}
}

func TestUserAgent(t *testing.T) {
	info := Info{Version: "2.0.0", Platform: "darwin/arm64"}
	if want := "n8n-cli/2.0.0 (darwin/arm64)"; info.UserAgent() != want {
		t.Errorf("UserAgent() = %q, want %q", info.UserAgent(), want)
	}
}

func TestStringReportsEveryField(t *testing.T) {
	info := Info{
		Version:   "3.1.4",
		Commit:    "cafebabe",
		Date:      "2026-09-17T00:00:00Z",
		GoVersion: "go1.27.1",
		Platform:  "linux/amd64",
	}
	got := info.String()
	for _, want := range []string{"3.1.4", "cafebabe", "2026-09-17T00:00:00Z", "go1.27.1", "linux/amd64"} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}
	if strings.HasSuffix(got, "\n") {
		t.Error("String() should not end with a newline; the caller adds it")
	}
}
