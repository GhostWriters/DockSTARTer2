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
		ValidateComposeOverride(context.Background(), conf)
	})

	// Case 3: Valid YAML with Ports (Reproduce strict validation)
	t.Run("ValidYAML_WithPorts", func(t *testing.T) {
		validContent := `services:
  app1:
    image: busybox
    ports:
      - "${PORT}:80"
      - ${OTHER_PORT}
    unknown_field: "should fail if strict"
`
		if err := os.WriteFile(overrideFile, []byte(validContent), 0644); err != nil {
			t.Fatalf("Failed to write valid override file: %v", err)
		}
		// This should pass. If validation is too strict, it might fail here.
		ValidateComposeOverride(context.Background(), conf)
	})

	// Case 4: Strict Validation (Future Proofing)
	t.Run("StrictValidation", func(t *testing.T) {
		// Valid YAML but might fail strict schema if variables aren't handled or if unknown fields exist
		// Let's use a clean valid file for strict pass
		validContent := `services:
  app1:
    image: busybox
    ports:
      - "80:80"
`
		if err := os.WriteFile(overrideFile, []byte(validContent), 0644); err != nil {
			t.Fatalf("Failed to write valid override file: %v", err)
		}
		ValidateComposeOverrideStrict(context.Background(), conf)

		// Invalid Strict (Unknown Field)
		invalidContent := `services:
  app1:
    image: busybox
    unknown_field: "fail"
`
		if err := os.WriteFile(overrideFile, []byte(invalidContent), 0644); err != nil {
			t.Fatalf("Failed to write invalid strict file: %v", err)
		}
		// This prints a warning to log, but doesn't panic. We just verify it runs.
		ValidateComposeOverrideStrict(context.Background(), conf)
	})
}
