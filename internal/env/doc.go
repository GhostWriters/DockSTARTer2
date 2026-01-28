// Package env provides functionality for managing environment variable files.
//
// This package handles reading, writing, and manipulating environment files
// (.env format) used by DockSTARTer for configuration. It mirrors the
// functionality of DockSTARTer's Bash env_* scripts.
//
// Key operations:
//
//   - Get/Set: Retrieve and modify environment variables
//   - GetLine: Access full variable definitions including quotes and comments
//   - ListVars: Enumerate all variables in a file
//   - MergeNewOnly: Merge variables from one file to another, skipping existing ones
//   - Create: Initialize environment files from templates
//
// Value parsing:
//
//   - Respects single and double quotes
//   - Handles inline comments (preceded by space + #)
//   - Preserves literal values including quotes when requested
//
// The package maintains strict compatibility with the original Bash
// implementation's parsing rules and behavior.
package env
