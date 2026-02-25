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

func handleTheme(ctx context.Context, group *CommandGroup) error {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadAppConfig()
		if len(group.Args) > 0 {
			newTheme := group.Args[0]
			// Validate theme existence
			themesDir := paths.GetThemesDir()
			themePath := filepath.Join(themesDir, newTheme+".ds2theme")
			if _, err := os.Stat(themePath); os.IsNotExist(err) {
				logger.Error(ctx, "Theme '{{|Theme|}}%s{{[-]}}' not found in '{{|Folder|}}%s{{[-]}}'.", newTheme, themesDir)
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
			} else {
				logger.Notice(ctx, "Theme updated to: {{|Theme|}}%s{{[-]}}", newTheme)
				// Reload theme for subsequent commands in the same execution
				_, _ = theme.Load(newTheme, "")
			}
		} else {
			// No args? Show current theme
			logger.Notice(ctx, "Current theme is: {{|Theme|}}%s{{[-]}}", conf.UI.Theme)
			logger.Notice(ctx, "Run '{{|UserCommand|}}%s --theme-list{{[-]}}' to see available themes.", version.CommandName)
		}
	case "--theme-list":
		themesDir := paths.GetThemesDir()
		entries, err := os.ReadDir(themesDir)
		if err != nil {
			logger.Error(ctx, "Failed to read themes directory: %v", err)
			return err
		}

		var themes []string
		for _, entry := range entries {
			if entry.IsDir() {
				// Basic check: does it have a theme.ini or .dialogrc?
				themePath := filepath.Join(themesDir, entry.Name())
				if _, err := os.Stat(filepath.Join(themePath, "theme.ini")); err == nil {
					themes = append(themes, entry.Name())
				} else if _, err := os.Stat(filepath.Join(themePath, ".dialogrc")); err == nil {
					themes = append(themes, entry.Name())
				}
			}
		}

		if len(themes) == 0 {
			logger.Warn(ctx, "No themes found in '{{|Folder|}}%s{{[-]}}'.", themesDir)
		} else {
			logger.Notice(ctx, "Available themes in '{{|Folder|}}%s{{[-]}}':", themesDir)
			for _, t := range themes {
				logger.Notice(ctx, "  - %s", t)
			}
		}
	}
	return nil
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
