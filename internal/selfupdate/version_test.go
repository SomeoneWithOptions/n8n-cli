package selfupdate

import (
	"errors"
	"testing"
)

func TestParseTagAcceptsPublishedShapes(t *testing.T) {
	tests := map[string]Release{
		"v1.2.3":       {Major: 1, Minor: 2, Patch: 3},
		"1.2.3":        {Major: 1, Minor: 2, Patch: 3},
		" v0.1.0 ":     {Minor: 1},
		"v10.20.30":    {Major: 10, Minor: 20, Patch: 30},
		"v1.0.0-rc.1":  {Major: 1, Pre: "rc.1"},
		"v2.0.0-beta2": {Major: 2, Pre: "beta2"},
	}
	for input, want := range tests {
		got, err := ParseTag(input)
		if err != nil {
			t.Errorf("ParseTag(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseTag(%q) = %+v, want %+v", input, got, want)
		}
	}
}

func TestParseTagRejectsEverythingElse(t *testing.T) {
	for _, input := range []string{"", "  ", "dev", "unknown", "v1.2", "v1.2.3.4", "v1.2.x", "va.b.c", "v1.2.3+build", "v1.2.3-", "v1.2.3-rc/1", "v01.2.3", "latest"} {
		if got, err := ParseTag(input); err == nil {
			t.Errorf("ParseTag(%q) = %+v, want an error", input, got)
		} else if !errors.Is(err, ErrNotRelease) {
			t.Errorf("ParseTag(%q) error = %v, want ErrNotRelease", input, err)
		}
	}
}

func TestParseLocalVersionRejectsDevelopmentBuilds(t *testing.T) {
	// The Makefile stamps git describe, and `go install` records a
	// pseudo-version: none of these can be compared with a release.
	for _, input := range []string{
		"dev",
		"unknown",
		"v0.1.0-dirty",
		"v0.1.0-5-gabc1234",
		"v0.1.0-5-gabc1234-dirty",
		"v0.0.0-20260101120000-abcdef123456",
	} {
		if got, err := ParseLocalVersion(input); err == nil {
			t.Errorf("ParseLocalVersion(%q) = %+v, want an error", input, got)
		} else if !errors.Is(err, ErrNotRelease) {
			t.Errorf("ParseLocalVersion(%q) error = %v, want ErrNotRelease", input, err)
		}
	}
	for _, input := range []string{"v0.1.0", "v1.2.3", "v1.0.0-rc.1"} {
		if _, err := ParseLocalVersion(input); err != nil {
			t.Errorf("ParseLocalVersion(%q): %v", input, err)
		}
	}
}

func TestCompareOrdersReleases(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "v1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.2.4", "v1.2.3", 1},
		{"v1.2.3", "v1.3.0", -1},
		{"v1.9.0", "v2.0.0", -1},
		{"v2.0.0-rc.1", "v2.0.0", -1},
		{"v2.0.0", "v2.0.0-rc.1", 1},
		{"v2.0.0-rc.1", "v2.0.0-rc.2", -1},
		{"v2.0.0-rc.2", "v2.0.0-rc.10", -1},
		{"v2.0.0-alpha", "v2.0.0-beta", -1},
		{"v2.0.0-rc.1", "v2.0.0-rc.1.1", -1},
		{"v2.0.0-1", "v2.0.0-alpha", -1},
	}
	for _, tt := range tests {
		a, err := ParseTag(tt.a)
		if err != nil {
			t.Fatalf("ParseTag(%q): %v", tt.a, err)
		}
		b, err := ParseTag(tt.b)
		if err != nil {
			t.Fatalf("ParseTag(%q): %v", tt.b, err)
		}
		if got := a.Compare(b); got != tt.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestStringIsCanonical(t *testing.T) {
	for input, want := range map[string]string{
		"1.2.3":       "v1.2.3",
		"v1.2.3":      "v1.2.3",
		"v1.0.0-rc.1": "v1.0.0-rc.1",
	} {
		r, err := ParseTag(input)
		if err != nil {
			t.Fatalf("ParseTag(%q): %v", input, err)
		}
		if got := r.String(); got != want {
			t.Errorf("ParseTag(%q).String() = %q, want %q", input, got, want)
		}
	}
}
