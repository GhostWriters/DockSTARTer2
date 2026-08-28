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

// TestIsAppReferenced_ServiceFileOnly is the regression test for the
// multi-service bug this session found: an app with only a service-scoped
// or shared/virtual .env.app.* file (no plain .env.app.appname at all)
// must still be recognized as referenced -- otherwise CleanupOrphanedEnvFiles
// would delete the file on the very next update.
func TestIsAppReferenced_ServiceFileOnly(t *testing.T) {
	tmpDir := t.TempDir()
	conf := config.AppConfig{ComposeDir: tmpDir}

	envFile := filepath.Join(tmpDir, constants.EnvFileName)
	if err := os.WriteFile(envFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("real per-service file (___service)", func(t *testing.T) {
		serviceFile := filepath.Join(tmpDir, constants.AppEnvFileNamePrefix+"immich___postgres")
		if err := os.WriteFile(serviceFile, []byte("POSTGRES_DB='immich'\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if !IsAppReferenced(context.Background(), "IMMICH", conf) {
			t.Error("IsAppReferenced(IMMICH) = false; want true (service-scoped file has content)")
		}
	})

	t.Run("shared/virtual file (-suffix)", func(t *testing.T) {
		tmpDir2 := t.TempDir()
		conf2 := config.AppConfig{ComposeDir: tmpDir2}
		if err := os.WriteFile(filepath.Join(tmpDir2, constants.EnvFileName), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}
		sharedFile := filepath.Join(tmpDir2, constants.AppEnvFileNamePrefix+"immich-database")
		if err := os.WriteFile(sharedFile, []byte("DB_PASSWORD=''\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if !IsAppReferenced(context.Background(), "IMMICH", conf2) {
			t.Error("IsAppReferenced(IMMICH) = false; want true (shared-file has content)")
		}
	})

	t.Run("unrelated app not referenced", func(t *testing.T) {
		if IsAppReferenced(context.Background(), "SONARR", conf) {
			t.Error("IsAppReferenced(SONARR) = true; want false (no files belong to it)")
		}
	})
}

// TestIsAppReferenced_OverrideServiceFile checks the override-file check
// specifically recognizes a service-qualified env_file reference, not just
// the exact plain filename.
func TestIsAppReferenced_OverrideServiceFile(t *testing.T) {
	tmpDir := t.TempDir()
	conf := config.AppConfig{ComposeDir: tmpDir}

	if err := os.WriteFile(filepath.Join(tmpDir, constants.EnvFileName), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	overrideContent := `services:
  immich-postgres:
    env_file:
      - .env.app.immich___postgres
`
	if err := os.WriteFile(filepath.Join(tmpDir, constants.ComposeOverrideFileName), []byte(overrideContent), 0644); err != nil {
		t.Fatal(err)
	}

	if !IsAppReferenced(context.Background(), "IMMICH", conf) {
		t.Error("IsAppReferenced(IMMICH) = false; want true (override references a service-qualified file)")
	}
}
