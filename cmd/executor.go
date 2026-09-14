package cmd

import (
	"os"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/graphics"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/tui"
	_ "DockSTARTer2/internal/tui/screens" // Register screen creators
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"context"
	"errors"
	"fmt"
	"strings"
)

// quoteCmdPart wraps part in double quotes if it's empty or contains
// whitespace, so a command-echo notice built by joining args with spaces
// (e.g. `ds2 -u "" main`) doesn't silently lose an empty argument or blur
// together an argument containing spaces with its neighbors. Mirrors
// DockSTARTer's bash `printf '%q'` quoting for the same notice, though not
// byte-for-byte identical -- this is for display, not shell re-execution.
func quoteCmdPart(part string) string {
	if part == "" || strings.ContainsAny(part, " \t\n\"") {
		return `"` + strings.ReplaceAll(part, `"`, `\"`) + `"`
	}
	return part
}

// CmdState aliases the shared state struct in internal/commands.
type CmdState = commands.CmdState

// commandDefs aliases the shared registry. All entries live in internal/commands/registry.go.
var commandDefs = commands.Registry

// Execute runs the logic for a sequence of command groups.
// It handles flag application, command switching, and state resetting.
//
// The pre-flight checks (update/status warnings) that run before this is
// called activate/release local's tint on their own, further up in main.go's
// run() (if AnsiPaletteConfig.ApplyToCLI); that scope ends before Execute
// starts. Execute manages its own activation from there, independently,
// re-checking ApplyToCLI fresh at the top of every group (see the loop
// below) -- a --theme-cli-tint/--theme-no-cli-tint command run as one group
// in this same invocation must be reflected starting with ITS OWN
// confirmation message and every later group, not just on the next separate
// invocation.
func Execute(ctx context.Context, groups []CommandGroup) int {
	conf := config.LoadAppConfig()
	tui.RegisterConnTypeTints(ctx, "local", conf.AnsiColors.ForConnType("local"))
	_, _ = theme.Load(conf.UI.Theme, "")
	var cliTintRestore func()
	defer func() {
		if cliTintRestore != nil {
			cliTintRestore()
		}
	}()
	console.LineCharacters = conf.UI.LineCharacters
	console.SpinnerEnabled = conf.UI.Spinner
	console.SpinnerSpeed = conf.UI.SpinnerSpeed
	console.HyperlinksMode = conf.UI.Hyperlinks
	exitCode := 0

	// Validate override file for operational commands
	shouldValidate := false
	for _, g := range groups {
		switch g.Command {
		case "-h", "--help", "-V", "--version", "--sysinfo", "--config-show", "--show-config",
			"--config-folder", "--config-compose-folder", "--config-panel", "-T", "--theme", "--theme-list",
			"--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
			"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
			"--theme-large-buttons", "--theme-no-large-buttons",
			"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
			"--theme-scrollbar", "--theme-no-scrollbar", "--theme-scrollbars", "--theme-no-scrollbars",
			"--theme-spinner", "--theme-no-spinner", "--theme-spinners", "--theme-no-spinners", "--theme-spinner-speed",
			"--theme-border-color", "--theme-table",
			"--theme-dialog-title", "--theme-submenu-title", "--theme-panel-title",
			"--theme-checkbox-brackets", "--theme-radio-brackets",
			"--theme-menu-brackets", "--theme-no-menu-brackets", "--theme-tab-layout", "--theme-markdown-hyperlinks", "--theme-hyperlinks",
			"--theme-show-preview", "--theme-no-show-preview",
			"--theme-extract", "--theme-extract-all", "--tint", "--tint-repo", "--tint-file", "--tint-embedded", "--tint-list-repo", "--tint-list-embedded", "--theme-tint", "--theme-no-tint", "--theme-cli-tint", "--theme-no-cli-tint", "--theme-programbox-tint", "--theme-no-programbox-tint", "--ansi-override", "--theme-ansi-override", "--theme-no-ansi-override", "--app-template-extract", "--app-template-new", "--man",
			"--env-appfiles":
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

		// Re-register local's tint fresh for every group, not just once
		// before the loop -- an earlier group in this same invocation
		// (e.g. --tint-repo) may have just changed it on disk, and without
		// this a later group (e.g. --version) in the same invocation would
		// still render with whatever was registered at startup, only
		// picking up the change on the next separate invocation.
		freshAnsiColors := config.LoadAppConfig().AnsiColors
		tui.RegisterConnTypeTints(ctx, "local", freshAnsiColors.ForConnType("local"))

		// Same reasoning, for the ApplyToCLI on/off switch itself: a
		// --theme-cli-tint/--theme-no-cli-tint group earlier in this same
		// invocation must take effect starting with its own confirmation
		// message, not just on the next separate invocation.
		//
		// Skipped whenever this group's task() runs its own Bubble Tea
		// Program -- p.Run()'s Update/View loop calls
		// ActivateSessionRenderContext/ActivateTintFor on every render (see
		// model_update.go/model_view.go), which would deadlock trying to
		// Lock() semstyle's single (non-reentrant) tint mutex while this
		// same goroutine already holds it here. That covers two shapes:
		// a command whose handler directly calls tui.Start/StartEditor/
		// StartVarEditor (or serve.StartServer for --server/--server-daemon,
		// which spins up sessions via tui.Start/StartForSession), and any
		// command at all run with -g/--gui, which routes through
		// tui.RunCommand -> RunProgramBox's own Program. For --server-daemon
		// specifically this isn't just a brief nested-lock stall either: its
		// task blocks until the daemon shuts down, so Execute's own deferred
		// release of cliTintRestore would never run, permanently holding the
		// mutex and deadlocking every SSH/web session's own BeginTintFor
		// call the moment one connects. Keep the named-command set in sync
		// with cmd.Execute's dispatch switch below -- any case whose handler
		// ends up calling one of those Start functions belongs in it too.
		hasGUIFlag := false
		for _, flag := range group.Flags {
			if flag == "-g" || flag == "--gui" {
				hasGUIFlag = true
				break
			}
		}
		launchesOwnProgram := hasGUIFlag || map[string]bool{
			"-M": true, "--menu": true,
			"-S": true, "--select": true, "--menu-config-app-select": true, "--menu-app-select": true,
			"--edit-global": true, "--start-edit-global": true, "--edit-app": true, "--start-edit-app": true,
			"--env-edit": true, "--env-edit-lower": true,
			"--server": true, "--server-daemon": true,
		}[commands.BaseCommand(group.Command)]
		if launchesOwnProgram {
			// Release rather than leave held -- an earlier group in this
			// same invocation (e.g. --theme-cli-tint --server-daemon) may
			// have already acquired it.
			if cliTintRestore != nil {
				cliTintRestore()
				cliTintRestore = nil
			}
		} else if freshAnsiColors.ApplyToCLI {
			if cliTintRestore == nil {
				cliTintRestore = tui.BeginTintFor("local")
			}
		} else if cliTintRestore != nil {
			cliTintRestore()
			cliTintRestore = nil
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
				state.Verbose = true
				logger.SetLevel(logger.LevelInfo)
				console.GlobalVerbose = true
			case "-x", "--debug":
				state.Debug = true
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
			case "-F", "--follow":
				state.Follow = true
			}
		}

		// Logging
		cmdStr := version.CommandName
		for _, part := range group.FullSlice() {
			cmdStr += " " + quoteCmdPart(part)
		}
		cmdLine := "{{[-]}} {{|CommandLine|}}" + cmdStr + "{{[-]}}"
		logger.Notice(ctx, fmt.Sprintf("%s command: '{{|UserCommand|}}%s{{[-]}}'", version.ApplicationName, cmdStr))

		// Command Execution
		task := func(subCtx context.Context) error {
			switch commands.BaseCommand(group.Command) {
			case "-h", "--help":
				ranCommand = true
				target := ""
				if len(group.Args) > 0 {
					target = group.Args[0]
				}
				commands.PrintHelp(subCtx, target)
				return nil
			case "-V", "--version":
				ranCommand = true
				return commands.HandleVersion(subCtx)
			case "--sysinfo":
				ranCommand = true
				return commands.HandleSysInfo(subCtx)
			case "--print-version":
				ranCommand = true
				return commands.HandlePrintVersion()
			case "--print-templates-version":
				ranCommand = true
				return commands.HandlePrintTemplatesVersion()
			case "--man":
				ranCommand = true
				// A bare CLI invocation owns stdin/stdout outright (no
				// Bubble Tea Program running yet), so it can safely run a
				// real capability query instead of falling back to a
				// connType guess.
				canDisplayGraphics := graphics.QuerySixelSupport(int(os.Stdin.Fd()), os.Stdin, os.Stdout, graphics.DefaultQueryTimeout)
				return commands.HandleMan(subCtx, &group, canDisplayGraphics)
			case "--env-appfiles":
				ranCommand = true
				return commands.HandleEnvAppFiles(subCtx, &group)
			case "-i", "--install":
				ranCommand = true
				return commands.HandleInstall(subCtx, &group, &state)
			case "--setcap":
				ranCommand = true
				return commands.HandleSetcap(subCtx)
			case "--config-setcap":
				ranCommand = true
				return commands.HandleConfigSetcap(subCtx, true)
			case "--config-no-setcap":
				ranCommand = true
				return commands.HandleConfigSetcap(subCtx, false)
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
				return commands.HandleTheme(subCtx, &group)

			case "-a", "--add":
				// appvars_create (single)
				ranCommand = true
				return commands.HandleAppVarsCreate(subCtx, &group, &state)
			case "-c", "--compose":
				ranCommand = true
				return commands.HandleCompose(subCtx, &group, &state)
			case "-e", "--env":
				ranCommand = true
				return commands.HandleAppVarsCreateAll(subCtx, &group, &state)
			case "-l", "--list", "--list-added", "--list-builtin", "--list-deprecated", "--list-enabled", "--list-disabled", "--list-nondeprecated", "--list-referenced":
				ranCommand = true
				return commands.HandleList(subCtx, &group)
			case "-s", "--status":
				ranCommand = true
				return commands.HandleStatus(subCtx, &group)
			case "--config-pm", "--config-pm-auto", "--config-pm-list", "--config-pm-table", "--config-pm-existing-list", "--config-pm-existing-table":
				ranCommand = true
				return commands.HandleConfigPm(subCtx, &group)
			case "--status-enable", "--status-disable":
				ranCommand = true
				return commands.HandleStatusChange(subCtx, &group)
			case "-r", "--remove":
				ranCommand = true
				return commands.HandleRemove(subCtx, &group, &state)
			case "-S", "--select", "--menu-config-app-select", "--menu-app-select":
				ranCommand = true
				return handleAppSelect(subCtx, &group)
			case "-t", "--test":
				ranCommand = true
				return commands.HandleTest(subCtx, &group)
			case "--env-appvars":
				ranCommand = true
				return commands.HandleEnvAppVars(subCtx, &group)
			case "--env-appvars-lines":
				ranCommand = true
				return commands.HandleEnvAppVarsLines(subCtx, &group)
			case "--env-get", "--env-get-line", "--env-get-literal", "--env-get-lower", "--env-get-lower-line", "--env-get-lower-literal":
				ranCommand = true
				return commands.HandleEnvGet(subCtx, &group)
			case "--env-set", "--env-set-lower", "--env-set-literal", "--env-set-lower-literal":
				ranCommand = true
				return commands.HandleEnvSet(subCtx, &group)
			case "--config-show", "--show-config":
				ranCommand = true
				return commands.HandleConfigShow(subCtx, &conf)
			case "--config-folder", "--config-compose-folder":
				ranCommand = true
				return commands.HandleConfigSettings(subCtx, &group)
			case "--theme-lines", "--theme-no-lines", "--theme-line", "--theme-no-line",
				"--theme-borders", "--theme-no-borders", "--theme-border", "--theme-no-border",
				"--theme-large-buttons", "--theme-no-large-buttons",
				"--theme-shadows", "--theme-no-shadows", "--theme-shadow", "--theme-no-shadow", "--theme-shadow-level",
				"--theme-scrollbar", "--theme-no-scrollbar", "--theme-scrollbars", "--theme-no-scrollbars",
				"--theme-spinner", "--theme-no-spinner", "--theme-spinners", "--theme-no-spinners", "--theme-spinner-speed",
				"--theme-border-color",
				"--theme-dialog-title", "--theme-submenu-title", "--theme-panel-title",
				"--theme-checkbox-brackets", "--theme-radio-brackets",
				"--theme-menu-brackets", "--theme-no-menu-brackets", "--theme-tab-layout", "--theme-markdown-hyperlinks", "--theme-hyperlinks",
				"--theme-show-preview", "--theme-no-show-preview":
				ranCommand = true
				return commands.HandleThemeSettings(subCtx, &group)
			case "-p", "--prune":
				ranCommand = true
				return commands.HandlePrune(subCtx, &state)
			case "--start":
				ranCommand = true
				return commands.HandleContainerStart(subCtx, &group, &state)
			case "--stop":
				ranCommand = true
				return commands.HandleContainerStop(subCtx, &group, &state)
			case "--restart":
				ranCommand = true
				return commands.HandleContainerRestart(subCtx, &group, &state)
			case "--start-all":
				ranCommand = true
				return commands.HandleContainerStartAll(subCtx, &group, &state)
			case "--stop-all":
				ranCommand = true
				return commands.HandleContainerStopAll(subCtx, &group, &state)
			case "--restart-all":
				ranCommand = true
				return commands.HandleContainerRestartAll(subCtx, &group, &state)
			case "--start-stopped":
				ranCommand = true
				return commands.HandleContainerStartStopped(subCtx, &group, &state)
			case "--stop-started":
				ranCommand = true
				return commands.HandleContainerStopStarted(subCtx, &group, &state)
			case "--restart-started":
				ranCommand = true
				return commands.HandleContainerRestartStarted(subCtx, &group, &state)
			case "--logs":
				ranCommand = true
				return commands.HandleContainerLogs(subCtx, &group, &state)
			case "-R", "--reset":
				ranCommand = true
				return commands.HandleReset(subCtx)
			case "--uninstall":
				ranCommand = true
				return commands.HandleUninstall(subCtx, &state)
			case "--theme-table":
				ranCommand = true
				return commands.HandleThemeTable(subCtx)
			case "--theme-extract", "--theme-extract-all":
				ranCommand = true
				return commands.HandleThemeExtract(subCtx, &group)
			case "--tint":
				ranCommand = true
				return commands.HandleTintStatus(subCtx, &group)
			case "--tint-repo":
				ranCommand = true
				return commands.HandleThemeTintRepo(subCtx, &group)
			case "--tint-file":
				ranCommand = true
				return commands.HandleThemeTintFile(subCtx, &group)
			case "--tint-embedded":
				ranCommand = true
				return commands.HandleThemeTintEmbedded(subCtx, &group)
			case "--tint-list-repo":
				ranCommand = true
				return commands.HandleTintListRepo(subCtx, &group)
			case "--tint-list-embedded":
				ranCommand = true
				return commands.HandleTintListEmbedded(subCtx, &group)
			case "--theme-tint", "--theme-no-tint":
				ranCommand = true
				return commands.HandleThemeTintOnOff(subCtx, &group)
			case "--theme-cli-tint", "--theme-no-cli-tint":
				ranCommand = true
				return commands.HandleThemeCLITintOnOff(subCtx, &group)
			case "--theme-programbox-tint", "--theme-no-programbox-tint":
				ranCommand = true
				return commands.HandleThemeProgramBoxTintOnOff(subCtx, &group)
			case "--ansi-override":
				ranCommand = true
				return commands.HandleAnsiOverride(subCtx, &group)
			case "--theme-ansi-override", "--theme-no-ansi-override":
				ranCommand = true
				return commands.HandleThemeAnsiOverrideOnOff(subCtx, &group)
			case "--app-template-extract":
				ranCommand = true
				return commands.HandleAppTemplateExtract(subCtx, &group)
			case "--app-template-new":
				ranCommand = true
				return commands.HandleAppTemplateNew(subCtx, &group)
			case "--config-panel":
				ranCommand = true
				return commands.HandleConfigPanel(subCtx, &group)
			case "--disconnect":
				ranCommand = true
				target := ""
				if len(group.Args) > 0 {
					target = group.Args[0]
				}
				return handleDisconnect(subCtx, &state, target)
			case "--server":
				ranCommand = true
				return handleServer(subCtx, &group, &state, &conf)
			case "--server-daemon":
				ranCommand = true
				return handleServeDaemon(subCtx, &group, &conf)
			default:
				// Custom command logic would be hooked in here.
				// If we just had flags (group.Command == ""), ranCommand remains false
			}
			return nil
		}

		// Block action commands when someone else is currently editing the configuration.
		def := commandDefs[commands.BaseCommand(group.Command)]
		if def.SessionLocked {
			cliTransport := "local"
			if os.Getenv("SSH_CONNECTION") != "" {
				cliTransport = "ssh"
			}
			if !sessionlocks.Sessions.AcquireEditLock("local", cmdStr, "cli", cliTransport, fmt.Sprintf("cli-%d", os.Getpid())) {
				info := sessionlocks.Sessions.ReadEditInfo()
				closing := fmt.Sprintf("Cannot run '{{|UserCommand|}}%s{{[-]}}' while the configuration is being edited.", cmdStr)
				logger.Error(ctx, sessionlocks.EditLockLines(info, closing))
				return 1
			}
		}

		if state.GUI && group.Command != "" && group.Command != "-M" && group.Command != "--menu" {
			title := def.Title
			if title == "" {
				title = "Running Command"
			}
			title = "{{|TitleSuccess|}}" + title + "{{[-]}}"
			subtitle := ""
			runCtx := ctx
			switch group.Command {
			case "-c", "--compose":
				subtitle = commands.ComposeSubtitle(&group)
			case "-u", "--update":
				subtitle = fmt.Sprintf("Updating %s and Templates to the latest versions.", version.ApplicationName)
			case "--update-app":
				subtitle = fmt.Sprintf("Updating %s.", version.ApplicationName)
			case "--update-templates":
				subtitle = "Updating app templates."
			case "--logs":
				if state.Follow {
					runCtx = console.WithFollowMode(ctx)
				}
			}
			err := tui.RunCommand(runCtx, title, subtitle, cmdLine, task)
			if err != nil {
				exitCode = 1
				if errors.Is(err, console.ErrUserAborted) {
					return exitCode // Stop execution immediately on user abort
				}
				logger.Error(ctx, "TUI Run Error: %v", err)
			}
		} else {
			// -M/--menu blocks for the entire interactive TUI session (not a single
			// quick task), so it must not wrap task() in the CLI spinner: StartSpinner's
			// stop function wouldn't run until the user exits the TUI entirely, leaving
			// its ticker goroutine alive and drawing into the TUI's terminal the whole time.
			isMenu := group.Command == "-M" || group.Command == "--menu"
			var stopSpinner func()
			if !isMenu {
				stopSpinner = console.StartSpinner()
			}
			err := task(ctx)
			if stopSpinner != nil {
				stopSpinner()
			}
			if err != nil {
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
			if def.SessionLocked {
				sessionlocks.Sessions.ReleaseEditLock()
			}
			break
		}

		if def.SessionLocked {
			sessionlocks.Sessions.ReleaseEditLock()
		}
	}

	// If no commands matched (or groups empty), launch TUI
	if !ranCommand {
		logger.Notice(ctx, fmt.Sprintf("%s command: '{{|UserCommand|}}%s{{[-]}}'", version.ApplicationName, version.CommandName))
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
