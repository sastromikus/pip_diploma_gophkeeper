// Package version provides build metadata for GophKeeper binaries.
package version

import "fmt"

const (
	defaultVersion   = "dev"
	defaultBuildDate = "unknown"
	defaultCommit    = "unknown"
)

// These variables are replaced at build time through -ldflags.
var (
	Version   = defaultVersion
	BuildDate = defaultBuildDate
	Commit    = defaultCommit
)

// Info contains application build metadata.
type Info struct {
	Version   string
	BuildDate string
	Commit    string
}

// Current returns the build metadata embedded in the current binary.
func Current() Info {
	return Info{
		Version:   normalized(Version, defaultVersion),
		BuildDate: normalized(BuildDate, defaultBuildDate),
		Commit:    normalized(Commit, defaultCommit),
	}
}

// Format returns build metadata formatted for command-line output.
func Format(application string) string {
	info := Current()
	return fmt.Sprintf(
		"%s\nVersion: %s\nBuild date: %s\nCommit: %s\n",
		application,
		info.Version,
		info.BuildDate,
		info.Commit,
	)
}

func normalized(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
