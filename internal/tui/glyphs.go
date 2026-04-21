package tui

const (
	// radioSelected (◉) - Fisheye (U+25C9)
	radioSelected = "\u25C9"
	// radioUnselected (○) - White Circle (U+25CB)
	radioUnselected = "\u25CB"

	// checkSelected (▣) - White Square Containing Black Small Square (U+25A3)
	checkSelected = "\u25A3"
	// checkUnselected (□) - White Square (U+25A1)
	checkUnselected = "\u25A1"

	// subMenuExpanded (▼) - Black Down-Pointing Triangle (U+25BC) — group header is always expanded
	subMenuExpanded      = "\u25BC"
	subMenuExpandedAscii = "[v]"

	// ASCII variants
	radioSelectedAscii   = "(*)"
	radioUnselectedAscii = "( )"
	checkSelectedAscii   = "[x]"
	checkUnselectedAscii = "[ ]"

	invalidMarker        = "!"
	invalidMarkerAscii   = "!"

	// lockedMarker (✗) - Ballot X (U+2717) + VS15 (\uFE0E) for text style
	lockedMarker      = "\u2717\uFE0E"
	lockedMarkerAscii = "!"
)
