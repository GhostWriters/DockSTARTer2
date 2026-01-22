package version

import (
	"os"
	"path/filepath"
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
var Version = "v0.0.0.0-dev"

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
