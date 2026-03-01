package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"

	tea "charm.land/bubbletea/v2"
)

var (
	// ErrUserAborted is returned when the user cancels an operation
	ErrUserAborted = console.ErrUserAborted

	// program holds the running Bubble Tea program
	program *tea.Program

	// currentConfig holds the loaded app configuration
	currentConfig config.AppConfig

	// confirmResult is used for synchronous confirmation dialogs
	confirmResult chan bool

	// CurrentPageName tracks the active menu page for re-execution parity
	CurrentPageName string

	// isRootSession is true when the TUI was started with a plain -M pagename (no start- prefix).
	// Root sessions suppress the Back button on the entry screen and re-exec to the same plain
	// pagename after a self-update. Non-root sessions re-exec with the "start-" prefix so the
	// update restores the full navigation stack.
	isRootSession bool

	// programExited is used to synchronize TUI shutdown for updates
	programExited chan struct{}
)

// Initialize sets up the TUI without starting the run loop
func Initialize(ctx context.Context) error {
	console.TUIConfirm = PromptConfirm
	console.TUIShutdown = Shutdown

	currentConfig = config.LoadAppConfig()
	if _, err := theme.Load(currentConfig.UI.Theme, ""); err != nil { // Initial theme load
		return fmt.Errorf("failed to load theme: %w", err)
	}

	// Initialize styles from theme
	InitStyles(currentConfig)

	return nil
}

// NewProgram creates a new Bubble Tea program with standardized options.
// It also sets the global program variable for cross-component communication.
func NewProgram(model tea.Model) *tea.Program {
	p := tea.NewProgram(model, tea.WithoutCatchPanics())
	program = p
	return p
}

// Start launches the TUI application
func Start(ctx context.Context, startMenu string) error {
	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	logger.Info(ctx, "TUI Starting...")

	// Global panic recovery
	defer func() {
		if r := recover(); r != nil {
			Shutdown()
			logger.FatalWithStack(ctx, "TUI Panic: %v", r)
		}
	}()

	if err := Initialize(ctx); err != nil {
		return err
	}

	// Resolve the menu target to a canonical page name and root-session flag.
	pageName, isRoot := resolveMenuTarget(startMenu)
	isRootSession = isRoot

	// Look up the screen entry; fall back to "main" if unrecognised.
	entry, ok := screenRegistry[pageName]
	if !ok {
		entry = screenRegistry["main"]
	}

	startScreen := entry.create(isRoot)

	// For non-root (start-*) sessions, push the canonical parent screens onto the
	// navigation stack so that Back navigates naturally rather than quitting.
	var initialStack []ScreenModel
	if !isRoot {
		for _, parentName := range entry.parents {
			if parentEntry, ok := screenRegistry[parentName]; ok {
				initialStack = append(initialStack, parentEntry.create(false))
			}
		}
	}

	// Create the app model
	model := NewAppModel(ctx, currentConfig, startScreen, initialStack...)

	// Create and run the Bubble Tea program
	// Note: AltScreen is set via View().AltScreen in v2
	p := NewProgram(model)

	// Initialize re-execution sync
	programExited = make(chan struct{})

	// Start background update checker
	go startUpdateChecker(ctx)

	// Listen for context cancellation to shutdown program
	go func() {
		<-ctx.Done()
		Shutdown()
	}()

	// Run the program
	finalModel, err := p.Run()

	// Signal that the program has exited
	close(programExited)

	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")

	if err != nil {
		logger.FatalWithStack(ctx, "TUI Error: %v", err)
	}

	// Check if the model exited via ForceQuit (ctrl-c)
	if m, ok := finalModel.(*AppModel); ok && m.Fatal {
		logger.TUIMode = false
		console.AbortHandler(ctx)
		return ErrUserAborted
	}

	return nil
}

// Shutdown signals the running Bubble Tea program to exit and waits for it to finish
func Shutdown() {
	if program != nil {
		program.Quit()
		// Wait for the program to actually restore terminal state
		if programExited != nil {
			<-programExited
		}
	}
}

// startUpdateChecker runs the background update check
func startUpdateChecker(ctx context.Context) {
	// Initial check
	update.GetUpdateStatus(ctx)
	if program != nil {
		program.Send(UpdateHeaderMsg{})
	}

	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			appUpdateOld := update.AppUpdateAvailable
			tmplUpdateOld := update.TmplUpdateAvailable
			errorOld := update.UpdateCheckError
			update.GetUpdateStatus(ctx)
			if update.AppUpdateAvailable != appUpdateOld || update.TmplUpdateAvailable != tmplUpdateOld || update.UpdateCheckError != errorOld {
				if program != nil {
					program.Send(UpdateHeaderMsg{})
				}
			}
		}
	}
}

// RunCommand executes a task with output displayed in a TUI dialog.
// If a Bubble Tea program is already running, it shows the dialog inline.
// Otherwise, it starts a standalone program box.
func RunCommand(ctx context.Context, title, subtitle string, task func(context.Context) error) error {
	// Wrap the task to pass the writer
	// We use WithTUIWriter to ensure logger output is captured by the TUI
	wrappedTask := func(ctx context.Context, w io.Writer) error {
		return task(console.WithTUIWriter(ctx, w))
	}

	// If TUI is already running, show dialog within existing program
	if program != nil {
		dialog := NewProgramBoxModel(title, subtitle, "")
		dialog.SetTask(wrappedTask)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)
		program.Send(ShowDialogMsg{Dialog: dialog})
		return nil
	}

	// Otherwise, run standalone with its own Bubble Tea program
	return RunProgramBox(ctx, title, subtitle, wrappedTask)
}

// Confirm shows a confirmation dialog and returns the user's choice.
// If a program is already running, it sends a message to the active program.
func Confirm(title, question string, defaultYes bool) bool {
	if program != nil {
		resultChan := make(chan bool)
		program.Send(ShowConfirmDialogMsg{
			Title:      title,
			Question:   question,
			DefaultYes: defaultYes,
			ResultChan: resultChan,
		})
		return <-resultChan
	}
	return ShowConfirmDialog(title, question, defaultYes)
}

// Message shows an info message dialog
func Message(title, message string) {
	ShowInfoDialog(title, message)
}

// Success shows a success message dialog
func Success(title, message string) {
	ShowSuccessDialog(title, message)
}

// Warning shows a warning message dialog
func Warning(title, message string) {
	ShowWarningDialog(title, message)
}

// Error shows an error message dialog
func Error(title, message string) {
	ShowErrorDialog(title, message)
}

// screenEntry holds a screen's creator function and its canonical parent stack.
// parents is ordered outermost-first (e.g. ["main", "options"] for the appearance screen).
type screenEntry struct {
	create  func(isRoot bool) ScreenModel
	parents []string
}

// screenRegistry maps canonical page names to their screen entries.
// Populated by RegisterScreen calls from the screens package.
var screenRegistry = map[string]*screenEntry{}

// screenAliases maps alternate -M sub-command names to their canonical page name.
var screenAliases = map[string]string{
	"display":          "appearance",
	"options-display":  "appearance",
	"theme":            "appearance",
	"options-theme":    "appearance",
	"theme-select":     "appearance",
	"display-options":  "appearance",
	"display_options":  "appearance",
	"select":           "app-select",
	"config-app-select": "app-select",
}

// RegisterScreen registers a screen with its canonical page name, a creator function
// that accepts isRoot, and an optional ordered list of parent page names.
// parents should be outermost-first; they are pushed onto the navigation stack
// when the screen is started via "-M start-<name>" so that Back navigates naturally.
func RegisterScreen(name string, create func(isRoot bool) ScreenModel, parents []string) {
	screenRegistry[name] = &screenEntry{create: create, parents: parents}
}

// resolveMenuTarget normalises a -M sub-command value into a canonical page name
// and determines whether this should be a root session (no Back button on entry screen).
//   - "" or "main" with no start- prefix → page "main", isRoot false (normal start)
//   - "config"                           → page "config", isRoot true
//   - "start-config"                     → page "config", isRoot false (pre-populated stack)
func resolveMenuTarget(startMenu string) (pageName string, isRoot bool) {
	if startMenu == "" {
		return "main", false
	}
	isRoot = true
	pageName = startMenu
	if strings.HasPrefix(startMenu, "start-") {
		isRoot = false
		pageName = strings.TrimPrefix(startMenu, "start-")
	}
	if canonical, ok := screenAliases[pageName]; ok {
		pageName = canonical
	}
	return pageName, isRoot
}

// reExecMenuArg returns the --menu sub-command value to use when re-executing after a self-update.
// Root sessions (started with "-M pagename") re-exec to the same plain pagename so they remain
// root. Non-root sessions use the "start-" prefix so the navigation stack is restored.
func reExecMenuArg() string {
	if CurrentPageName == "" {
		return ""
	}
	if isRootSession {
		return CurrentPageName
	}
	return "start-" + CurrentPageName
}

// TriggerAppUpdate returns a tea.Cmd that performs the application update.
// It detects the currently active screen to support sticky restarts (using --menu pagename).
func TriggerAppUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			// Redirect logger to the pipe so output is captured by the viewport
			ctx = console.WithTUIWriter(ctx, w)

			force := console.Force()
			yes := console.AssumeYes()

			// Re-exec args restore the active screen after update.
			// Root sessions re-exec to the same plain pagename; non-root sessions
			// use the "start-" prefix so the navigation stack is rebuilt.
			reExecArgs := append([]string{}, console.CurrentFlags...)
			reExecArgs = append(reExecArgs, "--menu")
			if menuArg := reExecMenuArg(); menuArg != "" {
				reExecArgs = append(reExecArgs, menuArg)
			}
			reExecArgs = append(reExecArgs, console.RestArgs...)
			err := update.SelfUpdate(ctx, force, yes, "", reExecArgs)
			if err == nil {
				// Refresh update status and UI
				update.GetUpdateStatus(ctx)
				Send(UpdateHeaderMsg{})
			}
			return err
		}

		dialog := NewProgramBoxModel("{{|Theme_TitleSuccess|}}Updating App{{[-]}}", "Checking for app updates...", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return ShowDialogMsg{Dialog: dialog}
	}
}

// TriggerTemplateUpdate returns a tea.Cmd that performs the template update.
func TriggerTemplateUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			// Redirect logger to the pipe so output is captured by the viewport
			ctx = console.WithTUIWriter(ctx, w)

			force := console.Force()
			yes := console.AssumeYes()

			err := update.UpdateTemplates(ctx, force, yes, "")
			if err == nil {
				// Refresh update status and UI
				update.GetUpdateStatus(ctx)
				Send(UpdateHeaderMsg{})
				Send(TemplateUpdateSuccessMsg{})
			}
			return err
		}

		dialog := NewProgramBoxModel("{{|Theme_TitleSuccess|}}Updating Templates{{[-]}}", "Checking for template updates...", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return ShowDialogMsg{Dialog: dialog}
	}
}

// TriggerUpdate returns a tea.Cmd that performs both application and template updates.
// Kept for backward compatibility with main menu update button.
func TriggerUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			// Redirect logger to the pipe so output is captured by the viewport
			ctx = console.WithTUIWriter(ctx, w)

			force := console.Force()
			yes := console.AssumeYes()

			if err := update.UpdateTemplates(ctx, force, yes, ""); err != nil {
				return err
			}

			// Re-exec args restore the active screen after update.
			reExecArgs := append([]string{}, console.CurrentFlags...)
			reExecArgs = append(reExecArgs, "--menu")
			if menuArg := reExecMenuArg(); menuArg != "" {
				reExecArgs = append(reExecArgs, menuArg)
			}
			reExecArgs = append(reExecArgs, console.RestArgs...)
			err := update.SelfUpdate(ctx, force, yes, "", reExecArgs)
			if err == nil {
				// Refresh update status and UI
				update.GetUpdateStatus(ctx)
				Send(UpdateHeaderMsg{})
				Send(TemplateUpdateSuccessMsg{})
			}
			return err
		}

		dialog := NewProgramBoxModel("{{|Theme_TitleSuccess|}}Updating DockSTARTer2{{[-]}}", "Checking for updates...", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return ShowDialogMsg{Dialog: dialog}
	}
}

// Send sends a message to the running Bubble Tea program
func Send(msg tea.Msg) {
	if program != nil {
		program.Send(msg)
	}
}

// IsShadowEnabled returns whether shadow is currently enabled in the global config.
// Use this for dialog chrome that should reflect the active setting, not preview changes.
func IsShadowEnabled() bool {
	return currentConfig.UI.Shadow
}

// CloseDialog returns a command to close the current modal dialog
func CloseDialog() tea.Cmd {
	return func() tea.Msg {
		return CloseDialogMsg{}
	}
}

// CloseDialogWithResult returns a command to close the current modal dialog with a result
func CloseDialogWithResult(result any) tea.Cmd {
	return func() tea.Msg {
		return CloseDialogMsg{Result: result}
	}
}
