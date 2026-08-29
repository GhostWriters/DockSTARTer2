package appenv

import (
	"DockSTARTer2/internal/paths"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
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

		formatted := FormatLinesCore(ctx, currentLines, defaultLines, envLines, appName, composeEnvFile, "", time.Time{}, ".env")

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

		formatted := FormatLinesCore(ctx, currentLines, defaultLines, envLines, appName, composeEnvFile, "", time.Time{}, ".env")

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

	t.Run("fileLabel adds a standalone filename line above an unchanged heading block", func(t *testing.T) {
		currentLines := []string{"ALLOW_CORS=1"}
		var defaultLines []string = nil
		envLines := []string{"AUDIOBOOKSHELF__ENABLED=true"}
		appName := "audiobookshelf"
		composeEnvFile := ""

		formatted := FormatLinesCore(ctx, currentLines, defaultLines, envLines, appName, composeEnvFile, ".env.app.audiobookshelf-database", time.Time{}, ".env")

		expected := []string{
			fileHeaderLines(".env.app.audiobookshelf-database", time.Time{})[0],
			"",
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

	t.Run("fileLabel applies to the global .env case too, with no appName", func(t *testing.T) {
		currentLines := []string{"ALLOW_CORS=1"}
		var defaultLines []string = nil
		var envLines []string = nil

		formatted := FormatLinesCore(ctx, currentLines, defaultLines, envLines, "", "", "/home/user/.config/compose/.env", time.Time{}, "")

		expected := []string{
			fileHeaderLines("/home/user/.config/compose/.env", time.Time{})[0],
			"",
			"###",
			"### Global Variables (User Defined)",
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

func TestFormatLinesCore_TemplateTimestamp(t *testing.T) {
	repoDir := t.TempDir()
	r, err := git.PlainInit(repoDir, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	appDir := filepath.Join(repoDir, ".apps", "audiobookshelf")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(appDir, ".env")
	if err := os.WriteFile(envFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(filepath.Join(".apps", "audiobookshelf", ".env")); err != nil {
		t.Fatal(err)
	}
	commitTime := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	sig := &object.Signature{Name: "Test", Email: "test@example.com", When: commitTime}
	if _, err := wt.Commit("add audiobookshelf template", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatal(err)
	}

	origTemplates := paths.TemplatesDirOverride
	paths.TemplatesDirOverride = repoDir
	origState := paths.StateHomeOverride
	paths.StateHomeOverride = t.TempDir()
	defer func() {
		paths.TemplatesDirOverride = origTemplates
		paths.StateHomeOverride = origState
	}()

	ctx := context.Background()
	envLines := []string{"AUDIOBOOKSHELF__ENABLED=true"}
	formatted := FormatLinesCore(ctx, nil, nil, envLines, "audiobookshelf", "", "", time.Time{}, ".env")

	templateLine, varsLine := findTimestampLines(formatted)
	if templateLine == "" || varsLine == "" {
		t.Fatalf("expected both a 'Template updated:' and 'Vars updated:' line, got: %v", formatted)
	}
	wantDate := commitTime.Local().Format("2006-01-02 15:04:05")
	wantLines := formatAlignedHeadingLines([][2]string{{"Template updated:", wantDate}, {"Vars updated:", wantDate}})
	if templateLine != wantLines[0] {
		t.Errorf("template line = %q, want %q", templateLine, wantLines[0])
	}
	if varsLine != wantLines[1] {
		t.Errorf("vars line = %q, want %q", varsLine, wantLines[1])
	}
}

// findTimestampLines locates the "Template updated:"/"Vars updated:" lines
// (if present) among formatted output lines.
func findTimestampLines(formatted []string) (templateLine, varsLine string) {
	for _, line := range formatted {
		if strings.HasPrefix(line, "### Template updated:") {
			templateLine = line
		} else if strings.HasPrefix(line, "### Vars updated:") {
			varsLine = line
		}
	}
	return
}

func TestFormatLinesCore_TemplateTimestamp_UserOverride(t *testing.T) {
	// A user app template override has no git history -- GetAppTemplateTimestamps
	// (git-based) must not be used for it; userTemplateTimestamps' mtime-based
	// fallback should be used instead.
	configDir := t.TempDir()
	origConfig := paths.ConfigHomeOverride
	paths.ConfigHomeOverride = configDir
	defer func() { paths.ConfigHomeOverride = origConfig }()

	appDir := filepath.Join(paths.GetUserAppsDir(), "testapp")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	ymlFile := filepath.Join(appDir, "testapp.yml")
	if err := os.WriteFile(ymlFile, []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(appDir, ".env")
	if err := os.WriteFile(envFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	labelsFile := filepath.Join(appDir, "testapp.labels.yml")
	if err := os.WriteFile(labelsFile, []byte("services: {}"), 0644); err != nil {
		t.Fatal(err)
	}

	// .yml oldest, .env in the middle, .labels.yml newest -- so "template
	// updated" (max over everything) and "vars updated" (max over just
	// .env/.env.app.*) should land on two different files/dates.
	ymlTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	envTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	labelsTime := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(ymlFile, ymlTime, ymlTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(envFile, envTime, envTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(labelsFile, labelsTime, labelsTime); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	envLines := []string{"TESTAPP__ENABLED=true"}
	formatted := FormatLinesCore(ctx, nil, nil, envLines, "testapp", "", "", time.Time{}, ".env")

	templateLine, varsLine := findTimestampLines(formatted)
	if templateLine == "" || varsLine == "" {
		t.Fatalf("expected both a 'Template updated:' and 'Vars updated:' line, got: %v", formatted)
	}
	wantLines := formatAlignedHeadingLines([][2]string{
		{"Template updated:", labelsTime.Local().Format("2006-01-02 15:04:05")},
		{"Vars updated:", envTime.Local().Format("2006-01-02 15:04:05")},
	})
	if templateLine != wantLines[0] {
		t.Errorf("template line = %q, want %q", templateLine, wantLines[0])
	}
	if varsLine != wantLines[1] {
		t.Errorf("vars line = %q, want %q", varsLine, wantLines[1])
	}
}
