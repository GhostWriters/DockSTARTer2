package tui

import (
	"context"
	"fmt"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"

	tea "github.com/charmbracelet/bubbletea"
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
func RunCommand(ctx context.Context, title string, task func(context.Context) error) error {
	// For now, just run the task directly
	// TODO: Implement ProgramBox dialog for streaming output
	return task(ctx)
}

// Confirm shows a confirmation dialog and returns the user's choice
func Confirm(title, question string, defaultYes bool) bool {
	if program == nil {
		// Fallback to default if TUI not running
		return defaultYes
	}

	// TODO: Implement proper async dialog handling with Bubble Tea
	// For now, return the default value
	// This is a placeholder until we implement the dialog system properly
	return defaultYes
}

// Message shows an info message dialog
func Message(title, message string) {
	if program == nil {
		fmt.Println(title + ": " + message)
		return
	}
	// TODO: Show message dialog
	fmt.Println(title + ": " + message)
}

// Success shows a success message dialog
func Success(title, message string) {
	if program == nil {
		fmt.Println("[SUCCESS] " + title + ": " + message)
		return
	}
	// TODO: Show success dialog
	fmt.Println("[SUCCESS] " + title + ": " + message)
}

// Warning shows a warning message dialog
func Warning(title, message string) {
	if program == nil {
		fmt.Println("[WARNING] " + title + ": " + message)
		return
	}
	// TODO: Show warning dialog
	fmt.Println("[WARNING] " + title + ": " + message)
}

// Error shows an error message dialog
func Error(title, message string) {
	if program == nil {
		fmt.Println("[ERROR] " + title + ": " + message)
		return
	}
	// TODO: Show error dialog
	fmt.Println("[ERROR] " + title + ": " + message)
}

// Screen creation functions (these will be replaced by proper imports)
// We use function variables to avoid circular imports

var (
	createMainScreen   func() ScreenModel
	createConfigScreen func() ScreenModel
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
