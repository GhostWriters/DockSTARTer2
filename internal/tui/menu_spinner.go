package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// ClearProcessingState resets any in-flight spinner indicators.
// Called when the menu is restored as the active screen after navigation.
func (m *MenuModel) ClearProcessingState() {
	m.processingItemIdx = -1
	if m.loadingText == "" {
		m.titleSpinner.Stop()
	}
	m.btnRow.Clear()
	m.InvalidateCache()
}

// SetProcessingBtnDeferred marks the given button as spinning and defers the action
// by a short fixed delay so the button's active state renders before the action runs.
// Use this instead of tea.Batch(SetProcessingBtn, action) for any action that
// is synchronous/blocking (e.g. opens a confirm dialog on the same goroutine).
// zoneID must match the ButtonDef.ZoneID exactly (no aliasing) so the spinner
// state matches the ZoneID checked when rendering (see getButtonSpecs).
func (m *MenuModel) SetProcessingBtnDeferred(zoneID string, action tea.Cmd) tea.Cmd {
	cmd := m.btnRow.SetProcessing(zoneID, action)
	m.InvalidateCache()
	return cmd
}

// AbsorbMessage lets a MenuModel observe its button row's deferred-action
// message without participating in general Update() dispatch, so callers
// routing other message types elsewhere cannot accidentally skip button
// spinner/action bookkeeping. Returns nil if the message is not a deferred
// button action targeted at this instance's button row. Intended for the
// button-owning MenuModel on screens that hold multiple MenuModels — call
// this unconditionally at the very top of the screen's Update(), before any
// early-return branches, so a future early return can never silently drop
// this menu's button click.
func (m *MenuModel) AbsorbMessage(msg tea.Msg) tea.Cmd {
	if cmd, ok := m.btnRow.Update(msg); ok {
		return cmd
	}
	return nil
}

// SetLoadingText sets a centered spinner+message in the list area instead of list items.
// Set to "" to stop.
func (m *MenuModel) SetLoadingText(text string) tea.Cmd {
	m.loadingText = text
	if text != "" {
		m.titleSpinner.Start()
	} else {
		m.titleSpinner.Stop()
	}
	m.InvalidateCache()
	return nil
}

// AdvanceSpinners advances the button spinner and the list-item/loading
// spinner by one frame each, if their intervals have elapsed. Returns true
// if anything changed. Called by the global tick.
func (m *MenuModel) AdvanceSpinners(now time.Time) bool {
	btnChanged := m.btnRow.AdvanceSpinner(now)
	if btnChanged {
		m.InvalidateCache()
	}
	if m.titleSpinner.AdvanceSpinner(now) {
		m.InvalidateCache()
		return true
	}
	return btnChanged
}
