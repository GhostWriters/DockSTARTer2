package tui

const (
	// vs15 (U+FE0E) forces the preceding character to be monochrome text style.
	vs15 = "\uFE0E"

	// radioSelected (◉) - Fisheye (U+25C9)
	radioSelected = "\u25C9"
	// radioUnselected (○) - White Circle (U+25CB)
	radioUnselected = "\u25CB"

	// checkSelected (▣) - White Square Containing Black Small Square (U+25A3)
	checkSelected = "\u25A3"
	// checkUnselected (□) - White Square (U+25A1)
	checkUnselected = "\u25A1"

	// ASCII variants
	radioSelectedAscii   = "(*) "
	radioUnselectedAscii = "( ) "
	checkSelectedAscii   = "[x] "
	checkUnselectedAscii = "[ ] "
)
