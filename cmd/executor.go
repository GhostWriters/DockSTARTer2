package cmd

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/serve"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	_ "DockSTARTer2/internal/tui/screens" // Register screen creators
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"context"
	"errors"
	"fmt"
)

// CmdState holds the state of flags for a single command group.
type CmdState struct {
	Force bool
	GUI   bool
	Yes   bool
}

// commandDef holds metadata for a CLI command.
// sessionLocked: true means the command modifies shared state (env files,
// compose config, etc.) and must be blocked when a TUI session is active.
// Add new commands here — the executor enforces sessionLocked automatically.
type commandDef struct {
	title         string
	sessionLocked bool
}

// commandDefs is the single registry of all CLI commands and their properties.
// Modelled after the bash version's CommandScript/CommandEnvBackup associative
// arrays in DockSTARTer/includes/cmdline.sh.
var commandDefs = map[string]commandDef{
	// ── Read-only ────────────────────────────────────────────────────────────
	"-h":                         {title: "Help"},
	"--help":                     {title: "Help"},
	"-V":                         {title: "Version"},
	"--version":                  {title: "Version"},
	"--man":                      {title: "Application Documentation"},
	"-l":                         {title: "List All Applications"},
	"--list":                     {title: "List All Applications"},
	"--list-builtin":             {title: "List Builtin Applications"},
	"--list-deprecated":          {title: "List Deprecated Applications"},
	"--list-nondeprecated":       {title: "List Non-Deprecated Applications"},
	"--list-added":               {title: "List Added Applications"},
	"--list-enabled":             {title: "List Enabled Applications"},
	"--list-disabled":            {title: "List Disabled Applications"},
	"--list-referenced":          {title: "List Referenced Applications"},
	"-s":                         {title: "Application Status"},
	"--status":                   {title: "Application Status"},
	"--env-appvars":              {title: "Variables for Application"},
	"--env-appvars-lines":        {title: "Variable Lines for Application"},
	"--env-get":                  {title: "Get Value of Variable"},
	"--env-get-lower":            {title: "Get Value of Variable"},
	"--env-get-line":             {title: "Get Line of Variable"},
	"--env-get-lower-line":       {title: "Get Line of Variable"},
	"--env-get-literal":          {title: "Get Literal Value of Variable"},
	"--env-get-lower-literal":    {title: "Get Literal Value of Variable"},
	"--config-show":              {title: "Show Configuration"},
	"--show-config":              {title: "Show Configuration"},
	"--theme-list":               {title: "List Themes"},
	"--theme-table":              {title: "List Themes"},
	"--theme-extract":            {title: "Extract Theme"},
	"--theme-extract-all":        {title: "Extract All Themes"},
	"--server":                   {title: "Server Management"},

	// ── Session-locked (modifies env files / shared state) ───────────────────
	"-a":                         {title: "Add Application",             sessionLocked: true},
	"--add":                      {title: "Add Application",             sessionLocked: true},
	"-r":                         {title: "Remove Application",          sessionLocked: true},
	"--remove":                   {title: "Remove Application",          sessionLocked: true},
	"-e":                         {title: "Creating Environment Variables", sessionLocked: true},
	"--env":                      {title: "Creating Environment Variables", sessionLocked: true},
	"--env-set":                  {title: "Set Value of Variable",       sessionLocked: true},
	"--env-set-lower":            {title: "Set Value of Variable",       sessionLocked: true},
	"--env-set-literal":          {title: "Set Value of Variable",       sessionLocked: true},
	"--env-set-lower-literal":    {title: "Set Value of Variable",       sessionLocked: true},
	"--env-edit":                 {title: "Edit Variable",               sessionLocked: true},
	"--env-edit-lower":           {title: "Edit Variable",               sessionLocked: true},
	"--status-enable":            {title: "Enable Application",          sessionLocked: true},
	"--status-disable":           {title: "Disable Application",         sessionLocked: true},
	"-c":                         {title: "Docker Compose",              sessionLocked: true},
	"--compose":                  {title: "Docker Compose",              sessionLocked: true},
	"-p":                         {title: "Docker Prune",                sessionLocked: true},
	"--prune":                    {title: "Docker Prune",                sessionLocked: true},
	"-i":                         {title: "Install",                     sessionLocked: true},
	"--install":                  {title: "Install",                     sessionLocked: true},
	"-u":                         {title: "Update",                      sessionLocked: true},
	"--update":                   {title: "Update",                      sessionLocked: true},
	"--update-app":               {title: "Update App",                  sessionLocked: true},
	"--update-templates":         {title: "Update Templates",            sessionLocked: true},
	"-R":                         {title: "Reset Actions",               sessionLocked: true},
	"--reset":                    {title: "Reset Actions",               sessionLocked: true},
	"-S":                         {title: "Select Applications",         sessionLocked: true},
	"--select":                   {title: "Select Applications",         sessionLocked: true},
	"-M":                         {title: "Menu",                        sessionLocked: true},
	"--menu":                     {title: "Menu",                        sessionLocked: true},
	"--edit-global":              {title: "Edit Global Variables",       sessionLocked: true},
	"--start-edit-global":        {title: "Edit Global Variables",       sessionLocked: true},
	"--edit-app":                 {title: "Edit App Variables",          sessionLocked: true},
	"--start-edit-app":           {title: "Edit App Variables",          sessionLocked: true},
	"--config-pm":                {title: "Select Package Manager",      sessionLocked: true},
	"--config-pm-auto":           {title: "Select Package Manager",      sessionLocked: true},
	"--config-pm-list":           {title: "List Known Package Managers", sessionLocked: true},
	"--config-pm-table":          {title: "List Known Package Managers", sessionLocked: true},
	"--config-pm-existing-list":  {title: "List Existing Package Managers", sessionLocked: true},
	"--config-pm-existing-table": {title: "List Existing Package Managers", sessionLocked: true},
	"--config-folder":            {title: "Set Config Folder",           sessionLocked: true},
	"--config-compose-folder":    {title: "Set Compose Folder",          sessionLocked: true},
	"-T":                         {title: "Set Theme",                   sessionLocked: true},
	"--theme":                    {title: "Set Theme",                   sessionLocked: true},
	"--theme-shadows":            {title: "Turned On Shadows",           sessionLocked: true},
	"--theme-no-shadows":         {title: "Turned Off Shadows",          sessionLocked: true},
	"--theme-shadow":             {title: "Turned On Shadows",           sessionLocked: true},
	"--theme-no-shadow":          {title: "Turned Off Shadows",          sessionLocked: true},
	"--theme-shadow-level":       {title: "Set Shadow Level",            sessionLocked: true},
	"--theme-scrollbar":          {title: "Turned On Scrollbars",        sessionLocked: true},
	"--theme-no-scrollbar":       {title: "Turned Off Scrollbars",       sessionLocked: true},
	"--theme-lines":              {title: "Turned On Line Drawing",      sessionLocked: true},
	"--theme-no-lines":           {title: "Turned Off Line Drawing",     sessionLocked: true},
	"--theme-line":               {title: "Turned On Line Drawing",      sessionLocked: true},
	"--theme-no-line":            {title: "Turned Off Line Drawing",     sessionLocked: true},
	"--theme-borders":            {title: "Turned On Borders",           sessionLocked: true},
	"--theme-no-borders":         {title: "Turned Off Borders",          sessionLocked: true},
	"--theme-border":             {title: "Turned On Borders",           sessionLocked: true},
	"--theme-no-border":          {title: "Turned Off Borders",          sessionLocked: true},
	"--theme-button-borders":     {title: "Turned On Button Borders",    sessionLocked: true},
	"--theme-no-button-borders":  {title: "Turned Off Button Borders",   sessionLocked: true},
	"--theme-border-color":       {title: "Set Border Color",            sessionLocked: true},
	"--theme-dialog-title":       {title: "Set Dialog Title Align",      sessionLocked: true},
	"--theme-submenu-title":      {title: "Set Submenu Title Align",     sessionLocked: true},
	"--theme-log-title":          {title: "Set Log Title Align",         sessionLocked: true},
}

func handleConfigSettings(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	switch group.Command {
	case "--config-folder":
		if len(group.Args) > 0 {
			conf.Paths.ConfigFolder = group.Args[0]
		} else {
			logger.Display(ctx, "Current config folder: {{|Folder|}}%s{{[-]}}", conf.Paths.ConfigFolder)
			return nil
		}
	case "--config-compose-folder":
		if len(group.Args) > 0 {
			conf.Paths.ComposeFolder = group.Args[0]
		} else {
			logger.Display(ctx, "Current compose folder: {{|Folder|}}%s{{[-]}}", conf.Paths.ComposeFolder)
			return nil
		}
	}
	if err := config.SaveAppConfig(conf); err != nil {
		logger.Error(ctx, "Failed to save configuration: %v", err)
		return err
	}
	logger.Notice(ctx, "Configuration updated successfully.")
	return nil
}

// Execute runs the logic for a sequence of command groups.
// It handles flag application, command switching, and state resetting.
func Execute(ctx context.Context, groups []CommandGroup) int {
	conf := config.LoadAppConfig()
	_, _ = theme.Load(conf.UI.Theme, "")
	exitCode := 0

	// Validate override file for operational commands
	shouldValidate := false
	for _, g := range groups {
		switch g.Command {
		case "-h", "--help", "-V", "--version", "--config-show", "--show-config",
			"--config-folder", "--config-compose-folder", "-T", "--theme", "--theme-list",
			"--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-button-borders", "--theme-no-button-borders",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
			"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color", "--theme-table",
			"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title",
			"--theme-extract", "--theme-extract-all", "--man":
			// Skip validation for meta/config commands
		default:
			shouldValidate = true
		}
	}

	if shouldValidate {
		appenv.ValidateComposeOverride(ctx, conf)
	}

	ranCommand := false

	for i, group := range groups {
		// Check for context cancellation (e.g. Ctrl-C)
		if ctx.Err() != nil {
			return 1
		}

		// Reset global state for this command set
		console.GlobalYes = false
		console.GlobalForce = false
		console.GlobalGUI = false
		console.GlobalVerbose = false
		console.GlobalDebug = false
		logger.SetLevel(logger.LevelNotice)

		state := CmdState{}

		// Prepare execution arguments
		flags := group.Flags
		restArgs := Flatten(groups[i+1:])
		console.CurrentFlags = flags
		console.RestArgs = restArgs

		// Apply Flags
		// This logic handles setting state before the command executes.
		for _, flag := range flags {
			switch flag {
			case "-v", "--verbose":
				logger.SetLevel(logger.LevelInfo)
				console.GlobalVerbose = true
			case "-x", "--debug":
				logger.SetLevel(logger.LevelDebug)
				console.GlobalDebug = true
			case "-f", "--force":
				state.Force = true
				console.GlobalForce = true
			case "-g", "--gui":
				state.GUI = true
				console.GlobalGUI = true
			case "-y", "--yes":
				state.Yes = true
				console.GlobalYes = true
			}
		}

		// Logging
		cmdStr := version.CommandName
		for _, part := range group.FullSlice() {
			cmdStr += " " + part
		}
		subtitle := " {{|CommandLine|}}" + cmdStr + "{{[-]}}"
		logger.Notice(ctx, fmt.Sprintf("%s command: '{{|UserCommand|}}%s{{[-]}}'", version.ApplicationName, cmdStr))

		// Command Execution
		task := func(subCtx context.Context) error {
			switch group.Command {
			case "-h", "--help":
				ranCommand = true
				return handleHelp(&group)
			case "-V", "--version":
				ranCommand = true
				return handleVersion(subCtx)
			case "--man":
				ranCommand = true
				return handleMan(subCtx, &group)
			case "-i", "--install":
				ranCommand = true
				return handleInstall(subCtx, &group, &state)
			case "-u", "--update", "--update-app", "--update-templates":
				ranCommand = true
				return handleUpdate(subCtx, &group, &state, restArgs)
			case "--edit-global", "--start-edit-global", "--edit-app", "--start-edit-app":
				ranCommand = true
				return handleEditVars(subCtx, &group)
			case "--env-edit", "--env-edit-lower":
				ranCommand = true
				return handleEnvEdit(subCtx, &group)
			case "-M", "--menu":
				ranCommand = true
				return handleMenu(subCtx, &group)
			case "-T", "--theme", "--theme-list":
				ranCommand = true
				return handleTheme(subCtx, &group)

			case "-a", "--add":
				// appvars_create (single)
				ranCommand = true
				return handleAppVarsCreate(subCtx, &group, &state)
			case "-c", "--compose":
				ranCommand = true
				return handleCompose(subCtx, &group, &state)
			case "-e", "--env":
				ranCommand = true
				return handleAppVarsCreateAll(subCtx, &group, &state)
			case "-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced":
				ranCommand = true
				return handleList(subCtx, &group)
			case "-s", "--status":
				ranCommand = true
				return handleStatus(subCtx, &group)
			case "--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table":
				ranCommand = true
				return handleConfigPm(subCtx, &group)
			case "--status-enable", "--status-disable":
				ranCommand = true
				return handleStatusChange(subCtx, &group)
			case "-r", "--remove":
				ranCommand = true
				return handleRemove(subCtx, &group, &state)
			case "-S", "--select", "--menu-config-app-select", "--menu-app-select":
				ranCommand = true
				return handleAppSelect(subCtx, &group)
			case "-t", "--test":
				ranCommand = true
				return handleTest(subCtx, &group)
			case "--env-appvars":
				ranCommand = true
				return handleEnvAppVars(subCtx, &group)
			case "--env-appvars-lines":
				ranCommand = true
				return handleEnvAppVarsLines(subCtx, &group)
			case "--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal":
				ranCommand = true
				return handleEnvGet(subCtx, &group)
			case "--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal":
				ranCommand = true
				return handleEnvSet(subCtx, &group)
			case "--config-show", "--show-config":
				ranCommand = true
				return handleConfigShow(subCtx, &conf)
			case "--config-folder", "--config-compose-folder":
				ranCommand = true
				return handleConfigSettings(subCtx, &group)
			case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
				"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
				"--theme-button-borders", "--theme-no-button-borders",
				"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
				"--theme-scrollbar", "--theme-no-scrollbar", "--theme-border-color",
				"--theme-dialog-title", "--theme-submenu-title", "--theme-log-title":
				ranCommand = true
				return handleThemeSettings(subCtx, &group)
			case "-p", "--prune":
				ranCommand = true
				return handlePrune(subCtx, &state)
			case "-R", "--reset":
				ranCommand = true
				return handleReset(subCtx)
			case "--theme-table":
				ranCommand = true
				return handleThemeTable(subCtx)
			case "--theme-extract", "--theme-extract-all":
				ranCommand = true
				return handleThemeExtract(subCtx, &group)
			case "--server":
				ranCommand = true
				return handleServer(subCtx, &group, &state, &conf)
			default:
				// Custom command logic would be hooked in here.
				// If we just had flags (group.Command == ""), ranCommand remains false
			}
			return nil
		}

		// Block session-locked commands when a TUI session is active.
		def := commandDefs[group.Command]
		if def.sessionLocked && serve.Sessions.IsPrimaryActive() {
			logger.Error(ctx, "Cannot run '%s' while a DockSTARTer2 session is active. "+
				"Use '--server disconnect' to force-release the session.", group.Command)
			return 1
		}

		if state.GUI && group.Command != "" && group.Command != "-M" && group.Command != "--menu" {
			title := def.title
			if title == "" {
				title = "Running Command"
			}
			title = "{{|TitleSuccess|}}" + title + "{{[-]}}"
			err := tui.RunCommand(ctx, title, subtitle, task)
			if err != nil {
				exitCode = 1
				if errors.Is(err, console.ErrUserAborted) {
					return exitCode // Stop execution immediately on user abort
				}
				logger.Error(ctx, "TUI Run Error: %v", err)
			}
		} else {
			if err := task(ctx); err != nil {
				exitCode = 1
				if errors.Is(err, console.ErrUserAborted) {
					return exitCode // Stop execution immediately on user abort
				}
				// Logic for non-abort errors if needed, but usually task handles its own logging
			}
		}

		// If a re-exec was scheduled (e.g. self-update), stop processing further
		// groups — they are already included in the re-exec args.
		if update.PendingReExec != nil {
			break
		}

	}

	// If no commands matched (or groups empty), launch TUI
	if !ranCommand {
		if err := tui.Start(ctx, ""); err != nil {
			exitCode = 1
			if errors.Is(err, tui.ErrUserAborted) {
				return exitCode // Stop execution immediately on user abort
			}
			logger.Error(ctx, "TUI Error: %v", err)
		}
	}

	return exitCode
}
