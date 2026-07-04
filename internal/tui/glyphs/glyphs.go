package glyphs

const (
	// RadioOn (•) - Bullet (U+2022) in parens
	RadioOn = "(•)"
	// RadioOff - empty parens
	RadioOff = "( )"

	// CheckOn (✓) - Check Mark (U+2713) in brackets
	CheckOn = "[✓]"
	// CheckOff - empty brackets
	CheckOff = "[ ]"

	// SubMenuExpanded (▾) - Black Down-Pointing Small Triangle (U+25BE), matching
	// the small-triangle family used by the title focus indicators (▸/◂) rather
	// than the larger ▼/▶ triangles, which render as colored emoji in some fonts.
	SubMenuExpanded      = "▾"
	SubMenuExpandedAscii = "v"

	// SubMenuCollapsed (▸) - Black Right-Pointing Small Triangle (U+25B8), same
	// glyph already used elsewhere for focus indicators (dialog_border_box.go,
	// dialog_render.go, menu_borders.go).
	SubMenuCollapsed      = "▸"
	SubMenuCollapsedAscii = ">"

	// ASCII variants
	RadioOnAscii   = "(*)"
	RadioOffAscii  = "( )"
	CheckOnAscii   = "[x]"
	CheckOffAscii  = "[ ]"

	// InvalidMarker (▲) - Black Up-Pointing Triangle (U+25B2)
	InvalidMarker      = "▲"
	InvalidMarkerAscii = "!"

	// LockedMarker (×) - Multiplication Sign (U+00D7)
	LockedMarker      = "×"
	LockedMarkerAscii = "!"

	// Status bar update markers
	UpdateAvailable      = "•" // U+2022 Bullet — update is available
	UpdateAvailableAscii = "*"
	UpdateApplied        = "✓" // U+2713 Check Mark — update was applied, restart pending
	UpdateAppliedAscii   = "!"
	UpdateError          = "▲" // U+25B2 Black Up-Pointing Triangle — update check error
	UpdateErrorAscii     = "?"

	// Title bar widgets
	HelpWidget          = "?"
	CloseWidget         = "×" // Multiplication Sign (U+00D7)
	CloseWidgetAscii    = "X"
	RefreshWidget       = "↺" // Anticlockwise Open Circle Arrow (U+21BA)
	RefreshWidgetAscii  = "R"

	// Panel resize widgets
	ResizeUpWidget      = "▲" // Black Up-Pointing Triangle (U+25B2)
	ResizeUpWidgetAscii = "^"
	ResizeDnWidget      = "▼" // Black Down-Pointing Triangle (U+25BC)
	ResizeDnWidgetAscii = "v"
)
