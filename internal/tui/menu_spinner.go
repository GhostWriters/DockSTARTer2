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
	// Recurse into content sections -- a plain *MenuModel container (no
	// custom screen wrapper, e.g. Main Menu) has no other place to propagate
	// this to. A section's own spinner/processing state would otherwise stay
	// stuck after the outer dialog is reactivated (e.g. returning from a
	// confirm dialog or a screen navigated to by a deferred item Action),
	// permanently blocking that section's input -- the same class of gap
	// DisplayOptionsScreen/ServerOptionsScreen fix by hand in their own
	// custom ClearProcessingState wrappers.
	for _, sec := range m.contentSections {
		if cp, ok := sec.(interface{ ClearProcessingState() }); ok {
			cp.ClearProcessingState()
		}
	}
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

// AbsorbMessage lets a MenuModel observe its own deferred-action messages
// (both button-row clicks and list-item Action clicks) without participating
// in general Update() dispatch, so callers routing other message types
// elsewhere cannot accidentally skip spinner/action bookkeeping. Returns nil
// if msg is not a deferred action targeted at this instance. Call this
// unconditionally at the very top of a multi-menu screen's Update(), before
// any early-return branches, for every *MenuModel the screen holds (not just
// the button-owning one) — any menu with items that set Action (e.g. a
// dropdown-opening item) schedules a menuDeferredActionMsg scoped to its own
// instanceID one tick after the click, and nothing else routes that message
// back to it once it's no longer reachable via a hit-region dispatch case.
func (m *MenuModel) AbsorbMessage(msg tea.Msg) tea.Cmd {
	if cmd, ok := m.btnRow.Update(msg); ok {
		return cmd
	}
	if deferred, ok := msg.(menuDeferredActionMsg); ok && deferred.id == m.instanceID {
		return deferred.action
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
