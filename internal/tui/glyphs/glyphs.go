package glyphs

const (
	// RadioSelected (◉) - Fisheye (U+25C9)
	RadioSelected = "◉"
	// RadioUnselected (○) - White Circle (U+25CB)
	RadioUnselected = "○"

	// CheckSelected (▣) - White Square Containing Black Small Square (U+25A3)
	CheckSelected = "▣"
	// CheckUnselected (□) - White Square (U+25A1)
	CheckUnselected = "□"

	// SubMenuExpanded (▼) - Black Down-Pointing Triangle (U+25BC) — group header is always expanded
	SubMenuExpanded      = "▼"
	SubMenuExpandedAscii = "[v]"

	// ASCII variants
	RadioSelectedAscii   = "(*)"
	RadioUnselectedAscii = "( )"
	CheckSelectedAscii   = "[x]"
	CheckUnselectedAscii = "[ ]"

	// InvalidMarker (!) - marks an item whose backing resource could not be parsed
	InvalidMarker      = "!"
	InvalidMarkerAscii = "!"

	// LockedMarker (✗) - Ballot X (U+2717) + VS15 (︎) for text style
	LockedMarker      = "✗︎"
	LockedMarkerAscii = "!"
)
