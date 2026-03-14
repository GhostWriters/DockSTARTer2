package screens

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/enveditor"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type envFocusArea int

const (
	envFocusEditor envFocusArea = iota
	envFocusButtons
)

type EnvTabSpec struct {
	Title    string
	App      string // Empty string for global vars, app name for app-specific vars
	IsGlobal bool   // Indicates if this tab edits the global .env file (potentially filtered by App)
}

type envTab struct {
	spec        EnvTabSpec
	editor      enveditor.Model
	initialVars     map[string]string // Captured when loaded, used for scoped syncing on save
	defaultFilePath string            // Cached for Refresh
	builtinDefaults map[string]string // Cached for Refresh
	readOnlyVars    []string          // Cached for Refresh
}

type TabbedVarsEditorModel struct {
	tabs      []envTab
	activeTab int

	width  int
	height int
	title  string

	focus envFocusArea

	// Action buttons
	buttons []string
	btnIdx  int

	// Callbacks
	onClose tea.Cmd

	// Last hit region offsets for coordinate translation
	lastOffsetX int
	lastOffsetY int
}

type envAddVarMsg struct {
	key string
}

type envSaveSuccessMsg struct{}
type envLoadDoneMsg struct{}

func NewEnvEditorGlobal(onClose tea.Cmd) *TabbedVarsEditorModel {
	return NewTabbedVarsEditorScreen(onClose, "Global Variables", []EnvTabSpec{
		{Title: ".env", App: "", IsGlobal: true},
	})
}

func NewTabbedVarsEditorScreen(onClose tea.Cmd, title string, specs []EnvTabSpec) *TabbedVarsEditorModel {
	var tabs []envTab
	for _, spec := range specs {
		editor := enveditor.New()
		editor.ShowLineNumbers = true
		editor.SetLineCharacters(tui.GetActiveContext().LineCharacters)
		tabs = append(tabs, envTab{spec: spec, editor: editor})
	}

	return &TabbedVarsEditorModel{
		tabs:      tabs,
		activeTab: 0,
		title:     title,
		buttons:   []string{"Save", "Back", "Exit"},
		btnIdx:    0,
		focus:     envFocusEditor,
		onClose:   onClose,
	}
}

func (m *TabbedVarsEditorModel) Init() tea.Cmd {
	return m.loadEnv
}

func (m *TabbedVarsEditorModel) loadEnv() tea.Msg {
	ctx := context.Background()
	cfg := config.LoadAppConfig()
	envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
	defaultGlobalEnvPath := filepath.Join(paths.GetConfigDir(), constants.EnvExampleFileName)

	readOnlyVars := []string{"HOME", "DOCKER_CONFIG_FOLDER", "DOCKER_COMPOSE_FOLDER"}

	for i, tab := range m.tabs {
		var currentLines []string
		var defaultFilePath string

		if tab.spec.IsGlobal {
			if tab.spec.App != "" {
				// Get current lines for specific app section in global .env
				currentLines, _ = appenv.ListAppVarLines(ctx, tab.spec.App, cfg)
			} else {
				// Pure global editor: load all lines from .env that are not part of an app section
				currentLines, _ = appenv.ListAppVarLines(ctx, "", cfg)
			}

			if tab.spec.App != "" {
				// App-specific variables from global .env
				// Bash: DefaultGlobalEnvFile="$(run_script 'app_instance_file' "${APPNAME}" ".env")"
				if !appenv.IsAppUserDefined(ctx, tab.spec.App, envPath) {
					defaultFilePath, _ = appenv.AppInstanceFile(ctx, tab.spec.App, ".env")
				}
			} else {
				// Pure global editor
				// Bash: DefaultGlobalEnvFile="${COMPOSE_ENV_DEFAULT_FILE}"
				defaultFilePath = defaultGlobalEnvPath
			}
		} else {
			// App-specific .env (e.g. .env.app.prowlarr)
			// Use appenv.ListAppVarLines with ":" suffix for app-specific file (mirrors appvars_lines.sh)
			currentLines, _ = appenv.ListAppVarLines(ctx, tab.spec.App+":", cfg)

			if !appenv.IsAppUserDefined(ctx, tab.spec.App, envPath) {
				// Bash: DefaultAppEnvFile="$(run_script 'app_instance_file' "${APPNAME}" ".env.app.*")"
				defaultFilePath, _ = appenv.AppInstanceFile(ctx, tab.spec.App, ".env.app.*")
			}
		}

		// Now format the lines using the identified current and default sources
		// We use a temp file for currentLines to pass to FormatLines (mirrors Bash mktemp usage)
		currentLinesFile, _ := os.CreateTemp("", "dockstarter2.env_editor_current.*.tmp")
		for _, line := range currentLines {
			currentLinesFile.WriteString(line + "\n")
		}
		currentLinesFile.Close()

		formattedLines, _ := appenv.FormatLines(
			ctx,
			currentLinesFile.Name(),
			defaultFilePath,
			tab.spec.App,
			envPath,
		)
		os.Remove(currentLinesFile.Name())

		content := strings.Join(formattedLines, "\n")

		// Apply to editor with builtin defaults and read-only vars
		tabBuiltinDefaults := make(map[string]string)
		if defaultFilePath != "" {
			if content, err := os.ReadFile(defaultFilePath); err == nil {
				lines := strings.Split(string(content), "\n")
				for _, line := range lines {
					idx := strings.Index(line, "=")
					if idx > 0 {
						key := strings.TrimSpace(line[:idx])
						if strings.HasPrefix(key, "#") {
							key = strings.TrimPrefix(key, "#")
							key = strings.TrimSpace(key)
						}
						val := strings.TrimSpace(line[idx+1:])
						tabBuiltinDefaults[key] = val
					}
				}
			}
		}

		var tabReadOnlyVars []string
		if tab.spec.IsGlobal && tab.spec.App == "" {
			tabReadOnlyVars = readOnlyVars
		}
		m.tabs[i].defaultFilePath = defaultFilePath
		m.tabs[i].builtinDefaults = tabBuiltinDefaults
		m.tabs[i].readOnlyVars = tabReadOnlyVars

		m.tabs[i].editor.ParseEnv(content, tabBuiltinDefaults, tabReadOnlyVars)

		// Intelligent addition configuration
		if tab.spec.IsGlobal {
			if tab.spec.App != "" {
				// App variables inside global .env
				m.tabs[i].editor.AddPrefix = "APPNAME__"
				m.tabs[i].editor.ValidationType = "APPNAME"
				m.tabs[i].editor.ValidationAppName = tab.spec.App
			} else {
				// Pure global variables
				m.tabs[i].editor.ValidationType = "_GLOBAL_"
			}
		} else {
			// App-specific .env file
			m.tabs[i].editor.ValidationType = "_BARE_"
		}
		m.tabs[i].editor.ValidateFunc = func(name string, vType string) bool {
			return appenv.VarNameIsValid(name, vType)
		}
		
		// Recapture initial vars for surgical syncing
		initialVars, _ := appenv.ListVarsLiteralsData(content)
		m.tabs[i].initialVars = initialVars

		// Apply theme-aware styles
		editorStyles := m.tabs[i].editor.Styles()
		editorStyles.Focused.InvalidText = tui.SemanticRawStyle("Theme_EnvInvalid")
		editorStyles.Focused.DuplicateText = tui.SemanticRawStyle("Theme_EnvDuplicate")
		editorStyles.Focused.BuiltinText = tui.SemanticRawStyle("Theme_EnvBuiltin")
		editorStyles.Focused.UserDefinedText = tui.SemanticRawStyle("Theme_EnvUser")
		
		editorStyles.Blurred.InvalidText = tui.SemanticRawStyle("Theme_EnvInvalid")
		editorStyles.Blurred.DuplicateText = tui.SemanticRawStyle("Theme_EnvDuplicate")
		editorStyles.Blurred.BuiltinText = tui.SemanticRawStyle("Theme_EnvBuiltin")
		editorStyles.Blurred.UserDefinedText = tui.SemanticRawStyle("Theme_EnvUser")
		
		m.tabs[i].editor.SetStyles(editorStyles)
	}

	// Ensure the active tab editor gets focus if envFocusEditor is set
	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		m.tabs[m.activeTab].editor.Focus()
	}

	return envLoadDoneMsg{}
}

func (m *TabbedVarsEditorModel) saveEnv() tea.Cmd {
	return func() tea.Msg {
		cfg := config.LoadAppConfig()
		envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)

		// Create a snapshot of the current state to pass to the task
		type tabUpdate struct {
			file        string
			initialVars map[string]string
			newVars     map[string]string
		}
		var updates []tabUpdate

		re := regexp.MustCompile(`^\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*=(.*)`)

		for _, tab := range m.tabs {
			content := tab.editor.GetContent()
			// Parse content into map (literals)
			newVars := make(map[string]string)
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				matches := re.FindStringSubmatch(line)
				if matches != nil {
					key := matches[1]
					val := matches[2] // Literal: everything after =
					newVars[key] = val
				}
			}

			var targetFile string
			if tab.spec.IsGlobal {
				targetFile = envPath
			} else {
				targetFile = filepath.Join(cfg.ComposeDir, constants.AppEnvFileNamePrefix+tab.spec.App)
			}
			updates = append(updates, tabUpdate{
				file:        targetFile,
				initialVars: tab.initialVars,
				newVars:     newVars,
			})
		}

		task := func(ctx context.Context, w io.Writer) error {
			// Wrap context with the TUI writer so all logs in this task fan out to the ProgramBox
			ctx = console.WithTUIWriter(ctx, w)

			// 1. Surgical Sync for each tab
			for _, u := range updates {
				if err := appenv.SyncVariables(ctx, u.file, u.initialVars, u.newVars); err != nil {
					return err
				}
			}

			// 2. Sanitize and Update
			// These already log details which will fan out to the ProgramBox
			if err := appenv.SanitizeEnv(ctx, envPath, cfg); err != nil {
				return err
			}
			if err := appenv.Update(ctx, true, envPath); err != nil {
				return err
			}

			// Signal completion to the TUI (this will refresh the editor via envSaveSuccessMsg)
			tui.Send(envSaveSuccessMsg{})
			return nil
		}

		dialog := tui.NewProgramBoxModel("{{|Theme_TitleSuccess|}}Saving Environment Variables{{[-]}}", "Applying surgical environment updates...", "")
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return tui.ShowDialogMsg{Dialog: dialog}
	}
}

func (m *TabbedVarsEditorModel) hasErrors() bool {
	for _, tab := range m.tabs {
		if tab.editor.HasValidationErrors() {
			return true
		}
	}
	return false
}

func (m *TabbedVarsEditorModel) refreshEnv() tea.Msg {
	ctx := context.Background()

	// 1. Build a global live variables map from all tabs
	// and create a master master temp file for header detection.
	liveEnabled := make(map[string]bool)
	masterTemp, _ := os.CreateTemp("", "dockstarter2.refresh.master.*.tmp")
	for _, t := range m.tabs {
		content := t.editor.GetContent()
		masterTemp.WriteString(content + "\n")
		vars, _ := appenv.ListVarsData(content)
		for k := range vars {
			trimmedKey := strings.TrimSpace(k)
			if strings.HasSuffix(trimmedKey, "__ENABLED") {
				appName := strings.TrimSuffix(trimmedKey, "__ENABLED")
				liveEnabled[strings.ToUpper(appName)] = true
			}
		}
	}
	masterTemp.Close()
	masterPath := masterTemp.Name()
	defer os.Remove(masterPath)

	for i := range m.tabs {
		tab := &m.tabs[i]
		content := tab.editor.GetContent()
		lines := strings.Split(content, "\n")

		var validLines []string
		var invalidLines []string

		for _, line := range lines {
			isInvalid := false
			eqIdx := strings.Index(line, "=")
			if eqIdx > 0 {
				key := strings.TrimSpace(line[:eqIdx])
				vType := tab.editor.ValidationType
				if vType == "APPNAME" {
					vType = tab.editor.ValidationAppName
				}
				if tab.editor.ValidateFunc != nil && !tab.editor.ValidateFunc(key, vType) {
					isInvalid = true
				}
			} else if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				// Content but no '=' - invalid/incomplete
				isInvalid = true
			}

			if isInvalid {
				invalidLines = append(invalidLines, line)
			} else {
				validLines = append(validLines, line)
			}
		}

		// Format validLines
		tempFile, _ := os.CreateTemp("", "dockstarter2.refresh.*.tmp")
		for _, l := range validLines {
			tempFile.WriteString(l + "\n")
		}
		tempFile.Close()
		defer os.Remove(tempFile.Name())

		// Selective template loading:
		// If an app's ENABLED variable is missing from ALL tabs, treat it as user-defined.
		useDefaultFilePath := tab.defaultFilePath
		useBuiltinDefaults := tab.builtinDefaults
		if tab.spec.App != "" {
			if !liveEnabled[strings.ToUpper(tab.spec.App)] {
				useDefaultFilePath = ""
				useBuiltinDefaults = nil
			}
		}

		formatted, _ := appenv.FormatLines(
			ctx,
			tempFile.Name(),
			useDefaultFilePath,
			tab.spec.App,
			masterPath, // Pass the master state here to determine accurate header status
		)

		// Append invalid lines to the end
		if len(invalidLines) > 0 {
			if len(formatted) > 0 && formatted[len(formatted)-1] != "" {
				formatted = append(formatted, "")
			}
			formatted = append(formatted, "### Invalid / Incomplete (To be fixed)")
			formatted = append(formatted, invalidLines...)
		}

		newContent := strings.Join(formatted, "\n")
		tab.editor.ParseEnv(newContent, useBuiltinDefaults, tab.readOnlyVars)

		// Recapture initial vars if we want save to be surgical against the refreshed state
		initialVars, _ := appenv.ListVarsLiteralsData(newContent)
		tab.initialVars = initialVars
	}

	return envLoadDoneMsg{}
}

func (m *TabbedVarsEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tui.LayerHitMsg:
		if strings.HasPrefix(msg.ID, "tabbed_vars.tab-") {
			tabIdxStr := strings.TrimPrefix(msg.ID, "tabbed_vars.tab-")
			if idx, err := strconv.Atoi(tabIdxStr); err == nil && idx >= 0 && idx < len(m.tabs) {
				m.focus = envFocusEditor
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Blur()
				}
				m.activeTab = idx
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Focus()
				}
				return m, nil
			}
		}

		if msg.ID == "tabbed_vars.editor" {
			m.focus = envFocusEditor
			if len(m.tabs) > 0 {
				m.tabs[m.activeTab].editor.Focus()

				// Calculate relative coordinates for the editor click
				// Hit region is at dialogOffsetX + 2, dialogOffsetY + 2
				relX := msg.X - (m.lastOffsetX + 2)
				relY := msg.Y - (m.lastOffsetY + 2)

				var cmd tea.Cmd
				m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(tea.MouseClickMsg{
					X:      relX,
					Y:      relY,
					Button: msg.Button,
				})
				return m, cmd
			}
			return m, nil
		}

		// Button clicks
		if strings.HasPrefix(msg.ID, "tabbed_vars.btn-") || strings.HasPrefix(msg.ID, "btn-") {
			btnID := strings.TrimPrefix(msg.ID, "tabbed_vars.")
			btnIdxStr := strings.TrimPrefix(btnID, "btn-")
			if idx, err := strconv.Atoi(btnIdxStr); err == nil {
				m.focus = envFocusButtons
				m.btnIdx = idx
				switch m.btnIdx {
				case 0: // Save
					if m.hasErrors() {
						return m, func() tea.Msg {
							return tui.ShowMessageDialogMsg{
								Title:   "Validation Error",
								Message: "Cannot save while there are invalid variable names (highlighted in red) or incomplete lines.",
								Type:    tui.MessageError,
							}
						}
					}
					return m, m.saveEnv()
				case 1: // Back
					return m, m.onClose
				case 2: // Exit
					return m, tui.ConfirmExitAction()
				}
			}
		}

	case tui.LayerWheelMsg, tea.MouseWheelMsg:
		var wheelBtn tea.MouseButton
		if mwMsg, ok := msg.(tea.MouseWheelMsg); ok {
			wheelBtn = mwMsg.Button
		} else if lwMsg, ok := msg.(tui.LayerWheelMsg); ok {
			wheelBtn = lwMsg.Button
		}

		if (wheelBtn == tea.MouseWheelUp || wheelBtn == tea.MouseWheelDown) && len(m.tabs) > 0 {
			var cmd tea.Cmd
			m.focus = envFocusEditor
			m.tabs[m.activeTab].editor.Focus()

			// Translate wheel to up/down arrows for enveditor
			keyMsg := tea.KeyPressMsg{Code: tea.KeyUp}
			if wheelBtn == tea.MouseWheelDown {
				keyMsg = tea.KeyPressMsg{Code: tea.KeyDown}
			}
			m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(keyMsg)
			return m, cmd
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, m.onClose
		case "ctrl+n", "ctrl+right": // Next Tab
			if m.focus == envFocusEditor && len(m.tabs) > 1 {
				m.tabs[m.activeTab].editor.Blur()
				m.activeTab = (m.activeTab + 1) % len(m.tabs)
				m.tabs[m.activeTab].editor.Focus()
				return m, nil
			}
		case "ctrl+p", "ctrl+left": // Prev Tab
			if m.focus == envFocusEditor && len(m.tabs) > 1 {
				m.tabs[m.activeTab].editor.Blur()
				m.activeTab--
				if m.activeTab < 0 {
					m.activeTab = len(m.tabs) - 1
				}
				m.tabs[m.activeTab].editor.Focus()
				return m, nil
			}
		case "tab", "shift+tab":
			if m.focus == envFocusEditor {
				m.focus = envFocusButtons
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Blur()
				}
			} else {
				m.focus = envFocusEditor
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.Focus()
				}
			}
			return m, nil
		case "f5", "ctrl+r":
			return m, m.refreshEnv
		}

		if m.focus == envFocusButtons {
			switch msg.String() {
			case "left":
				m.btnIdx--
				if m.btnIdx < 0 {
					m.btnIdx = len(m.buttons) - 1
				}
			case "right":
				m.btnIdx++
				if m.btnIdx >= len(m.buttons) {
					m.btnIdx = 0
				}
			case "enter":
				switch m.btnIdx {
				case 0: // Save
					if m.hasErrors() {
						return m, func() tea.Msg {
							return tui.ShowMessageDialogMsg{
								Title:   "Validation Error",
								Message: "Cannot save while there are invalid variable names (highlighted in red) or incomplete lines.",
								Type:    tui.MessageError,
							}
						}
					}
					return m, m.saveEnv()
				case 1: // Back
					return m, m.onClose
				case 2: // Exit
					return m, tui.ConfirmExitAction()
				}
			}
		} else {
			// Specific editor hotkeys
			switch msg.String() {
			case "ctrl+d", "alt+backspace":
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.DeleteCurrentVariable()
				}
				return m, nil
			case "ctrl+a":
				return m, func() tea.Msg {
					keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
					if err == nil && keyName != "" {
						return envAddVarMsg{key: strings.TrimSpace(keyName)}
					}
					return nil
				}
			case "ctrl+r":
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.ResetCurrentVariable()
				}
				return m, nil
			case "ctrl+up":
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.MoveVariableUp()
				}
				return m, nil
			case "ctrl+down":
				if len(m.tabs) > 0 {
					m.tabs[m.activeTab].editor.MoveVariableDown()
				}
				return m, nil
			}
		}
	case tea.MouseMotionMsg:
		if m.focus == envFocusEditor && len(m.tabs) > 0 {
			editor := m.tabs[m.activeTab].editor
			if editor.IsDragging() {
				relX := msg.X - (m.lastOffsetX + 2)
				relY := msg.Y - (m.lastOffsetY + 2)
				var cmd tea.Cmd
				m.tabs[m.activeTab].editor, cmd = editor.Update(tea.MouseMotionMsg{
					X: relX,
					Y: relY,
				})
				return m, cmd
			}
		}
		return m, nil
	case tea.MouseReleaseMsg:
		if m.focus == envFocusEditor && len(m.tabs) > 0 {
			relX := msg.X - (m.lastOffsetX + 2)
			relY := msg.Y - (m.lastOffsetY + 2)
			var cmd tea.Cmd
			m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(tea.MouseReleaseMsg{
				X:      relX,
				Y:      relY,
				Button: msg.Button,
			})
			return m, cmd
		}
	case envSaveSuccessMsg:
		// Refresh variables to update "user defined" status (e.g. if APP__ENABLED was added)
		return m, tea.Batch(
			m.loadEnv,
			func() tea.Msg {
				return tui.ShowMessageDialogMsg{Title: "Success", Message: "Environment variables saved successfully.", Type: tui.MessageInfo}
			},
		)
	case envAddVarMsg:
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.AddVariable(msg.key, "")
		}
		return m, nil
	case envLoadDoneMsg:
		// Just trigger a re-render
		return m, nil
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}

	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		// Filter out raw mouse messages that fall through (unhandled clicks).
		// These shouldn't reach the editor as they would trigger unwanted scrolling.
		// Mouse interaction is handled via LayerHitMsg (clicks) or explicit MouseMotionMsg (dragging).
		isMouse := false
		switch msg.(type) {
		case tea.MouseClickMsg, tea.MouseWheelMsg, tea.MouseMotionMsg, tea.MouseReleaseMsg:
			isMouse = true
		}

		if !isMouse {
			var cmd tea.Cmd
			m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *TabbedVarsEditorModel) ViewString() string {
	ctx := tui.GetActiveContext()
	borderBG := ctx.Dialog.GetBackground()

	var sb strings.Builder

	if len(m.tabs) == 0 {
		return "No tabs available"
	}

	editor := m.tabs[m.activeTab].editor
	// Ensure no trailing newlines from the editor, which can cause 1-line overflows
	// in layout helpers like RenderBorderedBoxCtx or JoinVertical.
	editorView := strings.TrimRight(editor.View(), "\n")

	// Create tab rendering for the title
	// Each tab is treated as an independent title segment: ┫ Tab 1 ┣┫ Tab 2 ┣
	focused := m.focus == envFocusEditor

	var tabSegments []string
	for i, tab := range m.tabs {
		title := tab.spec.Title
		styleTag := "Theme_TitleSubMenu"
		if i == m.activeTab && focused {
			styleTag = "Theme_TitleSubMenuFocused"
		}
		// borderFocused = focused (submenu focus)
		// contentFocused = (i == m.activeTab && focused) (tab focus)
		// showIndicators = true (always reserve space for the indicator position)
		seg := tui.RenderTitleSegmentCtx(title, focused, i == m.activeTab && focused, true, styleTag, ctx)
		tabSegments = append(tabSegments, seg)
	}

	// Join segments with zero gap as requested: ┫Tab1┣┫Tab2┣
	tabRow := strings.Join(tabSegments, "")

	// Create an inner box using RenderBorderedBoxCtx
	totalWidth := editor.Width()
	// Pass titleTag="RAW" to use our multi-title row exactly as constructed
	editorWithBorder := tui.RenderBorderedBoxCtx(tabRow, editorView, totalWidth, editor.Height()+2, focused, false, false, ctx.SubmenuTitleAlign, "RAW", ctx)

	// Replace the bottom border with the scroll-percent variant if the editor content overflows
	// The editor's totalDisplayLines() vs Height() tells us if it can scroll.
	if editor.LineCount() > editor.Height() {
		// RenderBorderedBoxCtx produces content with height (editor.Height() + 2)
		// We need to replace the last line.
		lines := strings.Split(editorWithBorder, "\n")
		if len(lines) > 0 {
			// Replace the last line of the box (the bottom border)
			// Total visual width of the box is totalWidth + 2 (borders)
			bottomLine := tui.BuildScrollPercentBottomBorder(totalWidth+2, editor.ScrollPercent(), focused, ctx)
			lines[len(lines)-1] = bottomLine
			editorWithBorder = strings.Join(lines, "\n")
		}
	}

	sb.WriteString(editorWithBorder)
	sb.WriteString("\n")

	// Buttons
	var specs []tui.ButtonSpec
	for i, btn := range m.buttons {
		specs = append(specs, tui.ButtonSpec{
			Text:   btn,
			Active: m.focus == envFocusButtons && m.btnIdx == i,
		})
	}
	btnsRendered := tui.RenderCenteredButtonsCtx(totalWidth, ctx, specs...)

	spacer := lipgloss.NewStyle().Width(totalWidth).Background(borderBG).Render(" ")

	fullContent := lipgloss.JoinVertical(lipgloss.Left, strings.TrimRight(sb.String(), "\n"), spacer, btnsRendered)

	// Wrap in dialog styling
	dialogBox := tui.RenderDialogWithType(m.title, fullContent, true, m.height, tui.DialogTypeInfo)

	return dialogBox
}

func (m *TabbedVarsEditorModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *TabbedVarsEditorModel) Title() string {
	return m.title
}

func (m *TabbedVarsEditorModel) HelpText() string {
	if len(m.tabs) > 1 {
		return "Use Ctrl+Left and Ctrl+Right to switch tabs. Edit variables for the selected configuration."
	}
	return "Edit global variables for all containers"
}

func (m *TabbedVarsEditorModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	layout := tui.GetLayout()

	// width/height are already the content area dimensions.
	// We use helpers to avoid hardcoding overheads.
	// availableH is the total interior space of the dialog after outer borders and buttons.
	// We pass 1 for headerHeight to account for the tab row.
	availableH := layout.DialogContentHeight(height, 1, true, false)

	// Inner overhead inside the dialog:
	// - 1 line for the spacer above buttons
	// - 2 lines for the inner border added by RenderBorderedBoxCtx
	editorHeight := availableH - 3
	editorWidth := width - 4

	if editorWidth < 10 {
		editorWidth = 10
	}
	if editorHeight < 5 {
		editorHeight = 5
	}

	for i := range m.tabs {
		m.tabs[i].editor.SetWidth(editorWidth)
		m.tabs[i].editor.SetHeight(editorHeight)
	}
}

func (m *TabbedVarsEditorModel) SetFocused(f bool) {
	if f {
		if m.focus == envFocusEditor && len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.Focus()
		}
	} else {
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.Blur()
		}
	}
}

func (m *TabbedVarsEditorModel) IsMaximized() bool {
	return true
}

func (m *TabbedVarsEditorModel) HasDialog() bool {
	return false
}

func (m *TabbedVarsEditorModel) MenuName() string {
	return "tabbed_vars"
}

func (m *TabbedVarsEditorModel) ShortHelp() []key.Binding {
	if m.focus == envFocusEditor {
		b := []key.Binding{
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "buttons")),
			key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete var")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
		if len(m.tabs) > 1 {
			b = append(b, key.NewBinding(key.WithKeys("ctrl+left/right", "ctrl+p/n"), key.WithHelp("ctrl+←/→", "switch tabs")))
		}
		return b
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "select")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("sh+tab", "editor")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (m *TabbedVarsEditorModel) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion

	viewStr := m.ViewString()
	dialogW := lipgloss.Width(viewStr)
	dialogH := lipgloss.Height(viewStr)

	m.lastOffsetX = offsetX
	m.lastOffsetY = offsetY

	// Y=0: dialog top border
	// Y=1: editorWithBorder top border (where tabs are)
	// Y=2: editor text start
	contentY := offsetY + 1

	// Editor hit region
	editorW := 10
	editorH := 10
	if len(m.tabs) > 0 {
		editorW = m.tabs[m.activeTab].editor.Width() // Use editor.Width()
		editorH = m.tabs[m.activeTab].editor.Height()
	}

	regions = append(regions, tui.HitRegion{
		ID:     "tabbed_vars.editor",
		X:      offsetX + 2,  // inner border left
		Y:      contentY + 1, // skip the tab row
		Width:  editorW,
		Height: editorH,
		ZOrder: tui.ZDialog + 5,
	})

	// Tab hit regions
	if len(m.tabs) > 0 {
		ctx := tui.GetActiveContext()
		editorWidth := m.tabs[m.activeTab].editor.Width()

		// Calculate total width of all tabs to determine centering offset
		totalTitleWidth := 0
		var tabWidths []int
		for _, tab := range m.tabs {
			w := tui.WidthOfTitleSegment(tab.spec.Title, true, ctx)
			tabWidths = append(tabWidths, w)
			totalTitleWidth += w
		}

		// Replicate RenderBorderedBoxCtx centering logic
		actualWidth := editorWidth
		if totalTitleWidth > actualWidth {
			actualWidth = totalTitleWidth
		}

		leftPad := 0
		if ctx.SubmenuTitleAlign != "left" {
			leftPad = (actualWidth - totalTitleWidth) / 2
		}

		tabX := offsetX + 1 + leftPad // start after Border.TopLeft
		for i, tabWidth := range tabWidths {
			regions = append(regions, tui.HitRegion{
				ID:     "tabbed_vars.tab-" + strconv.Itoa(i),
				X:      tabX,
				Y:      contentY,
				Width:  tabWidth,
				Height: 1,
				ZOrder: tui.ZDialog + 10,
			})
			tabX += tabWidth
		}
	}

	// Buttons hit regions
	var buttonSpecs []tui.ButtonSpec
	for i, btn := range m.buttons {
		buttonSpecs = append(buttonSpecs, tui.ButtonSpec{
			Text:   btn,
			Active: m.focus == envFocusButtons && m.btnIdx == i,
			ZoneID: "btn-" + strconv.Itoa(i),
		})
	}

	// Buttons are at the bottom of the dialog. One border char at bottom.
	buttonY := offsetY + dialogH - 4
	regions = append(regions, tui.GetButtonHitRegions("tabbed_vars", offsetX+1, buttonY, dialogW-2, tui.ZDialog+10, buttonSpecs...)...)

	return regions
}

func (m *TabbedVarsEditorModel) FullHelp() [][]key.Binding {
	return [][]key.Binding{m.ShortHelp()}
}

// IsScrollbarDragging returns true if the current editor is dragging a line or a scrollbar.
func (m *TabbedVarsEditorModel) IsScrollbarDragging() bool {
	if len(m.tabs) > 0 {
		return m.tabs[m.activeTab].editor.IsDragging()
	}
	return false
}
