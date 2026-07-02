package classic

import (
	"DockSTARTer2/internal/config"

	tea "charm.land/bubbletea/v2"
)

// ToggleFocusedMsg requests toggling/activating the currently focused item.
// This is triggered by middle mouse click and acts like pressing Space.
type ToggleFocusedMsg struct{}

// LayerHitMsg is sent when a native compositor layer is hit by a mouse event.
type LayerHitMsg struct {
	ID     string
	X      int
	Y      int
	Button tea.MouseButton
	Hit    *HitRegion
}

// LayerWheelMsg is sent when a native compositor layer is hit by a mouse wheel event.
type LayerWheelMsg struct {
	ID     string
	Button tea.MouseButton // MouseWheelUp or MouseWheelDown
	Hit    *HitRegion
}

// HoverButton is a synthetic mouse-button value used to signal a hover-only hit
// (e.g. a click blocked by an open dialog) rather than a real click.
const HoverButton tea.MouseButton = 99

// LockStateChangedMsg is sent when the global configuration lock state changes.
type LockStateChangedMsg struct {
	LockedByOthers bool
}

// PanelCommandLockChangedMsg is sent by the panel when its command-in-progress
// state changes, so the app can immediately update the exit locked marker.
type PanelCommandLockChangedMsg struct {
	Locked bool
}

// CloseDialogMsg closes the current dialog.
type CloseDialogMsg struct {
	Result any
	// ForwardToParent, when true, pops only one dialog level and delivers
	// Result to the restored parent dialog rather than draining the entire
	// stack and sending to the active screen.  Use this for results that
	// belong to a parent dialog (e.g. sinput clipboard operations).
	ForwardToParent bool
}

// TriggerHelpMsg is a message that tells the app to open the help dialog.
type TriggerHelpMsg struct {
	CapturedContext *HelpContext
	// ScreenLevelOnly strips item-specific fields so help shows screen/page context only.
	// Used when [?] is activated from the title bar widget.
	ScreenLevelOnly bool
}

// TitleBarRefreshMsg is dispatched when the [↺] title bar widget is activated.
// Screens that support refresh should handle this message.
type TitleBarRefreshMsg struct{}

// ShowDialogMsg shows a modal dialog.
type ShowDialogMsg struct {
	Dialog tea.Model
}

// activeOutputWidth is the content width (columns) of whichever output viewport is
// currently showing compose output — the program box or the console panel. It is updated
// by those components as they resize/receive output, and read via OutputContentWidth so
// compose can size proportional bars to the viewport instead of the raw terminal.
var activeOutputWidth int

// SetActiveOutputWidth records the active output viewport's content width.
func SetActiveOutputWidth(w int) {
	if w > 0 {
		activeOutputWidth = w
	}
}

// OutputContentWidth returns the active output viewport's content width, or 0 if unknown.
func OutputContentWidth() int { return activeOutputWidth }

// ConfigChangedMsg is sent when configuration (like theme) is updated.
type ConfigChangedMsg struct {
	Config config.AppConfig
}

// ReplaceOutputMsg replaces all current output lines (used for live-updating displays).
type ReplaceOutputMsg struct {
	Lines []string
}

// EffectivePanelMode returns the configured panel mode for the given connection type.
func EffectivePanelMode(cfg config.AppConfig, connType string) string {
	if connType == "local" {
		return cfg.UI.PanelLocal
	}
	return cfg.UI.PanelRemote
}
