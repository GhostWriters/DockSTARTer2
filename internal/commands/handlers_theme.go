package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

// splitThemeValueArgs separates a --theme/--theme-* command's optional value
// from its optional connection-type list (see parseConnTypeList). A lone
// argument is a type list if it parses as one, otherwise the value. An
// omitted type list means every connection type.
func splitThemeValueArgs(args []string) (value string, connTypes []string, err error) {
	switch len(args) {
	case 0:
		return "", config.ConnTypes, nil
	case 1:
		if types, typeErr := parseConnTypeList(args[0]); typeErr == nil {
			return "", types, nil
		}
		return args[0], config.ConnTypes, nil
	default:
		types, typeErr := parseConnTypeList(args[1])
		if typeErr != nil {
			return "", nil, typeErr
		}
		return args[0], types, nil
	}
}

// reloadLocalTheme re-registers the unprefixed theme when connTypes includes
// "local", whose theme it is.
func reloadLocalTheme(conf config.AppConfig, connTypes []string) {
	if slices.Contains(connTypes, "local") {
		_, _ = theme.Load(conf.Appearance.Local.Theme, "")
	}
}

func HandleTheme(ctx context.Context, group *CommandGroup) error {
	switch group.Command {
	case "-T", "--theme":
		conf := config.LoadAppConfig()
		arg, connTypes, err := splitThemeValueArgs(group.Args)
		if err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		if arg == "" {
			for _, ct := range connTypes {
				logger.Notice(ctx, "Current %s theme is: {{|Theme|}}%s{{[-]}}", config.ConnTypeLabel(ct), theme.ThemeDisplayName(conf.Appearance.ForConnType(ct).Theme))
			}
			logger.Notice(ctx, "Run '{{|UserCommand|}}%s --theme-list{{[-]}}' to see available themes.", version.CommandName)
			return nil
		}

		if isThemeFilePath(arg) {
			absPath, err := filepath.Abs(themeFilePath(arg))
			if err != nil {
				logger.Error(ctx, "Invalid path: %v", err)
				return err
			}
			if _, err := os.Stat(absPath); err != nil {
				logger.Error(ctx, "Theme file not found: '"+console.FormatFolderPath(absPath)+"'")
				return err
			}
			configValue := "file:" + absPath
			for _, ct := range connTypes {
				conf.Appearance.Ptr(ct).Theme = configValue
			}
			if err := config.SaveAppConfig(conf); err != nil {
				logger.Error(ctx, "Failed to save theme setting: %v", err)
				return err
			}
			logger.Notice(ctx, "Theme for %s set to file: "+console.FormatFolderPath(absPath), config.ConnTypeLabels(connTypes))
			reloadLocalTheme(conf, connTypes)
			return nil
		}

		newTheme := arg
		if strings.HasPrefix(newTheme, "user:") {
			// Normalize away a user-typed ".ds2theme" suffix -- ConfigValue
			// never includes it (see theme.List), so an unnormalized value
			// here would never match its own list entry, breaking the
			// "keep a dot-prefixed active theme visible" exception in
			// theme.List for any dot-file set with its extension included.
			newTheme = "user:" + theme.FileStemFromURI(newTheme)
		}
		if _, err := theme.EnsureThemeExtracted(newTheme); err != nil {
			logger.Error(ctx, "Theme '{{|Theme|}}%s{{[-]}}' not found.", theme.ThemeDisplayName(newTheme))
			return err
		}
		var defaults *theme.ThemeDefaults
		if tf, err := theme.GetThemeFile(newTheme); err == nil {
			if d, derr := theme.FileDefaults(tf); derr == nil {
				defaults = d
			}
		}
		var changes map[string]string
		for _, ct := range connTypes {
			a := conf.Appearance.Ptr(ct)
			a.Theme = newTheme
			if defaults != nil {
				changes = theme.ApplyThemeDefaults(a, *defaults)
			}
		}
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
			slices.Sort(lines)
			logger.Notice(ctx, "Applying settings from theme file:\n%s", strings.Join(lines, "\n"))
		}
		if err := config.SaveAppConfig(conf); err != nil {
			logger.Error(ctx, "Failed to save theme setting: %v", err)
			return err
		}
		logger.Notice(ctx, "Theme for %s updated to: {{|Theme|}}%s{{[-]}}", config.ConnTypeLabels(connTypes), theme.ThemeDisplayName(newTheme))
		reloadLocalTheme(conf, connTypes)

	case "--theme-list":
		themes, err := theme.List(config.LoadAppConfig().Appearance.Local.Theme)
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

func parseBracketMode(ctx context.Context, arg, label string) (string, error) {
	switch strings.ToLower(arg) {
	case "never", "selected", "always":
		return strings.ToLower(arg), nil
	}
	logger.Error(ctx, "Invalid %s brackets mode: %s (use never, selected, or always)", label, arg)
	return "", fmt.Errorf("invalid %s brackets mode", label)
}

func parseTabLayout(ctx context.Context, arg string) (string, error) {
	switch strings.ToLower(arg) {
	case "maximized", "sidebyside", "stacked":
		return strings.ToLower(arg), nil
	}
	logger.Error(ctx, "Invalid tab layout: %s (use maximized, sidebyside, or stacked)", arg)
	return "", fmt.Errorf("invalid tab layout")
}

func parseMarkdownHyperlinks(ctx context.Context, arg string) (string, error) {
	switch strings.ToLower(arg) {
	case "off", "inline", "auto":
		return strings.ToLower(arg), nil
	}
	logger.Error(ctx, "Invalid markdown hyperlinks mode: %s (use off, inline, or auto)", arg)
	return "", fmt.Errorf("invalid markdown hyperlinks mode")
}

func parseHyperlinks(ctx context.Context, arg string) (string, error) {
	switch strings.ToLower(arg) {
	case "off", "inline", "auto":
		return strings.ToLower(arg), nil
	}
	logger.Error(ctx, "Invalid hyperlinks mode: %s (use off, inline, or auto)", arg)
	return "", fmt.Errorf("invalid hyperlinks mode")
}

// themeToggle is one on/off --theme-* command: which Appearance field it
// sets, and to what.
type themeToggle struct {
	set func(a *config.Appearance, v bool)
	on  bool
}

var (
	setLineCharacters = func(a *config.Appearance, v bool) { a.LineCharacters = v }
	setBorders        = func(a *config.Appearance, v bool) { a.Borders = v }
	setLargeButtons   = func(a *config.Appearance, v bool) { a.LargeButtons = v }
	setLargeTitleBars = func(a *config.Appearance, v bool) { a.LargeTitleBars = v }
	setShadow         = func(a *config.Appearance, v bool) { a.Shadow = v }
	setScrollbar      = func(a *config.Appearance, v bool) { a.Scrollbar = v }
	setSpinner        = func(a *config.Appearance, v bool) { a.Spinner = v }
	setMenuBrackets   = func(a *config.Appearance, v bool) { a.MenuBrackets = v }
)

// themeToggles holds every per-connection-type on/off --theme-* command.
var themeToggles = map[string]themeToggle{
	"--theme-lines":              {setLineCharacters, true},
	"--theme-line":               {setLineCharacters, true},
	"--theme-no-lines":           {setLineCharacters, false},
	"--theme-no-line":            {setLineCharacters, false},
	"--theme-borders":            {setBorders, true},
	"--theme-border":             {setBorders, true},
	"--theme-no-borders":         {setBorders, false},
	"--theme-no-border":          {setBorders, false},
	"--theme-large-buttons":      {setLargeButtons, true},
	"--theme-no-large-buttons":   {setLargeButtons, false},
	"--theme-large-titlebars":    {setLargeTitleBars, true},
	"--theme-no-large-titlebars": {setLargeTitleBars, false},
	"--theme-shadows":            {setShadow, true},
	"--theme-shadow":             {setShadow, true},
	"--theme-no-shadows":         {setShadow, false},
	"--theme-no-shadow":          {setShadow, false},
	"--theme-scrollbar":          {setScrollbar, true},
	"--theme-scrollbars":         {setScrollbar, true},
	"--theme-no-scrollbar":       {setScrollbar, false},
	"--theme-no-scrollbars":      {setScrollbar, false},
	"--theme-spinner":            {setSpinner, true},
	"--theme-spinners":           {setSpinner, true},
	"--theme-no-spinner":         {setSpinner, false},
	"--theme-no-spinners":        {setSpinner, false},
	"--theme-menu-brackets":      {setMenuBrackets, true},
	"--theme-no-menu-brackets":   {setMenuBrackets, false},
}

// themeValueSetting is one per-connection-type --theme-* command taking a
// value: how to show its current value, and how to parse and apply a new one.
type themeValueSetting struct {
	label string
	get   func(a config.Appearance) string
	parse func(ctx context.Context, arg string) (func(a *config.Appearance), error)
}

// stringSetting builds a themeValueSetting for a string Appearance field.
func stringSetting(label string, field func(a *config.Appearance) *string, parse func(ctx context.Context, arg string) (string, error)) themeValueSetting {
	return themeValueSetting{
		label: label,
		get:   func(a config.Appearance) string { return *field(&a) },
		parse: func(ctx context.Context, arg string) (func(a *config.Appearance), error) {
			v, err := parse(ctx, arg)
			if err != nil {
				return nil, err
			}
			return func(a *config.Appearance) { *field(a) = v }, nil
		},
	}
}

// themeValueSettings holds every per-connection-type --theme-* command that
// takes a value.
var themeValueSettings = map[string]themeValueSetting{
	"--theme-shadow-level": {
		label: "shadow level",
		get:   func(a config.Appearance) string { return strconv.Itoa(a.ShadowLevel) },
		parse: func(ctx context.Context, arg string) (func(a *config.Appearance), error) {
			level, ok := parseShadowLevel(arg)
			if !ok {
				logger.Error(ctx, "Invalid shadow level: %s (use 0-4, or: off, light, medium, dark, solid, or percentage e.g. 50%%)", arg)
				return nil, fmt.Errorf("invalid shadow level")
			}
			return func(a *config.Appearance) {
				a.ShadowLevel = level
				a.Shadow = level > 0
			}, nil
		},
	},
	"--theme-border-color": {
		label: "border color setting",
		get:   func(a config.Appearance) string { return strconv.Itoa(a.BorderColor) },
		parse: func(ctx context.Context, arg string) (func(a *config.Appearance), error) {
			switch arg {
			case "1", "2", "3":
				n, _ := strconv.Atoi(arg)
				return func(a *config.Appearance) { a.BorderColor = n }, nil
			}
			logger.Error(ctx, "Invalid border color: %s (use 1, 2, or 3)", arg)
			return nil, fmt.Errorf("invalid border color")
		},
	},
	"--theme-dialog-title": stringSetting("dialog title alignment",
		func(a *config.Appearance) *string { return &a.DialogTitleAlign },
		func(ctx context.Context, arg string) (string, error) {
			return parseTitleAlign(ctx, arg, "dialog title")
		}),
	"--theme-submenu-title": stringSetting("submenu title alignment",
		func(a *config.Appearance) *string { return &a.SubmenuTitleAlign },
		func(ctx context.Context, arg string) (string, error) {
			return parseTitleAlign(ctx, arg, "submenu title")
		}),
	"--theme-panel-title": stringSetting("panel title alignment",
		func(a *config.Appearance) *string { return &a.PanelTitleAlign },
		func(ctx context.Context, arg string) (string, error) { return parseTitleAlign(ctx, arg, "log title") }),
	"--theme-checkbox-brackets": stringSetting("checkbox brackets mode",
		func(a *config.Appearance) *string { return &a.CheckboxBrackets },
		func(ctx context.Context, arg string) (string, error) { return parseBracketMode(ctx, arg, "checkbox") }),
	"--theme-radio-brackets": stringSetting("radio brackets mode",
		func(a *config.Appearance) *string { return &a.RadioBrackets },
		func(ctx context.Context, arg string) (string, error) { return parseBracketMode(ctx, arg, "radio") }),
	"--theme-tab-layout": stringSetting("tab layout",
		func(a *config.Appearance) *string { return &a.TabLayout },
		parseTabLayout),
}

// parseShadowLevel parses a --theme-shadow-level argument: 0-4, a level
// name, or a percentage.
func parseShadowLevel(arg string) (int, bool) {
	switch strings.ToLower(arg) {
	case "0", "off", "none", "false", "no":
		return 0, true
	case "1", "light":
		return 1, true
	case "2", "medium":
		return 2, true
	case "3", "dark":
		return 3, true
	case "4", "solid", "full":
		return 4, true
	}
	var percent int
	if strings.HasSuffix(arg, "%") {
		if _, err := fmt.Sscanf(arg, "%d%%", &percent); err == nil {
			switch {
			case percent <= 12:
				return 0, true
			case percent <= 37:
				return 1, true
			case percent <= 62:
				return 2, true
			case percent <= 87:
				return 3, true
			default:
				return 4, true
			}
		}
	}
	return 0, false
}

func HandleThemeSettings(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()

	if toggle, ok := themeToggles[group.Command]; ok {
		connTypes := config.ConnTypes
		if len(group.Args) > 0 {
			var err error
			if connTypes, err = parseConnTypeList(group.Args[0]); err != nil {
				logger.Error(ctx, "%v", err)
				return err
			}
		}
		for _, ct := range connTypes {
			toggle.set(conf.Appearance.Ptr(ct), toggle.on)
		}
		if err := config.SaveAppConfig(conf); err != nil {
			logger.Error(ctx, "Failed to save theme setting: %v", err)
			return err
		}
		if def, ok := Registry[group.Command]; ok && def.Title != "" {
			logger.Notice(ctx, "%s for %s.", strings.TrimSuffix(def.Title, "."), config.ConnTypeLabels(connTypes))
		}
		return nil
	}

	if setting, ok := themeValueSettings[group.Command]; ok {
		value, connTypes, err := splitThemeValueArgs(group.Args)
		if err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		if value == "" {
			for _, ct := range connTypes {
				logger.Display(ctx, "Current %s %s: %s", config.ConnTypeLabel(ct), setting.label, setting.get(conf.Appearance.ForConnType(ct)))
			}
			return nil
		}
		apply, err := setting.parse(ctx, value)
		if err != nil {
			return err
		}
		for _, ct := range connTypes {
			apply(conf.Appearance.Ptr(ct))
		}
		if err := config.SaveAppConfig(conf); err != nil {
			logger.Error(ctx, "Failed to save theme setting: %v", err)
			return err
		}
		logger.Notice(ctx, "%s%s for %s set to: {{|Var|}}%s{{[-]}}", strings.ToUpper(setting.label[:1]), setting.label[1:],
			config.ConnTypeLabels(connTypes), setting.get(conf.Appearance.ForConnType(connTypes[0])))
		return nil
	}

	switch group.Command {
	case "--theme-spinner-speed":
		if len(group.Args) == 0 {
			logger.Error(ctx, "Usage: --theme-spinner-speed <milliseconds>")
			return fmt.Errorf("missing argument")
		}
		ms, err := strconv.Atoi(strings.TrimSpace(group.Args[0]))
		if err != nil || ms < 50 || ms > 5000 {
			logger.Error(ctx, "Invalid spinner speed: %s (use 50-5000 ms)", group.Args[0])
			return fmt.Errorf("invalid spinner speed")
		}
		conf.Appearance.SpinnerSpeed = ms
	case "--theme-refresh-rate":
		if len(group.Args) == 0 {
			logger.Error(ctx, "Usage: --theme-refresh-rate <milliseconds>")
			return fmt.Errorf("missing argument")
		}
		ms, err := strconv.Atoi(strings.TrimSpace(group.Args[0]))
		if err != nil || ms < config.MinRefreshRateMS || ms > config.MaxRefreshRateMS {
			logger.Error(ctx, "Invalid refresh rate: %s (use %d-%d ms)", group.Args[0], config.MinRefreshRateMS, config.MaxRefreshRateMS)
			return fmt.Errorf("invalid refresh rate")
		}
		conf.Appearance.RefreshRate = ms
	case "--theme-show-preview":
		conf.Appearance.ShowPreview = true
	case "--theme-no-show-preview":
		conf.Appearance.ShowPreview = false
	case "--theme-markdown-hyperlinks":
		if len(group.Args) == 0 {
			logger.Display(ctx, "Current markdown hyperlinks mode: %s", conf.Appearance.MarkdownHyperlinks)
			return nil
		}
		v, err := parseMarkdownHyperlinks(ctx, group.Args[0])
		if err != nil {
			return err
		}
		conf.Appearance.MarkdownHyperlinks = v
	case "--theme-hyperlinks":
		if len(group.Args) == 0 {
			logger.Display(ctx, "Current hyperlinks mode: %s", conf.Appearance.Hyperlinks)
			return nil
		}
		v, err := parseHyperlinks(ctx, group.Args[0])
		if err != nil {
			return err
		}
		conf.Appearance.Hyperlinks = v
	}

	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save theme setting: %v", err)
		return err
	}

	switch group.Command {
	case "--theme-show-preview", "--theme-no-show-preview":
		if def, ok := Registry[group.Command]; ok && def.Title != "" {
			logger.Notice(ctx, "%s", def.Title)
		}
	case "--theme-spinner-speed":
		logger.Notice(ctx, "Spinner speed set to: {{|Var|}}%dms{{[-]}}", conf.Appearance.SpinnerSpeed)
	case "--theme-refresh-rate":
		logger.Notice(ctx, "Refresh rate set to: {{|Var|}}%dms{{[-]}}", conf.Appearance.RefreshRate)
	case "--theme-markdown-hyperlinks":
		logger.Notice(ctx, "Markdown hyperlinks mode set to: {{|Var|}}%s{{[-]}}", conf.Appearance.MarkdownHyperlinks)
	case "--theme-hyperlinks":
		logger.Notice(ctx, "Hyperlinks mode set to: {{|Var|}}%s{{[-]}}", conf.Appearance.Hyperlinks)
	}
	return nil
}

func resolveExtractDest(arg string) string {
	if arg == "" {
		return "."
	}
	if sub, ok := strings.CutPrefix(arg, "user:"); ok {
		return filepath.Join(paths.GetThemesDir(), sub)
	}
	return arg
}

func HandleThemeExtract(ctx context.Context, group *CommandGroup) error {
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

		outName := theme.FileStemFromURI(themeName) + ".ds2theme"
		if len(group.Args) >= 3 {
			outName = group.Args[2]
			if !strings.HasSuffix(outName, ".ds2theme") {
				outName += ".ds2theme"
			}
		}
		outPath := filepath.Join(destDir, outName)

		// filepath.Dir(outPath) collapses to destDir itself when outName has
		// no subfolder segments (the common case), and to destDir plus the
		// nested path when it does (a user theme extracted from a subfolder,
		// see theme.FileStemFromURI) -- one MkdirAll covers both.
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			logger.Error(ctx, "Failed to create directory '"+console.FormatUserFolderPath(paths.GetThemesDir(), filepath.Dir(outPath))+"': %v", err)
			return err
		}

		if err := os.WriteFile(outPath, data, 0644); err != nil {
			logger.Error(ctx, "Failed to write theme file: %v", err)
			return err
		}
		logger.Notice(ctx, "Theme '{{|Theme|}}%s{{[-]}}' extracted to: "+console.FormatUserFilePath(paths.GetThemesDir(), outPath), theme.ThemeDisplayName(themeName))
		if theme.FileStemFromURI(themeName) == ".TEMPLATE" {
			logger.Notice(ctx, "This is a reference starter theme, not one meant to be used as-is. Rename it and edit the copy.")
		}

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
			logger.Error(ctx, "Failed to create directory '"+console.FormatUserFolderPath(paths.GetThemesDir(), destDir)+"': %v", err)
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
				logger.Warn(ctx, "Failed to write '"+console.FormatUserFilePath(paths.GetThemesDir(), outPath)+"': %v", err)
				continue
			}
			logger.Notice(ctx, "  Extracted: {{|Theme|}}%s{{[-]}}", stem+".ds2theme")
			extracted++
		}
		logger.Notice(ctx, "%d theme(s) extracted to: "+console.FormatUserFolderPath(paths.GetThemesDir(), destDir), extracted)
	}
	return nil
}

func HandleThemeTable(ctx context.Context) error {
	headers := []string{"Theme", "Description", "Author"}
	themes, err := theme.List(config.LoadAppConfig().Appearance.Local.Theme)
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
