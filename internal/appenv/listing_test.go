package appenv

import (
	"DockSTARTer2/internal/config"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestListReferencedApps_Comments(t *testing.T) {
	tmpDir := t.TempDir()

	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Override file with mixed content
	overrideContent := `version: "3.7"
services:
  app1:
    env_file:
      - .env.app.app1
  app2:
    env_file: .env.app.app2
  # app3:
  #   env_file:
  #     - .env.app.app3
  app4:
    # env_file: .env.app.app4
    image: busybox
`
	overrideFile := filepath.Join(tmpDir, "docker-compose.override.yml")
	if err := os.WriteFile(overrideFile, []byte(overrideContent), 0644); err != nil {
		t.Fatalf("Failed to write override file: %v", err)
	}

	conf := config.AppConfig{
		ComposeDir: tmpDir,
	}

	apps, err := ListReferencedApps(context.Background(), conf)
	if err != nil {
		t.Fatalf("ListReferencedApps failed: %v", err)
	}

	// APP1 and APP2 should be present
	// APP3 and APP4 should NOT be present

	if !slices.Contains(apps, "APP1") {
		t.Errorf("Expected APP1 to be referenced, got: %v", apps)
	}
	if !slices.Contains(apps, "APP2") {
		t.Errorf("Expected APP2 to be referenced, got: %v", apps)
	}
	if slices.Contains(apps, "APP3") {
		t.Errorf("Expected APP3 to NOT be referenced (commented service), got: %v", apps)
	}
	if slices.Contains(apps, "APP4") {
		t.Errorf("Expected APP4 to NOT be referenced (commented env_file), got: %v", apps)
	}
}

func TestListReferencedApps_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()

	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	// Invalid YAML content (tab indentation is illegal in YAML, or just garbage)
	overrideContent := `version: "3.7"
services:
  app1:
    env_file: [ unclosed bracket
`
	overrideFile := filepath.Join(tmpDir, "docker-compose.override.yml")
	if err := os.WriteFile(overrideFile, []byte(overrideContent), 0644); err != nil {
		t.Fatalf("Failed to write override file: %v", err)
	}

	conf := config.AppConfig{
		ComposeDir: tmpDir,
	}

	// Should not return error, just ignore the invalid file
	apps, err := ListReferencedApps(context.Background(), conf)
	if err != nil {
		t.Fatalf("ListReferencedApps failed with invalid YAML: %v", err)
	}

	if len(apps) != 0 {
		t.Errorf("Expected 0 referenced apps, got: %v", apps)
	}
}

// TestListReferencedApps_ServiceScopedFile is the regression test for the
// bug this session found: a service-scoped or shared/virtual .env.app.*
// file's discovery must credit the base app it belongs to, not be silently
// dropped by the final IsAppNameValid filter.
func TestListReferencedApps_ServiceScopedFile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	serviceFile := filepath.Join(tmpDir, ".env.app.immich___postgres")
	if err := os.WriteFile(serviceFile, []byte("POSTGRES_DB='immich'\n"), 0644); err != nil {
		t.Fatal(err)
	}

	conf := config.AppConfig{ComposeDir: tmpDir}
	apps, err := ListReferencedApps(context.Background(), conf)
	if err != nil {
		t.Fatalf("ListReferencedApps failed: %v", err)
	}

	if !slices.Contains(apps, "IMMICH") {
		t.Errorf("Expected IMMICH in referenced apps (via service-scoped file), got: %v", apps)
	}
	if slices.Contains(apps, "IMMICH___POSTGRES") {
		t.Errorf("IMMICH___POSTGRES should never appear as its own app entry, got: %v", apps)
	}
}

// TestListReferencedApps_OverrideServiceScopedFile is the same regression,
// but via the override-file scan (step 3) instead of the direct file scan
// (step 2) -- a separate code path with the same bug.
func TestListReferencedApps_OverrideServiceScopedFile(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	overrideContent := `services:
  immich-postgres:
    env_file:
      - .env.app.immich___postgres
`
	if err := os.WriteFile(filepath.Join(tmpDir, "docker-compose.override.yml"), []byte(overrideContent), 0644); err != nil {
		t.Fatal(err)
	}

	conf := config.AppConfig{ComposeDir: tmpDir}
	apps, err := ListReferencedApps(context.Background(), conf)
	if err != nil {
		t.Fatalf("ListReferencedApps failed: %v", err)
	}

	if !slices.Contains(apps, "IMMICH") {
		t.Errorf("Expected IMMICH in referenced apps (via override reference), got: %v", apps)
	}
}
