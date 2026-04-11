package tui

import (
	"DockSTARTer2/internal/strutil"
	"DockSTARTer2/internal/theme"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

// MenuItem defines an item in a menu
type MenuItem struct {
	Tag         string  // Display name (first letter used as shortcut)
	Desc        string  // Description text
	Help        string  // Help line text shown when item is selected
	Shortcut    rune    // Keyboard shortcut (usually first letter of Tag)
	Action      tea.Cmd // Command to execute when selected (Enter)
	SpaceAction tea.Cmd // Command to execute when Space is pressed

	// Checklist support
	Selectable    bool // Whether this item can be toggled
	Selected      bool // Current selection state
	IsCheckbox    bool // Whether this is a checkbox [ ] / [x]
	IsRadioButton bool // Whether this is a radio button ( ) / (*)
	Checked       bool // Current checkbox/radio state (= "Added" in app-selection)

	// Enabled state (app-selection): separate from Added (Checked)
	// Enabled means APP__ENABLED='true' in .env
	Enabled    bool // Current enabled state
	WasEnabled bool // Enabled state when the screen loaded (for gutter diff)

	// Layout support
	IsSeparator bool // Whether this is a non-selectable header/separator

	// Grouped list support (app selection with instances)
	IsGroupHeader  bool   // App name header row; checkbox shows group-enabled state (read-only)
	IsSubItem      bool   // Indented instance row under a group header
	IsAddInstance  bool   // "[+] Add instance…" action row
	IsEditing      bool   // Inline text-input row for new instance name entry
	IsNew          bool   // Newly added this session (not yet saved; used to allow rename)
	IsReferenced   bool   // Has env vars / compose reference but no __ENABLED; locked from rename
	WasAdded       bool   // Whether this item was added (present in .env) when the screen loaded (for gutter diff)
	ShowEnabledGutter bool // Whether to show the Enabled (E/D) gutter column
	BaseApp        string // Base app name this row belongs to (sub-items / add-instance / editing)

	// Metadata
	IsUserDefined bool              // Whether this is a user-defined app (for coloring)
	IsInvalid     bool              // Whether this item is invalid (e.g. broken theme)
	Metadata      map[string]string // Optional extra data (e.g. internal app name)
}

// Implement list.Item interface for bubbles/list
func (i MenuItem) FilterValue() string { return i.Tag }
func (i MenuItem) Title() string       { return i.Tag }
func (i MenuItem) Description() string { return i.Desc }

// calculateMaxTagLength returns the maximum visual width of item tags
func calculateMaxTagLength(items []MenuItem) int {
	maxTagLen := 0
	for _, item := range items {
		tagWidth := lipgloss.Width(GetPlainText(item.Tag))
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
	}
	return maxTagLen
}

// calculateMaxTagAndDescLength returns the maximum visual width of item tags and descriptions
func calculateMaxTagAndDescLength(items []MenuItem) (maxTagLen, maxDescLen int) {
	for _, item := range items {
		tagWidth := lipgloss.Width(GetPlainText(item.Tag))
		descWidth := lipgloss.Width(GetPlainText(item.Desc))
		if tagWidth > maxTagLen {
			maxTagLen = tagWidth
		}
		if descWidth > maxDescLen {
			maxDescLen = descWidth
		}
	}
	return
}

// menuItemDelegate implements list.ItemDelegate for standard navigation menus
type menuItemDelegate struct {
	menuID    string
	maxTagLen int
	focused   bool
	flowMode  bool
}

func (d menuItemDelegate) Height() int                             { return 1 }
func (d menuItemDelegate) Spacing() int                            { return 0 }
func (d menuItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d menuItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	// Handle separator items
	if menuItem.IsSeparator {
		// Set width to exactly m.Width() so inner text of m.Width()-2 plus 2 chars padding fits perfectly without wrapping
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = RenderThemeText(menuItem.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
		} else {
			content = strutil.Repeat("─", m.Width()-2)
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	itemStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")

	if isSelected {
		itemStyle = theme.ThemeSemanticStyle("{{|ItemSelected|}}")
		tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
		keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	}

	// Render tag with first-letter highlighting (if no semantic tags present)
	tag := menuItem.Tag
	var tagStr string
	if len(tag) > 0 {
		// If tag already contains theme tags, render it normally (highlights might be ruined)
		if strings.Contains(tag, "{{") {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			runes := []rune(tag)
			letterIdx := 0
			if strings.HasPrefix(tag, "[") && len(runes) > 1 {
				letterIdx = 1
			}
			if letterIdx < len(runes) {
				tagStr = tagStyle.Render(string(runes[:letterIdx])) + keyStyle.Render(string(runes[letterIdx])) + tagStyle.Render(string(runes[letterIdx+1:]))
			} else {
				tagStr = RenderThemeText(tag, tagStyle)
			}
		}
	}

	// tagWidth removed as it was unused

	// Checkbox visual [ ] or [x] / Radio visual ( ) or (*)
	checkbox := ""
	if menuItem.IsInvalid {
		var cb string
		if ctx.LineCharacters {
			cb = invalidMarker
		} else {
			cb = invalidMarkerAscii
		}
		checkbox = theme.ThemeSemanticStyle("{{|MarkerInvalid|}}").Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsRadioButton {
		var cb string
		if ctx.LineCharacters {
			cb = radioUnselected
			if menuItem.Checked {
				cb = radioSelected
			}
		} else {
			cb = radioUnselectedAscii
			if menuItem.Checked {
				cb = radioSelectedAscii
			}
		}
		// Render just the glyph with tagStyle, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsCheckbox {
		var cb string
		if ctx.LineCharacters {
			cb = checkUnselected
			if menuItem.Checked {
				cb = checkSelected
			}
		} else {
			cb = checkUnselectedAscii
			if menuItem.Checked {
				cb = checkSelectedAscii
			}
		}
		// Render just the glyph with tagStyle, and add a neutral space after it
		checkbox = tagStyle.Render(cb) + neutralStyle.Render(" ")
	}

	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))

	// Calculate checkbox/radio width dynamically
	cbWidth := lipgloss.Width(GetPlainText(checkbox))

	// Available width: list width - outer padding(2) - (cbWidth + maxTagLen + 3)
	availableWidth := m.Width() - 2 - (cbWidth + d.maxTagLen + 3)
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, itemStyle)
	// Use TruncateRight for proper truncation instead of MaxWidth which wraps
	descLine := TruncateRight(descStr, availableWidth)

	// Build the line
	line := checkbox + tagStr + gapStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += gapStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
	line = lineStyle.Render(line)
	fmt.Fprint(w, line)

}

// checkboxItemDelegate implements specialized styling for app selection screens
type checkboxItemDelegate struct {
	menuID    string
	maxTagLen int
	focused   bool
	flowMode  bool
}

func (d checkboxItemDelegate) Height() int                             { return 1 }
func (d checkboxItemDelegate) Spacing() int                            { return 0 }
func (d checkboxItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d checkboxItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = RenderThemeText(menuItem.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
		} else {
			content = strutil.Repeat("─", max(0, m.Width()-2))
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)

	itemStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")

	if isSelected {
		itemStyle = theme.ThemeSemanticStyle("{{|ItemSelected|}}")
		tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
		keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	}

	// Render checkbox for selectable items
	var checkbox string
	if menuItem.IsInvalid {
		var cb string
		if ctx.LineCharacters {
			cb = invalidMarker
		} else {
			cb = invalidMarkerAscii
		}
		checkbox = theme.ThemeSemanticStyle("{{|MarkerInvalid|}}").Render(cb) + neutralStyle.Render(" ")
	} else if menuItem.IsCheckbox {
		if ctx.LineCharacters {
			cbGlyph := checkUnselected
			if menuItem.Checked {
				cbGlyph = checkSelected
			}
			// Use tag style for checkbox to match user request
			// Render just the glyph with tagStyle, and add a neutral space after it
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := checkUnselectedAscii
			if menuItem.Checked {
				cbContent = checkSelectedAscii
			}
			// Use tag style for checkbox to match user request
			checkbox = tagStyle.Render(cbContent) + neutralStyle.Render(" ")
		}
	} else if menuItem.IsRadioButton {
		if ctx.LineCharacters {
			cbGlyph := radioUnselected
			if menuItem.Checked {
				cbGlyph = radioSelected
			}
			checkbox = tagStyle.Render(cbGlyph) + neutralStyle.Render(" ")
		} else {
			cbContent := radioUnselectedAscii
			if menuItem.Checked {
				cbContent = radioSelectedAscii
			}
			checkbox = tagStyle.Render(cbContent) + neutralStyle.Render(" ")
		}
	}

	var tagStr string
	tag := menuItem.Tag
	if len(tag) > 0 {
		if isSelected {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			if strings.Contains(tag, "{{") {
				tagStr = RenderThemeText(tag, tagStyle)
			} else {
				runes := []rune(tag)
				letterIdx := 0
				if strings.HasPrefix(tag, "[") && len(runes) > 1 {
					letterIdx = 1
				}
				if letterIdx < len(runes) {
					tagStr = tagStyle.Render(string(runes[:letterIdx])) + keyStyle.Render(string(runes[letterIdx])) + tagStyle.Render(string(runes[letterIdx+1:]))
				} else {
					tagStr = RenderThemeText(tag, tagStyle)
				}
			}
		}
	}

	// tagWidth removed as it was unused

	// Highlighting for gap and description
	// Use itemStyle as base for description so highlight applies, or dialogBG if not selected
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected {
		descStyle = itemStyle
	}
	// Whitespace (gaps and trailing) should always use neutral background
	gapStyle := neutralStyle

	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
	// Available width: list width - outer padding(2) - (cbWidth + maxTagLen + 3)
	// For checkboxItemDelegate, we assume cbWidth is 2 ([ ]) or 4 ([ ] ) depending on characters
	cbWidth := 2
	if !ctx.LineCharacters {
		cbWidth = 4
	}
	availableWidth := m.Width() - 2 - (cbWidth + d.maxTagLen + 3)
	if availableWidth < 0 {
		availableWidth = 0
	}

	descStr := RenderThemeText(menuItem.Desc, descStyle)
	// Use TruncateRight for proper truncation instead of MaxWidth which wraps
	descLine := TruncateRight(descStr, availableWidth)

	// Build the line
	line := checkbox + tagStr + gapStyle.Render(paddingSpaces) + descLine

	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += gapStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
	}

	lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
	line = lineStyle.Render(line)
	fmt.Fprint(w, line)

}

// groupedItemDelegate renders the hierarchical app-selection list:
//   - IsGroupHeader rows: app label with read-only group-enabled checkbox + description
//   - IsSubItem rows:     indented instance checkbox rows
//   - IsAddInstance rows: indented "[+] Add instance…" action label
//   - IsEditing rows:     indented inline text-input display (Tag holds current text + cursor)
//   - IsSeparator rows:   unchanged (letter headers / blank spacers)
type groupedItemDelegate struct {
	menuID    string
	maxTagLen int // max tag width of header rows only
	focused   bool
	activeCol CheckboxColumn
}

func (d groupedItemDelegate) Height() int                             { return 1 }
func (d groupedItemDelegate) Spacing() int                            { return 0 }
func (d groupedItemDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }

func (d groupedItemDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	menuItem, ok := item.(MenuItem)
	if !ok {
		return
	}

	ctx := GetActiveContext()
	dialogBG := ctx.Dialog.GetBackground()
	isSelected := index == m.Index()

	// Highlight the parent header if a child item is selected
	isParentOfSelected := false
	if menuItem.IsGroupHeader {
		if selItemRaw := m.SelectedItem(); selItemRaw != nil {
			if selItem, ok := selItemRaw.(MenuItem); ok {
				if (selItem.IsSubItem || selItem.IsAddInstance || selItem.IsEditing) && selItem.BaseApp == menuItem.BaseApp {
					isParentOfSelected = true
				}
			}
		}
	}

	// Separator rows (letter headers and blank spacers)
	if menuItem.IsSeparator {
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		var content string
		if menuItem.Tag != "" {
			content = RenderThemeText(menuItem.Tag, theme.ThemeSemanticStyle("{{|TagKey|}}"))
		} else {
			content = strutil.Repeat("─", max(0, m.Width()-2))
		}
		fmt.Fprint(w, lineStyle.Render(content))
		return
	}

	neutralStyle := lipgloss.NewStyle().Background(dialogBG)
	tagStyle := theme.ThemeSemanticStyle("{{|Tag|}}")
	itemStyle := theme.ThemeSemanticStyle("{{|Item|}}")
	keyStyle := theme.ThemeSemanticStyle("{{|TagKey|}}")
	if isSelected || isParentOfSelected {
		tagStyle = theme.ThemeSemanticStyle("{{|TagSelected|}}")
		itemStyle = theme.ThemeSemanticStyle("{{|ItemSelected|}}")
		keyStyle = theme.ThemeSemanticStyle("{{|TagKeySelected|}}")
	}
	descStyle := lipgloss.NewStyle().Background(dialogBG)
	if isSelected || isParentOfSelected {
		descStyle = itemStyle
	}

	subIndent := "    " // 4 spaces for sub-items

	// "[+] Add instance…" row — rendered like a standard unchecked sub-item checkbox
	if menuItem.IsAddInstance {
		var cb string
		if ctx.LineCharacters {
			cb = checkUnselected
		} else {
			cb = checkUnselectedAscii
		}
		cbStr := tagStyle.Render(cb) + neutralStyle.Render(" ")
		subTagStr := ""
		if len(menuItem.Tag) > 0 {
			runes := []rune(menuItem.Tag)
			subTagStr = keyStyle.Render(string(runes[0])) + tagStyle.Render(string(runes[1:]))
		}
		line := neutralStyle.Render(" ") + neutralStyle.Render(subIndent) + cbStr + subTagStr
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// Inline editing row — Tag holds "SUFFIX▌" or "SUFFIX▌ {{|StatusError|}}msg{{[-]}}"
	if menuItem.IsEditing {
		cbStr := ""
		if menuItem.IsCheckbox {
			var cb string
			if ctx.LineCharacters {
				if menuItem.Checked {
					cb = checkSelected
				} else {
					cb = checkUnselected
				}
			} else {
				if menuItem.Checked {
					cb = checkSelectedAscii
				} else {
					cb = checkUnselectedAscii
				}
			}
			cbStr = tagStyle.Render(cb) + neutralStyle.Render(" ")
		}
		rendered := RenderThemeText(menuItem.Tag, descStyle)
		line := neutralStyle.Render(" ") + neutralStyle.Render(subIndent) + cbStr + rendered
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Padding(0, 1).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// Gutter: 2 chars on left edge.
	// g0 = Add diff marker (+/-/R/space), g1 = Enabled diff marker (E/D/space, changes only)
	var g0, g1 string
	if menuItem.IsReferenced && menuItem.Checked {
		g0 = RenderThemeText("{{|MarkerAdded|}}R{{[-]}}", neutralStyle)
	} else if menuItem.IsReferenced {
		g0 = RenderThemeText("{{|MarkerModified|}}r{{[-]}}", neutralStyle)
	} else if menuItem.Checked && !menuItem.WasAdded {
		g0 = RenderThemeText("{{|MarkerAdded|}}+{{[-]}}", neutralStyle)
	} else if !menuItem.Checked && menuItem.WasAdded {
		g0 = RenderThemeText("{{|MarkerDeleted|}}-{{[-]}}", neutralStyle)
	} else {
		g0 = neutralStyle.Render(" ")
	}
	if menuItem.Enabled && !menuItem.WasEnabled {
		g1 = RenderThemeText("{{|MarkerAdded|}}E{{[-]}}", neutralStyle)
	} else if !menuItem.Enabled && menuItem.WasEnabled {
		g1 = RenderThemeText("{{|MarkerDeleted|}}D{{[-]}}", neutralStyle)
	} else {
		g1 = neutralStyle.Render(" ")
	}

	// buildCb3 renders a 3-character wide checkbox block with a fixed style.
	buildCb3 := func(checked bool, cbStyle lipgloss.Style) string {
		if ctx.LineCharacters {
			g := checkUnselected
			if checked {
				g = checkSelected
			}
			// Draw with the chosen style (Red for focused, Blue for neutral)
			// Each block is exactly 3 chars: [space][glyph][space]
			inner := cbStyle.Render(g)
			// Apply the checkbox's background color to the surrounding spaces
			bgStyle := lipgloss.NewStyle().Background(cbStyle.GetBackground())
			return bgStyle.Render(" ") + inner + bgStyle.Render(" ")
		}

		content := "[ ]"
		if checked {
			content = "[x]"
		}
		return cbStyle.Render(content)
	}


	// Group headers never show checkboxes — just the disclosure glyph.
	// IsSubItem and IsCheckbox rows use the full two-checkbox layout.
	if menuItem.IsSubItem {
		addStyle := neutralStyle
		enableStyle := neutralStyle
		if isSelected {
			switch d.activeCol {
			case ColAdd:
				addStyle = tagStyle
			case ColEnable:
				enableStyle = tagStyle
			}
		}

		cbAdd := buildCb3(menuItem.Checked, addStyle)
		cbEnabled := buildCb3(menuItem.Enabled, enableStyle)
		tagStr := RenderThemeText(menuItem.Tag, tagStyle)
		// Layout matches border: Gutter(2) + cbAdd(3) + Spacer(1) + cbEnabled(3) + Spacer(1) + Tag.
		line := g0 + g1 + cbAdd + neutralStyle.Render(" ") + cbEnabled + neutralStyle.Render(" ") + tagStr
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// IsGroupHeader: expansion arrows + name.
	if menuItem.IsGroupHeader {
		tag := menuItem.Tag
		var tagStr string
		if strings.Contains(tag, "{{") {
			tagStr = RenderThemeText(tag, tagStyle)
		} else {
			firstLetter := string([]rune(tag)[0])
			rest := string([]rune(tag)[1:])
			tagStr = keyStyle.Render(firstLetter) + tagStyle.Render(rest)
		}

		// Disclosure arrow glyph (▼ or [v])
		var disclosureGlyph string
		if ctx.LineCharacters {
			disclosureGlyph = subMenuExpanded
		} else {
			disclosureGlyph = "[v]"
		}

		// Styled arrows for Add and Enable columns
		// As per previous Turn, only the arrow in the 'E' column is shown.
		// However, to support 'Add' toggle on groups if needed, we define both.
		cbStyle := theme.ThemeSemanticStyle("{{|TitleCheckbox|}}")
		cbAdd := neutralStyle.Render("   ")
		if menuItem.IsCheckbox {
			s := cbStyle
			if isSelected && d.activeCol == ColAdd {
				s = tagStyle
			}
			if ctx.LineCharacters {
				cbAdd = neutralStyle.Render(" ") + s.Render(disclosureGlyph) + neutralStyle.Render(" ")
			} else {
				cbAdd = s.Render(disclosureGlyph)
			}
		}

		cbEnabled := neutralStyle.Render("   ")
		if menuItem.ShowEnabledGutter {
			s := cbStyle
			if isSelected && d.activeCol == ColEnable {
				s = tagStyle
			}
			if ctx.LineCharacters {
				cbEnabled = neutralStyle.Render(" ") + s.Render(disclosureGlyph) + neutralStyle.Render(" ")
			} else {
				cbEnabled = s.Render(disclosureGlyph)
			}
		}

		paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
		prefixW := 11
		availableWidth := m.Width() - 2 - prefixW - (d.maxTagLen + 3)
		if availableWidth < 0 {
			availableWidth = 0
		}
		descStr := RenderThemeText(menuItem.Desc, descStyle)
		descLine := TruncateRight(descStr, availableWidth)

		// Layout restores the dual or single arrow look as it was before.
		// Layout matches border: Gutter(2) + cbAdd(3) + Spacer(1) + cbEnabled(3) + Spacer(1) + Tag.
		line := g0 + g1 + cbAdd + neutralStyle.Render(" ") + cbEnabled + neutralStyle.Render(" ") + tagStr + neutralStyle.Render(paddingSpaces) + descLine
		
		actualWidth := lipgloss.Width(line)
		if actualWidth < m.Width()-2 {
			line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
		}
		lineStyle := lipgloss.NewStyle().Background(dialogBG).Width(m.Width())
		fmt.Fprint(w, lineStyle.Render(line))
		return
	}

	// IsCheckbox simple row: g0 g1  [cb_add]  [cb_enabled]  AppName  Desc
	tag := menuItem.Tag
	var tagStr string
	if strings.Contains(tag, "{{") {
		tagStr = RenderThemeText(tag, tagStyle)
	} else {
		firstLetter := string([]rune(tag)[0])
		rest := string([]rune(tag)[1:])
		tagStr = keyStyle.Render(firstLetter) + tagStyle.Render(rest)
	}
	addStyle := neutralStyle
	enableStyle := neutralStyle
	if isSelected {
		switch d.activeCol {
		case ColAdd:
			addStyle = tagStyle
		case ColEnable:
			enableStyle = tagStyle
		}
	}

	cbAdd := buildCb3(menuItem.Checked, addStyle)
	cbEnabled := buildCb3(menuItem.Enabled, enableStyle)

	// Layout matches border: Gutter(2) + cbAdd(3) + Spacer(1) + cbEnabled(3) + Spacer(1) + Tag.
	line := g0 + g1 + cbAdd + neutralStyle.Render(" ") + cbEnabled + neutralStyle.Render(" ") + tagStr
	
	const rowPrefixW = 11
	paddingSpaces := strutil.Repeat(" ", max(0, d.maxTagLen-lipgloss.Width(GetPlainText(tag))+3))
	availableWidth := m.Width() - 2 - rowPrefixW - (d.maxTagLen + 3)
	if availableWidth < 0 {
		availableWidth = 0
	}
	descStr := RenderThemeText(menuItem.Desc, descStyle)
	descLine := TruncateRight(descStr, availableWidth)

	line = line + neutralStyle.Render(paddingSpaces) + descLine
	actualWidth := lipgloss.Width(line)
	if actualWidth < m.Width()-2 {
		line += neutralStyle.Render(strutil.Repeat(" ", m.Width()-2-actualWidth))
	}
	lineStyle := lipgloss.NewStyle().Background(dialogBG).Width(m.Width())
	fmt.Fprint(w, lineStyle.Render(line))
}

// MenuModel represents a selectable menu
type MenuModel struct {
	id       string // Unique identifier for selection persistence
	title    string // Menu title
	subtitle         string // Optional subtitle/description shown on-screen
	helpPageTitle    string // Optional title for the description box in the help dialog
	helpPageText     string // Optional description shown only in the help dialog (overrides subtitle)
	helpLegend     string // Optional legend shown in help dialog with title "Legend" (overrides helpPageText)
	helpItemPrefix string // Optional prefix for item titles in help dialog, e.g. "App", "Option", "Theme"
	items    []MenuItem
	cursor   int // Current selection
	width    int
	height   int

	// Focus state
	focused      bool
	focusedItem  FocusItem      // Which element has focus
	activeColumn CheckboxColumn // Which checkbox column has focus

	// Sub-menu mode (for consolidated screens)
	subMenuMode bool
	focusedSub  bool // If false, use normal borders. If true, use thick borders.

	// Back action (nil if no back button)
	backAction tea.Cmd

	// Bubbles list model
	list        list.Model
	maximized   bool // Whether to maximize the dialog to fill available space
	showExit    bool // Whether to show Exit button (default true for main menus)
	showButtons bool // Whether to show any buttons (default true)

	// Key override actions
	escAction   tea.Cmd
	enterAction tea.Cmd
	spaceAction tea.Cmd

	// Custom button labels
	selectLabel string
	backLabel   string
	exitLabel   string

	// Checkbox mode (for app selection)
	checkboxMode bool
	groupedMode  bool // Grouped hierarchical mode (app selection with instances)
	flowMode     bool // Whether to layout items horizontally instead of vertically

	// Dialog positioning
	isDialog bool // True when used as a modal dialog — raises hit-region Z priority above screen regions

	// Unified layout (deterministic sizing)
	layout DialogLayout

	dialogType DialogType

	// Variable height support (for dynamic word wrapping)
	variableHeight bool                                      // Allow list to expand naturally up to layout limits
	interceptor    func(tea.Msg, *MenuModel) (tea.Cmd, bool) // Optional custom message handler

	// Memoization for expensive rendering
	lastView   string
	cacheValid bool // Indicates if lastView is up-to-date with current state

	// Memoization specifically for the variable-height list (separated to avoid border recursion loops)
	lastListView   string
	lastWidth      int
	lastHeight     int
	lastIndex      int
	lastFilter     string
	lastActive     bool
	lastLineChars  bool
	lastVersion    int
	lastColumn     CheckboxColumn
	lastHitRegions []HitRegion // Cache for variable height hit regions
	viewStartY      int         // Persistent scroll offset for variable height lists
	lastViewStartY  int         // Previous scroll offset for memoization check
	lastScrollTotal int        // Total content height from last renderVariableHeightList (for scrollbar)

	renderVersion  int // Incremented on item changes to invalidate list cache
	menuName string // Name used for --menu or -M to return to this screen

	// Content sections: sub-menus rendered stacked inside the outer border.
	// When present, replaces the standard list+inner-border rendering.
	contentSections []*MenuModel

	// Optional hook to enrich the ItemText shown in the help dialog for a menu item.
	// If set, called by showContextMenu (right-click Help) and HelpContext.
	// Return ("", "") to keep the default item.Help text.
	itemHelpFunc func(item MenuItem) (itemTitle, itemText string)

	// Scrollbar interaction state
	sbInfo          ScrollbarInfo             // geometry from last render (set by menu_render.go)
	sbAbsTopY       int                       // absolute screen Y of scrollbar column top (set by GetHitRegions)
	sbAbsLeftX      int                       // absolute screen X of scrollbar column (set by GetHitRegions)
	sbDrag          ScrollbarDragState        // scrollbar drag tracking state (includes throttling fields)
	scrollPending   bool                      // true while a wheel scroll is queued but not yet rendered
	contextMenuFunc func(idx int) []ContextMenuItem // hook for screen-specific operations
}

// ScrollDoneMsg is sent after a wheel scroll is processed to clear the scrollPending flag.
// Exported so wrapper screens (e.g. DisplayOptionsScreen) can forward it to inner menus.
type ScrollDoneMsg struct{ ID string }

// scrollDoneCmd returns a zero-delay Cmd that emits ScrollDoneMsg for the given menu ID.
func scrollDoneCmd(id string) tea.Cmd {
	return func() tea.Msg { return ScrollDoneMsg{ID: id} }
}

// ScrollPending reports whether a scroll event is currently queued but not yet rendered.
func (m *MenuModel) ScrollPending() bool { return m.scrollPending }

// MarkScrollPending sets the scrollPending flag and returns a Cmd that will clear it
// after the next render cycle. Call this in interceptors after processing a wheel event.
func (m *MenuModel) MarkScrollPending() tea.Cmd {
	m.scrollPending = true
	return scrollDoneCmd(m.id)
}

// IsScrollbarDragging reports whether the menu is currently processing a scrollbar thumb drag.
// AppModel uses this interface to give the active screen drag priority.
func (m *MenuModel) IsScrollbarDragging() bool {
	return m.sbDrag.Dragging
}

// FocusItem represents which UI element has focus
type FocusItem int

const (
	FocusList FocusItem = iota
	FocusSelectBtn
	FocusBackBtn
	FocusExitBtn
)

// CheckboxColumn represents which column (Add or Enable) has focus in a row
type CheckboxColumn int

const (
	ColAdd CheckboxColumn = iota
	ColEnable
)

// SetContextMenuFunc sets the callback that provides custom context menu items for this menu
func (m *MenuModel) SetContextMenuFunc(f func(idx int) []ContextMenuItem) {
	m.contextMenuFunc = f
}

// menuSelectedIndices persists menu selection across visits
var menuSelectedIndices = make(map[string]int)

// NewMenuModel creates a new menu model
func NewMenuModel(id, title, subtitle string, items []MenuItem, backAction tea.Cmd) *MenuModel {
	// Set default shortcuts from first letter of Tag
	for i := range items {
		if items[i].Shortcut == 0 && len(items[i].Tag) > 0 {
			items[i].Shortcut = []rune(items[i].Tag)[0]
		}
	}

	// Restore previous selection
	cursor := 0
	if idx, ok := menuSelectedIndices[id]; ok && idx >= 0 && idx < len(items) {
		cursor = idx
	} else {
		// New: Auto-focus the currently selected radio option if no persistent session
		for i, item := range items {
			if item.IsRadioButton && item.Checked {
				cursor = i
				break
			}
		}
	}

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}

	// Calculate max tag and desc length for sizing
	maxTagLen, maxDescLen := calculateMaxTagAndDescLength(items)

	// Calculate initial width based on actual content
	// Width = tag + spacing(2) + desc + margins(2)
	initialWidth := maxTagLen + 2 + maxDescLen + 2

	// Create bubbles list

	// Size based on actual number of items for dynamic sizing
	delegate := menuItemDelegate{menuID: id, maxTagLen: maxTagLen, focused: true}

	// Calculate height
	itemHeight := delegate.Height()
	spacing := delegate.Spacing()
	totalItemHeight := len(items) * itemHeight
	if len(items) > 1 && spacing > 0 {
		totalItemHeight += (len(items) - 1) * spacing
	}
	initialHeight := totalItemHeight

	l := list.New(listItems, delegate, initialWidth, initialHeight)
	// Don't set l.Title - we render title in border instead
	l.SetShowTitle(false) // Disable list's built-in title rendering
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false) // Disable pagination indicators

	// Set list background to match dialog background (not black!)
	styles := GetStyles()
	dialogBg := styles.Dialog.GetBackground()
	l.Styles.NoItems = l.Styles.NoItems.Background(dialogBg)
	l.Styles.PaginationStyle = l.Styles.PaginationStyle.Background(dialogBg)
	l.Styles.HelpStyle = l.Styles.HelpStyle.Background(dialogBg)

	// Set initial cursor position
	if cursor > 0 && cursor < len(items) {
		l.Select(cursor)
	}

	return &MenuModel{
		id:          id,
		title:       title,
		subtitle:    subtitle,
		items:       items,
		cursor:      cursor,
		backAction:  backAction,
		focused:     true,
		focusedItem: FocusSelectBtn,
		activeColumn: ColAdd,
		list:        l,
		showExit:    true, // Default to show Exit button
		showButtons: true, // Default to show buttons
	}
}

// Title returns the menu title
func (m *MenuModel) Title() string {
	return m.title
}

// Subtitle returns the menu subtitle
func (m *MenuModel) Subtitle() string {
	return m.subtitle
}

// SetTitle sets the menu title
func (m *MenuModel) SetTitle(title string) { m.title = title }

// SetHelpPageText sets a description shown only in the help dialog, overriding the subtitle there.
func (m *MenuModel) SetHelpPageText(text string) { m.helpPageText = text }

// SetHelpPageTitle sets a title for the description box shown in the help dialog.
func (m *MenuModel) SetHelpPageTitle(title string) { m.helpPageTitle = title }

// SetHelpLegend sets a legend shown in the help dialog with the title "Legend".
// When set, it takes precedence over helpPageText for both F1 and context-menu Help.
func (m *MenuModel) SetHelpLegend(text string) { m.helpLegend = text }

// SetHelpItemPrefix sets a prefix prepended to item titles in the help dialog, e.g. "App", "Option", "Theme".
func (m *MenuModel) SetHelpItemPrefix(prefix string) { m.helpItemPrefix = prefix }

// SetItemHelpFunc sets an optional callback that enriches the ItemTitle and ItemText shown in
// the help dialog for a focused menu item. Used by showContextMenu (right-click Help) and
// HelpContext. Return ("", "") to keep the default item.Help text.
func (m *MenuModel) SetItemHelpFunc(f func(item MenuItem) (itemTitle, itemText string)) {
	m.itemHelpFunc = f
}

// ID returns the unique identifier for this menu
func (m *MenuModel) ID() string { return m.id }

// SetDialogType sets the visual style/type of the menu dialog
func (m *MenuModel) SetDialogType(t DialogType) { m.dialogType = t }

// MenuName returns the name used for --menu or -M to return to this screen
func (m *MenuModel) MenuName() string {
	return m.menuName
}

// SetMenuName sets the persistent menu name
func (m *MenuModel) SetMenuName(name string) {
	m.menuName = name
}

// AddContentSection appends a sub-menu as a stacked section rendered inside this menu's border.
// When sections are present the standard list is not rendered.
func (m *MenuModel) AddContentSection(section *MenuModel) {
	m.contentSections = append(m.contentSections, section)
}

// SetFocusedItem explicitly sets which UI element has focus (list or a button).
func (m *MenuModel) SetFocusedItem(item FocusItem) {
	m.focusedItem = item
}

// GetButtonHeight returns the current button row height (1 = flat, 3 = bordered).
func (m *MenuModel) GetButtonHeight() int {
	return m.layout.ButtonHeight
}

// View implements tea.Model and ScreenModel
func (m *MenuModel) View() tea.View {
	return tea.View{Content: m.ViewString()}
}

// SetFocused sets whether this menu's dialog border is rendered as focused (thick)
// or unfocused (normal). Called by AppModel when the log panel takes focus.
func (m *MenuModel) SetFocused(f bool) {
	m.focused = f
	m.updateDelegate()
	m.InvalidateCache()
}

// SetMaximized sets whether the menu should expand to fill available space
func (m *MenuModel) SetMaximized(maximized bool) {
	m.maximized = maximized
	m.calculateLayout()
}

// SetShowExit sets whether to show the Exit button
func (m *MenuModel) SetShowExit(show bool) {
	m.showExit = show
}

// SetShowButtons sets whether to show the button row at all
func (m *MenuModel) SetShowButtons(show bool) {
	m.showButtons = show
	m.calculateLayout() // Layout needs recalculation when buttons are toggled
}

// IsMaximized returns whether the menu is maximized
func (m MenuModel) IsMaximized() bool {
	return m.maximized
}

// HasDialog returns whether the menu has an active dialog overlay
func (m MenuModel) HasDialog() bool {
	return false // Menus don't have nested dialogs
}

// SetSubMenuMode enables a compact mode for menus inside other screens/containers
func (m *MenuModel) SetSubMenuMode(v bool) {
	m.subMenuMode = v
	if v {
		m.showButtons = false
	}
	m.calculateLayout()
}

// SetSubFocused sets the focus state specifically for sub-menu mode (thick vs normal border)
func (m *MenuModel) SetSubFocused(focused bool) {
	m.focusedSub = focused
	m.updateDelegate()
}

// IsActive returns whether this menu actually has focus (accounting for subMenuMode)
func (m *MenuModel) IsActive() bool {
	if m.subMenuMode {
		return m.focusedSub
	}
	return m.focused
}

// updateDelegate refreshes the list delegate with the current focus state
// calculateMaxTagLengthForHeaders returns the max tag width among IsGroupHeader items only.
func calculateMaxTagLengthForHeaders(items []MenuItem) int {
	maxLen := 0
	for _, item := range items {
		if !item.IsGroupHeader {
			continue
		}
		w := lipgloss.Width(GetPlainText(item.Tag))
		if w > maxLen {
			maxLen = w
		}
	}
	return maxLen
}

func (m *MenuModel) updateDelegate() {
	focused := m.IsActive()
	if m.groupedMode {
		maxTagLen := calculateMaxTagLengthForHeaders(m.items)
		m.list.SetDelegate(groupedItemDelegate{menuID: m.id, maxTagLen: maxTagLen, focused: focused, activeCol: m.activeColumn})
	} else if m.checkboxMode {
		maxTagLen := calculateMaxTagLength(m.items)
		m.list.SetDelegate(checkboxItemDelegate{menuID: m.id, maxTagLen: maxTagLen, focused: focused, flowMode: m.flowMode})
	} else {
		maxTagLen := calculateMaxTagLength(m.items)
		m.list.SetDelegate(menuItemDelegate{menuID: m.id, maxTagLen: maxTagLen, focused: focused, flowMode: m.flowMode})
	}
}

// SetCheckboxMode enables checkbox rendering for app selection
func (m *MenuModel) SetCheckboxMode(enabled bool) {
	m.checkboxMode = enabled
	m.updateDelegate()
}

// SetGroupedMode enables the hierarchical grouped delegate for the app-selection screen.
// This renders IsGroupHeader, IsSubItem, IsAddInstance, and IsEditing items correctly.
func (m *MenuModel) SetGroupedMode(enabled bool) {
	m.groupedMode = enabled
	m.updateDelegate()
}

// SetItem updates a single menu item in-place without replacing the whole list.
// Useful for live updates (e.g. refreshing the inline editing row on each keypress).
func (m *MenuModel) SetItem(index int, item MenuItem) {
	if index < 0 || index >= len(m.items) {
		return
	}
	m.items[index] = item
	m.list.SetItem(index, item)
	m.renderVersion++
	m.InvalidateCache()
}

// SetVariableHeight allows the list viewport to expand instead of forcing pagination
func (m *MenuModel) SetVariableHeight(variable bool) {
	m.variableHeight = variable
}

// SetUpdateInterceptor allows setting a custom handler that runs before normal message processing
func (m *MenuModel) SetUpdateInterceptor(interceptor func(tea.Msg, *MenuModel) (tea.Cmd, bool)) {
	m.interceptor = interceptor
}

// Index returns the current cursor index
func (m *MenuModel) Index() int {
	return m.cursor
}

// FocusedItem returns the currently focused UI element
func (m *MenuModel) FocusedItem() FocusItem {
	return m.focusedItem
}

// Select programmatically sets the cursor index
func (m *MenuModel) Select(index int) {
	if index >= 0 && index < len(m.items) {
		m.cursor = index
		m.list.Select(index)
		menuSelectedIndices[m.id] = index
	}
}

// GetItems returns the current list of MenuItems
func (m *MenuModel) GetItems() []MenuItem {
	return m.items
}

// GetInnerContentWidth returns the width of the space inside the outer borders
func (m *MenuModel) GetInnerContentWidth() int {
	layout := GetLayout()
	if m.subMenuMode {
		return m.width - layout.BorderWidth()
	}

	var contentWidth int
	if m.maximized {
		contentWidth, _ = layout.InnerContentSize(m.width, m.height)
	} else {
		contentWidth = m.list.Width() + layout.BorderWidth() + 2
		maxWidth, _ := layout.InnerContentSize(m.width, m.height)
		if contentWidth > maxWidth {
			contentWidth = maxWidth
		}
	}
	return contentWidth
}

// SetItems updates the menu items and refreshes the bubbles list
func (m *MenuModel) SetItems(items []MenuItem) {
	m.items = items

	// Convert MenuItems to list.Items
	listItems := make([]list.Item, len(items))
	for i, item := range items {
		listItems[i] = item
	}
	m.list.SetItems(listItems)

	m.renderVersion++
	m.InvalidateCache()

	// Update delegate with new max tag length and focus
	m.updateDelegate()
}

// SetEscAction sets a custom action for the Escape key
func (m *MenuModel) SetEscAction(action tea.Cmd) {
	m.escAction = action
}

// SetEnterAction sets a custom action for the Enter key
func (m *MenuModel) SetEnterAction(action tea.Cmd) {
	m.enterAction = action
}

// SetSpaceAction sets a custom action for the Space key
func (m *MenuModel) SetSpaceAction(action tea.Cmd) {
	m.spaceAction = action
}

// ActiveColumn returns the currently focused checkbox column (Add or Enable)
func (m *MenuModel) ActiveColumn() CheckboxColumn {
	return m.activeColumn
}

// SelectedItem returns the MenuItem currently under the cursor
func (m *MenuModel) SelectedItem() MenuItem {
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.items) {
		return m.items[idx]
	}
	return MenuItem{}
}
// SetActiveColumn sets the focused checkbox column (Add or Enable)
func (m *MenuModel) SetActiveColumn(col CheckboxColumn) {
	m.activeColumn = col
	m.renderVersion++
	m.updateDelegate()
}

// SetButtonLabels sets custom labels for the buttons
func (m *MenuModel) SetButtonLabels(selectLabel, backLabel, exitLabel string) {
	m.selectLabel = selectLabel
	m.backLabel = backLabel
	m.exitLabel = exitLabel
}

// ToggleSelectedItem toggles the selected state of the current item (for checkbox mode)
func (m *MenuModel) ToggleSelectedItem() {
	idx := m.list.Index()
	if idx >= 0 && idx < len(m.items) && m.items[idx].Selectable {
		if m.items[idx].IsCheckbox || m.items[idx].IsRadioButton {
			if m.groupedMode && m.activeColumn == ColEnable {
				m.items[idx].Enabled = !m.items[idx].Enabled
				if m.items[idx].Enabled {
					m.items[idx].Checked = true // Auto-add if user enables
					m.items[idx].ShowEnabledGutter = true
				}
			} else {
				m.items[idx].Checked = !m.items[idx].Checked
				m.items[idx].Selected = m.items[idx].Checked
				if m.items[idx].Checked {
					m.items[idx].Enabled = true
					m.items[idx].ShowEnabledGutter = true
				} else {
					m.items[idx].Enabled = false
					m.items[idx].ShowEnabledGutter = false
				}
			}
		} else {
			m.items[idx].Selected = !m.items[idx].Selected
		}
		// Update the list item too
		m.list.SetItem(idx, m.items[idx])
		m.renderVersion++
		m.InvalidateCache()
	}
}

// helpContextForIdx builds a HelpContext for the item at the given index.
// Both HelpContext (F1) and showContextMenu (right-click Help) call this so the output is identical.
func (m *MenuModel) helpContextForIdx(idx int) HelpContext {
	itemTitle := "Help"
	itemText := ""
	if idx >= 0 && idx < len(m.items) {
		item := m.items[idx]
		if item.Tag != "" {
			itemTitle = item.Tag
		}
		itemText = item.Help
		if m.itemHelpFunc != nil {
			if t, txt := m.itemHelpFunc(item); txt != "" {
				if t != "" {
					itemTitle = t
				}
				itemText = txt
			}
		}
	}
	if m.helpItemPrefix != "" && itemTitle != "Help" {
		itemTitle = m.helpItemPrefix + ": " + itemTitle
	}

	pageTitle := m.helpPageTitle
	pageText := m.helpPageText
	if pageText == "" {
		pageText = m.subtitle
	}
	if m.helpLegend != "" {
		pageText = "" // legend takes precedence; suppress the description
		if pageTitle == "Description" { // Fallback cleanup if previously relied on
			pageTitle = ""
		}
	}
	return HelpContext{
		ScreenName: m.title,
		PageTitle:  pageTitle,
		PageText:   pageText,
		Legend:     m.helpLegend,
		ItemTitle:  itemTitle,
		ItemText:   itemText,
	}
}

// HelpContext implements HelpContextProvider.
func (m *MenuModel) HelpContext(contentWidth int) HelpContext {
	return m.helpContextForIdx(m.list.Index())
}

// ShowContextMenu returns a command to show the context menu for the item at the given index.
func (m *MenuModel) ShowContextMenu(idx int, x, y int) tea.Cmd {
	var tag, desc string
	var hCtx *HelpContext

	if idx >= 0 && idx < len(m.items) {
		item := m.items[idx]
		tag = GetPlainText(item.Tag)
		desc = item.Desc
		ctx := m.helpContextForIdx(idx)
		hCtx = &ctx
	}

	var items []ContextMenuItem
	if tag != "" {
		items = append(items, ContextMenuItem{IsHeader: true, Label: tag})
		items = append(items, ContextMenuItem{IsSeparator: true})
	}

	// NEW: Inject custom operational items from the screen provider
	if m.contextMenuFunc != nil {
		customItems := m.contextMenuFunc(idx)
		if len(customItems) > 0 {
			items = append(items, customItems...)
			items = append(items, ContextMenuItem{IsSeparator: true})
		}
	}

	var clipItems []ContextMenuItem

	if tag != "" {
		t := tag
		clipItems = append(clipItems, ContextMenuItem{
			Label: "Copy Item Title",
			Help:  "Copy the item's title (tag) to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(t)
				return CloseDialogMsg{}
			},
		})
	}
	if desc != "" {
		d := desc
		clipItems = append(clipItems, ContextMenuItem{
			Label: "Copy Item Description",
			Help:  "Copy the item's description to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(d)
				return CloseDialogMsg{}
			},
		})
	}

	items = AppendContextMenuTail(items, clipItems, hCtx)

	return func() tea.Msg {
		return ShowDialogMsg{Dialog: NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}

// Init implements tea.Model
func (m *MenuModel) Init() tea.Cmd {
	return nil
}
