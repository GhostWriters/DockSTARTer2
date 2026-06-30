package tui

import (
	"time"

	"DockSTARTer2/internal/console"

	tea "charm.land/bubbletea/v2"
)

// ClearProcessingState resets any in-flight spinner indicators.
// Called when the menu is restored as the active screen after navigation.
func (m *MenuModel) ClearProcessingState() {
	m.processingItemIdx = -1
	m.processingBtnID = ""
	m.InvalidateCache()
}

// mapBtnZoneID translates public ID constants (IDApplyButton, IDBackButton, IDExitButton)
// to the internal zone IDs used by menu_buttons.go ("btn-select", "btn-back", "btn-exit").
func mapBtnZoneID(zoneID string) string {
	switch zoneID {
	case IDApplyButton:
		return "btn-select"
	case IDBackButton:
		return "btn-back"
	case IDExitButton:
		return "btn-exit"
	}
	return zoneID
}

// SetProcessingBtn marks the given button zone ID as spinning and starts the tick loop.
// Use this when the screen handles button clicks itself (bypassing MenuModel.Update)
// but still wants the MenuModel to render the spinner on that button.
func (m *MenuModel) SetProcessingBtn(zoneID string) tea.Cmd {
	m.processingBtnID = mapBtnZoneID(zoneID)
	m.InvalidateCache()
	return nil
}

// SetProcessingBtnDeferred marks the given button as spinning and defers the action
// by a short fixed delay so the button's active state renders before the action runs.
// Use this instead of tea.Batch(SetProcessingBtn, action) for any action that
// is synchronous/blocking (e.g. opens a confirm dialog on the same goroutine).
func (m *MenuModel) SetProcessingBtnDeferred(zoneID string, action tea.Cmd) tea.Cmd {
	m.processingBtnID = mapBtnZoneID(zoneID)
	m.InvalidateCache()
	return m.deferAction(action)
}

// SetLoadingText sets a centered spinner+message in the list area instead of list items.
// Set to "" to stop.
func (m *MenuModel) SetLoadingText(text string) tea.Cmd {
	m.loadingText = text
	m.spinnerFrame = 0
	m.InvalidateCache()
	return nil
}

// AdvanceSpinners advances the menu spinner by one frame if its interval has
// elapsed. Returns true if anything changed. Called by the global tick.
func (m *MenuModel) AdvanceSpinners(now time.Time) bool {
	if !console.SpinnerEnabled {
		return false
	}
	if m.loadingText == "" && m.processingItemIdx < 0 && m.processingBtnID == "" {
		return false
	}
	fps := time.Duration(console.SpinnerSpeed) * time.Millisecond
	if fps <= 0 || now.Sub(m.lastSpinner) < fps {
		return false
	}
	m.lastSpinner = now
	ctx := GetActiveContext()
	frames := console.SpinnerFramesTitleUnicode
	if !ctx.LineCharacters {
		frames = console.SpinnerFramesTitleASCII
	}
	m.spinnerFrame = (m.spinnerFrame + 1) % len(frames)
	m.InvalidateCache()
	return true
}
