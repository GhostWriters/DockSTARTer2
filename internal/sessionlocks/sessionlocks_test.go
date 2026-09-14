package sessionlocks

import (
	"path/filepath"
	"testing"

	"DockSTARTer2/internal/paths"
)

func newTestSessionManager(t *testing.T) *SessionManager {
	t.Helper()
	prevState, prevConfig := paths.StateHomeOverride, paths.ConfigHomeOverride
	dir := t.TempDir()
	paths.StateHomeOverride = filepath.Join(dir, "state")
	paths.ConfigHomeOverride = filepath.Join(dir, "config")
	t.Cleanup(func() {
		paths.StateHomeOverride = prevState
		paths.ConfigHomeOverride = prevConfig
	})
	return NewSessionManager()
}

// TestReleaseEditLockAsScopedToOwner verifies the fix for a sip web session
// that ends while holding the edit lock (e.g. the browser's own gear panel
// triggering Apply-and-Reconnect while the env editor is open) -- its
// cleanup must release only its own lock, not one a different, still-active
// session legitimately holds in the same daemon process.
func TestReleaseEditLockAsScopedToOwner(t *testing.T) {
	m := newTestSessionManager(t)

	if !m.AcquireEditLock("1.2.3.4", "web", "menu", "web", "session-a") {
		t.Fatal("expected session-a to acquire the edit lock")
	}
	if !m.HoldEditLockLocal() {
		t.Fatal("expected the lock to be held after Acquire")
	}

	// A different session ending must NOT release session-a's lock.
	m.ReleaseEditLockAs("session-b")
	if !m.HoldEditLockLocal() {
		t.Fatal("ReleaseEditLockAs released a lock held by a different session")
	}

	// The owning session ending DOES release it.
	m.ReleaseEditLockAs("session-a")
	if m.HoldEditLockLocal() {
		t.Fatal("ReleaseEditLockAs did not release the owning session's lock")
	}
}

// TestReleaseEditLockAsNoopWhenUnheld verifies calling it when nothing is
// locked (the common case: a session ending that was never in a
// destructive/edit-locking screen) is a safe no-op.
func TestReleaseEditLockAsNoopWhenUnheld(t *testing.T) {
	m := newTestSessionManager(t)
	m.ReleaseEditLockAs("session-a") // must not panic or misbehave
	if m.HoldEditLockLocal() {
		t.Fatal("expected no lock to be held")
	}
}

// TestReleaseEditLockUnconditional documents ReleaseEditLock's own
// unconditional (whole-process-shutdown) semantics, distinct from
// ReleaseEditLockAs -- it releases regardless of who holds it.
func TestReleaseEditLockUnconditional(t *testing.T) {
	m := newTestSessionManager(t)
	if !m.AcquireEditLock("1.2.3.4", "web", "menu", "web", "session-a") {
		t.Fatal("expected session-a to acquire the edit lock")
	}
	m.ReleaseEditLock()
	if m.HoldEditLockLocal() {
		t.Fatal("expected ReleaseEditLock to release regardless of owner")
	}
}
