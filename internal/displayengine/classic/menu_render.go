package classic

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderCheckboxGlyphSplit renders a 3-rune checkbox/radio glyph (e.g. "[✓]",
// "( )", " x "), applying bracketStyle to the outer two characters (the
// brackets/parens, or their blank placeholder when bare) and contentStyle to
// the inner checkmark/bullet/space character.
func renderCheckboxGlyphSplit(cb string, contentStyle, bracketStyle lipgloss.Style) string {
	runes := []rune(cb)
	if len(runes) < 3 {
		return contentStyle.Render(cb)
	}
	return bracketStyle.Render(string(runes[0])) +
		contentStyle.Render(string(runes[1:len(runes)-1])) +
		bracketStyle.Render(string(runes[len(runes)-1]))
}

// checkboxStylePair resolves the content and bracket theme styles for a
// checkbox/radio glyph, from the state-suffixed tag family the user's
// ds2theme defines: Checkbox|Radio + Brackets? + On|Off + Focused?, e.g.
// "CheckboxOn", "RadioBracketsOffFocused". isRadio picks Checkbox vs Radio;
// on picks On vs Off; focused appends the Focused suffix. disabled routes
// both through ResolveDisabledStyle instead (a whole disabled section, or a
// single locked item) -- an explicit "<...>Disabled" tag if the theme
// defines one, else the normal style dimmed.
func checkboxStylePair(isRadio, on, focused, disabled bool) (content, bracket lipgloss.Style) {
	base := "Checkbox"
	if isRadio {
		base = "Radio"
	}
	state := "Off"
	if on {
		state = "On"
	}
	suffix := ""
	if focused {
		suffix = "Focused"
	}
	contentTag := base + state + suffix
	bracketTag := base + "Brackets" + state + suffix
	if disabled {
		content, _ = ResolveDisabledStyle(contentTag)
		bracket, _ = ResolveDisabledStyle(bracketTag)
		return content, bracket
	}
	content = theme.ThemeSemanticStyle("{{|" + contentTag + "|}}")
	bracket = theme.ThemeSemanticStyle("{{|" + bracketTag + "|}}")
	return content, bracket
}

// renderCheckbox selects the correct glyph for a checkbox or radio button and renders it.
//
// focused is the row's actual keyboard-focus state; brackets/parens always
// show when true, regardless of mode -- otherwise the cursor position would
// be illegible in "never" mode. mode is the user's ui.checkbox_brackets or
// ui.radio_brackets setting ("never", "selected", or "always"):
//
//	"never"    -- only the focused row is bracketed.
//	"selected" -- bracketed when focused OR checked (the historical default).
//	"always"   -- every row is bracketed.
//
// When brackets are hidden, the glyph renders "bare" (no brackets/parens,
// same width, but the checkmark/bullet itself still shows if checked).
// Callers rendering a flow/grid list should pass mode="always" (and
// focused=true) to keep that list's checkboxes/radios bracketed
// unconditionally, matching how flow lists rendered before this -- flow
// items don't respect the user setting.
//
// contentStyle/bracketStyle are typically resolved via checkboxStylePair,
// separated so a caller can drive bracket visibility (mode/focused above)
// independently from which style pair colors the glyph -- e.g. a flow list
// forces brackets on unconditionally but still wants the color to reflect
// real keyboard focus, not the forced bracket state.
func renderCheckbox(isRadio, checked, lineChars, focused bool, mode string, contentStyle, bracketStyle lipgloss.Style) string {
	showBrackets := focused
	if !showBrackets {
		switch mode {
		case "always":
			showBrackets = true
		case "selected":
			showBrackets = checked
		}
		// "never" (or any unrecognized value) leaves showBrackets false.
	}

	var cb string
	if lineChars {
		switch {
		case isRadio:
			cb = radioOffBare
			if checked {
				cb = radioOnBare
			}
			if showBrackets {
				cb = radioOff
				if checked {
					cb = radioOn
				}
			}
		default:
			cb = checkOffBare
			if checked {
				cb = checkOnBare
			}
			if showBrackets {
				cb = checkOff
				if checked {
					cb = checkOn
				}
			}
		}
	} else {
		switch {
		case isRadio:
			cb = radioOffBareAscii
			if checked {
				cb = radioOnBareAscii
			}
			if showBrackets {
				cb = radioOffAscii
				if checked {
					cb = radioOnAscii
				}
			}
		default:
			cb = checkOffBareAscii
			if checked {
				cb = checkOnBareAscii
			}
			if showBrackets {
				cb = checkOffAscii
				if checked {
					cb = checkOnAscii
				}
			}
		}
	}
	return renderCheckboxGlyphSplit(cb, contentStyle, bracketStyle)
}

// listScrollPercent returns the current scroll position in [0.0, 1.0] for the list.
func (m *MenuModel) listScrollPercent() float64 {
	var offset, total int
	if m.variableHeight {
		offset = m.ViewStartY
		total = m.lastScrollTotal
	} else if m.FlowColumns >= 2 && m.MaxFlowRows > 0 {
		offset = m.ViewStartY
		total = m.ScrollTotal()
	} else {
		offset = m.ViewStartY
		total = len(m.items)
	}
	maxOff := total - m.Layout.ViewportHeight
	if maxOff <= 0 {
		return 1.0
	}
	pct := float64(offset) / float64(maxOff)
	if pct > 1.0 {
		pct = 1.0
	}
	return pct
}

// ViewString renders the menu content as a string (for compositing)
func (m *MenuModel) ViewString() string {
	if m.width == 0 {
		return ""
	}

	// Return cached view if the state hasn't changed since the last render
	if cachedView, valid := m.CheckCache(); valid {
		return cachedView
	}

	// Plain-text kind: a single borderless, theme-styled line -- e.g. a
	// dialog's subtitle expressed as its own content section. Checked before
	// subMenuMode since a plain-text section never wants viewSubMenu's border.
	if m.plainText != "" {
		return m.viewPlainText()
	}

	// Borderless contentRenderer sections (e.g. a header or streaming
	// viewport section that renders its own fully self-contained content,
	// optionally with its own inner border) skip viewSubMenu's outer
	// bordered-box wrap entirely -- same bypass plainText uses, generalized
	// for content that isn't a single themed text line.
	if m.borderless && m.ContentRenderer != nil {
		return m.SaveCache(m.ContentRenderer(m.width))
	}

	// In Sub-menu mode, we render a simpler view without the global backdrop logic
	if m.subMenuMode {
		return m.viewSubMenu()
	}

	// Sections-based layout: stack sub-menus inside the outer border.
	if len(m.contentSections) > 0 {
		return m.viewWithSections()
	}

	if m.flowMode {
		return m.renderFlow()
	}

	styles := GetStyles()
	ctx := GetActiveContext()

	// 1. Render core list content with scrollbar
	listView := m.renderVerticalListBlock(ctx)

	// 2. Wrap list in its own inner border (only for non-subMenu mode)
	// Full dialogs use a nested "border-in-border" look.
	listStyle := styles.Dialog.Padding(0, 0, 0, 0)
	listStyle = ApplyInnerBorder(listStyle, m.focused, ctx.LineCharacters)
	listStyle = listStyle.BorderBottom(false)
	borderedList := InjectBorderFlags(listStyle.Render(listView), styles.BorderFlags, styles.Border2Flags, false)
	totalWidth := m.list.Width() + ScrollbarGutterWidth + 2
	borderedList = strings.TrimSuffix(borderedList, "\n")

	// 3. Add bottom border (AE or Scroll Percent)
	showAEFocus := m.focused && m.loadingText == "" && !m.SelectedItem().IsSubItem && !m.SelectedItem().IsAddInstance && !m.SelectedItem().IsEditing
	if m.groupedMode {
		pct := -1.0
		if m.Scroll.Info.Needed {
			pct = m.listScrollPercent()
		}
		borderedList = borderedList + "\n" + BuildAEBottomBorder(totalWidth, 2, showAEFocus, m.activeColumn, pct, ctx)
	} else if m.Scroll.Info.Needed {
		borderedList = borderedList + "\n" + BuildScrollPercentBottomBorder(totalWidth, m.listScrollPercent(), m.focused, ctx)
	} else {
		borderedList = borderedList + "\n" + BuildPlainBottomBorder(totalWidth, m.focused, ctx)
	}

	// 4. Add AE top border
	if m.groupedMode {
		if nl := strings.Index(borderedList, "\n"); nl >= 0 {
			borderedList = BuildAETopBorder(totalWidth, 2, showAEFocus, m.activeColumn, ctx) + borderedList[nl:]
		}
	}

	// 5. Build Content Area
	layout := GetLayout()
	contentWidth := m.GetInnerContentWidth()
	innerBoxWidth := contentWidth - layout.ContentMarginWidth()

	buttonRow := m.renderSimpleButtons(innerBoxWidth)
	borderedButtonBox := m.renderButtonBox(buttonRow, innerBoxWidth)

	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin)

	paddedList := marginStyle.Width(contentWidth).Render(borderedList)
	paddedButtons := marginStyle.Width(contentWidth).Render(borderedButtonBox)

	var innerParts []string
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, layout.ContentSideMargin).
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText("{{|Subtitle|}}"+m.subtitle, styles.Dialog)
		innerParts = append(innerParts, subtitleStyle.Render(subStr))
	}
	innerParts = append(innerParts, paddedList)
	innerParts = append(innerParts, paddedButtons)

	// Combine all parts
	content := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// Force total content height to match the calculated budget
	// only if maximized. Otherwise it should have its intrinsic height.
	if m.maximized {
		heightBudget := m.Layout.Height - layout.BorderHeight()
		if m.Layout.LargeTitleBar {
			heightBudget -= LargeTitleBarOverhead
		}
		if heightBudget > 0 {
			content = lipgloss.NewStyle().
				Height(heightBudget).
				Background(styles.Dialog.GetBackground()).
				Render(content)
		}
	}

	// Determine target height for the outer dialog
	targetHeight := 0
	if m.maximized {
		targetHeight = m.height
	}

	// Wrap in bordered dialog with title embedded in border
	var dialog string
	if m.title != "" {
		dialog = m.renderBorderWithTitle(content, contentWidth, targetHeight, m.focused, m.borderStyle == BorderStyleRounded, "Title")
	} else {
		// No title: use focus-aware inner rounded border
		// We must ensure the style width accounts for the layout's actual visual borders
		dialogStyle := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Width(contentWidth + layout.BorderWidth())

		dialogStyle = ApplyInnerBorder(dialogStyle, m.focused, styles.LineCharacters)
		if targetHeight > 0 {
			dialogStyle = dialogStyle.Height(targetHeight)
		}
		dialog = InjectBorderFlags(dialogStyle.Render(content), styles.BorderFlags, styles.Border2Flags, true)
	}

	// Save to cache before returning
	return m.SaveCache(dialog)
}

// Layers returns a single layer with the menu content for visual compositing
func (m *MenuModel) Layers() []*lipgloss.Layer {
	return []*lipgloss.Layer{
		lipgloss.NewLayer(m.ViewString()).Z(ZScreen).ID(m.id),
	}
}

// SetIsDialog marks the menu as a modal dialog, raising its hit-region Z priority above screen regions
func (m *MenuModel) SetIsDialog(isDialog bool) {
	m.isDialog = isDialog
}

// MinHeight returns the minimum content-area height for this menu to remain usable
// (buttons visible with the viewport at its 3-line clamp floor). Only meaningful for
// maximized menus; returns 0 for non-maximized so MinDialogHeight applies instead.
// Used by AppModel to limit log panel expansion.
//
// Derived from the inverse of calculateLayout's maxListHeight formula:
//
//	flat-button path: maxListHeight = h - 5 - subtitleH  →  h = 8 + subtitleH at floor=3
//	no-button path:   maxListHeight = h - 4 - subtitleH  →  h = 7 + subtitleH at floor=3
func (m *MenuModel) MinHeight() int {
	if !m.maximized {
		return 0
	}
	subtitleH := m.Layout.SubtitleHeight // real rendered height stored by calculateLayout
	base := 8 + subtitleH                // flat-button minimum (bordered buttons drop to flat first)
	if !m.showButtons {
		base = 7 + subtitleH
	}
	// Large titlebar is part of the overhead; use the stored decision so the
	// adaptive fallback in calculateLayout and the panel ceiling stay in sync.
	if m.Layout.LargeTitleBar {
		base += LargeTitleBarOverhead
	}
	return base
}

func (m *MenuModel) renderBorderWithTitle(content string, contentWidth int, targetHeight int, focused bool, rounded bool, titleTag string) string {
	align := GetActiveContext().DialogTitleAlign
	if m.subMenuMode {
		align = GetActiveContext().SubmenuTitleAlign
		if m.disabled {
			titleTag = "TitleSubMenuDisabled"
		} else if focused {
			titleTag = "TitleSubMenuFocused"
		} else {
			titleTag = "TitleSubMenu"
		}
	}

	if !m.subMenuMode {
		switch m.dialogType {
		case DialogTypeConfirm:
			titleTag = "TitleQuestion"
		case DialogTypeWarning:
			titleTag = "TitleWarning"
		case DialogTypeError:
			titleTag = "TitleError"
		case DialogTypeSuccess:
			titleTag = "TitleSuccess"
		}
	}

	ctx := GetActiveContext()
	ctx.Type = m.dialogType
	ctx.AngledBorder = m.borderStyle == BorderStyleAngled
	ctx.SquareBorder = m.borderStyle == BorderStyleSquare
	// Use pre-computed layout decision; submenus always use small titlebar.
	ctx.LargeTitleBars = m.Layout.LargeTitleBar
	if m.disabled {
		ctx.BorderColor = ctx.BorderDisabledColor
		ctx.Border2Color = ctx.Border2DisabledColor
		ctx.BorderFlags = ctx.BorderDisabledFlags
		ctx.Border2Flags = ctx.Border2DisabledFlags
	}
	tbs := m.State()
	tbs.Show = m.title != "" && !m.subMenuMode
	if m.titleSpinnerIndicator != nil {
		tbs.SpinnerIndicator, tbs.SpinnerIndicatorRight = m.titleSpinnerIndicator()
	} else if m.loadingText != "" {
		tbs.SpinnerIndicator, tbs.SpinnerIndicatorRight = m.titleSpinner.Indicators()
	}
	rendered := RenderBorderedBoxCtx(m.title, content, contentWidth, targetHeight, focused || m.TitleBarFocused(), true, rounded, align, titleTag, ctx, tbs)
	if m.bottomBorderLabel != "" {
		lines := strings.Split(rendered, "\n")
		if n := len(lines); n > 0 {
			// BuildLabeledBottomBorderCtx wants the box's full visual width
			// (including the two side border columns), unlike
			// RenderBorderedBoxCtx's contentWidth which is inner-only.
			lines[n-1] = BuildLabeledBottomBorderCtx(contentWidth+GetLayout().BorderWidth(), m.bottomBorderLabel, focused || m.TitleBarFocused(), ctx)
			rendered = strings.Join(lines, "\n")
		}
	}
	return rendered
}

func (m *MenuModel) viewSubMenu() string {
	styles := GetStyles()
	layout := GetLayout()
	ctx := GetActiveContext()

	// The target outer dimensions
	contentWidth := m.width - layout.BorderWidth()

	// 1. Build inner content components
	var innerParts []string
	if m.subtitle != "" {
		subtitleStyle := styles.Dialog.
			Width(contentWidth).
			Padding(0, layout.ContentSideMargin).
			Align(lipgloss.Left).
			Border(lipgloss.Border{})

		subStr := RenderThemeText("{{|Subtitle|}}"+m.subtitle, styles.Dialog)
		innerParts = append(innerParts, subtitleStyle.Render(subStr))
	}

	// Render core list with scrollbar (or flow layout if flowMode is set)
	if m.ContentRenderer != nil {
		innerParts = append(innerParts, m.ContentRenderer(contentWidth))
	} else if m.flowMode {
		innerParts = append(innerParts, m.renderFlowContent(contentWidth))
	} else {
		content := m.renderVerticalListBlock(ctx)
		leftPad := layout.ContentSideMargin
		if m.noLeftMargin {
			leftPad = 0
		}
		paddedContent := lipgloss.NewStyle().
			Padding(0, 0, 0, leftPad).
			Width(contentWidth).
			Render(content)
		innerParts = append(innerParts, paddedContent)
	}

	// Render buttons (if any)
	buttons := m.GetButtonSpecsForState()
	if len(buttons) > 0 {
		useBorders := m.Layout.ButtonHeight == DialogButtonHeight
		buttonView := renderCenteredButtonsImpl(contentWidth, useBorders, ctx, buttons...)
		innerParts = append(innerParts, buttonView)
	}

	combined := lipgloss.JoinVertical(lipgloss.Left, innerParts...)

	// 2. Wrap in bordered dialog
	targetHeight := lipgloss.Height(combined) + 2
	if m.maximized {
		targetHeight = m.height
	} else if targetHeight > m.height {
		targetHeight = m.height
	}
	result := m.renderBorderWithTitle(combined, contentWidth, targetHeight, m.focusedSub, true, "Title")

	// 3. Replace bottom border with scroll-percent indicator if needed
	if (!m.flowMode || m.MaxFlowRows > 0) && m.Scroll.Info.Needed {
		if lastNL := strings.LastIndex(result, "\n"); lastNL >= 0 {
			bottomLine := BuildScrollPercentBottomBorder(m.width, m.listScrollPercent(), m.focusedSub, ctx)
			result = result[:lastNL+1] + bottomLine
		}
	}
	return result
}

// viewPlainText renders a single line of theme-styled text with no border --
// the plain-text Content kind's render path. Not routed through
// viewSubMenu/renderBorderWithTitle since this kind never wants a box.
func (m *MenuModel) viewPlainText() string {
	return m.renderPlainText(m.width)
}

// renderPlainText renders the plain-text kind's content at an explicit width,
// so SectionHeight can measure it at its post-layout sectionWidth without
// depending on m.width having already been assigned via SetSize.
func (m *MenuModel) renderPlainText(width int) string {
	styles := GetStyles()
	layout := GetLayout()
	leftPad := layout.ContentSideMargin
	if m.noLeftMargin {
		leftPad = 0
	}
	textStyle := styles.Dialog.
		Width(width).
		Padding(0, leftPad).
		Align(lipgloss.Left)
	rendered := textStyle.Render(RenderThemeText(m.plainTextThemeTag+m.plainText, styles.Dialog))
	if m.plainTextVPad > 0 {
		blank := textStyle.Render("")
		lines := make([]string, 0, m.plainTextVPad*2+1)
		for i := 0; i < m.plainTextVPad; i++ {
			lines = append(lines, blank)
		}
		lines = append(lines, rendered)
		for i := 0; i < m.plainTextVPad; i++ {
			lines = append(lines, blank)
		}
		rendered = lipgloss.JoinVertical(lipgloss.Left, lines...)
	}
	return rendered
}

// renderVerticalListBlock renders the core list content and applies the scrollbar.
// This is the single source of truth for list viewport rendering.
func (m *MenuModel) renderVerticalListBlock(ctx StyleContext) string {
	if m.loadingText != "" {
		styles := GetStyles()
		h := m.Layout.ViewportHeight
		if h < 1 {
			h = 1
		}
		w := m.list.Width()
		centered := lipgloss.NewStyle().
			Background(styles.Dialog.GetBackground()).
			Foreground(styles.DialogTitle.GetForeground()).
			Width(w).
			Height(h).
			Align(lipgloss.Center, lipgloss.Center).
			Render(m.loadingText)
		return ApplyScrollbar(&m.Scroll, centered, 0, h, 0, ctx.LineCharacters, ctx)
	}

	content := m.renderVariableHeightList()

	// Ensure content is exactly ViewportHeight lines before applying the scrollbar,
	// so the gutter column spans the full border box.
	content = strings.TrimSuffix(content, "\n")
	h := strings.Count(content, "\n") + 1
	if h < m.Layout.ViewportHeight {
		content += strutil.Repeat("\n", m.Layout.ViewportHeight-h)
	}

	total := len(m.items)
	if m.variableHeight {
		total = m.lastScrollTotal
	}
	return ApplyScrollbar(&m.Scroll, content, total, m.Layout.ViewportHeight, m.ViewStartY, ctx.LineCharacters, ctx)
}

// viewWithSections renders an outer dialog that stacks content sections (sub-menus)
// vertically inside its border, followed by a standard button row.
// This path is taken when m.contentSections is non-empty and m.subMenuMode is false.
func (m *MenuModel) viewWithSections() string {
	layout := GetLayout()
	styles := GetStyles()
	// Content width is the space inside the outer border.
	contentWidth := m.width - layout.BorderWidth()
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Sections are rendered at the inset width; the margin brings them back to contentWidth.
	marginStyle := lipgloss.NewStyle().
		Background(styles.Dialog.GetBackground()).
		Padding(0, layout.ContentSideMargin)
	sectionWidth := contentWidth - layout.ContentMarginWidth()

	var parts []string

	// Stack sections with margin — each section already renders its own bordered panel.
	for _, sec := range m.contentSections {
		// Use the rendered string without trimming trailing newlines that are part of the height budget.
		v := sec.ViewString()
		if v != "" {
			parts = append(parts, marginStyle.Render(v))
		}
	}

	// Button row also inset by the same margin -- omitted entirely when
	// buttons are hidden (m.showButtons false), so no empty reserved row
	// remains and the expandable section correctly reclaims that space
	// (see calculateSectionLayout's buttonHeight/buttonBudget, both 0 in
	// that case).
	if m.showButtons {
		buttonRowRaw := m.renderSimpleButtons(sectionWidth)
		if m.Layout.ButtonHeight > 1 {
			buttonRowRaw = lipgloss.NewStyle().
				Height(m.Layout.ButtonHeight).
				Align(lipgloss.Center, lipgloss.Center).
				Render(buttonRowRaw)
		}
		buttonRow := marginStyle.Render(buttonRowRaw)
		parts = append(parts, buttonRow)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	return m.SaveCache(
		m.renderBorderWithTitle(content, contentWidth, m.height, m.focused, m.borderStyle == BorderStyleRounded, "Title"),
	)
}
