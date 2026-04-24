package graphics



// CanDisplayGraphics returns true if the current environment is known to support 
// high-fidelity terminal graphics (Kitty or Sixel).
func CanDisplayGraphics() bool {
	// TODO: Implement real detection (e.g. via DSR sequences or known TERM vars)
	return false 
}
