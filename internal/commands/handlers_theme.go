package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/version"
)

func isThemeFilePath(arg string) bool {
	if strings.HasPrefix(arg, "user:") {
		return false
	}
	return strings.HasPrefix(arg, "file:") || strings.HasSuffix(arg, ".ds2theme") || strings.ContainsAny(arg, "/\\")
}

func themeFilePath(arg string) string {
	return strings.TrimPrefix(arg, "file:")
}

func handleTheme(ctx context.Context, group *CommandGroup) error {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadAppConfig()
		if len(group.Args) == 0 {
			logger.Notice(ctx, "Current theme is: {{|Theme|}}%s{{[-]}}", theme.ThemeDisplayName(conf.UI.Theme))
			logger.Notice(ctx, "Run '{{|UserCommand|}}%s --theme-list{{[-]}}' to see available themes.", version.CommandName)
			return nil
		}

		arg := group.Args[0]

		if isThemeFilePath(arg) {
			absPath, err := filepath.Abs(themeFilePath(arg))
			if err != nil {
				logger.Error(ctx, "Invalid path: %v", err)
				return err
			}
			if _, err := os.Stat(absPath); err != nil {
				logger.Error(ctx, "Theme file not found: '{{|Folder|}}%s{{[-]}}'", absPath)
				return err
			}
			configValue := "file:" + absPath
			conf.UI.Theme = configValue
			if err := config.SaveAppConfig(conf); err != nil {
				logger.Error(ctx, "Failed to save theme setting: %v", err)
				return err
			}
			logger.Notice(ctx, "Theme set to file: {{|Folder|}}%s{{[-]}}", absPath)
			_, _ = theme.Load(configValue, "")
			return nil
		}

		newTheme := arg
		if _, err := theme.EnsureThemeExtracted(newTheme); err != nil {
			logger.Error(ctx, "Theme '{{|Theme|}}%s{{[-]}}' not found.", theme.ThemeDisplayName(newTheme))
			return err
		}
		conf.UI.Theme = newTheme
		if tf, err := theme.GetThemeFile(newTheme); err == nil && tf.Defaults != nil {
			changes := theme.ApplyThemeDefaults(&conf, *tf.Defaults)
			if len(changes) > 0 {
				var lines []string
				for k, v := range changes {
					var status string
					switch v {
					case "true":
						status = "{{|Var|}}ON{{[-]}}"
					case "false":
						status = "{{|Var|}}OFF{{[-]}}"
					default:
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
		if err != nil {
			return err
		}
		for _, t := range themes {
			logger.Display(ctx, t.ConfigValue)
		}
		return nil
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

	if group.Command == "--theme-border-color" && len(group.Args) > 0 {
		logger.Notice(ctx, "Border color set to: {{|Var|}}%s{{[-]}}", group.Args[0])
	}
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

func resolveExtractDest(arg string) string {
	if arg == "user:" {
		return paths.GetThemesDir()
	}
	if arg == "" {
		return "."
	}
	return arg
}

func handleThemeExtract(ctx context.Context, group *CommandGroup) error {
	switch group.Command {
	case "--theme-extract":
		if len(group.Args) == 0 {
			logger.Error(ctx, "Usage: --theme-extract <ThemeName> [dest] [filename]")
			return fmt.Errorf("missing theme name")
		}
		themeName := group.Args[0]
		destDir := resolveExtractDest("")
		if len(group.Args) >= 2 {
			destDir = resolveExtractDest(group.Args[1])
		}

		data, err := theme.ResolveThemeData(themeName)
		if err != nil {
			logger.Error(ctx, "Theme '{{|Theme|}}%s{{[-]}}' not found: %v", theme.ThemeDisplayName(themeName), err)
			return err
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			logger.Error(ctx, "Failed to create directory '{{|Folder|}}%s{{[-]}}': %v", destDir, err)
			return err
		}

		outName := theme.FileStemFromURI(themeName) + ".ds2theme"
		if len(group.Args) >= 3 {
			outName = group.Args[2]
			if !strings.HasSuffix(outName, ".ds2theme") {
				outName += ".ds2theme"
			}
		}
		outPath := filepath.Join(destDir, outName)
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			logger.Error(ctx, "Failed to write theme file: %v", err)
			return err
		}
		logger.Notice(ctx, "Theme '{{|Theme|}}%s{{[-]}}' extracted to: {{|Folder|}}%s{{[-]}}", theme.ThemeDisplayName(themeName), outPath)

	case "--theme-extract-all":
		destDir := resolveExtractDest("")
		if len(group.Args) >= 1 {
			destDir = resolveExtractDest(group.Args[0])
		}

		if theme.EmbeddedThemeLister == nil || theme.EmbeddedThemeReader == nil {
			logger.Error(ctx, "Embedded theme reader not initialised.")
			return fmt.Errorf("embedded theme reader not initialised")
		}

		stems, err := theme.EmbeddedThemeLister()
		if err != nil || len(stems) == 0 {
			logger.Warn(ctx, "No embedded themes found.")
			return nil
		}

		if err := os.MkdirAll(destDir, 0755); err != nil {
			logger.Error(ctx, "Failed to create directory '{{|Folder|}}%s{{[-]}}': %v", destDir, err)
			return err
		}

		extracted := 0
		for _, stem := range stems {
			data, err := theme.EmbeddedThemeReader(stem)
			if err != nil {
				logger.Warn(ctx, "Skipping '{{|Theme|}}%s{{[-]}}': %v", stem, err)
				continue
			}
			outPath := filepath.Join(destDir, stem+".ds2theme")
			if err := os.WriteFile(outPath, data, 0644); err != nil {
				logger.Warn(ctx, "Failed to write '{{|Folder|}}%s{{[-]}}': %v", outPath, err)
				continue
			}
			logger.Notice(ctx, "  Extracted: {{|Theme|}}%s{{[-]}}", stem+".ds2theme")
			extracted++
		}
		logger.Notice(ctx, "%d theme(s) extracted to: {{|Folder|}}%s{{[-]}}", extracted, destDir)
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
		data = append(data, t.ConfigValue, t.Description, t.Author)
	}

	console.PrintTableCtx(ctx, headers, data, true)
	return nil
}
