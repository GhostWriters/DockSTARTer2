package tui

import (
	"context"
	"image/color"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
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
	MenuName() string      // Returns the name used for --menu or -M to return to this screen
	IsDestructive() bool   // Returns true if this screen modifies data and needs an edit lock
}

// LayeredView is an interface for models that provide multiple visual layers
type LayeredView interface {
	Layers() []*lipgloss.Layer
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
	// NavigateMsg requests navigation to a new screen
	NavigateMsg struct {
		Screen ScreenModel
	}

	// NavigateBackMsg requests navigation back to previous screen.
	// When Refresh is true, a RefreshAppsListMsg is dispatched to the restored
	// screen after the stack pop (avoids the batch-ordering race condition).
	NavigateBackMsg struct {
		Refresh bool
	}

	// ShowDialogMsg shows a modal dialog
	ShowDialogMsg struct {
		Dialog tea.Model
	}

	// CloseDialogMsg closes the current dialog
	CloseDialogMsg struct {
		Result any
		// ForwardToParent, when true, pops only one dialog level and delivers
		// Result to the restored parent dialog rather than draining the entire
		// stack and sending to the active screen.  Use this for results that
		// belong to a parent dialog (e.g. sinput clipboard operations).
		ForwardToParent bool
	}

	// UpdateHeaderMsg triggers a header refresh
	UpdateHeaderMsg struct{}

	// RefreshAppsListMsg requests a refresh of the application configuration menu list
	RefreshAppsListMsg struct{}

	// QuitMsg requests application exit
	QuitMsg struct{}

	// ConfigChangedMsg is sent when configuration (like theme) is updated
	ConfigChangedMsg struct {
		Config config.AppConfig
	}

	// TemplateUpdateSuccessMsg indicates that templates have been successfully updated
	TemplateUpdateSuccessMsg struct{}

	// ToggleFocusedMsg requests toggling/activating the currently focused item
	// This is triggered by middle mouse click and acts like pressing Space
	ToggleFocusedMsg struct{}

	// LockStateChangedMsg is sent when the global configuration lock state changes
	LockStateChangedMsg struct {
		LockedByOthers bool
	}

	// FinalizeSelectionMsg combines navigation and dialog display for atomic transitions
	FinalizeSelectionMsg struct {
		Dialog tea.Model
	}

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
		Title      string
		Question   string
		Sensitive  bool
		ResultChan chan promptResultMsg
	}

	// LayerHitMsg is sent when a native compositor layer is hit by a mouse event
	LayerHitMsg struct {
		ID     string
		X      int
		Y      int
		Button tea.MouseButton
		Hit    *HitRegion
	}

	// LayerWheelMsg is sent when a native compositor layer is hit by a mouse wheel event
	LayerWheelMsg struct {
		ID     string
		Button tea.MouseButton // MouseWheelUp or MouseWheelDown
		Hit    *HitRegion
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
		AppMeta          *appenv.AppMeta
	}

	// UniversalPromptMsg is a generic message for triggering a prompt
	// that can be routing to either a global dialog or a sub-dialog.
	UniversalPromptMsg struct {
		Title      string
		Question   string
		DefaultYes bool                  // For Confirm
		Sensitive  bool                  // For Prompt
		ResultChan any                   // chan bool or chan promptResultMsg
		Type       UniversalPromptType
	}
)

type UniversalPromptType int

const (
	PromptTypeConfirm UniversalPromptType = iota
	PromptTypeText
)

const HoverButton tea.MouseButton = 99

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

	// Persistent backdrop (header + separator + helpline)
	backdrop *BackdropModel

	// Slide-up log panel (always present below helpline)
	panel           PanelModel
	panelFocused    bool
	panelSbDrag    ScrollbarDragState // log-panel scrollbar drag tracking state
	panelSbAbsTopY int                // absolute Y of the scrollbar's first row (for drag computation)
	panelSbInfo    ScrollbarInfo      // scrollbar geometry captured at drag start

	// Modal dialog overlay (nil when no dialog)
	dialog      tea.Model
	dialogStack []tea.Model

	// Channel for receiving confirmation result from a modal dialog
	pendingConfirm chan bool

	// Channel for receiving prompt result from a modal dialog
	pendingPrompt chan promptResultMsg

	// Ready flag (set after first WindowSizeMsg)
	ready bool

	// Fatal indicates if the program should exit with a fatal error message
	Fatal bool

	// Hit regions for mouse click detection (simpler than compositor hit testing)
	hitRegions HitRegions

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

	return &AppModel{
		ctx:          ctx,
		config:       cfg,
		clientIP:     clientIP,
		connType:     connType,
		activeScreen: startScreen,
		screenStack:  stack,
		backdrop:     NewBackdropModel(helpText),
		panel:     NewPanelModel(EffectivePanelMode(cfg, connType), connType),
	}
}

// NewAppModelStandalone creates a new application model that starts with a modal dialog only
func NewAppModelStandalone(ctx context.Context, cfg config.AppConfig, clientIP, connType string, dialog tea.Model) *AppModel {
	return &AppModel{
		ctx:      ctx,
		config:   cfg,
		clientIP: clientIP,
		connType: connType,
		backdrop: NewBackdropModel(""),
		panel: NewPanelModel(EffectivePanelMode(cfg, connType), connType),
		dialog:   dialog,
	}
}

// Init implements tea.Model
func (m *AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
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
