package appenv

import (
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGetDeprecatedFromLabels(t *testing.T) {
	ctx := context.Background()

	// 1. Setup mock environment
	tempDir, err := os.MkdirTemp("", "ds_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Use paths overrides
	paths.StateHomeOverride = filepath.Join(tempDir, "state")
	paths.TemplatesDirOverride = filepath.Join(tempDir, "templates")
	defer func() {
		paths.StateHomeOverride = ""
		paths.TemplatesDirOverride = ""
	}()

	// The app name we'll test
	appName := "TESTAPP"
	baseApp := "testapp"

	// Create template directory structure
	templatesDir := paths.TemplatesDirOverride
	appTemplateDir := filepath.Join(templatesDir, constants.TemplatesDirName, baseApp)
	if err := os.MkdirAll(appTemplateDir, 0755); err != nil {
		t.Fatal(err)
	}

	labelsContent := `
services:
  testapp<__instance>:
    labels:
      com.dockstarter.appinfo.deprecated: "true"
      com.dockstarter.appinfo.description: "This is a deprecated test app"
      com.dockstarter.appinfo.nicename: "Test App"
`
	labelsTemplateFile := filepath.Join(appTemplateDir, baseApp+".labels.yml")
	if err := os.WriteFile(labelsTemplateFile, []byte(labelsContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Get the instance file path through AppInstanceFile to verify it's created
	instFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil {
		t.Fatalf("AppInstanceFile failed: %v", err)
	}
	content, _ := os.ReadFile(instFile)
	t.Logf("Instance file content:\n%s", string(content))

	// 2.5 Setup mock .env file (required for IsAppUserDefined check)
	envFile := filepath.Join(tempDir, constants.EnvFileName)
	if err := os.WriteFile(envFile, []byte("TESTAPP__ENABLED=true\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 2. Test IsAppDeprecated
	deprecated := IsAppDeprecated(ctx, appName)
	if !deprecated {
		t.Error("Expected app to be deprecated, but it wasn't")
	}

	// 3. Test GetDescription
	desc := GetDescription(ctx, appName, envFile)
	if desc != "This is a deprecated test app" {
		t.Errorf("Expected description 'This is a deprecated test app', got '%s'", desc)
	}

	// 4. Test GetNiceName
	nice := GetNiceName(ctx, appName)
	if nice != "Test App" {
		t.Errorf("Expected nicename 'Test App', got '%s'", nice)
	}

	// 5. Test with instance
	appNameInstanced := "TESTAPP__4K"
	deprecatedInst := IsAppDeprecated(ctx, appNameInstanced)
	if !deprecatedInst {
		t.Error("Expected instanced app to be deprecated, but it wasn't")
	}

	// 6. Test GetNiceName (instanced)
	niceInst := GetNiceName(ctx, appNameInstanced)
	if niceInst != "Test App" {
		t.Errorf("Expected instanced nicename 'Test App', got '%s'", niceInst)
	}
}
