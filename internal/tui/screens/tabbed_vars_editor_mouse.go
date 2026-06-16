package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/version"
	"context"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

func (m *TabbedVarsEditorModel) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion

	m.lastOffsetX = offsetX
	m.lastOffsetY = offsetY

	layout := tui.GetLayout()

	// Tabs hit regions
	if len(m.tabs) > 0 {
		ctx := tui.GetActiveContext()
		// Calculate total width of all tabs to determine centering offset
		totalTitleWidth := 0
		var tabWidths []int
		for _, tab := range m.tabs {
			w := tui.WidthOfTitleSegment(tab.spec.Title, true, ctx)
			tabWidths = append(tabWidths, w)
			totalTitleWidth += w
		}

		// Replicate RenderBorderedBoxCtx centering logic for the title/tabs row.
		// Inner box content width = m.contentWidth - BorderWidth (accounts for inner box borders).
		innerContentW := m.contentWidth - layout.BorderWidth()
		if innerContentW < 1 {
			innerContentW = 1
		}
		actualWidth := innerContentW
		if totalTitleWidth > actualWidth {
			actualWidth = totalTitleWidth
		}

		leftPad := 0
		if ctx.SubmenuTitleAlign != "left" {
			leftPad = (actualWidth - totalTitleWidth) / 2
		}

		// Tabs are in the top border of the inner box.
		// Inner box starts at offsetX + outer border(1) + margin; tabs start after inner TopLeft corner(1).
		tabX := offsetX + 1 + layout.ContentSideMargin + 1 + leftPad // outer border + margin + inner TopLeft + leftPad
		for i, tabWidth := range tabWidths {
			regions = append(regions, tui.HitRegion{
				ID:     "tabbed_vars.tab-" + strconv.Itoa(i),
				X:      tabX,
				Y:      offsetY + 1 + m.largeTitleOverhead + m.subtitleHeight, // outer border + large title + subtitle + inner border line with tabs
				Width:  tabWidth,
				Height: 1,
				ZOrder: tui.ZDialog + 10,
				Label:  m.tabs[i].spec.Title,
				Help: &tui.HelpContext{
					ScreenName: m.title,
					ItemTitle:  m.tabs[i].spec.Title,
					ItemText:   "Tab: " + m.tabs[i].spec.Title + ". Click to switch to this category of variables.",
				},
			})
			tabX += tabWidth
		}
	}

	// Editor hit region
	// Editor content is inside nesting (outer border + margin + inner border).
	regions = append(regions, tui.HitRegion{
		ID:     "tabbed_vars.editor",
		X:      offsetX + layout.NestedLeftOffset(),
		Y:      offsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight,
		Width:  m.contentWidth - layout.BorderWidth(), // inner box content width
		Height: m.editorHeight,
		ZOrder: tui.ZDialog + 5,
		Label:  "Variables Editor",
		Help: &tui.HelpContext{
			ScreenName: m.title,
			PageTitle:  "Variables Editor",
			PageText:   "Grouped environment variable editor. Right-click any row for specific options.",
		},
	})

	// INS/OVR hit region — bottom-left of the inner editor box border.
	// Inner editor box bottom border = NestedTopOffset + largeTitleOverhead + subtitleHeight + editorHeight
	// (NestedTopOffset already accounts for outer border + inner top border/tab row)
	insOvrY := offsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight + m.editorHeight
	regions = append(regions, tui.HitRegion{
		ID:     "tabbed_vars." + tui.IDInsOvr,
		X:      offsetX + layout.NestedLeftOffset() + 1, // +1 to skip the corner char
		Y:      insOvrY,
		Width:  3,
		Height: 1,
		ZOrder: tui.ZDialog + 15,
		Label:  "INS/OVR",
		Help:   &tui.HelpContext{ScreenName: m.title, PageTitle: "Insert/Overwrite", PageText: "Toggle between insert and overwrite mode."},
	})

	// Button regions (standardized width — matches m.contentWidth which is already margin-reduced)
	btnY := m.height - m.buttonHeight - 1
	regions = append(regions, tui.GetButtonHitRegions(
		tui.HelpContext{
			ScreenName: m.title,
			PageTitle:  "Variables Editor",
			PageText:   "Grouped environment variable editor. Right-click any row for specific options.",
		},
		"tabbed_vars", offsetX+1+layout.ContentSideMargin, offsetY+btnY, m.contentWidth, tui.ZDialog+20,
		m.getButtonSpecs()...,
	)...)

	// Title widget regions — widgets are on the title row (row 1) for large titlebars,
	// or on the top border (row 0) for small titlebars.
	activeW := m.ActiveWidgets()
	ctx := tui.GetActiveContext()
	widgetStr := tui.BuildInactiveTitleWidgetsFor(activeW, ctx)
	widgetWidth := lipgloss.Width(tui.GetPlainText(widgetStr))
	widgetsStartX := offsetX + m.width - 1 - 1 - widgetWidth
	widgetY := tui.TitleBarWidgetY(offsetY, m.largeTitleOverhead > 0)
	regions = append(regions, tui.TitleBarWidgetRegions("tabbed_vars", activeW, widgetsStartX, widgetY, tui.ZDialog)...)

	return regions
}

// showContextMenuForClick builds and shows a right-click context menu at screen
// position (x, y).  Returns nil only when the click is outside any editor line.
// Variable-specific options are omitted when the clicked line is not a variable.
func (m *TabbedVarsEditorModel) showContextMenuForClick(x, y int) tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	tab := &m.tabs[m.activeTab]
	editor := tab.editor

	layout := tui.GetLayout()
	// Compute which editor row was clicked.
	// Editor content starts at: lastOffsetY + layout.NestedTopOffset() + largeTitleOverhead + m.subtitleHeight
	editorTopY := m.lastOffsetY + layout.NestedTopOffset() + m.largeTitleOverhead + m.subtitleHeight
	clickedVisualRow := (y - editorTopY) + editor.YOffset()

	// Convert visual (screen) row to logical line index to handle wrapped lines correctly.
	clickedRow := editor.VisualRowToLogical(clickedVisualRow)
	if clickedRow < 0 {
		return nil
	}

	meta, ok := editor.LineMetaAt(clickedRow)
	if !ok {
		return nil // click is outside all editor lines
	}

	// Determine whether we are on a well-formed variable row.
	isVarLine := meta.IsVariable
	var varName, currentVal string
	var opts []appenv.VarOption
	if isVarLine {
		varName = meta.Text
		if idx := strings.Index(varName, "="); idx > 0 {
			varName = strings.TrimSpace(varName[:idx])
		}
		if varName == "" {
			isVarLine = false
		} else {
			currentVal = editor.GetVariableValue(varName)
			opts = appenv.GetVarOptions(varName, strings.ToUpper(tab.spec.App), tab.defaultVal(varName), tab.appMeta)
		}
	}

	var items []tui.ContextMenuItem

	if isVarLine {
		items = append(items, tui.ContextMenuItem{IsHeader: true, Label: varName})
		items = append(items, tui.ContextMenuItem{IsSeparator: true})
	}

	if isVarLine {
		// Set Value ▶ (most-used — first)
		var subItems []tui.ContextMenuItem
		origVn, origV := varName, currentVal
		subItems = append(subItems, tui.ContextMenuItem{
			Label:    "Original Value",
			SubLabel: origV,
			Help:     "Keep the current value as-is.",
			Action: func() tea.Msg {
				return tui.CloseDialogMsg{Result: ApplyVarValueMsg{VarName: origVn, Value: origV}}
			},
		})
		for _, opt := range opts {
			opt := opt
			vn := varName
			subItems = append(subItems, tui.ContextMenuItem{
				Label:    opt.Display,
				SubLabel: opt.Value,
				Help:     opt.Help,
				Action: func() tea.Msg {
					return tui.CloseDialogMsg{Result: ApplyVarValueMsg{VarName: vn, Value: opt.Value}}
				},
			})
		}
		items = append(items, tui.ContextMenuItem{
			Label:    "Set Value",
			Help:     "Choose or reset this variable's value.",
			SubItems: subItems,
		})

		// Edit Value
		evVarName := varName
		evOrigVal := currentVal
		evOpts := make([]appenv.VarOption, len(opts)+1)
		evOpts[0] = appenv.VarOption{Display: "Original Value", Value: evOrigVal, Help: "Restore the value from before editing."}
		copy(evOpts[1:], opts)
		evTab := tab

		varHelp := ""
		if desc := appenv.GetVarHelpText(evVarName); desc != "" {
			varHelp = desc
		} else if vm, ok := evTab.appMeta.GetVarMeta(evVarName, evTab.spec.App); ok && vm.HelpText != "" {
			varHelp = vm.HelpText
		}

		items = append(items, tui.ContextMenuItem{
			Label: "Edit Value",
			Help:  "Open the value editor for this variable.",
			Action: func() tea.Msg {
				docMarkdown, docAppName := "", ""
				if evTab.spec.App != "" {
					ctx := context.Background()
					if !appenv.IsAppUserDefined(ctx, evTab.spec.App, evTab.composeEnvPath) {
						doc, err := appenv.GetAppMarkdown(ctx, evTab.spec.App)
						if err == nil {
							docMarkdown = doc
							docAppName = evTab.niceName
						}
					}
				}
				dlg := newSetValueDialog(evVarName, evTab.niceName, evTab.description, evTab.envFilePath, evOrigVal, evOpts, varHelp, docMarkdown, docAppName, nil, nil)
				return tui.CloseDialogMsg{Result: tui.ShowDialogMsg{Dialog: dlg}}
			},
		})
	}

	// Copy / Cut / Paste / Delete variables
	selectedText := editor.GetSelectedText()
	copyText := selectedText
	if copyText == "" && isVarLine {
		copyText = currentVal
	}

	// Build Clipboard Submenu items
	var clipItems []tui.ContextMenuItem
	if copyText != "" {
		copyLabel := "Copy Value"
		cutLabel := "Cut Value"
		cutHelp := "Copy this variable's value to the clipboard and delete the variable."
		if selectedText != "" {
			copyLabel = "Copy Selection"
			cutLabel = "Cut Selection"
			cutHelp = "Copy selected text to clipboard and delete the selection."
		}
		ct := copyText
		clipItems = append(clipItems, tui.ContextMenuItem{
			Label: copyLabel,
			Help:  "Copy to clipboard.",
			Action: func() tea.Msg {
				_ = clipboard.WriteAll(ct)
				return tui.CloseDialogMsg{}
			},
		})
		if isVarLine || selectedText != "" {
			cutVarName := varName
			hasSelection := selectedText != ""
			clipItems = append(clipItems, tui.ContextMenuItem{
				Label: cutLabel,
				Help:  cutHelp,
				Action: func() tea.Msg {
					_ = clipboard.WriteAll(ct)
					if hasSelection {
						return tui.CloseDialogMsg{}
					}
					return tui.CloseDialogMsg{Result: deleteVarMsg{VarName: cutVarName}}
				},
			})
		}
	}

	if isVarLine {
		vn2 := varName
		clipItems = append(clipItems, tui.ContextMenuItem{
			Label: "Paste Value",
			Help:  "Replace the entire variable value with clipboard text.",
			Action: func() tea.Msg {
				text, err := clipboard.ReadAll()
				if err != nil || text == "" {
					return tui.CloseDialogMsg{}
				}
				return tui.CloseDialogMsg{Result: ApplyVarValueMsg{VarName: vn2, Value: text}}
			},
		})
	}

	addM := m
	items = append(items, tui.ContextMenuItem{
		Label: "Add Variable",
		Help:  "Add a new variable to this file.",
		Action: func() tea.Msg {
			cmd := addM.showAddVarDialog()
			if cmd == nil {
				return tui.CloseDialogMsg{}
			}
			msg := cmd()
			if msg == nil {
				return tui.CloseDialogMsg{}
			}
			return tui.CloseDialogMsg{Result: msg}
		},
	})

	// Delete / Restore — only on a variable line, grouped with Add Variable.
	if isVarLine {
		if meta.PendingDelete {
			restVarName := varName
			items = append(items, tui.ContextMenuItem{
				Label: "Restore Variable",
				Help:  "Cancel the pending deletion of this variable (same as Ctrl+U).",
				Action: func() tea.Msg {
					return tui.CloseDialogMsg{Result: restoreVarMsg{VarName: restVarName}}
				},
			})
		} else {
			delVarName := varName
			items = append(items, tui.ContextMenuItem{
				Label: "Delete Variable",
				Help:  "Remove this variable from the file (same as Ctrl+D).",
				Action: func() tea.Msg {
					return tui.CloseDialogMsg{Result: deleteVarMsg{VarName: delVarName}}
				},
			})
		}
	}

	// Refresh (separator added here; AppendContextMenuTail won't double it since Refresh is last)
	items = append(items, tui.ContextMenuItem{IsSeparator: true})
	items = append(items, tui.ContextMenuItem{
		Label: "Refresh",
		Help:  "Reformat all variables based on current staged state (same as F5).",
		Action: func() tea.Msg {
			return tui.CloseDialogMsg{Result: envRefreshMsg{}}
		},
	})

	// Build captured help context for this specific variable.
	// Use the same width calculation as showHelpCmd so pre-wrapped text matches the dialog.
	helpW := tui.HelpContextWidth(m.width, m.height)
	capturedCtx := m.getVariableHelpContext(varName, tab, helpW)

	// Final tail (Clipboard submenu + Help)
	items = tui.AppendContextMenuTail(items, clipItems, capturedCtx)

	return func() tea.Msg {
		return tui.ShowDialogMsg{Dialog: tui.NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}

// showAddVarDialog opens the Add Variable dialog (ctrl+a) for the active app tab.
// Falls back to the free-form PromptText for non-app tabs.
func (m *TabbedVarsEditorModel) showAddVarDialog() tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	tab := &m.tabs[m.activeTab]

	// Non-app tabs: free-form name prompt.
	if tab.spec.App == "" || !tab.spec.IsGlobal {
		return func() tea.Msg {
			keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
			if err == nil && keyName != "" {
				return envAddVarMsg{key: strings.TrimSpace(keyName)}
			}
			return nil
		}
	}

	appUpper := strings.ToUpper(tab.spec.App)
	editor := tab.editor

	templates := []struct{ prefix, label, tag, help string }{
		{appUpper + "__", appUpper + "__", "Generic", "Complete this with any variable name you want."},
		{appUpper + "__ENVIRONMENT_", appUpper + "__ENVIRONMENT_", "Environment", "Complete with a var for the environment: section of your override."},
		{appUpper + "__PORT_", appUpper + "__PORT_", "Port", "Complete with a port number for the ports: section of your override."},
		{appUpper + "__VOLUME_", appUpper + "__VOLUME_", "Volume", "Complete with a path for the volumes: section of your override."},
	}

	type stockDef struct {
		name string
		help string
	}
	allStock := []stockDef{
		{appUpper + "__CONTAINER_NAME", "Used in the container_name: section of your override."},
		{appUpper + "__HOSTNAME", "Used in the hostname: section of your override."},
		{appUpper + "__NETWORK_MODE", "Used in the network_mode: section of your override."},
		{appUpper + "__RESTART", "Used in the restart: section of your override."},
		{appUpper + "__TAG", "Used in the image: tag section of your override."},
	}
	if appenv.IsAppBuiltIn(appUpper) && !editor.HasVariable(appUpper+"__ENABLED") {
		allStock = append([]stockDef{{appUpper + "__ENABLED", "Creating this variable causes " + version.ApplicationName + " to manage this app with no override needed."}}, allStock...)
	}

	var stockItems []addVarItem
	addAllVars := make([]string, 0)
	addAllDefaults := make(map[string]string)
	for _, s := range allStock {
		if !editor.HasVariable(s.name) {
			defVal := tab.defaultVal(s.name)
			stockItems = append(stockItems, addVarItem{
				kind:     addVarKindStock,
				name:     s.name,
				label:    s.name,
				subLabel: defVal,
				help:     s.help,
			})
			addAllVars = append(addAllVars, s.name)
			addAllDefaults[s.name] = defVal
		}
	}

	dlg := newAddVarDialog(tab.niceName, tab.description, templates, stockItems, addAllVars, addAllDefaults)
	return func() tea.Msg {
		return tui.ShowDialogMsg{Dialog: dlg}
	}
}

// showSetValueDialog opens the Set Value dialog (F2) for the current variable row.
func (m *TabbedVarsEditorModel) showSetValueDialog() tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	tab := &m.tabs[m.activeTab]
	editor := tab.editor
	meta, ok := editor.CurrentLineMeta()
	if !ok || !meta.IsVariable {
		return nil
	}
	varName := meta.Text
	if idx := strings.Index(varName, "="); idx > 0 {
		varName = strings.TrimSpace(varName[:idx])
	}
	if varName == "" {
		return nil
	}
	origVal := editor.GetVariableValue(varName)
	appUpper := strings.ToUpper(tab.spec.App)
	opts := appenv.GetVarOptions(varName, appUpper, tab.defaultVal(varName), tab.appMeta)
	// Always offer "Original Value" first so the user can revert.
	opts = append([]appenv.VarOption{{
		Display: "Original Value",
		Value:   origVal,
		Help:    "Restore the value from before editing.",
	}}, opts...)
	varHelp := ""
	if desc := appenv.GetVarHelpText(varName); desc != "" {
		varHelp = desc
	} else if vm, ok := tab.appMeta.GetVarMeta(varName, tab.spec.App); ok && vm.HelpText != "" {
		varHelp = vm.HelpText
	}

	docMarkdown, docAppName := "", ""
	if tab.spec.App != "" {
		ctx := context.Background()
		if !appenv.IsAppUserDefined(ctx, tab.spec.App, tab.composeEnvPath) {
			doc, err := appenv.GetAppMarkdown(ctx, tab.spec.App)
			if err == nil {
				docMarkdown = doc
				docAppName = tab.niceName
			}
		}
	}

	dlg := newSetValueDialog(varName, tab.niceName, tab.description, tab.envFilePath, origVal, opts, varHelp, docMarkdown, docAppName, nil, nil)
	return func() tea.Msg {
		return tui.ShowDialogMsg{Dialog: dlg}
	}
}
