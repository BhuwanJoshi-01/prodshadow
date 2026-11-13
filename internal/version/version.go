// Package version exposes build metadata for the prodshadow binary.
//
// The variables are overridden at link time by the release pipeline, e.g.
//
//	go build -ldflags "-X github.com/BhuwanJoshi-01/Open-Source-1/internal/version.Version=v0.1.0"
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// These values are replaced through -ldflags during release builds.
var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Info describes the running binary.
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	Date      string `json:"date,omitempty"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

// Get returns build information, falling back to VCS data embedded by the Go
// toolchain when the binary was built without release ldflags.
func Get() Info {
	info := Info{
		Version:   Version,
		Commit:    Commit,
		Date:      Date,
		GoVersion: runtime.Version(),
		Platform:  runtime.GOOS + "/" + runtime.GOARCH,
	}
	if info.Commit == "" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			for _, s := range bi.Settings {
				switch s.Key {
				case "vcs.revision":
					info.Commit = s.Value
				case "vcs.time":
					if info.Date == "" {
						info.Date = s.Value
					}
				}
			}
		}
	}
	return info
}

// String renders a single human-readable line.
func (i Info) String() string {
	commit := i.Commit
	if len(commit) > 12 {
		commit = commit[:12]
	}
	if commit == "" {
		commit = "unknown"
	}
	return fmt.Sprintf("prodshadow %s (commit %s, %s, %s)", i.Version, commit, i.GoVersion, i.Platform)
}
