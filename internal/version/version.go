// Package version exposes build metadata for the n8n binary.
//
// The package-level variables below are the only globals in the project. They
// exist because -X linker flags can write to nothing else. Everything outside
// this package consumes the immutable [Info] returned by [Get].
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Injected at build time with -ldflags -X. See the Makefile.
var (
	version = ""
	commit  = ""
	date    = ""
)

// Unknown is reported for metadata the build did not provide.
const Unknown = "unknown"

// Info is a snapshot of build metadata.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// Get resolves build metadata, preferring linker-injected values and falling
// back to the module build info recorded by `go build` and `go install`.
func Get() Info {
	info := Info{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}

	if bi, ok := debug.ReadBuildInfo(); ok {
		if info.Version == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			info.Version = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if info.Commit == "" {
					info.Commit = s.Value
				}
			case "vcs.time":
				if info.Date == "" {
					info.Date = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" {
					info.Version = strings.TrimSuffix(info.Version, "-dirty") + "-dirty"
				}
			}
		}
	}

	if info.Version == "" {
		info.Version = "dev"
	}
	if info.Commit == "" {
		info.Commit = Unknown
	}
	if info.Date == "" {
		info.Date = Unknown
	}
	return info
}

// UserAgent is the value sent on every API request once a transport exists.
func (i Info) UserAgent() string {
	return "n8n-cli/" + i.Version + " (" + i.Platform + ")"
}

// String renders one human-readable line per field.
func (i Info) String() string {
	return fmt.Sprintf("n8n %s\ncommit: %s\nbuilt:  %s\ngo:     %s\nos/arch: %s",
		i.Version, i.Commit, i.Date, i.GoVersion, i.Platform)
}
