package tui

import (
	"context"
	"fmt"
	"io"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// program holds the running Bubble Tea program
	program *tea.Program

	// currentConfig holds the loaded app configuration
	currentConfig config.AppConfig

	// confirmResult is used for synchronous confirmation dialogs
	confirmResult chan bool
)

// Initialize sets up the TUI without starting the run loop
func Initialize(ctx context.Context) error {
	console.TUIConfirm = Confirm

	currentConfig = config.LoadAppConfig()
	if err := theme.Load(currentConfig.Theme); err != nil {
		return fmt.Errorf("failed to load theme: %w", err)
	}

	// Initialize styles from theme
	InitStyles(currentConfig)

	// Set color profile from centralized detection (respects COLORTERM/TERM)
	lipgloss.SetColorProfile(console.GetPreferredProfile())

	return nil
}

// Start launches the TUI application
func Start(ctx context.Context, startMenu string) error {
	logger.Info(ctx, "TUI Starting...")

	if err := Initialize(ctx); err != nil {
		return err
	}

	// Import screens package dynamically to avoid circular imports
	// We'll create the screen based on startMenu parameter
	var startScreen ScreenModel

	// Defer to the screens package for creating screens
	// This will be set up in the calling code
	switch startMenu {
	case "config":
		startScreen = createConfigScreen()
	case "options":
		startScreen = createOptionsScreen()
	default:
		startScreen = createMainScreen()
	}

	// Create the app model
	model := NewAppModel(ctx, currentConfig, startScreen)

	// Create and run the Bubble Tea program
	program = tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)

	// Start background update checker
	go startUpdateChecker(ctx)

	// Run the program
	_, err := program.Run()
	// Reset terminal colors on exit to prevent "bleeding" into the shell prompt
	fmt.Print("\x1b[0m\n")
	return err
}

// startUpdateChecker runs the background update check
func startUpdateChecker(ctx context.Context) {
	// Initial check
	update.GetUpdateStatus(ctx)
	if program != nil {
		program.Send(UpdateHeaderMsg{})
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			appUpdateOld := update.AppUpdateAvailable
			tmplUpdateOld := update.TmplUpdateAvailable
			update.GetUpdateStatus(ctx)
			if update.AppUpdateAvailable != appUpdateOld || update.TmplUpdateAvailable != tmplUpdateOld {
				if program != nil {
					program.Send(UpdateHeaderMsg{})
				}
			}
		}
	}
}

// RunCommand executes a task with output displayed in a TUI dialog
func RunCommand(ctx context.Context, title, subtitle string, task func(context.Context) error) error {
	// Wrap the task to pass the writer
	// Note: We don't use WithTUIWriter here because stdout/stderr redirection
	// in RunProgramBox already captures all output including logger output
	wrappedTask := func(ctx context.Context, w io.Writer) error {
		return task(ctx)
	}

	return RunProgramBox(ctx, title, subtitle, wrappedTask)
}

// Confirm shows a confirmation dialog and returns the user's choice
func Confirm(title, question string, defaultYes bool) bool {
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

// Screen creation functions (these will be replaced by proper imports)
// We use function variables to avoid circular imports

var (
	createMainScreen    func() ScreenModel
	createConfigScreen  func() ScreenModel
	createOptionsScreen func() ScreenModel
)

// RegisterScreenCreators allows the screens package to register screen creation functions
func RegisterScreenCreators(main, cfg, opts func() ScreenModel) {
	createMainScreen = main
	createConfigScreen = cfg
	createOptionsScreen = opts
}

// init sets up default screen creators
func init() {
	// Default to nil - must be registered by screens package
	createMainScreen = func() ScreenModel { return nil }
	createConfigScreen = func() ScreenModel { return nil }
	createOptionsScreen = func() ScreenModel { return nil }
}
