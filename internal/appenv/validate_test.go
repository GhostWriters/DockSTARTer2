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

	// This should not panic and ideally log a warning (verified by visual inspection if running -v)
	ValidateComposeOverride(context.Background(), conf)
}
