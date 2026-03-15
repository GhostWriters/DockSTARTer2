package cmd

import (
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/version"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// isThemeFilePath returns true if the argument looks like a file path rather than a theme name.
func isThemeFilePath(arg string) bool {
	return strings.HasSuffix(arg, ".ds2theme") || strings.ContainsAny(arg, "/\\")
}

func handleTheme(ctx context.Context, group *CommandGroup) error {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadAppConfig()
		if len(group.Args) == 0 {
			// No args — show current theme
			logger.Notice(ctx, "Current theme is: {{|Theme|}}%s{{[-]}}", theme.ThemeDisplayName(conf.UI.Theme))
			logger.Notice(ctx, "Run '{{|UserCommand|}}%s --theme-list{{[-]}}' to see available themes.", version.CommandName)
			return nil
		}

		arg := group.Args[0]

		if isThemeFilePath(arg) {
			// --- File import ---
			absPath, err := filepath.Abs(arg)
			if err != nil {
				logger.Error(ctx, "Invalid path: %v", err)
				return err
			}
			src, err := os.ReadFile(absPath)
			if err != nil {
				logger.Error(ctx, "Cannot read theme file '{{|Folder|}}%s{{[-]}}': %v", absPath, err)
				return err
			}
			themeName := strings.TrimSuffix(filepath.Base(absPath), ".ds2theme")
			destDir := paths.GetThemesDir()
			if err := os.MkdirAll(destDir, 0755); err != nil {
				logger.Error(ctx, "Failed to create themes directory: %v", err)
				return err
			}
			destPath := filepath.Join(destDir, theme.UserThemeFilename(themeName))
			if err := os.WriteFile(destPath, src, 0644); err != nil {
				logger.Error(ctx, "Failed to write theme file: %v", err)
				return err
			}
			configValue := "user://" + themeName
			conf.UI.Theme = configValue
			if err := config.SaveAppConfig(conf); err != nil {
				logger.Error(ctx, "Failed to save theme setting: %v", err)
				return err
			}
			logger.Notice(ctx, "Theme '{{|Theme|}}%s{{[-]}}' imported and set as active.", themeName)
			_, _ = theme.Load(configValue, "")
			return nil
		}

		// --- Named theme (embedded or user://) ---
		newTheme := arg
		if _, err := theme.EnsureThemeExtracted(newTheme); err != nil {
			logger.Error(ctx, "Theme '{{|Theme|}}%s{{[-]}}' not found.", theme.ThemeDisplayName(newTheme))
			return err
		}
		conf.UI.Theme = newTheme
		// Apply theme defaults if any
		if tf, err := theme.GetThemeFile(newTheme); err == nil && tf.Defaults != nil {
			changes := theme.ApplyThemeDefaults(&conf, *tf.Defaults)
			if len(changes) > 0 {
				var lines []string
				for k, v := range changes {
					status := v
					if v == "true" {
						status = "{{|Var|}}ON{{[-]}}"
					} else if v == "false" {
						status = "{{|Var|}}OFF{{[-]}}"
					} else {
						status = fmt.Sprintf("{{|Var|}}%s{{[-]}}", v)
					}
					lines = append(lines, fmt.Sprintf("\t- %s: %s", k, status))
				}
				logger.Notice(ctx, "Applying settings from theme file:\n%s", strings.Join(lines, "\n"))
			}
		}
		if err := config.SaveAppConfig(conf); err != nil {
			logger.Error(ctx, "Failed to save theme setting: %v", err)
			return err
		}
		logger.Notice(ctx, "Theme updated to: {{|Theme|}}%s{{[-]}}", theme.ThemeDisplayName(newTheme))
		_, _ = theme.Load(newTheme, "")

	case "--theme-list":
		themes, err := theme.List()
		if err != nil || len(themes) == 0 {
			logger.Warn(ctx, "No themes found.")
			return nil
		}
		logger.Notice(ctx, "Available themes:")
		for _, t := range themes {
			marker := ""
			if t.IsUserTheme {
				marker = " [user]"
			}
			logger.Notice(ctx, "  - %s%s", t.Name, marker)
		}
	}
	return nil
}

func parseTitleAlign(ctx context.Context, arg, label string) (string, error) {
	switch strings.ToLower(arg) {
	case "left", "center":
		return strings.ToLower(arg), nil
	}
	logger.Error(ctx, "Invalid %s alignment: %s (use left or center)", label, arg)
	return "", fmt.Errorf("invalid %s alignment", label)
}

func handleThemeSettings(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	switch group.Command {
	case "--theme-lines", "--theme-line":
		conf.UI.LineCharacters = true
	case "--theme-no-lines", "--theme-no-line":
		conf.UI.LineCharacters = false
	case "--theme-borders", "--theme-border":
		conf.UI.Borders = true
	case "--theme-no-borders", "--theme-no-border":
		conf.UI.Borders = false
	case "--theme-button-borders":
		conf.UI.ButtonBorders = true
	case "--theme-no-button-borders":
		conf.UI.ButtonBorders = false
	case "--theme-shadows", "--theme-shadow":
		conf.UI.Shadow = true
	case "--theme-no-shadows", "--theme-no-shadow":
		conf.UI.Shadow = false
	case "--theme-shadow-level":
		// Set shadow level (0-4 or aliases)
		if len(group.Args) > 0 {
			arg := strings.ToLower(group.Args[0])
			switch arg {
			case "0", "off", "none", "false", "no":
				conf.UI.ShadowLevel = 0
				conf.UI.Shadow = false
			case "1", "light":
				conf.UI.ShadowLevel = 1
				conf.UI.Shadow = true
			case "2", "medium":
				conf.UI.ShadowLevel = 2
				conf.UI.Shadow = true
			case "3", "dark":
				conf.UI.ShadowLevel = 3
				conf.UI.Shadow = true
			case "4", "solid", "full":
				conf.UI.ShadowLevel = 4
				conf.UI.Shadow = true
			default:
				// specialized handling for percentage strings
				if strings.HasSuffix(arg, "%") {
					var percent int
					if _, err := fmt.Sscanf(arg, "%d%%", &percent); err == nil {
						if percent <= 12 {
							conf.UI.ShadowLevel = 0
							conf.UI.Shadow = false
						} else if percent <= 37 {
							conf.UI.ShadowLevel = 1
							conf.UI.Shadow = true
						} else if percent <= 62 {
							conf.UI.ShadowLevel = 2
							conf.UI.Shadow = true
						} else if percent <= 87 {
							conf.UI.ShadowLevel = 3
							conf.UI.Shadow = true
						} else {
							conf.UI.ShadowLevel = 4
							conf.UI.Shadow = true
						}
						break
					}
				}
				logger.Error(ctx, "Invalid shadow level: %s (use 0-4, or: off, light, medium, dark, solid, or percentage e.g. 50%%)", arg)
				return fmt.Errorf("invalid shadow level")
			}
		} else {
			logger.Display(ctx, "Current shadow level: %d", conf.UI.ShadowLevel)
			return nil
		}
	case "--theme-scrollbar":
		conf.UI.Scrollbar = true
	case "--theme-no-scrollbar":
		conf.UI.Scrollbar = false
	case "--theme-border-color":
		if len(group.Args) > 0 {
			switch group.Args[0] {
			case "1":
				conf.UI.BorderColor = 1
			case "2":
				conf.UI.BorderColor = 2
			case "3":
				conf.UI.BorderColor = 3
			default:
				logger.Error(ctx, "Invalid border color: %s (use 1, 2, or 3)", group.Args[0])
				return fmt.Errorf("invalid border color")
			}
		} else {
			logger.Display(ctx, "Current border color setting: %d", conf.UI.BorderColor)
			return nil
		}
	case "--theme-dialog-title":
		if len(group.Args) > 0 {
			v, err := parseTitleAlign(ctx, group.Args[0], "dialog title")
			if err != nil {
				return err
			}
			conf.UI.DialogTitleAlign = v
		} else {
			logger.Display(ctx, "Current dialog title alignment: %s", conf.UI.DialogTitleAlign)
			return nil
		}
	case "--theme-submenu-title":
		if len(group.Args) > 0 {
			v, err := parseTitleAlign(ctx, group.Args[0], "submenu title")
			if err != nil {
				return err
			}
			conf.UI.SubmenuTitleAlign = v
		} else {
			logger.Display(ctx, "Current submenu title alignment: %s", conf.UI.SubmenuTitleAlign)
			return nil
		}
	case "--theme-log-title":
		if len(group.Args) > 0 {
			v, err := parseTitleAlign(ctx, group.Args[0], "log title")
			if err != nil {
				return err
			}
			conf.UI.LogTitleAlign = v
		} else {
			logger.Display(ctx, "Current log title alignment: %s", conf.UI.LogTitleAlign)
			return nil
		}
	}

	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save theme setting: %v", err)
		return err
	}

	// Log specific update if appropriate
	if group.Command == "--theme-border-color" && len(group.Args) > 0 {
		logger.Notice(ctx, "Border color set to: {{|Var|}}%s{{[-]}}", group.Args[0])
	}
	// Specialized output for shadow level
	if group.Command == "--theme-shadow-level" && len(group.Args) > 0 {
		var percent int
		switch conf.UI.ShadowLevel {
		case 0:
			percent = 0
		case 1:
			percent = 25
		case 2:
			percent = 50
		case 3:
			percent = 75
		case 4:
			percent = 100
		}
		logger.Notice(ctx, "Shadow level set to %d%%.", percent)
		logger.Notice(ctx, "Theme setting updated: %s", group.Command)
	}

	return nil
}

func handleThemeTable(ctx context.Context) error {
	headers := []string{"Theme", "Description", "Author"}
	themes, err := theme.List()
	if err != nil {
		logger.Error(ctx, "Failed to list themes: %v", err)
		return err
	}

	var data []string
	for _, t := range themes {
		data = append(data, t.Name, t.Description, t.Author)
	}

	// Default theme info (hardcoded for now as it's built-in)
	// Bash implementation iterates folders, so it only shows what's in the folder.
	// If we want to show 'default' or 'classic' if it's not a file, we should add it?
	// But theme.go defines Default() which is hardcoded.
	// We should probably include it if it's not in the list.
	// Bash: lists folders. If "Default" is a folder, it shows it.

	console.PrintTable(headers, data, true)
	return nil
}
