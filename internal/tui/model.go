package tui

import (
	"context"
	"image/color"
	"time"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/logger"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ScreenType identifies different screens in the TUI
type ScreenType int

const (
	ScreenMainMenu ScreenType = iota
	ScreenConfigMenu
	ScreenOptionsMenu
	ScreenAppSelect
	ScreenThemeSelect
	ScreenDisplayOptions
)

// ScreenModel is the interface for all screen models
type ScreenModel interface {
	tea.Model
	Title() string
	HelpText() string
	SetSize(width, height int)
	IsMaximized() bool
	HasDialog() bool
	MenuName() string    // Returns the name used for --menu or -M to return to this screen
	IsDestructive() bool // Returns true if this screen modifies data and needs an edit lock
}

// SpinnerAdvancer is implemented by any screen or dialog that drives spinners
// via the global tick rather than independent tea.Tick commands.
type SpinnerAdvancer interface {
	AdvanceSpinners(now time.Time) bool
}

// LayeredView is an interface for models that provide multiple visual layers
type LayeredView interface {
	Layers() []*lipgloss.Layer
}

// EscapeActioner is implemented by screens that have a back/cancel action.
// The close widget [×] uses this to follow the same path as pressing Esc.
type EscapeActioner interface {
	EscapeAction() tea.Cmd
}

// HaloProvider is an interface for models that require a background halo decoration.
// When a model implements this, AppModel.View() will add a background halo layer
// using the specified color.
type HaloProvider interface {
	HasHalo() bool
	HaloColor() color.Color
}

// Navigation messages
type (
	// NavigateMsg requests navigation to a new screen. PushStack, if set, is
	// pushed onto the navigation stack (bottom to top) below Screen at the
	// same time -- e.g. for a wizard-like flow of N screens navigated to in
	// one shot, where each screen's own (unmodified) Back action naturally
	// reveals the next one in sequence, purely from existing stack-pop
	// behavior (see NavigateBackMsg) rather than needing every screen in
	// the chain to support a custom "on complete" callback.
	NavigateMsg struct {
		Screen    ScreenModel
		PushStack []ScreenModel
	}

	// NavigateBackMsg requests navigation back to previous screen.
	// When Refresh is true, a RefreshAppsListMsg is dispatched to the restored
	// screen after the stack pop (avoids the batch-ordering race condition).
	NavigateBackMsg struct {
		Refresh bool
	}

	// HideDialogsMsg temporarily hides all open dialogs without closing them.
	// Use UnhideDialogsMsg to restore them.
	HideDialogsMsg struct{}

	// UnhideDialogsMsg restores dialogs previously hidden by HideDialogsMsg.
	UnhideDialogsMsg struct{}

	// FreezeDisplayMsg clears the terminal then suppresses all rendering until
	// ThawDisplayMsg arrives. Used when an external event (e.g. font change) will
	// cause the browser to reflow and send a WindowSizeMsg.
	FreezeDisplayMsg struct{}

	// ThawDisplayMsg invalidates all caches and resumes rendering in one clean frame.
	ThawDisplayMsg struct{}

	// ForceRepaintMsg sends a synthetic WindowSizeMsg with current dimensions
	// after a short delay, triggering BubbleTea's full repaint path.
	ForceRepaintMsg struct{}

	// UpdateHeaderMsg triggers a header refresh
	UpdateHeaderMsg struct{}

	// RefreshAppsListMsg requests a refresh of the application configuration menu list
	RefreshAppsListMsg struct{}

	// QuitMsg requests application exit
	QuitMsg struct{}

	// TemplateUpdateSuccessMsg indicates that templates have been successfully updated
	TemplateUpdateSuccessMsg struct{}

	// FinalizeSelectionMsg combines navigation and dialog display for atomic transitions
	FinalizeSelectionMsg struct {
		Dialog tea.Model
	}

	// ConfirmQuitMsg requests a "Do you want to exit?" confirm dialog.
	// On Yes the AppModel sends tea.Quit; on No it simply closes the dialog.
	// Use this instead of calling Confirm() inside a tea.Cmd (which deadlocks).
	ConfirmQuitMsg struct{}

	// ShowConfirmDialogMsg shows a confirmation dialog with a result channel
	ShowConfirmDialogMsg struct {
		Title      string
		Question   string
		DefaultYes bool
		ResultChan chan bool
	}

	// ShowMessageDialogMsg shows a message dialog (info/success/warning/error)
	ShowMessageDialogMsg struct {
		Title   string
		Message string
		Type    MessageType
	}

	// ShowPromptDialogMsg shows a text prompt dialog with a result channel
	ShowPromptDialogMsg struct {
		Title        string
		Question     string
		Sensitive    bool
		InitialValue string
		ResultChan   chan promptResultMsg
	}

	// SubDialogMsg signals a request to show a sub-dialog and blocks the task
	SubDialogMsg struct {
		Model tea.Model
		Chan  any
	}

	// SubDialogResultMsg signals the completion of a sub-dialog
	SubDialogResultMsg struct {
		Result any
	}

	// globalTickMsg is the single shared ticker that drives all spinner advances
	// and triggers the periodic screen repaint. Fired at RefreshRate ms.
	globalTickMsg struct{ time time.Time }

	// outputLinesMsg carries one or more lines of output
	outputLinesMsg struct {
		lines []string
	}

	// outputDoneMsg signals that output is complete
	outputDoneMsg struct {
		err error
	}

	// EnvLoadDoneMsg is a message sent when environment metadata and variables have been loaded.
	EnvLoadDoneMsg struct {
		Tabs []EnvTabData
	}

	EnvTabData struct {
		Index           int
		Content         string
		DefaultFilePath string
		DefaultLines    []string
		ComposeEnvPath  string
		ReadOnlyVars    []string
		InitialVars     map[string]string
		NiceName        string
		Description     string
		EnvFilePath     string
		DefaultFunc     func(string) string
		AddPrefix       string
		ValidationType  string
		ValidationApp   string
		IsGlobal        bool
		AppMeta         *appenv.AppMeta
	}

	// UniversalPromptMsg is a generic message for triggering a prompt
	// that can be routing to either a global dialog or a sub-dialog.
	UniversalPromptMsg struct {
		Title        string
		Question     string
		DefaultYes   bool   // For Confirm
		Sensitive    bool   // For Prompt
		InitialValue string // For Text prompts: pre-filled value
		ResultChan   any    // chan bool or chan promptResultMsg
		Type         UniversalPromptType
	}
)

type UniversalPromptType int

const (
	PromptTypeConfirm UniversalPromptType = iota
	PromptTypeText
)

// AppModel is the root Bubble Tea model
type AppModel struct {
	ctx      context.Context
	config   config.AppConfig
	clientIP string
	connType string

	// lockedByOthers indicates if the configuration is locked by another session
	lockedByOthers bool

	// Terminal dimensions
	width  int
	height int

	// Screen management
	activeScreen ScreenModel
	screenStack  []ScreenModel

	// needsInit tracks screens pushed onto screenStack via NavigateMsg.PushStack
	// that haven't been made active (and therefore haven't had Init() called)
	// yet. Every message -- including whatever a screen's own Init() cmd
	// eventually produces -- is routed only to m.activeScreen.Update(), so
	// calling Init() on a screen while it's still buried in the stack would
	// fire the cmd but strand its result message nowhere; NavigateBackMsg's
	// handler calls Init() (and clears the entry here) at the moment a
	// pushed screen actually becomes active instead, matching how every
	// other screen already gets Init() called via NavigateMsg.
	needsInit map[ScreenModel]bool

	// Persistent backdrop (header + separator + helpline)
	backdrop *displayengine.BackdropModel

	// Slide-up log panel (always present below helpline)
	panel                displayengine.PanelModel
	panelFocused         bool
	panelTitleFocused    bool
	panelInputWasFocused bool                             // saved state before title-bar focus so F9 can restore it
	panelSbDrag          displayengine.ScrollbarDragState // log-panel scrollbar drag tracking state
	panelSbAbsTopY       int                              // absolute Y of the scrollbar's first row (for drag computation)
	panelSbInfo          displayengine.ScrollbarInfo      // scrollbar geometry captured at drag start

	// Modal dialog overlay (nil when no dialog)
	dialog      tea.Model
	dialogStack []tea.Model

	// Stashed dialogs while hidden (see HideDialogsMsg / UnhideDialogsMsg)
	hiddenDialog      tea.Model
	hiddenDialogStack []tea.Model

	// suppressRender, when true, causes View() to return the last rendered frame
	// unchanged. Used during hide/unhide to prevent spinner ticks or other async
	// msgs from painting a partial frame between hide and reflow settling.
	suppressRender bool

	// Channel for receiving confirmation result from a modal dialog
	pendingConfirm chan bool

	// Channel for receiving prompt result from a modal dialog
	pendingPrompt chan promptResultMsg

	// Ready flag (set after first WindowSizeMsg)
	ready bool

	// Fatal indicates if the program should exit with a fatal error message
	Fatal bool

	// Hit regions for mouse click detection (simpler than compositor hit testing)
	hitRegions displayengine.HitRegions
}

// NewAppModel creates a new application model.
// initialStack is optional; pass parent screens (outermost first) to pre-populate
// the navigation stack so that Back navigates to the parent rather than quitting.
func NewAppModel(ctx context.Context, cfg config.AppConfig, clientIP, connType string, startScreen ScreenModel, initialStack ...ScreenModel) *AppModel {
	// Get initial help text from screen if available
	helpText := ""
	if startScreen != nil {
		helpText = startScreen.HelpText()
		CurrentPageName = startScreen.MenuName()
	}

	stack := make([]ScreenModel, len(initialStack))
	copy(stack, initialStack)

	bd := displayengine.NewBackdropModel(helpText)
	bd.SetConnType(connType)
	return &AppModel{
		ctx:          ctx,
		config:       cfg,
		clientIP:     clientIP,
		connType:     connType,
		activeScreen: startScreen,
		screenStack:  stack,
		needsInit:    make(map[ScreenModel]bool),
		backdrop:     bd,
		panel:        displayengine.NewPanelModel(displayengine.EffectivePanelMode(cfg, connType), connType),
	}
}

// NewAppModelStandalone creates a new application model that starts with a modal dialog only
func NewAppModelStandalone(ctx context.Context, cfg config.AppConfig, clientIP, connType string, dialog tea.Model) *AppModel {
	bd := displayengine.NewBackdropModel("")
	bd.SetConnType(connType)
	return &AppModel{
		ctx:       ctx,
		config:    cfg,
		clientIP:  clientIP,
		connType:  connType,
		needsInit: make(map[ScreenModel]bool),
		backdrop:  bd,
		panel:     displayengine.NewPanelModel(displayengine.EffectivePanelMode(cfg, connType), connType),
		dialog:    dialog,
	}
}

// Init implements tea.Model
func (m *AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		globalTickCmd(),
		m.backdrop.Init(),
		m.panel.Init(),
	}
	if m.activeScreen != nil {
		cmds = append(cmds, m.activeScreen.Init())
	}
	if m.dialog != nil {
		cmds = append(cmds, m.dialog.Init())
	}
	return logger.BatchRecoverTUI(m.ctx, cmds...)
}
