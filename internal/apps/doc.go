// Package apps provides functionality for managing DockSTARTer applications.
//
// This package handles application discovery, validation, status checking, and
// environment variable generation for Docker applications. It mirrors the
// functionality of DockSTARTer's Bash scripts for application management,
// including:
//
//   - Application listing and filtering (builtin, deprecated, enabled, etc.)
//   - Application name validation and instance handling
//   - Environment variable creation and management
//   - Application status determination
//
// Key concepts:
//
//   - Builtin apps: Applications with templates in the .apps directory
//   - Added apps: Applications with __ENABLED variables in .env
//   - App instances: Variants of apps with __ suffix (e.g., RADARR__4K)
//   - NiceName: Human-readable application names from labels.yml
//
// The package maintains strict compatibility with the original Bash
// implementation's behavior and output.
package apps
