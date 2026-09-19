package selfupdate

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Release is a parsed release tag: vMAJOR.MINOR.PATCH with an optional
// prerelease part, which is the only shape the release workflow publishes.
type Release struct {
	Major, Minor, Patch int
	// Pre is the prerelease part without its leading hyphen, empty for a
	// final release.
	Pre string
}

// ErrNotRelease reports a version string that is not a published release tag,
// which is what every development build reports.
var ErrNotRelease = errors.New("not a release version")

// String renders the canonical tag, including the leading v, which is how the
// release assets and the `version` command spell it.
func (r Release) String() string {
	s := fmt.Sprintf("v%d.%d.%d", r.Major, r.Minor, r.Patch)
	if r.Pre != "" {
		s += "-" + r.Pre
	}
	return s
}

// Compare orders two releases: -1 when r is older, 0 when they are the same
// release, +1 when r is newer. Prereleases sort before the final release of the
// same number, as semantic versioning requires.
func (r Release) Compare(o Release) int {
	for _, pair := range [][2]int{{r.Major, o.Major}, {r.Minor, o.Minor}, {r.Patch, o.Patch}} {
		if c := cmpInt(pair[0], pair[1]); c != 0 {
			return c
		}
	}
	return comparePre(r.Pre, o.Pre)
}

// ParseTag parses a published release tag. The leading v is optional on input
// so a user may pass either spelling to --version. Build metadata is rejected:
// no published tag carries it, and accepting it would let two different strings
// name the same asset.
func ParseTag(s string) (Release, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Release{}, fmt.Errorf("%w: empty", ErrNotRelease)
	}
	if strings.Contains(s, "+") {
		return Release{}, fmt.Errorf("%w: %q carries build metadata", ErrNotRelease, s)
	}
	trimmed := strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	number, pre, hasPre := strings.Cut(trimmed, "-")

	fields := strings.Split(number, ".")
	if len(fields) != 3 {
		return Release{}, fmt.Errorf("%w: %q is not vMAJOR.MINOR.PATCH", ErrNotRelease, s)
	}
	var r Release
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 || field != strconv.Itoa(n) {
			return Release{}, fmt.Errorf("%w: %q has a non-numeric component %q", ErrNotRelease, s, field)
		}
		switch i {
		case 0:
			r.Major = n
		case 1:
			r.Minor = n
		case 2:
			r.Patch = n
		}
	}
	if hasPre {
		if pre == "" || strings.Trim(pre, "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz.-") != "" {
			return Release{}, fmt.Errorf("%w: %q has an invalid prerelease part", ErrNotRelease, s)
		}
		r.Pre = pre
	}
	return r, nil
}

// ParseLocalVersion parses the version stamped into this binary and fails with
// [ErrNotRelease] unless it is an exact published release.
//
// The Makefile stamps `git describe --tags --always --dirty`, and `go install`
// records a pseudo-version, so a development build reports "dev", a commit
// distance ("v0.1.0-5-gabc1234"), a dirty marker or a pseudo-version. None of
// those can be compared with a release tag, which is why updating over one
// needs an explicit --force.
func ParseLocalVersion(s string) (Release, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "dirty") {
		return Release{}, fmt.Errorf("%w: %q is a modified working tree", ErrNotRelease, s)
	}
	r, err := ParseTag(s)
	if err != nil {
		return Release{}, err
	}
	// A prerelease part with a hyphen is a commit distance or a pseudo-version,
	// never a tag: the release workflow only ever publishes what it is given.
	if strings.Contains(r.Pre, "-") {
		return Release{}, fmt.Errorf("%w: %q is a build between releases", ErrNotRelease, s)
	}
	return r, nil
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// comparePre orders prerelease parts: absent outranks present, and identifiers
// are compared one by one, numerically when both sides are numeric.
func comparePre(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(left) && i < len(right); i++ {
		ln, lErr := strconv.Atoi(left[i])
		rn, rErr := strconv.Atoi(right[i])
		switch {
		case lErr == nil && rErr == nil:
			if c := cmpInt(ln, rn); c != 0 {
				return c
			}
		case lErr == nil:
			return -1 // numeric identifiers rank below alphanumeric ones
		case rErr == nil:
			return 1
		default:
			if c := strings.Compare(left[i], right[i]); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(left), len(right))
}
