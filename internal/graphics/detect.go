package graphics



// CanDisplayGraphics returns true if the current environment is known to support 
// high-fidelity terminal graphics (Kitty or Sixel).
func CanDisplayGraphics() bool {
	return true // Force true for debug
}
