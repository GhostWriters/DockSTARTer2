// Package webmsg provides a per-session outbound message channel for sending
// JSON control messages from the TUI back to the browser over WebSocket.
package webmsg

import "sync"

// DisplaySettings holds the browser's current xterm.js display settings.
type DisplaySettings struct {
	FontFamily string
	FontSize   int
	// UseDefaultFont is whether "Use default browser font" is checked.
	UseDefaultFont bool
	RefreshRate    int
}

// defaultDisplaySettings is returned for sessions with no stored settings yet.
func defaultDisplaySettings() DisplaySettings {
	return DisplaySettings{FontFamily: "monospace", FontSize: 14, UseDefaultFont: true, RefreshRate: 100}
}

type session struct {
	outbound chan<- []byte
	display  DisplaySettings
}

var (
	mu       sync.Mutex
	sessions = map[string]*session{}
)

// Register creates and stores an outbound channel for the given token.
// The caller (web handler) should read from the returned channel and forward
// messages to the WebSocket. Call Unregister when the session ends.
func Register(token string) <-chan []byte {
	ch := make(chan []byte, 16)
	mu.Lock()
	sessions[token] = &session{outbound: ch, display: defaultDisplaySettings()}
	mu.Unlock()
	return ch
}

// Unregister removes and closes the session for the given token.
func Unregister(token string) {
	mu.Lock()
	if s, ok := sessions[token]; ok {
		delete(sessions, token)
		close(s.outbound)
	}
	mu.Unlock()
}

// Get returns the write end of the outbound channel for the given token,
// or nil if no session is registered for that token.
func Get(token string) chan<- []byte {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := sessions[token]; ok {
		return s.outbound
	}
	return nil
}

// SetDisplaySettings stores the browser's current display settings for the session.
func SetDisplaySettings(token string, d DisplaySettings) {
	mu.Lock()
	if s, ok := sessions[token]; ok {
		s.display = d
	}
	mu.Unlock()
}

// GetDisplaySettings returns the stored display settings for the session.
func GetDisplaySettings(token string) DisplaySettings {
	mu.Lock()
	defer mu.Unlock()
	if s, ok := sessions[token]; ok {
		return s.display
	}
	return defaultDisplaySettings()
}
