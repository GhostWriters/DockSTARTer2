package tui

import (
	"context"
	"sort"
	"strings"

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

// NewAppModel creates a new application model
func NewAppModel(ctx context.Context, cfg config.AppConfig, startScreen ScreenModel) *AppModel {
	// Get initial help text from screen if available
	helpText := ""
	if startScreen != nil {
		helpText = startScreen.HelpText()
		CurrentPageName = startScreen.MenuName()
	}

	return &AppModel{
		ctx:          ctx,
		config:       cfg,
		activeScreen: startScreen,
		screenStack:  make([]ScreenModel, 0),
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
		// Return focus to screen when panel collapses
		if !m.logPanel.expanded {
			m.setLogPanelFocus(false)
		}
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

	case tea.WindowSizeMsg:
		logger.Debug(m.ctx, "Update: Received WindowSizeMsg: %dx%d", msg.Width, msg.Height)
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
			return m, tea.Quit
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
			// Explicitly focus the new dialog
			if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
				focusable.SetFocused(true)
			}
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
		// Explicitly focus the new confirmation dialog
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(true)
		}
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
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(true)
		}
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
			if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
				focusable.SetFocused(true)
			}
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
		// Reaching here usually means the main dialog that started the program is closing itself.
		// This handles NewAppModelStandalone cases and standalone screens after NavigateBack is handled.
		if m.activeScreen == nil {
			return m, tea.Quit
		}

		// Ensure header is unfocused when returning to screen
		m.backdrop.header.SetFocus(HeaderFocusNone)
		return m, nil

	case UpdateHeaderMsg:
		m.backdrop.header.Refresh()
		return m, nil

	case ConfigChangedMsg:
		m.config = msg.Config
		InitStyles(m.config)
		m.backdrop.header.Refresh()

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
		return m, logger.RecoverTUI(m.ctx, tea.Batch(tea.Quit, tea.Batch(cmds...)))
	}

	return m, logger.RecoverTUI(m.ctx, tea.Batch(cmds...))
}

// setLogPanelFocus updates logPanelFocused and tells the active screen to
// unfocus/refocus its border accordingly (if it supports the interface).
func (m *AppModel) setLogPanelFocus(focused bool) {
	m.logPanelFocused = focused
	m.logPanel.focused = focused
	if m.activeScreen != nil {
		if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(!focused)
		}
	}
	if m.dialog != nil {
		if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
			focusable.SetFocused(!focused)
		}
	}
}

// backdropHeight returns the height available for the backdrop (terminal minus log panel).
func (m AppModel) backdropHeight() int {
	return m.height - m.logPanel.Height()
}

// getContentArea returns the dimensions available for screens and dialogs.
func (m AppModel) getContentArea() (int, int) {
	return m.backdrop.GetContentArea()
}

// getDialogArea returns the dimensions available for a specific dialog.
// Help dialog uses the full terminal height, other dialogs use the backdrop's content area.
func (m AppModel) getDialogArea(d tea.Model) (int, int) {
	if _, isHelp := d.(*HelpDialogModel); isHelp {
		// Use Layout helpers directly to get full-screen content area for help
		layout := GetLayout()
		headerH := 1
		if m.backdrop != nil && m.backdrop.header != nil {
			headerH = m.backdrop.header.Height()
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

	// Screen Navigation / Element Cycling
	// Cycle: Screen -> LogPanel -> Header(App) -> Header(Tmpl) -> Screen
	if key.Matches(msg, Keys.Tab) {
		if m.logPanelFocused {
			m.setLogPanelFocus(false)
			// setLogPanelFocus(false) refocuses screen/dialog. We need to unfocus them for Header focus.
			if m.dialog != nil {
				// Dialog open: Skip header, return focus to dialog
				if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
				return m, nil, true
			} else if m.activeScreen != nil {
				if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(false)
				}
			}
			m.backdrop.header.SetFocus(HeaderFocusApp)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
			m.backdrop.header.SetFocus(HeaderFocusTmpl)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
			m.backdrop.header.SetFocus(HeaderFocusNone)
			// Focus returns to screen/dialog
			if m.dialog != nil {
				if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
			} else if m.activeScreen != nil {
				if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
			}
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
			// Focus returns to screen/dialog (reverse cycle)
			// setLogPanelFocus(false) already restores focus, so we are good.
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusApp {
			m.backdrop.header.SetFocus(HeaderFocusNone)
			m.setLogPanelFocus(true)
			return m, nil, true
		} else if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
			m.backdrop.header.SetFocus(HeaderFocusApp)
			return m, nil, true
		} else {
			// From screen to header (tmpl)
			if m.dialog != nil {
				// Dialog open: Skip header, go to LogPanel (reverse cycle from Dialog is LogPanel)
				if focusable, ok := m.dialog.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(false)
				}
				m.setLogPanelFocus(true)
				return m, nil, true
			}

			if m.activeScreen != nil {
				if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(false)
				}
			}
			m.backdrop.header.SetFocus(HeaderFocusTmpl)
			return m, nil, true
		}
	}

	// Arrow Key Navigation within Header
	if m.dialog == nil && m.backdrop.header.GetFocus() != HeaderFocusNone {
		if key.Matches(msg, Keys.Right) {
			if m.backdrop.header.GetFocus() == HeaderFocusApp {
				m.backdrop.header.SetFocus(HeaderFocusTmpl)
			}
			// Consume the key event even if already on last item
			return m, nil, true
		}
		if key.Matches(msg, Keys.Left) {
			if m.backdrop.header.GetFocus() == HeaderFocusTmpl {
				m.backdrop.header.SetFocus(HeaderFocusApp)
			}
			// Consume the key event even if already on first item
			return m, nil, true
		}
		// Escape to return to screen
		if key.Matches(msg, Keys.Esc) {
			m.backdrop.header.SetFocus(HeaderFocusNone)
			if m.activeScreen != nil {
				if focusable, ok := m.activeScreen.(interface{ SetFocused(bool) }); ok {
					focusable.SetFocused(true)
				}
			}
			return m, nil, true
		}
	}

	// Handle Enter on focused header items
	if m.dialog == nil && key.Matches(msg, Keys.Enter) {
		switch m.backdrop.header.GetFocus() {
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

// isButtonHitID returns true if the hit ID belongs to a button region.
// Button IDs from menus use the "btn-" prefix; button IDs from screens/dialogs
// use the "_button" suffix (e.g. "apply_button", "back_button", "exit_button").
func isButtonHitID(id string) bool {
	return strings.HasPrefix(id, "btn-") || strings.HasSuffix(id, "_button")
}

// hitIDToPanelID converts a hit ID to its parent panel ID for hover-based interactions.
// Returns the panel that should receive focus when hovering over the given region.
// For example: "item-theme_list-0" -> "theme_panel", "apply_button" -> "button_panel"
func hitIDToPanelID(hitID string) string {
	// Map menu item prefixes to their panel IDs
	if strings.HasPrefix(hitID, "item-theme_list-") {
		return IDThemePanel
	}
	if strings.HasPrefix(hitID, "item-options_list-") {
		return IDOptionsPanel
	}
	// Any other "item-" hit (regular menu list items) maps to the list panel so
	// hovering back over the list restores FocusList before the wheel is forwarded.
	if strings.HasPrefix(hitID, "item-") {
		return IDListPanel
	}
	// Map all button row IDs (both menu "btn-" buttons and display_options named buttons)
	// to the button panel so hover+scroll cycles buttons and middle-click activates focused button
	if strings.HasPrefix(hitID, "btn-") {
		return IDButtonPanel
	}
	if hitID == IDApplyButton || hitID == IDBackButton || hitID == IDExitButton || hitID == IDButtonPanel {
		return IDButtonPanel
	}
	// For panel IDs themselves, return as-is
	if hitID == IDThemePanel || hitID == IDOptionsPanel {
		return hitID
	}
	// All other IDs (regular menu items, etc.) don't map to a panel
	return ""
}

// handleMouseMsg processes mouse input.
// Returns (model, cmd, handled) where handled indicates if the event was consumed.
func (m *AppModel) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd, bool) {
	// 1. RESIZE DRAG PRIORITY: If log panel is dragging, it intercepts EVERYTHING
	if m.logPanel.isDragging {
		updated, cmd := m.logPanel.Update(msg)
		m.logPanel = updated.(LogPanelModel)

		// If height changed, we need to resize other components
		m.backdrop.SetSize(m.width, m.backdropHeight())

		caW, caH := m.getContentArea()
		if m.activeScreen != nil {
			m.activeScreen.SetSize(caW, caH)
		}
		if m.dialog != nil {
			if sizable, ok := m.dialog.(interface{ SetSize(int, int) }); ok {
				sizable.SetSize(caW, caH)
			}
		}
		return m, cmd, true
	}

	// 2. FOCUS PRIORITY: If log panel has keyboard focus, it owns the scroll wheel and middle click.
	// We do this BEFORE dialog checks so that if a user tabs to logs and a dialog is behind it,
	// the wheel still scrolls the logs.
	if m.logPanelFocused {
		if _, ok := msg.(tea.MouseWheelMsg); ok {
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}
		if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseMiddle {
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}
	}

	// 3. HELP BLOCKADE: If help is open, ANY click closes it for convenience.
	if m.dialog != nil {
		if _, ok := m.dialog.(*HelpDialogModel); ok {
			if _, ok := msg.(tea.MouseClickMsg); ok {
				var cmd tea.Cmd
				m.dialog, cmd = m.dialog.Update(msg)
				return m, cmd, true
			}
		}
	}

	// 4. HOVER-AWARE WHEEL AND MIDDLE-CLICK
	// When wheel scrolling or middle-clicking, first focus the panel under the mouse,
	// then perform the action. This allows hover+scroll/click without needing to click first.
	// If not hovering over a scrollable area, do nothing.
	if wheelMsg, isWheel := msg.(tea.MouseWheelMsg); isWheel {
		// Hit test to find what's under the mouse
		hitID := m.hitRegions.FindHit(wheelMsg.X, wheelMsg.Y)

		// No hit = not over a scrollable area, ignore the wheel
		if hitID == "" {
			return m, nil, true
		}

		// Status bar: route wheel to the header for version cycling
		if hitID == IDStatusBar || hitID == IDAppVersion || hitID == IDTmplVersion {
			var cmd tea.Cmd
			if m.backdrop != nil {
				updated, bCmd := m.backdrop.Update(LayerWheelMsg{ID: IDStatusBar, Button: wheelMsg.Button})
				if backdrop, ok := updated.(*BackdropModel); ok {
					m.backdrop = backdrop
				}
				cmd = bCmd
			}
			return m, cmd, true
		}

		// Check if hovering over log panel - if so, focus and scroll it
		if hitID == IDLogPanel || hitID == IDLogViewport {
			m.setLogPanelFocus(true)
			updated, cmd := m.logPanel.Update(msg)
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}

		// Unfocus log panel since we're over something else
		m.setLogPanelFocus(false)

		// Clear header focus — wheel moved away from the status bar
		if m.backdrop != nil && m.backdrop.header != nil {
			m.backdrop.header.SetFocus(HeaderFocusNone)
		}

		panelID := hitIDToPanelID(hitID)

		// List panel: send a semantic LayerWheelMsg so screens can scroll the list
		// without changing button focus — mirrors keyboard up/down arrow behaviour.
		if panelID == IDListPanel {
			listWheel := LayerWheelMsg{ID: IDListPanel, Button: wheelMsg.Button}
			var cmd tea.Cmd
			if m.dialog != nil {
				m.dialog, cmd = m.dialog.Update(listWheel)
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(listWheel)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				cmd = sCmd
			}
			return m, cmd, true
		}

		// For other panels (submenus, button row), switch focus to the hovered panel first
		if panelID != "" {
			focusMsg := LayerHitMsg{ID: panelID, Button: tea.MouseLeft}
			if m.dialog != nil {
				m.dialog, _ = m.dialog.Update(focusMsg)
			} else if m.activeScreen != nil {
				updated, _ := m.activeScreen.Update(focusMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
			}
		}

		// Forward the wheel event to scroll
		var cmd tea.Cmd
		if m.dialog != nil {
			m.dialog, cmd = m.dialog.Update(msg)
		} else if m.activeScreen != nil {
			updated, sCmd := m.activeScreen.Update(msg)
			if s, ok := updated.(ScreenModel); ok {
				m.activeScreen = s
			}
			cmd = sCmd
		}
		return m, cmd, true
	}

	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseMiddle {
		// Hit test to find what's under the mouse
		hitID := m.hitRegions.FindHit(click.X, click.Y)

		// Status bar: middle-click activates the currently focused version item
		if hitID == IDStatusBar || hitID == IDAppVersion || hitID == IDTmplVersion {
			var cmd tea.Cmd
			if m.backdrop != nil {
				updated, bCmd := m.backdrop.Update(ToggleFocusedMsg{})
				if backdrop, ok := updated.(*BackdropModel); ok {
					m.backdrop = backdrop
				}
				cmd = bCmd
			}
			return m, cmd, true
		}

		// Check if hovering over log panel - focus it and send toggle
		if hitID == IDLogPanel || hitID == IDLogViewport {
			m.setLogPanelFocus(true)
			updated, cmd := m.logPanel.Update(ToggleFocusedMsg{})
			m.logPanel = updated.(LogPanelModel)
			return m, cmd, true
		}

		// Unfocus log panel
		m.setLogPanelFocus(false)

		// Clear header focus — middle-click landed away from the status bar
		if m.backdrop != nil && m.backdrop.header != nil {
			m.backdrop.header.SetFocus(HeaderFocusNone)
		}

		// Check if the hit ID maps to a panel (submenu or button row).
		// Panel-mapped IDs use the hover model: focus the panel, then activate the
		// currently focused item in that panel via ToggleFocusedMsg.
		// This covers display_options submenus (theme/options) and button row.
		panelID := hitIDToPanelID(hitID)
		if panelID != "" {
			focusMsg := LayerHitMsg{ID: panelID, Button: tea.MouseLeft}
			if m.dialog != nil {
				m.dialog, _ = m.dialog.Update(focusMsg)
			} else if m.activeScreen != nil {
				updated, _ := m.activeScreen.Update(focusMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
			}
			var toggleCmd tea.Cmd
			if m.dialog != nil {
				m.dialog, toggleCmd = m.dialog.Update(ToggleFocusedMsg{})
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(ToggleFocusedMsg{})
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				toggleCmd = sCmd
			}
			return m, toggleCmd, true
		}

		// For buttons not mapped to a panel (regular menu/dialog buttons like btn-select,
		// Yes/No confirm buttons, OK dismiss buttons), dispatch as a left click so the
		// button action fires normally.
		if isButtonHitID(hitID) {
			layerMsg := LayerHitMsg{ID: hitID, Button: tea.MouseLeft}
			var btnCmd tea.Cmd
			if m.dialog != nil {
				m.dialog, btnCmd = m.dialog.Update(layerMsg)
			} else if m.activeScreen != nil {
				updated, sCmd := m.activeScreen.Update(layerMsg)
				if s, ok := updated.(ScreenModel); ok {
					m.activeScreen = s
				}
				btnCmd = sCmd
			}
			return m, btnCmd, true
		}

		// For anything else with no panel and no button mapping, send ToggleFocusedMsg
		// generically (e.g., middle-clicking empty screen space).
		var mainCmd tea.Cmd
		if m.dialog != nil {
			m.dialog, mainCmd = m.dialog.Update(ToggleFocusedMsg{})
		} else if m.activeScreen != nil {
			updated, sCmd := m.activeScreen.Update(ToggleFocusedMsg{})
			if s, ok := updated.(ScreenModel); ok {
				m.activeScreen = s
			}
			mainCmd = sCmd
		}
		return m, mainCmd, true
	}

	// 5. HIT TESTING & SEMANTIC DISPATCH (for other buttons/wheel)
	var hitID string
	var hitButton tea.MouseButton

	switch me := msg.(type) {
	case tea.MouseClickMsg:
		hitID = m.hitRegions.FindHit(me.X, me.Y)
		hitButton = me.Button
	case tea.MouseWheelMsg:
		hitID = m.hitRegions.FindHit(me.X, me.Y)
		hitButton = me.Button
	}

	if hitID != "" {
		// Create semantic message with button info
		var semanticMsg tea.Msg
		if _, ok := msg.(tea.MouseWheelMsg); ok {
			semanticMsg = LayerWheelMsg{ID: hitID, Button: hitButton}
		} else {
			semanticMsg = LayerHitMsg{ID: hitID, Button: hitButton}
		}

		// A. AppModel Internal IDs (handled globally)
		switch hitID {
		case IDLogToggle:
			if me, ok := msg.(tea.MouseClickMsg); ok && me.Button == tea.MouseLeft {
				m.setLogPanelFocus(false) // Toggle also unfocuses
				return m, func() tea.Msg { return toggleLogPanelMsg{} }, true
			}
		case IDLogResize:
			if me, ok := msg.(tea.MouseClickMsg); ok && me.Button == tea.MouseLeft {
				// Correctly deliver raw msg to start dragging
				updated, cmd := m.logPanel.Update(msg)
				m.logPanel = updated.(LogPanelModel)
				return m, cmd, true
			}
		case IDLogPanel, IDLogViewport:
			m.setLogPanelFocus(true)
			if _, ok := msg.(tea.MouseWheelMsg); ok {
				updated, cmd := m.logPanel.Update(msg)
				m.logPanel = updated.(LogPanelModel)
				return m, cmd, true
			}
			return m, nil, true
		default:
			// If we hit anything else (dialog, screen, header), ensure logs and header are unfocused
			m.setLogPanelFocus(false)
			if m.backdrop != nil && m.backdrop.header != nil {
				m.backdrop.header.SetFocus(HeaderFocusNone)
			}
		}

		// B. Component-specific Dispatch (Semantic Pre-pass)
		var semanticCmd tea.Cmd
		if m.dialog != nil {
			m.dialog, semanticCmd = m.dialog.Update(semanticMsg)
		} else if m.activeScreen != nil {
			updated, sCmd := m.activeScreen.Update(semanticMsg)
			if screen, ok := updated.(ScreenModel); ok {
				m.activeScreen = screen
			}
			semanticCmd = sCmd
		}

		// C. Global Backdrop (Header etc)
		var backdropCmd tea.Cmd
		if m.backdrop != nil {
			updated, bCmd := m.backdrop.Update(semanticMsg)
			if backdrop, ok := updated.(*BackdropModel); ok {
				m.backdrop = backdrop
			}
			backdropCmd = bCmd
		}

		// RETURN FALSE: Allow raw message to fall through for full compatibility
		return m, tea.Batch(semanticCmd, backdropCmd), false
	}

	// 6. MODAL FALLBACK (No hit, but dialog is open)
	if m.dialog != nil {
		m.setLogPanelFocus(false)
		return m, nil, false // Let raw msg fall through to dialog in standard loop
	}

	// 7. DEFAULT: No hits, no modal.
	return m, nil, false
}

// ViewStringer is an interface for models that provide string content for compositing
type ViewStringer interface {
	ViewString() string
}

// View implements tea.Model
// Uses backdrop + overlay pattern (same as dialogs)
func (m *AppModel) View() (v tea.View) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error(m.ctx, "AppModel.View Panic: %v", r)
			// Use plain ANSI only — no theme tags — to prevent a recursive panic
			// if the theme system itself was the source of the original panic.
			v = tea.NewView("\x1b[31mRendering Error\x1b[0m\n\nPress any key to continue.")
		}
	}()

	if !m.ready {
		return tea.NewView("Initializing...")
	}

	// Use Layout helpers for consistent positioning
	layout := GetLayout()
	headerH := 1
	if m.backdrop != nil && m.backdrop.header != nil {
		headerH = m.backdrop.header.Height()
	}
	contentYOffset := layout.ContentStartY(headerH) // header + separator

	// Create native compositor for rendering
	comp := lipgloss.NewCompositor()

	// Reset hit regions for this frame
	m.hitRegions = nil

	// 1. Layer: Backdrop
	if m.backdrop != nil {
		comp.AddLayers(m.backdrop.Layers()...)
		// Collect hit regions from backdrop (header version labels)
		m.hitRegions = append(m.hitRegions, m.backdrop.GetHitRegions(0, 0)...)
	}

	// 2. Layer: Log panel
	logY := m.height - m.logPanel.Height()
	if lv, ok := interface{}(m.logPanel).(LayeredView); ok {
		for _, l := range lv.Layers() {
			comp.AddLayers(l.Y(l.GetY() + logY))
		}
	} else if vs, ok := interface{}(m.logPanel).(ViewStringer); ok {
		if logContent := vs.ViewString(); logContent != "" {
			comp.AddLayers(lipgloss.NewLayer(logContent).
				X(0).Y(logY).Z(ZLogPanel).ID(IDLogPanel))
		}
	}
	// Collect hit regions from log panel
	m.hitRegions = append(m.hitRegions, m.logPanel.GetHitRegions(0, logY)...)

	// Base coordinates for maximized elements (edge indent from left, content start from top)
	maxX := layout.EdgeIndent
	maxY := contentYOffset

	// 3. Layer: Active Screen
	if m.activeScreen != nil {
		var screenContent string
		if vs, ok := m.activeScreen.(ViewStringer); ok {
			screenContent = vs.ViewString()
		}

		if screenContent != "" {
			// Calculate centered position for non-maximized screens
			caW, caH := m.getContentArea()
			screenW := WidthWithoutZones(screenContent)
			screenH := lipgloss.Height(screenContent)

			screenX := maxX
			screenY := maxY

			// Center if smaller than content area
			if screenW < caW {
				screenX = maxX + (caW-screenW)/2
			}
			if screenH < caH {
				screenY = maxY + (caH-screenH)/2
			}

			if lv, ok := m.activeScreen.(LayeredView); ok {
				for _, l := range lv.Layers() {
					comp.AddLayers(l.X(l.GetX() + screenX).Y(l.GetY() + screenY))
				}
			} else {
				comp.AddLayers(lipgloss.NewLayer(screenContent).X(screenX).Y(screenY).Z(ZScreen))
			}

			// Collect hit regions from active screen with the actual position
			if hrp, ok := m.activeScreen.(HitRegionProvider); ok {
				m.hitRegions = append(m.hitRegions, hrp.GetHitRegions(screenX, screenY)...)
			}
		}
	}

	// 4. Layer: Modal Dialog
	if m.dialog != nil {
		var content string
		if vs, ok := m.dialog.(ViewStringer); ok {
			content = vs.ViewString()
		} else {
			content = m.dialog.View().Content
		}

		if content != "" {
			maximized := false
			if md, ok := m.dialog.(interface{ IsMaximized() bool }); ok {
				maximized = md.IsMaximized()
			}

			fgWidth := WidthWithoutZones(content)
			fgHeight := lipgloss.Height(content)

			mode := DialogAbsoluteCentered
			targetHeight := m.backdropHeight()

			if _, ok := m.dialog.(*HelpDialogModel); ok {
				targetHeight = m.height
			}

			if maximized {
				mode = DialogMaximized
				targetHeight = m.height
			}

			lx, ly := layout.DialogPosition(mode, fgWidth, fgHeight, m.width, targetHeight, m.config.UI.Shadow, headerH)

			if lv, ok := m.dialog.(LayeredView); ok {
				for _, l := range lv.Layers() {
					// Offset each layer by the dialog position
					comp.AddLayers(l.X(l.GetX() + lx).Y(l.GetY() + ly))
				}
			} else {
				comp.AddLayers(lipgloss.NewLayer(content).X(lx).Y(ly).Z(ZDialog))
			}

			// Collect hit regions from dialog
			if hrp, ok := m.dialog.(HitRegionProvider); ok {
				m.hitRegions = append(m.hitRegions, hrp.GetHitRegions(lx, ly)...)
			}
		}
	}

	// Sort hit regions ascending by ZOrder so FindHit (reverse iteration) checks highest-Z first
	sort.Slice(m.hitRegions, func(i, j int) bool {
		return m.hitRegions[i].ZOrder < m.hitRegions[j].ZOrder
	})

	// Render the compositor
	v = tea.NewView(comp.Render())
	v.MouseMode = tea.MouseModeAllMotion
	v.AltScreen = true
	return v
}

// GetActiveScreen returns the currently active screen
func (m AppModel) GetActiveScreen() ScreenModel {
	return m.activeScreen
}

// Backdrop returns the shared backdrop model
func (m *AppModel) Backdrop() *BackdropModel {
	return m.backdrop
}

// GetLogPanel returns the log panel model
func (m AppModel) GetLogPanel() LogPanelModel {
	return m.logPanel
}
