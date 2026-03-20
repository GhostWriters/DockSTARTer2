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

// GetSolidBoxCtx returns a solid block of characters with the provided background color.
func GetSolidBoxCtx(width, height int, bgColor color.Color) string {
	if width <= 0 || height <= 0 || bgColor == nil {
		return ""
	}
	style := lipgloss.NewStyle().Background(bgColor)
	line := style.Render(strings.Repeat(" ", width))
	var sb strings.Builder
	for i := 0; i < height; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line)
	}
	return sb.String()
}

// AddHaloCtx adds a halo effect using a specific context.
func AddHaloCtx(content string, haloBg color.Color, ctx StyleContext) string {
	if content == "" {
		return ""
	}

	contentW := WidthWithoutZones(content)
	contentH := lipgloss.Height(content)

	// Halo extends 2 cells horizontally and 1 line vertically on each side
	haloW := contentW + 4
	haloH := contentH + 2

	var haloBox string
	if haloBg != nil {
		haloBox = GetSolidBoxCtx(haloW, haloH, haloBg)
	} else {
		// Dithered/ASCII mode
		shadowCtx := ctx
		shadowCtx.ShadowLevel = ctx.ShadowLevel
		// Pass a dummy string of the halo size to get the dithered box
		dummy := lipgloss.NewStyle().Width(haloW).Height(haloH).Render("")
		haloBox = GetShadowBoxCtx(dummy, shadowCtx)
	}

	if haloBox == "" {
		return content
	}

	// Create base layer with screen background to prevent leaks
	base := lipgloss.NewStyle().
		Width(haloW).
		Height(haloH).
		Background(ctx.Screen.GetBackground()).
		Render("")

	return MultiOverlay(
		LayerSpec{Content: base, X: 0, Y: 0, Z: 0},
		LayerSpec{Content: haloBox, X: 0, Y: 0, Z: 1},
		LayerSpec{Content: content, X: 2, Y: 1, Z: 2},
	)
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

	// Ensure the base shadow style has NO background to maintain transparency
	shadowStyle := ctx.Shadow.UnsetBackground()
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
			// Solid ASCII: use reverse so ShadowColor becomes the visible cell background.
			shadowStyle = shadowStyle.Foreground(ctx.ShadowColor).Reverse(true)
			shadeChar = " "
		default:
			shadeChar = " "
		}
	}

	// Set the background to match the screen so dither characters blend correctly.
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
