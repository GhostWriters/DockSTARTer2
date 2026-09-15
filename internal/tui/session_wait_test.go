package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestWaitForActiveSessionsBlocksUntilDone verifies the ordering contract
// registerSession's doc comment describes: WaitForActiveSessions must not
// return while a registered session's own sessionDone() call is still
// pending, and must return once every registered session has called it.
func TestWaitForActiveSessionsBlocksUntilDone(t *testing.T) {
	// registerSession/sessionsWG are package-level, shared with any other
	// test -- run in isolation from the package's other (currently none)
	// tests by using distinct *tea.Program keys, and don't assume the
	// counter starts at zero.
	pA := &tea.Program{}
	pB := &tea.Program{}
	registerSession(pA)
	registerSession(pB)
	defer unregisterSession(pA)
	defer unregisterSession(pB)

	waited := make(chan struct{})
	go func() {
		WaitForActiveSessions()
		close(waited)
	}()

	// Neither session done yet -- must still be blocking.
	select {
	case <-waited:
		t.Fatal("WaitForActiveSessions returned before any session finished")
	case <-time.After(50 * time.Millisecond):
	}

	sessionDone() // for pA

	// One of two done -- must still be blocking.
	select {
	case <-waited:
		t.Fatal("WaitForActiveSessions returned before the second session finished")
	case <-time.After(50 * time.Millisecond):
	}

	sessionDone() // for pB

	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("WaitForActiveSessions did not return after every session finished")
	}
}
