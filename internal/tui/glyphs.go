package tui

const (
	// radioSelected (◉) - Fisheye (U+25C9)
	radioSelected = "◉"
	// radioUnselected (○) - White Circle (U+25CB)
	radioUnselected = "○"

	// checkSelected (▣) - White Square Containing Black Small Square (U+25A3)
	checkSelected = "▣"
	// checkUnselected (□) - White Square (U+25A1)
	checkUnselected = "□"

	// subMenuExpanded (▼) - Black Down-Pointing Triangle (U+25BC) — group header is always expanded
	subMenuExpanded      = "▼"
	subMenuExpandedAscii = "[v]"

	// ASCII variants
	radioSelectedAscii   = "(*)"
	radioUnselectedAscii = "( )"
	checkSelectedAscii   = "[x]"
	checkUnselectedAscii = "[ ]"

	// invalidMarker (!) - marks a menu item whose backing file could not be parsed
	invalidMarker      = "!"
	invalidMarkerAscii = "!"

	// lockedMarker (✗) - Ballot X (U+2717) + VS15 (︎) for text style
	lockedMarker      = "✗︎"
	lockedMarkerAscii = "!"
)
