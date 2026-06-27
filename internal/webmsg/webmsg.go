// Package webmsg provides a per-session outbound message channel for sending
// JSON control messages from the TUI back to the browser over WebSocket.
package webmsg

import "sync"

var (
	mu       sync.Mutex
	sessions = map[string]chan<- []byte{}
)

// Register creates and stores an outbound channel for the given client address.
// The caller (web handler) should read from the returned channel and forward
// messages to the WebSocket. Call Unregister when the session ends.
func Register(clientAddr string) <-chan []byte {
	ch := make(chan []byte, 16)
	mu.Lock()
	sessions[clientAddr] = ch
	mu.Unlock()
	return ch
}

// Unregister removes and closes the outbound channel for the given client address.
func Unregister(clientAddr string) {
	mu.Lock()
	if ch, ok := sessions[clientAddr]; ok {
		delete(sessions, clientAddr)
		close(ch)
	}
	mu.Unlock()
}

// Get returns the write end of the outbound channel for the given client address,
// or nil if no session is registered for that address.
func Get(clientAddr string) chan<- []byte {
	mu.Lock()
	defer mu.Unlock()
	return sessions[clientAddr]
}
