package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/docker"
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


	// CurrentPageName tracks the active menu page for re-execution
	CurrentPageName string

	// CurrentEditorApp tracks which app is being edited in the tabbed vars editor.
	// Empty string means the global vars editor is active.
	// Only meaningful when CurrentPageName == "tabbed_vars".
	CurrentEditorApp string

	// isRootSession is true when the TUI was started with a plain -M pagename (no start- prefix).
	// Root sessions suppress the Back button on the entry screen and re-exec to the same plain
	// pagename after a self-update. Non-root sessions re-exec with the "start-" prefix so the
	// update restores the full navigation stack.
	isRootSession bool

	// programExited is used to synchronize TUI shutdown for updates
	programExited chan struct{}
)

// registerCallbacks wires TUI prompt/shutdown handlers into the console package.
func registerCallbacks() {
	console.TUIConfirm = PromptConfirm
	console.TUIPrompt = PromptText
	console.TUIShutdown = Shutdown
}

// deregisterCallbacks removes TUI prompt/shutdown handlers from the console package.
func deregisterCallbacks() {
	console.TUIConfirm = nil
	console.TUIPrompt = nil
	console.TUIShutdown = nil
}

// Initialize sets up the TUI without starting the run loop
func Initialize(ctx context.Context) error {
	registerCallbacks()

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

	// Drain any buffered mouse events from stdin before disabling mouse tracking.
	// When the user clicks to confirm exit, SGR-encoded mouse motion/release events
	// may already be in the stdin buffer. If not discarded, the shell reads them as
	// raw text after the program exits (producing visible ANSI garbage).
	drainStdin()

	// Reset terminal state on exit:
	// 1. Reset colors (\x1b[0m)
	// 2. Disable all mouse modes (1000, 1002, 1003)
	// 3. Disable SGR mouse mode (1006) - prevents ANSI codes leaking to shell
	// 4. Exit AltScreen (1049) if still active
	fmt.Print("\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\n")

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

// EditorFactory creates a tabbed vars editor screen.
// appName is empty for the global vars editor, or an app name for the app-specific editor.
// onClose is the Cmd to fire when the user navigates back/exits the editor.
type EditorFactory func(appName string, onClose tea.Cmd) ScreenModel

// editorFactory is registered by the screens package to avoid a circular import.
var editorFactory EditorFactory

// RegisterEditorFactory stores the factory function for use by StartEditor.
// Called from screens/init.go during package initialization.
func RegisterEditorFactory(f EditorFactory) {
	editorFactory = f
}

// StartEditor launches the TUI with the tabbed vars editor as the entry screen.
// appName is empty for the global vars editor, or an app name for the app-specific editor.
// isRoot controls whether Back navigation exits immediately (true) or uses a pre-populated stack (false).
func StartEditor(ctx context.Context, appName string, isRoot bool) error {
	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	defer func() {
		if r := recover(); r != nil {
			Shutdown()
			logger.FatalWithStack(ctx, "TUI Panic: %v", r)
		}
	}()

	if err := Initialize(ctx); err != nil {
		return err
	}

	isRootSession = isRoot

	onClose := func() tea.Msg { return NavigateBackMsg{} }
	startScreen := editorFactory(appName, onClose)

	// For non-root sessions, pre-populate the navigation stack so Back returns naturally.
	// Global editor: main → config
	// App editor:    main → config → app-select
	var initialStack []ScreenModel
	if !isRoot {
		parentNames := []string{"main", "config"}
		if appName != "" {
			parentNames = append(parentNames, "app-select")
		}
		for _, name := range parentNames {
			if entry, ok := screenRegistry[name]; ok {
				initialStack = append(initialStack, entry.create(false))
			}
		}
	}

	model := NewAppModel(ctx, currentConfig, startScreen, initialStack...)
	p := NewProgram(model)
	programExited = make(chan struct{})

	go startUpdateChecker(ctx)
	go func() {
		<-ctx.Done()
		Shutdown()
	}()

	finalModel, err := p.Run()
	close(programExited)
	drainStdin()
	fmt.Print("\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\n")

	if err != nil {
		logger.FatalWithStack(ctx, "TUI Error: %v", err)
	}

	if m, ok := finalModel.(*AppModel); ok && m.Fatal {
		logger.TUIMode = false
		console.AbortHandler(ctx)
		return ErrUserAborted
	}

	return nil
}

// VarEditorFactory creates a standalone Set Value screen for a single variable.
// Called by screens/init.go to avoid a circular import.
type VarEditorFactory func(
	varName, appName, appDesc, filePath, origVal string,
	opts []appenv.VarOption,
	onSave func(string) tea.Cmd,
	onCancel tea.Cmd,
) ScreenModel

var varEditorFactory VarEditorFactory

// RegisterVarEditorFactory stores the factory for use by StartVarEditor.
// Called from screens/init.go during package initialization.
func RegisterVarEditorFactory(f VarEditorFactory) {
	varEditorFactory = f
}

// StartVarEditor launches the TUI with the standalone Set Value dialog for a single variable.
// appName is "" for the global .env file, or an app name (from APP:VAR syntax) for .env.app.<appname>.
// varName is the variable to edit (upper-cased by the caller).
// file is the pre-resolved env file path (from resolveEnvVar).
func StartVarEditor(ctx context.Context, appName, varName, file string) error {
	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	defer func() {
		if r := recover(); r != nil {
			Shutdown()
			logger.FatalWithStack(ctx, "TUI Panic: %v", r)
		}
	}()

	if err := Initialize(ctx); err != nil {
		return err
	}

	// file is pre-resolved by the caller (resolveEnvVar).
	// Derive app name for metadata/display from APP:VAR syntax or APPNAME__ prefix.
	metaAppName := appName
	if metaAppName == "" {
		metaAppName = appenv.VarNameToAppName(varName)
	}

	// Get current value
	origVal, _ := appenv.Get(varName, file)

	// Load app metadata for description and preset options
	var meta *appenv.AppMeta
	if metaAppName != "" {
		if m, err := appenv.LoadAppMeta(ctx, metaAppName); err == nil {
			meta = m
		}
	}

	appDesc := ""
	if metaAppName != "" {
		if desc := appenv.GetDescription(ctx, metaAppName, file); desc != "! Missing description !" {
			appDesc = desc
		}
	}

	opts := appenv.GetVarOptions(varName, strings.ToUpper(metaAppName), origVal, meta)
	// Prepend "Original Value" so the user can revert easily
	opts = append([]appenv.VarOption{{
		Display: "Original Value",
		Value:   origVal,
		Help:    "Restore the value that was set before editing.",
	}}, opts...)

	onSave := func(val string) tea.Cmd {
		if err := appenv.SetLiteral(ctx, varName, val, file); err != nil {
			logger.Error(ctx, "Failed to set %s: %v", varName, err)
		}
		return tea.Quit
	}

	// Use the display-friendly app name (e.g. "Plex" not "PLEX") to match the tabbed editor
	displayAppName := appenv.GetNiceName(ctx, metaAppName)
	startScreen := varEditorFactory(varName, displayAppName, appDesc, file, origVal, opts, onSave, tea.Quit)

	model := NewAppModel(ctx, currentConfig, startScreen)
	p := NewProgram(model)
	programExited = make(chan struct{})

	go startUpdateChecker(ctx)
	go func() {
		<-ctx.Done()
		Shutdown()
	}()

	finalModel, err := p.Run()
	close(programExited)
	drainStdin()
	fmt.Print("\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\n")

	if err != nil {
		logger.FatalWithStack(ctx, "TUI Error: %v", err)
	}

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
		dialog.SetContext(ctx)
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

// ConfirmExitAction returns a tea.Cmd that prompts the user to confirm exiting.
// If confirmed, it returns tea.QuitMsg to gracefully terminate the application.
func ConfirmExitAction() tea.Cmd {
	return func() tea.Msg {
		if Confirm("Exit DockSTARTer", "Do you want to exit DockSTARTer?", true) {
			return tea.Quit() // Returns tea.QuitMsg{}
		}
		return nil
	}
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
	"display":           "appearance",
	"options-display":   "appearance",
	"theme":             "appearance",
	"options-theme":     "appearance",
	"theme-select":      "appearance",
	"display-options":   "appearance",
	"display_options":   "appearance",
	"select":            "app-select",
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
// Pages that are not registered as valid -M targets (e.g. transient dialogs, tabbed editors)
// fall back to "" so the re-exec lands on the main menu rather than failing to parse.
func reExecMenuArg() string {
	if CurrentPageName == "" {
		return ""
	}
	// Only pages that exist in the screen registry are valid -M targets.
	// Transient screens (e.g. tabbed vars editor) are not registered and would
	// cause a CLI parse error if passed to -M after re-exec.
	if _, ok := screenRegistry[CurrentPageName]; !ok {
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
			reExecArgs := append([]string{}, console.CurrentFlags...)
			if CurrentPageName == "tabbed_vars" {
				// Restore the vars editor directly, preserving which app was being edited.
				if CurrentEditorApp == "" {
					reExecArgs = append(reExecArgs, "--start-edit-global")
				} else {
					reExecArgs = append(reExecArgs, "--start-edit-app", CurrentEditorApp)
				}
			} else {
				// Restore the active menu screen (root or non-root).
				reExecArgs = append(reExecArgs, "--menu")
				if menuArg := reExecMenuArg(); menuArg != "" {
					reExecArgs = append(reExecArgs, menuArg)
				}
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

		dialog := NewProgramBoxModel("{{|TitleSuccess|}}Updating App{{[-]}}", "Checking for app updates...", "")
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

		dialog := NewProgramBoxModel("{{|TitleSuccess|}}Updating Templates{{[-]}}", "Checking for template updates...", "")
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

		dialog := NewProgramBoxModel("{{|TitleSuccess|}}Updating DockSTARTer2{{[-]}}", "Checking for updates...", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return ShowDialogMsg{Dialog: dialog}
	}
}

// PromptChoice displays a blocking multi-choice sub-dialog over the active ProgramBox.
// choices are the button labels. Returns the chosen index (0-based), or -1 on cancel/Esc.
func PromptChoice(title, question string, choices ...string) int {
	if program == nil {
		return -1
	}
	ch := make(chan int)
	dialog := newChoiceDialog(title, question, choices)
	dialog.onResult = func(i int) tea.Msg {
		return SubDialogResultMsg{Result: i}
	}
	program.Send(SubDialogMsg{
		Model: dialog,
		Chan:  ch,
	})
	return <-ch
}

// TriggerComposeUpdate returns a tea.Cmd that starts all enabled apps via docker compose update.
func TriggerComposeUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			ctx = console.WithTUIWriter(ctx, w)
			if err := compose.ExecuteCompose(ctx, console.AssumeYes(), console.Force(), "update"); err != nil {
				logger.Error(ctx, "%v", err)
				return err
			}
			return nil
		}
		dialog := NewProgramBoxModel("{{|TitleSuccess|}}Docker Compose{{[-]}}", "", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)
		return ShowDialogMsg{Dialog: dialog}
	}
}

// TriggerComposeStop returns a tea.Cmd that prompts Stop/Down/Cancel then runs the chosen compose op.
func TriggerComposeStop() tea.Cmd {
	return func() tea.Msg {
		question := "Would you like to {{|Highlight|}}Stop{{[-]}} all containers, or bring all containers {{|Highlight|}}Down{{[-]}}?\n\n{{|Highlight|}}Stop{{[-]}} will stop them, {{|Highlight|}}Down{{[-]}} will stop and remove them."
		task := func(ctx context.Context, w io.Writer) error {
			ctx = console.WithTUIWriter(ctx, w)
			choice := PromptChoice("Docker Compose", question, "Stop", "Down", "Cancel")
			switch choice {
			case 0: // Stop
				if err := compose.ExecuteCompose(ctx, true, console.Force(), "stop"); err != nil {
					logger.Error(ctx, "%v", err)
					return err
				}
			case 1: // Down
				if err := compose.ExecuteCompose(ctx, true, console.Force(), "down"); err != nil {
					logger.Error(ctx, "%v", err)
					return err
				}
			}
			return nil
		}
		dialog := NewProgramBoxModel("{{|TitleSuccess|}}Docker Compose{{[-]}}", "", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)
		return ShowDialogMsg{Dialog: dialog}
	}
}

// TriggerDockerPrune returns a tea.Cmd that runs docker system prune.
func TriggerDockerPrune() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			ctx = console.WithTUIWriter(ctx, w)
			if err := docker.Prune(ctx, console.AssumeYes()); err != nil {
				logger.Error(ctx, "%v", err)
				return err
			}
			return nil
		}
		dialog := NewProgramBoxModel("{{|TitleSuccess|}}Docker Prune{{[-]}}", "", "")
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
