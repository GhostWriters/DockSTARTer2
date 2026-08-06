package appenv

import (
	"DockSTARTer2/internal/paths"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFormatLinesCore(t *testing.T) {
	// Set up temporary templates directory structure so IsAppBuiltIn returns true.
	tempDir := t.TempDir()
	origTemplates := paths.TemplatesDirOverride
	paths.TemplatesDirOverride = tempDir
	defer func() {
		paths.TemplatesDirOverride = origTemplates
	}()

	appsDir := filepath.Join(tempDir, ".apps")
	if err := os.Mkdir(appsDir, 0755); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(appsDir, "audiobookshelf")
	if err := os.Mkdir(appDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("Empty template with user-defined variables adds a blank line", func(t *testing.T) {
		currentLines := []string{"ALLOW_CORS=1"}
		defaultLines := []string{} // non-nil but empty
		envLines := []string{"AUDIOBOOKSHELF__ENABLED=true"}
		appName := "audiobookshelf"
		composeEnvFile := ""

		formatted := FormatLinesCore(ctx, currentLines, defaultLines, envLines, appName, composeEnvFile)

		expected := []string{
			"###",
			"### Audiobookshelf",
			"###",
			"### ! Missing description !",
			"###",
			"", // This is the expected blank line!
			"###",
			"### Audiobookshelf (User Defined Variables)",
			"###",
			"ALLOW_CORS=1",
		}

		if len(formatted) != len(expected) {
			t.Errorf("Expected %d lines, got %d. Lines: %v", len(expected), len(formatted), formatted)
			return
		}

		for i, line := range formatted {
			if line != expected[i] {
				t.Errorf("At index %d: expected %q, got %q", i, expected[i], line)
			}
		}
	})

	t.Run("Non-existent template does not add extra blank line before user-defined variables", func(t *testing.T) {
		currentLines := []string{"ALLOW_CORS=1"}
		var defaultLines []string = nil // nil template
		envLines := []string{"AUDIOBOOKSHELF__ENABLED=true"}
		appName := "audiobookshelf"
		composeEnvFile := ""

		formatted := FormatLinesCore(ctx, currentLines, defaultLines, envLines, appName, composeEnvFile)

		expected := []string{
			"###",
			"### Audiobookshelf",
			"###",
			"### ! Missing description !",
			"###",
			"###",
			"### Audiobookshelf (User Defined Variables)",
			"###",
			"ALLOW_CORS=1",
		}

		if len(formatted) != len(expected) {
			t.Errorf("Expected %d lines, got %d. Lines: %v", len(expected), len(formatted), formatted)
			return
		}

		for i, line := range formatted {
			if line != expected[i] {
				t.Errorf("At index %d: expected %q, got %q", i, expected[i], line)
			}
		}
	})
}
