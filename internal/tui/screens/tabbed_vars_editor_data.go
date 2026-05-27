package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/envutil"
	"DockSTARTer2/internal/lockfile"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/tui"
	"context"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// loadEnv is a tea.Cmd: reads all data from disk and returns it as envLoadDoneMsg.
// It must NOT modify model state directly — all changes are applied in Update.
func (m *TabbedVarsEditorModel) loadEnv() tea.Msg {
	ctx := context.Background()
	cfg := config.LoadAppConfig()
	envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
	defaultGlobalEnvPath := filepath.Join(paths.GetConfigDir(), constants.EnvExampleFileName)

	globalReadOnlyVars := []string{"HOME", "DOCKER_CONFIG_FOLDER", "DOCKER_COMPOSE_FOLDER"}

	// Read the main .env file into a slice once — used as envLines for all tabs
	// so staged-state lookups (IsAppEnabled, IsAppUserDefined, etc.) always use
	// in-memory slices rather than re-reading disk per call.
	envLines, _ := envutil.ReadLines(envPath)

	var loaded []tui.EnvTabData
	for i, tab := range m.tabs {
		var currentLines []string
		var defaultFilePath string

		if tab.spec.IsGlobal {
			if tab.spec.App != "" {
				currentLines, _ = appenv.ListAppVarLines(ctx, tab.spec.App, cfg)
				if !appenv.IsAppUserDefined(ctx, tab.spec.App, envPath) {
					defaultFilePath, _ = appenv.AppInstanceFile(ctx, tab.spec.App, ".env")
				}
			} else {
				currentLines, _ = appenv.ListAppVarLines(ctx, "", cfg)
				defaultFilePath = defaultGlobalEnvPath
			}
		} else {
			currentLines, _ = appenv.ListAppVarLines(ctx, tab.spec.App+":", cfg)
			if !appenv.IsAppUserDefined(ctx, tab.spec.App, envPath) {
				defaultFilePath, _ = appenv.AppInstanceFile(ctx, tab.spec.App, ".env.app.*")
			}
		}

		defaultLines := appenv.ReadDefaultLines(defaultFilePath)
		formattedLines := appenv.FormatLinesCore(ctx, currentLines, defaultLines, envLines, tab.spec.App, envPath)

		content := strings.Join(formattedLines, "\n")

		var tabReadOnlyVars []string
		if tab.spec.IsGlobal && tab.spec.App == "" {
			tabReadOnlyVars = globalReadOnlyVars
		}

		capturedCfg := cfg
		capturedApp := strings.ToUpper(tab.spec.App)
		var defaultFunc func(string) string
		if !tab.spec.IsGlobal {
			// For app-env tabs (.env.app.appname), pass APPNAME:VARNAME so
			// VarDefaultValue uses the APPENV path (template file lookup).
			defaultFunc = func(varName string) string {
				return appenv.VarDefaultValue(context.Background(), capturedApp+":"+varName, capturedCfg)
			}
		} else {
			defaultFunc = func(varName string) string {
				return appenv.VarDefaultValue(context.Background(), varName, capturedCfg)
			}
		}

		initialVars, _ := appenv.ListVarsLiteralsData(content)

		var niceName, description, envFilePath string
		var tabAppMeta *appenv.AppMeta
		if tab.spec.App != "" {
			niceName = appenv.GetNiceName(ctx, tab.spec.App)
			if desc := appenv.GetDescription(ctx, tab.spec.App, envPath); desc != "! Missing description !" {
				description = desc
			}
			if !tab.spec.IsGlobal {
				var metaErr error
				tabAppMeta, metaErr = appenv.LoadAppMeta(ctx, tab.spec.App)
				if metaErr != nil {
					logger.Error(ctx, "Failed to load metadata for %s: %v", tab.spec.App, metaErr)
				}
			}
		}
		if tab.spec.IsGlobal {
			envFilePath = envPath
		} else {
			envFilePath = appenv.GetAppEnvFile(tab.spec.App, cfg)
		}

		addPrefix, validationType, validationApp := "", "", ""
		if tab.spec.App != "" {
			addPrefix = tab.spec.App + "__"
			validationType = tab.spec.App
			if !tab.spec.IsGlobal {
				validationType += ":"
			}
			validationApp = tab.spec.App
		} else if tab.spec.IsGlobal {
			validationType = "_GLOBAL_"
		}

		loaded = append(loaded, tui.EnvTabData{
			Index:           i,
			Content:         content,
			DefaultFilePath: defaultFilePath,
			DefaultLines:    defaultLines,
			ComposeEnvPath:  envPath,
			ReadOnlyVars:    tabReadOnlyVars,
			InitialVars:     initialVars,
			NiceName:        niceName,
			Description:     description,
			EnvFilePath:     envFilePath,
			DefaultFunc:     defaultFunc,
			AddPrefix:       addPrefix,
			ValidationType:  validationType,
			ValidationApp:   validationApp,
			IsGlobal:        tab.spec.IsGlobal,
			AppMeta:         tabAppMeta,
		})
	}

	return tui.EnvLoadDoneMsg{Tabs: loaded}
}

func (m *TabbedVarsEditorModel) saveEnv() tea.Cmd {
	return func() tea.Msg {
		if m.lockedByOthers {
			return nil
		}
		cfg := config.LoadAppConfig()
		envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)

		// Create a snapshot of the current state to pass to the task
		type tabUpdate struct {
			file        string
			initialVars map[string]string
			newVars     map[string]string
		}
		var updates []tabUpdate

		re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*=(.*)`)

		for _, tab := range m.tabs {
			content := tab.editor.GetContent()
			// Parse content into map (literals)
			newVars := make(map[string]string)
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				matches := re.FindStringSubmatch(line)
				if matches != nil {
					key := matches[1]
					val := matches[2] // Literal: everything after =
					newVars[key] = val
				}
			}

			var targetFile string
			if tab.spec.IsGlobal {
				targetFile = envPath
			} else {
				targetFile = filepath.Join(cfg.ComposeDir, constants.AppEnvFileNamePrefix+tab.spec.App)
			}
			updates = append(updates, tabUpdate{
				file:        targetFile,
				initialVars: tab.initialVars,
				newVars:     newVars,
			})
		}

		task := func(ctx context.Context, w io.Writer) error {
			// Wrap context with the TUI writer so all logs in this task fan out to the ProgramBox
			ctx = console.WithTUIWriter(ctx, w)
			_ = lockfile.Acquire(paths.GetLocalLockPath())
			defer lockfile.Release(paths.GetLocalLockPath())

			// 1. Surgical Sync for each tab
			for _, u := range updates {
				if err := appenv.SyncVariables(ctx, u.file, u.initialVars, u.newVars); err != nil {
					return err
				}
			}

			// 2. Migrate old-style APPNAME_ENABLED vars to APPNAME__ENABLED
			_ = appenv.MigrateEnabledLines(ctx, cfg)

			// 3. Sanitize then CreateAll (re-adds missing template vars; CreateAll calls Update internally)
			// These already log details which will fan out to the ProgramBox
			if err := appenv.SanitizeEnv(ctx, envPath, cfg); err != nil {
				return err
			}
			if err := appenv.CreateAll(ctx, true, cfg); err != nil {
				return err
			}

			return nil
		}

		dialog := tui.NewProgramBoxModel("Saving Environment Variables", "Please be patient, this can take a while.\n"+tui.CmdLine("--env"), "").WithDialogType(tui.DialogTypeSuccess)
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)
		dialog.SuccessMsg = envSaveSuccessMsg{}

		return tui.ShowDialogMsg{Dialog: dialog}
	}
}
