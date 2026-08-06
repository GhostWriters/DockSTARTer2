//go:build windows

package console

// viewportSendSignal is a no-op on Windows — the viewport is not active there.
func viewportSendSignal(_ byte) {}
