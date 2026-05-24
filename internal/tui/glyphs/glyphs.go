package glyphs

const (
	// RadioSelected (•) - Bullet (U+2022) in parens
	RadioSelected = "(•)"
	// RadioUnselected - empty parens
	RadioUnselected = "( )"

	// CheckSelected (✓) - Check Mark (U+2713) in brackets
	CheckSelected = "[✓]"
	// CheckUnselected - empty brackets
	CheckUnselected = "[ ]"

	// SubMenuExpanded (▼) - Black Down-Pointing Triangle (U+25BC) — group header is always expanded
	SubMenuExpanded      = "▼"
	SubMenuExpandedAscii = "v"

	// ASCII variants
	RadioSelectedAscii   = "(*)"
	RadioUnselectedAscii = "( )"
	CheckSelectedAscii   = "[x]"
	CheckUnselectedAscii = "[ ]"

	// InvalidMarker (▲) - Black Up-Pointing Triangle (U+25B2)
	InvalidMarker      = "▲"
	InvalidMarkerAscii = "!"

	// LockedMarker (×) - Multiplication Sign (U+00D7)
	LockedMarker      = "×"
	LockedMarkerAscii = "!"

	// Title bar widgets
	HelpWidget       = "?"
	CloseWidget      = "×" // Multiplication Sign (U+00D7)
	CloseWidgetAscii = "X"

	// Panel resize widgets
	ResizeUpWidget      = "▲" // Black Up-Pointing Triangle (U+25B2)
	ResizeUpWidgetAscii = "^"
	ResizeDnWidget      = "▼" // Black Down-Pointing Triangle (U+25BC)
	ResizeDnWidgetAscii = "v"
)
