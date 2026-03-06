package tui

import (
	"image/color"
	"strings"

	"DockSTARTer2/internal/strutil"

	"charm.land/lipgloss/v2"
)

// Render3DBorderCtx renders content with 3D border using specific context
func Render3DBorderCtx(content string, padding int, ctx StyleContext) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	maxWidth := 0
	for _, line := range lines {
		w := lipgloss.Width(line)
		if w > maxWidth {
			maxWidth = w
		}
	}

	totalWidth := maxWidth + padding*2
	borderBG := ctx.Dialog.GetBackground()

	lightStyle := lipgloss.NewStyle().
		Foreground(ctx.BorderColor).
		Background(borderBG)

	darkStyle := lipgloss.NewStyle().
		Foreground(ctx.Border2Color).
		Background(borderBG)

	contentStyle := lipgloss.NewStyle().
		Background(borderBG).
		Width(totalWidth)

	border := ctx.Border
	var result strings.Builder

	topLine := lightStyle.Render(border.TopLeft + strutil.Repeat(border.Top, totalWidth) + border.TopRight)
	result.WriteString(topLine)
	result.WriteString("\n")

	paddingStr := strutil.Repeat(" ", padding)
	for _, line := range lines {
		lineWidth := lipgloss.Width(line)
		rightPad := maxWidth - lineWidth
		fullLine := paddingStr + line + strutil.Repeat(" ", rightPad) + paddingStr

		leftBorder := lightStyle.Render(border.Left)
		rightBorder := darkStyle.Render(border.Right)
		styledContent := contentStyle.Width(0).Render(fullLine)

		lineStr := lipgloss.JoinHorizontal(lipgloss.Top, leftBorder, styledContent, rightBorder)
		result.WriteString(lineStr)
		result.WriteString("\n")
	}

	bottomLine := darkStyle.Render(border.BottomLeft + strutil.Repeat(border.Bottom, totalWidth) + border.BottomRight)
	result.WriteString(bottomLine)

	return result.String()
}

// AddPatternHalo surrounds content with a 1-cell halo.
// If haloBg is provided, it uses shared background for a solid look.
func AddPatternHalo(content string, haloBg ...color.Color) string {
	var bg color.Color
	if len(haloBg) > 0 {
		bg = haloBg[0]
	}
	return AddHalo(content, bg)
}

// AddHalo adds a halo effect (uniform outline) to rendered content if shadow is enabled
func AddHalo(content string, haloBg color.Color) string {
	return AddHaloCtx(content, haloBg, GetActiveContext())
}

// AddHaloCtx adds a halo effect using a specific context.
// If haloBg is provided, it renders a solid halo matching that color.
func AddHaloCtx(content string, haloBg color.Color, ctx StyleContext) string {
	// Split content into lines
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Use WidthWithoutZones to get accurate visual width
	contentWidth := 0
	for _, line := range lines {
		w := WidthWithoutZones(line)
		if w > contentWidth {
			contentWidth = w
		}
	}

	var haloCell, horizontalHalo string

	if haloBg != nil {
		// Solid mode: use space characters with the provided background color
		solidStyle := lipgloss.NewStyle().Background(haloBg)
		haloCell = solidStyle.Render("  ")
		horizontalHalo = solidStyle.Render(strutil.Repeat(" ", contentWidth+4))
	} else if ctx.LineCharacters {
		// Unicode mode: use shade characters (░▒▓█)
		haloStyle := ctx.Shadow.Background(ctx.Screen.GetBackground())

		var shadeChar string
		switch ctx.ShadowLevel {
		case 0:
			shadeChar = " "
		case 1:
			shadeChar = "░"
		case 2:
			shadeChar = "▒"
		case 3:
			shadeChar = "▓"
		case 4:
			shadeChar = "█"
		default:
			shadeChar = "▒"
		}

		haloCell = haloStyle.Render(strutil.Repeat(shadeChar, 2))
		horizontalHalo = haloStyle.Render(strutil.Repeat(shadeChar, contentWidth+4))
	} else {
		// ASCII mode
		if ctx.ShadowLevel == 4 {
			solidStyle := lipgloss.NewStyle().Background(ctx.ShadowColor)
			haloCell = solidStyle.Render("  ")
			horizontalHalo = solidStyle.Render(strutil.Repeat(" ", contentWidth+4))
		} else {
			asciiHaloStyle := ctx.Shadow.Background(ctx.Screen.GetBackground())
			var asciiShadeChar string
			switch ctx.ShadowLevel {
			case 0:
				asciiShadeChar = " "
			case 1:
				asciiShadeChar = "."
			case 2:
				asciiShadeChar = ":"
			case 3:
				asciiShadeChar = "#"
			default:
				asciiShadeChar = ":"
			}
			haloCell = asciiHaloStyle.Render(strutil.Repeat(asciiShadeChar, 2))
			horizontalHalo = asciiHaloStyle.Render(strutil.Repeat(asciiShadeChar, contentWidth+4))
		}
	}

	var result strings.Builder

	// Top Halo
	result.WriteString(horizontalHalo)
	result.WriteString("\n")

	// Content with side halos
	for _, line := range lines {
		w := WidthWithoutZones(line)
		padding := ""
		if w < contentWidth {
			padding = strutil.Repeat(" ", contentWidth-w)
		}
		result.WriteString(haloCell)
		result.WriteString(line + padding)
		result.WriteString(haloCell)
		result.WriteString("\n")
	}

	// Bottom Halo
	result.WriteString(horizontalHalo)

	return result.String()
}

// AddShadow adds a shadow effect to rendered content if shadow is enabled
func AddShadow(content string) string {
	return AddShadowCtx(content, GetActiveContext())
}

// GetShadowBoxCtx returns a solid block of shadow characters the same size as the provided content.
// It uses the style and character set from the provided context.
func GetShadowBoxCtx(content string, ctx StyleContext) string {
	contentWidth := WidthWithoutZones(content)
	contentHeight := lipgloss.Height(content)
	if contentWidth <= 0 || contentHeight <= 0 {
		return ""
	}

	themeStyles := GetStyles()
	// Ensure the base shadow style has NO background to maintain transparency
	shadowStyle := themeStyles.Shadow.UnsetBackground()
	var shadeChar string

	if ctx.LineCharacters {
		switch ctx.ShadowLevel {
		case 1:
			shadeChar = "░"
		case 2:
			shadeChar = "▒"
		case 3:
			shadeChar = "▓"
		case 4:
			shadeChar = "█"
		default:
			shadeChar = "▒" // Default to medium
		}
	} else {
		// ASCII mode uses character density
		switch ctx.ShadowLevel {
		case 1:
			shadeChar = "."
		case 2:
			shadeChar = ":"
		case 3:
			shadeChar = "#"
		case 4:
			// For solid ASCII, we'll use a space and reverse the style
			// This allows the shadow color to be the effective background
			// while line 934 blends the "reversed foreground" with the screen.
			shadowStyle = shadowStyle.Foreground(ctx.ShadowColor).Reverse(true)
			shadeChar = " "
		default:
			shadeChar = " "
		}
	}

	// BLENDING FIX: Set the background to match the screen so dither "blends" correctly.
	// This ensures that dithered characters (░▒▓) carry the background color
	// of the surface they are overlaying. Using lipgloss directly avoids
	// trailing background bleeds that string-based MaintainBackground causes.
	shadowStyle = shadowStyle.Background(ctx.Screen.GetBackground())

	// Create the shadow line
	shadowLine := shadowStyle.Render(strings.Repeat(shadeChar, contentWidth))

	var sb strings.Builder
	for i := 0; i < contentHeight; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(shadowLine)
	}

	return sb.String()
}

// AddShadowCtx applies a drop shadow effect to the content by compositing it
// over a solid shadow box with an offset. Note: This creates a single string
// which may have "blank" padding in corners. For full transparency, use
// the compositor and draw the shadow and content as separate layers.
func AddShadowCtx(content string, ctx StyleContext) string {
	if !ctx.DrawShadow {
		return content
	}

	shadowBox := GetShadowBoxCtx(content, ctx)
	if shadowBox == "" {
		return content
	}

	// Create a base layer that covers the entire area (content + shadow offset)
	// and is filled with the screen background to prevent "transparent" leaks.
	contentW := WidthWithoutZones(content)
	contentH := lipgloss.Height(content)

	// Standard shadow offset is 2 right, 1 down
	totalW := contentW + 2
	totalH := contentH + 1

	base := lipgloss.NewStyle().
		Width(totalW).
		Height(totalH).
		Background(ctx.Screen.GetBackground()).
		Render("")

	return MultiOverlay(
		LayerSpec{Content: base, X: 0, Y: 0, Z: 0},
		LayerSpec{Content: shadowBox, X: 2, Y: 1, Z: 1},
		LayerSpec{Content: content, X: 0, Y: 0, Z: 2},
	)
}
