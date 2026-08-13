package version

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// ApplicationName is the human-readable name of the application.
var ApplicationName = "DockSTARTer2"

// CommandName is the name of the executable command (e.g., "ds2").
// It is initialized dynamically from the executable filename.
var CommandName = "ds2"

// Version is the current version of the application.
// This is intended to be overwritten at build time using:
// -ldflags "-X DockSTARTer2/internal/version.Version=v2.YYYYMMDD.N"
var Version = "v0.0.0-dev"

// SourceRepo is the "owner/repo" slug (see update.ParseRepoAndRef) this
// binary was actually built from, set at build time from GitHub Actions'
// GITHUB_REPOSITORY env var -- "GhostWriters/DockSTARTer2" for an official
// release, or "someuser/DockSTARTer2" for a build produced by someone's
// fork's own release workflow. Empty for a local/dev build (no ldflags
// applied). This is the ground truth for "what repo is this binary
// actually from" -- unlike a config file, it can't go stale from an
// out-of-band binary replacement (manual download, distro package, local
// build), since it's baked into whatever binary is actually running.
// This is intended to be overwritten at build time using:
// -ldflags "-X DockSTARTer2/internal/version.SourceRepo=owner/repo"
var SourceRepo = ""

// Commit is the git commit hash of the build.
var Commit = "none"

// BuildDate is the date the binary was built.
var BuildDate = "unknown"

func init() {
	// Dynamically determine the command name from the executable
	exePath := os.Args[0]
	baseName := filepath.Base(exePath)
	// Strip extension (e.g., .exe on Windows)
	ext := filepath.Ext(baseName)
	CommandName = strings.TrimSuffix(baseName, ext)

	// Fallback to "ds2" if command name matches application name (e.g. dev run)
	if strings.EqualFold(CommandName, ApplicationName) || strings.EqualFold(CommandName, "main") {
		CommandName = "ds2"
	}
}

// GetComposeSdkVersion returns the version of the Docker Compose SDK dependency.
func GetComposeSdkVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/docker/compose/v5" {
			return dep.Version
		}
	}
	return "unknown"
}
