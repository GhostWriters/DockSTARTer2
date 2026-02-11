package appenv

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateComposeOverride(t *testing.T) {
	tmpDir := t.TempDir()

	// Create invalid override file
	overrideContent := `version: "3.7"
services:
  app1:
    env_file: [ unclosed bracket
`
	overrideFile := filepath.Join(tmpDir, constants.ComposeOverrideFileName)
	if err := os.WriteFile(overrideFile, []byte(overrideContent), 0644); err != nil {
		t.Fatalf("Failed to write override file: %v", err)
	}

	conf := config.AppConfig{
		ComposeDir: tmpDir,
	}

	// Case 1: Invalid YAML
	t.Run("InvalidYAML", func(t *testing.T) {
		if err := os.WriteFile(overrideFile, []byte(overrideContent), 0644); err != nil {
			t.Fatalf("Failed to write override file: %v", err)
		}
		// This should log a warning but not panic
		ValidateComposeOverride(context.Background(), conf)
	})

	// Case 2: Valid YAML (Project Name Test)
	t.Run("ValidYAML", func(t *testing.T) {
		validContent := `version: "3.7"
services:
  app1:
    image: busybox
`
		if err := os.WriteFile(overrideFile, []byte(validContent), 0644); err != nil {
			t.Fatalf("Failed to write valid override file: %v", err)
		}
		// This should pass without error (and thus no warning logged)
		// We rely on absence of panic and logs (if we could capture logs, but focused on crash/error first)
		ValidateComposeOverride(context.Background(), conf)
	})
}
