package tui

import (
	"context"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	"charm.land/bubbles/v2/key"
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
	MenuName() string // Returns the name used for --menu or -M to return to this screen
}

// LayeredView is an interface for models that provide multiple visual layers
type LayeredView interface {
	Layers() []*lipgloss.Layer
}

// Navigation messages
type (
	// NavigateMsg requests navigation to a new screen
	NavigateMsg struct {
		Screen ScreenModel
	}

	// NavigateBackMsg requests navigation back to previous screen
	NavigateBackMsg struct{}

	// ShowDialogMsg shows a modal dialog
	ShowDialogMsg struct {
		Dialog tea.Model
	}

	// CloseDialogMsg closes the current dialog
	CloseDialogMsg struct {
		Result any
	}

	// UpdateHeaderMsg triggers a header refresh
	UpdateHeaderMsg struct{}

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

	// LayerHitMsg is sent when a native compositor layer is hit by a mouse event
	LayerHitMsg struct {
		ID     string
		Button tea.MouseButton
	}

	// LayerWheelMsg is sent when a native compositor layer is hit by a mouse wheel event
	LayerWheelMsg struct {
		ID     string
		Button tea.MouseButton // MouseWheelUp or MouseWheelDown
	}
)

// AppModel is the root Bubble Tea model
type AppModel struct {
	ctx    context.Context
	config config.AppConfig

	// Terminal dimensions
	width  int
	height int

	// Screen management
	activeScreen ScreenModel
	screenStack  []ScreenModel

	// Persistent backdrop (header + separator + helpline)
	backdrop *BackdropModel

	// Slide-up log panel (always present below helpline)
	logPanel        LogPanelModel
	logPanelFocused bool

	// Modal dialog overlay (nil when no dialog)
	dialog      tea.Model
	dialogStack []tea.Model

	// Channel for receiving confirmation result from a modal dialog
	pendingConfirm chan bool

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
func NewAppModel(ctx context.Context, cfg config.AppConfig, startScreen ScreenModel, initialStack ...ScreenModel) *AppModel {
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
		activeScreen: startScreen,
		screenStack:  stack,
		backdrop:     NewBackdropModel(helpText),
		logPanel:     NewLogPanelModel(),
	}
}

// NewAppModelStandalone creates a new application model that starts with a modal dialog only
func NewAppModelStandalone(ctx context.Context, cfg config.AppConfig, dialog tea.Model) *AppModel {
	return &AppModel{
		ctx:      ctx,
		config:   cfg,
		backdrop: NewBackdropModel(""),
		logPanel: NewLogPanelModel(),
		dialog:   dialog,
	}
}

// Init implements tea.Model
func (m *AppModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.backdrop.Init(),
		m.logPanel.Init(),
	}
	if m.activeScreen != nil {
		cmds = append(cmds, m.activeScreen.Init())
	}
	if m.dialog != nil {
		cmds = append(cmds, m.dialog.Init())
	}
	return logger.RecoverTUI(m.ctx, tea.Batch(cmds...))
}

// Update implements tea.Model
func (m *AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	defer func() {
		if r := recover(); r != nil {
			// Suppress further panics during recovery
			defer func() { recover() }()

			// Restore terminal
			Shutdown()
			console.SetTUIEnabled(false)

			// Check if it's already a FatalError
			if _, ok := r.(logger.FatalError); ok {
				return
			}

			// Report as fatal
			logger.FatalWithStackSkip(m.ctx, 2, "TUI Update Panic: %v", r)
		}
	}()

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case toggleLogPanelMsg:
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		// Sync focus with expansion state
		m.setLogPanelFocus(m.logPanel.expanded)
		// Resize backdrop, screen, and dialog to match new panel height
		m.backdrop.SetSize(m.width, m.backdropHeight())

		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
		}
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				dW, dH := m.getDialogArea(m.dialog)
				sizable.SetSize(dW, dH)
			}
		}
		return m, logger.RecoverTUI(m.ctx, cmd)

	case logLineMsg:
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		return m, logger.RecoverTUI(m.ctx, cmd)

	case tea.KeyMsg:
		if model, cmd, handled := m.handleKeyMsg(msg); handled {
			return model, cmd
		}

	case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
		// Convert to tea.MouseMsg interface for the handler
		resModel, resCmd, handled := m.handleMouseMsg(msg.(tea.MouseMsg))
		m = resModel.(*AppModel) // Preserve any state changes from handleMouseMsg
		if handled {
			return m, resCmd
		}
		if resCmd != nil {
			cmds = append(cmds, resCmd)
		}
		// Fall through to common update logic (backdrop and activeScreen)

	case ShowGlobalFlagsMsg:
		return m, func() tea.Msg { return ShowDialogMsg{Dialog: NewFlagsToggleDialog()} }

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update log panel with full dimensions first (so Height() is correct)
		m.logPanel.SetSize(m.width, m.height)

		// Update backdrop with adjusted height so helpline is visible above log panel
		m.backdrop.SetSize(m.width, m.backdropHeight())

		caW, caH := m.getContentArea()
		contentSizeMsg := tea.WindowSizeMsg{Width: caW, Height: caH}

		// Update dialog or active screen with content area dimensions
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				dW, dH := m.getDialogArea(m.dialog)
				sizable.SetSize(dW, dH)
			}
			var dialogCmd tea.Cmd
			m.dialog, dialogCmd = m.dialog.Update(contentSizeMsg)
			if dialogCmd != nil {
				cmds = append(cmds, dialogCmd)
			}
		} else if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
			updated, screenCmd := m.activeScreen.Update(contentSizeMsg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			if screenCmd != nil {
				cmds = append(cmds, screenCmd)
			}
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
		}
		return m, logger.RecoverTUI(m.ctx, tea.Batch(cmds...))

	case NavigateMsg:
		// Push current screen to stack and switch to new screen
		if m.activeScreen != nil {
			m.screenStack = append(m.screenStack, m.activeScreen)
		}
		m.activeScreen = msg.Screen
		if m.activeScreen != nil {
			CurrentPageName = m.activeScreen.MenuName()
			caW, caH := m.getContentArea()
			m.activeScreen.SetSize(caW, caH)
			m.backdrop.SetHelpText(m.activeScreen.HelpText())
			cmds = append(cmds, m.activeScreen.Init())
		}
		return m, logger.RecoverTUI(m.ctx, tea.Batch(cmds...))

	case NavigateBackMsg:
		// Pop from stack and return to previous screen
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
			if m.activeScreen != nil {
				CurrentPageName = m.activeScreen.MenuName()
				caW, caH := m.getContentArea()
				m.activeScreen.SetSize(caW, caH)
				m.backdrop.SetHelpText(m.activeScreen.HelpText())
			}
		} else {
			// If stack is empty, we "go back" to nothing (which triggers quit at the bottom)
			m.activeScreen = nil
			CurrentPageName = ""
		}

		// Check for application exit immediately if we just cleared the last screen and have no dialog.
		// This avoids the "No active screen" splotch and ensures ESC works for standalone tools.
		if m.ready && m.activeScreen == nil && m.dialog == nil {
			return m, ConfirmExitAction()
		}
		return m, nil

	case ShowDialogMsg:
		// Push current dialog to stack if one exists
		if m.dialog != nil {
			m.dialogStack = append(m.dialogStack, m.dialog)
		}

		m.dialog = msg.Dialog
		if m.dialog != nil {
			dW, dH := m.getDialogArea(m.dialog)
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(dW, dH)
			}
			m.updateComponentFocus()
			cmds = append(cmds, m.dialog.Init())
		}
		return m, tea.Batch(cmds...)

	case ShowConfirmDialogMsg:
		// If a dialog is already open, push it to stack and show the confirm dialog as the new top
		if m.dialog != nil {
			m.dialogStack = append(m.dialogStack, m.dialog)
		}

		// Show it as the main dialog (top of stack)
		dialog := newConfirmDialog(msg.Title, msg.Question, msg.DefaultYes)
		dW, dH := m.getDialogArea(dialog)
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(dW, dH)
		}
		m.dialog = dialog
		m.updateComponentFocus()
		m.pendingConfirm = msg.ResultChan
		return m, logger.RecoverTUI(m.ctx, m.dialog.Init())

	case ShowMessageDialogMsg:
		// If a dialog is already open, push it to stack
		if m.dialog != nil {
			m.dialogStack = append(m.dialogStack, m.dialog)
		}

		// Show message dialog as the main dialog
		dialog := newMessageDialog(msg.Title, msg.Message, msg.Type)
		dW, dH := m.getDialogArea(dialog)
		if sizable, ok := interface{}(dialog).(interface{ SetSize(int, int) }); ok {
			sizable.SetSize(dW, dH)
		}
		m.dialog = dialog
		m.updateComponentFocus()
		return m, m.dialog.Init()

	case FinalizeSelectionMsg:
		// Atomically clear/navigate and show dialog to avoid race conditions in batches
		if len(m.screenStack) > 0 {
			m.activeScreen = m.screenStack[len(m.screenStack)-1]
			m.screenStack = m.screenStack[:len(m.screenStack)-1]
		} else {
			m.activeScreen = nil
		}

		// Show the new dialog
		m.dialog = msg.Dialog
		if m.dialog != nil {
			dW, dH := m.getDialogArea(m.dialog)
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(dW, dH)
			}
			m.updateComponentFocus()
			return m, logger.RecoverTUI(m.ctx, m.dialog.Init())
		}
		return m, nil

	case CloseDialogMsg:
		// If we (AppModel) have a pending confirmation, we are the direct parent of the closing dialog.
		if m.pendingConfirm != nil {
			if result, ok := msg.Result.(bool); ok {
				m.pendingConfirm <- result
			} else {
				m.pendingConfirm <- false // Default to false if result invalid
			}
			m.pendingConfirm = nil
		}

		// Clear current dialog and try to pop from stack
		m.dialog = nil
		if len(m.dialogStack) > 0 {
			// Pop from stack
			m.dialog = m.dialogStack[len(m.dialogStack)-1]
			m.dialogStack = m.dialogStack[:len(m.dialogStack)-1]

			// Re-focus and re-size the popped dialog
			if m.dialog != nil {
				dW, dH := m.getDialogArea(m.dialog)
				if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
					sizable.SetSize(dW, dH)
				}
				if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
			}
			return m, nil
		}

		// Fallback: if stack is empty and no screen, we quit.
		if m.activeScreen == nil {
			return m, ConfirmExitAction()
		}

		// When returning to screen:
		// If header was already focused (e.g. status bar flags), KEEP it focused.
		// Only clear header focus if it wasn't already focused.
		if m.backdrop.header.GetFocus() == HeaderFocusNone {
			m.backdrop.header.SetFocus(HeaderFocusNone) // No-op, but for clarity
		}
		// In fact, the user wants focus to return to status bar.
		// If we opened the flags dialog, HeaderFocusFlags was already set.
		// So we just return.
		return m, nil

	case UpdateHeaderMsg:
		m.backdrop.header.SyncFlags()
		return m, nil

	case ConfigChangedMsg:
		m.config = msg.Config
		InitStyles(m.config)
		m.backdrop.header.SyncFlags()

		// Manually trigger sizing to avoid the complexities of tea.WindowSizeMsg re-triggering
		m.backdrop.SetSize(m.width, m.backdropHeight())
		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
		}
		return m, nil

	case QuitMsg:
		return m, tea.Quit
	}

	// Update backdrop
	// If a dialog is open, filter out key events to prevent background interaction (like re-triggering header actions)
	backdropMsg := msg
	if m.dialog != nil {
		if _, ok := msg.(tea.KeyPressMsg); ok {
			backdropMsg = nil
		}
	}

	if backdropMsg != nil {
		var backdropCmd tea.Cmd
		backdropModel, backdropCmd := m.backdrop.Update(backdropMsg)
		m.backdrop = backdropModel.(*BackdropModel)
		if backdropCmd != nil {
			cmds = append(cmds, backdropCmd)
		}
	}

	// Update dialog if present
	if m.dialog != nil {
		var dialogCmd tea.Cmd
		m.dialog, dialogCmd = m.dialog.Update(msg)
		if dialogCmd != nil {
			cmds = append(cmds, dialogCmd)
		}
		// If dialog supports help text, update backdrop
		if h, ok := m.dialog.(interface{ HelpText() string }); ok {
			m.backdrop.SetHelpText(h.HelpText())
		}
	} else if m.activeScreen != nil {
		var screenCmd tea.Cmd
		updated, screenCmd := m.activeScreen.Update(msg)
		if screen, ok := updated.(ScreenModel); ok {
			m.activeScreen = screen
		}
		if screenCmd != nil {
			cmds = append(cmds, screenCmd)
		}
		// Update helpline from screen
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
	}

	// Check for application exit when both activeScreen and dialog are nil
	// This happens when NavigateBack is used on the "root" screen.
	// We wait until the end of Update to handle batches (e.g. ShowDialog + NavigateBack)
	if m.ready && m.activeScreen == nil && m.dialog == nil {
		return m, logger.RecoverTUI(m.ctx, tea.Batch(ConfirmExitAction(), tea.Batch(cmds...)))
	}

	return m, logger.RecoverTUI(m.ctx, tea.Batch(cmds...))
}

// setLogPanelFocus updates logPanelFocused and tells the active screen to
// unfocus/refocus its border accordingly (if it supports the interface).
func (m *AppModel) setLogPanelFocus(focused bool) {
	m.logPanelFocused = focused
	m.logPanel.focused = focused
	if focused {
		m.backdrop.header.SetFocus(HeaderFocusNone)
	}
	m.updateComponentFocus()
}

func (m *AppModel) setHeaderFocus(focus HeaderFocus) {
	m.backdrop.header.SetFocus(focus)
	if focus != HeaderFocusNone {
		m.logPanelFocused = false
		m.logPanel.focused = false
	}
	m.updateComponentFocus()
}

func (m *AppModel) updateComponentFocus() {
	// Screen/Dialog is focused ONLY if neither log panel nor header have focus
	mainFocused := !m.logPanelFocused && m.backdrop.header.GetFocus() == HeaderFocusNone
	dialogOpen := m.dialog != nil

	if m.activeScreen != nil {
		if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
			// Screen is focused only if main area is focused AND no dialog is open
			focusable.SetFocused(mainFocused && !dialogOpen)
		}
	}
	if m.dialog != nil {
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(mainFocused)
		}
	}
}

// backdropHeight returns the height available for the backdrop (terminal minus log panel).
func (m AppModel) backdropHeight() int {
	return m.height - m.logPanel.Height()
}

// getContentArea returns the dimensions available for screens and dialogs.
func (m AppModel) getContentArea() (int, int) {
	// Use backdropHeight() to account for log panel
	bh := m.backdropHeight()
	layout := GetLayout()
	hasShadow := currentConfig.UI.Shadow
	headerH := 1
	if m.backdrop != nil {
		headerH = m.backdrop.header.Height()
	}

	return layout.ContentArea(m.width, bh, hasShadow, headerH)
}

// getDialogArea returns the dimensions available for a specific dialog.
// Help dialog uses the full terminal height, other dialogs use the backdrop's content area.
func (m AppModel) getDialogArea(d tea.Model) (int, int) {
	if _, isHelp := d.(*HelpDialogModel); isHelp {
		// Use Layout helpers directly to get full-screen content area for help
		layout := GetLayout()
		headerH := 1
		if m.backdrop != nil {
			headerH = m.backdrop.ChromeHeight() - 1
		}
		return layout.ContentArea(m.width, m.height, m.config.UI.Shadow, headerH)
	}
	return m.getContentArea()
}

// handleKeyMsg processes keyboard input.
// Returns (model, cmd, handled) where handled indicates if the key was consumed.
func (m *AppModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	// Specialized Help Blockade
	// If help is open, ANY key closes it and we return immediately to prevent leaks.
	if m.dialog != nil {
		if _, ok := m.dialog.(*HelpDialogModel); ok {
			var cmd tea.Cmd
			m.dialog, cmd = m.dialog.Update(msg)
			return m, logger.RecoverTUI(m.ctx, cmd), true
		}
	}

	// Global Priority Actions (always work, regardless of focus)
	if key.Matches(msg, Keys.ToggleLog) {
		return m, logger.RecoverTUI(m.ctx, func() tea.Msg { return toggleLogPanelMsg{} }), true
	}
	if key.Matches(msg, Keys.Help) {
		return m, logger.RecoverTUI(m.ctx, func() tea.Msg { return ShowDialogMsg{Dialog: NewHelpDialogModel()} }), true
	}
	if key.Matches(msg, Keys.ForceQuit) {
		m.Fatal = true
		return m, logger.RecoverTUI(m.ctx, tea.Quit), true
	}

	// Cycle: Screen -> LogPanel -> Header(Flags) -> Header(App) -> Header(Tmpl) -> Screen
	if key.Matches(msg, Keys.Tab) {
		if m.logPanelFocused {
			if m.dialog != nil {
				// Dialog open: Skip header, return focus to dialog
				m.setLogPanelFocus(false)
				return m, nil, true
			}
			m.setHeaderFocus(HeaderFocusFlags)
			return m, nil, true
		} else if m.dialog != nil {
			// Dialog open: Do not allow tab cycling into the header. Focus just stays in dialog.
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusFlags {
			m.setHeaderFocus(HeaderFocusApp)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
			m.setHeaderFocus(HeaderFocusTmpl)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
			m.setHeaderFocus(HeaderFocusNone)
			return m, nil, true
		} else {
			// From screen to log panel
			m.setLogPanelFocus(true)
			return m, nil, true
		}
	}

	if key.Matches(msg, Keys.ShiftTab) {
		if m.logPanelFocused {
			m.setLogPanelFocus(false)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusFlags {
			m.setLogPanelFocus(true)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
			m.setHeaderFocus(HeaderFocusFlags)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
			m.setHeaderFocus(HeaderFocusApp)
			return m, nil, true
		} else {
			// From screen to header (tmpl)
			if m.dialog != nil {
				// Dialog open: Skip header, go to LogPanel
				m.setLogPanelFocus(true)
				return m, nil, true
			}
			m.setHeaderFocus(HeaderFocusTmpl)
			return m, nil, true
		}
	}

	// Arrow Key Navigation within Header
	// We handle this regardless of m.dialog != nil because the header should trap its keys if focused
	if m.backdrop.header.GetFocus() != HeaderFocusNone {
		if key.Matches(msg, Keys.Right) {
			switch m.backdrop.header.GetFocus() {
			case HeaderFocusFlags:
				m.setHeaderFocus(HeaderFocusApp)
			case HeaderFocusApp:
				m.setHeaderFocus(HeaderFocusTmpl)
			}
			return m, nil, true
		}
		if key.Matches(msg, Keys.Left) {
			switch m.backdrop.header.GetFocus() {
			case HeaderFocusTmpl:
				m.setHeaderFocus(HeaderFocusApp)
			case HeaderFocusApp:
				m.setHeaderFocus(HeaderFocusFlags)
			}
			return m, nil, true
		}
		// Trap Up/Down keys so they don't leak to underlying screens/dialogs
		if key.Matches(msg, Keys.Up) || key.Matches(msg, Keys.Down) {
			return m, nil, true
		}
		// Escape to return to screen
		if key.Matches(msg, Keys.Esc) {
			m.setHeaderFocus(HeaderFocusNone)
			return m, nil, true
		}
	}

	// Handle Enter on focused header items
	if key.Matches(msg, Keys.Enter) && m.backdrop.header.GetFocus() != HeaderFocusNone {
		switch m.backdrop.header.GetFocus() {
		case HeaderFocusFlags:
			return m, func() tea.Msg { return ShowGlobalFlagsMsg{} }, true
		case HeaderFocusApp:
			return m, logger.RecoverTUI(m.ctx, TriggerAppUpdate()), true
		case HeaderFocusTmpl:
			return m, logger.RecoverTUI(m.ctx, TriggerTemplateUpdate()), true
		}
	}

	// Focused Log Panel Actions
	// When log panel is focused, it gets all scroll/navigation keys exclusively
	// We handle this AFTER global cycling (Tab/ShiftTab) so we don't trap those keys.
	if m.logPanelFocused {
		// Esc unfocuses the panel and returns focus to the screen/dialog
		if key.Matches(msg, Keys.Esc) {
			m.setLogPanelFocus(false)
			return m, nil, true
		}
		// Enter or Space toggles the panel open/closed
		if key.Matches(msg, Keys.Enter) || msg.String() == " " {
			return m, func() tea.Msg { return toggleLogPanelMsg{} }, true
		}
		// All other keys go to the panel viewport
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)
		return m, logger.RecoverTUI(m.ctx, cmd), true
	}

	// Modal Dialog Support
	if m.dialog != nil {
		var cmd tea.Cmd
		m.dialog, cmd = m.dialog.Update(msg)
		// Ensure helpline reflects current state after update
		if h, ok := m.dialog.(interface{ HelpText() string }); ok {
			m.backdrop.SetHelpText(h.HelpText())
		}
		return m, logger.RecoverTUI(m.ctx, cmd), true
	}

	// Active Screen Support (fallback)
	if m.activeScreen != nil {
		updated, cmd := m.activeScreen.Update(msg)
		if screen, ok := updated.(ScreenModel); ok {
			m.activeScreen = screen
		}
		// Ensure helpline reflects current state after update
		m.backdrop.SetHelpText(m.activeScreen.HelpText())
		return m, logger.RecoverTUI(m.ctx, cmd), true
	}

	return m, nil, false
}
