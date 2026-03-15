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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
)

type envFocusArea int

const (
	envFocusEditor envFocusArea = iota
	envFocusButtons
)

// headingLabelW is the fixed label column width, matching bash menu_heading.sh's
// LabelWidth which is the max across ALL possible labels: "Original Value: " = 16.
// Using the maximum keeps values aligned at the same column across all screens.
const headingLabelW = menuLabelW

// headingLabel right-aligns label to headingLabelW (e.g. "    Variable: ").
func headingLabel(label string) string {
	return fmt.Sprintf("%*s", headingLabelW, label)
}

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
	// Cached heading display info (populated during loadEnv)
	envFilePath string // Actual .env file being edited
	niceName    string // App nicename (empty for global tabs)
	description string // App description (empty for global tabs or if unavailable)
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
	buttonHeight  int
	subtitleHeight int
	editorHeight  int
	contentWidth  int
	focused       bool

	// Callbacks
	onClose tea.Cmd

	// Last hit region offsets for coordinate translation
	lastOffsetX int
	lastOffsetY int
}

type envAddVarMsg struct {
	key string
}

type envAddVarTemplateMsg struct {
	prefix string
}

type envAddAllStockMsg struct {
	vars     []string
	defaults map[string]string
}

type envSaveSuccessMsg struct{}
type envLoadDoneMsg struct{}

// ApplyVarValueMsg is dispatched by the context menu to set a variable's value in the active editor.
type ApplyVarValueMsg struct {
	VarName string
	Value   string
}

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

		// Apply to editor with builtin defaults and read-only vars.
		// Read the template file to discover which keys have defaults, then call
		// VarDefaultValue for each to get dynamically detected values (e.g.
		// GLOBAL_LAN_NETWORK, DOCKER_HOSTNAME) rather than static template placeholders.
		tabBuiltinDefaults := make(map[string]string)
		if defaultFilePath != "" {
			if fileContent, err := os.ReadFile(defaultFilePath); err == nil {
				lines := strings.Split(string(fileContent), "\n")
				for _, line := range lines {
					idx := strings.Index(line, "=")
					if idx > 0 {
						key := strings.TrimSpace(line[:idx])
						if strings.HasPrefix(key, "#") {
							key = strings.TrimPrefix(key, "#")
							key = strings.TrimSpace(key)
						}
						tabBuiltinDefaults[key] = appenv.VarDefaultValue(ctx, key, cfg)
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

		capturedCfg := cfg
		m.tabs[i].editor.DefaultValueFunc = func(varName string) string {
			return appenv.VarDefaultValue(context.Background(), varName, capturedCfg)
		}

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

		// Cache heading display info
		if tab.spec.App != "" {
			m.tabs[i].niceName = appenv.GetNiceName(ctx, tab.spec.App)
			if desc := appenv.GetDescription(ctx, tab.spec.App, envPath); desc != "! Missing description !" {
				m.tabs[i].description = desc
			}
		}
		if tab.spec.IsGlobal {
			m.tabs[i].envFilePath = envPath
		} else {
			m.tabs[i].envFilePath = appenv.GetAppEnvFile(tab.spec.App, cfg)
		}
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

				// Right-click opens the context menu for the clicked variable row.
				if msg.Button == tea.MouseRight {
					return m, m.showContextMenuForClick(msg.X, msg.Y)
				}

				// Calculate relative coordinates for the editor click
				// Hit region is at dialogOffsetX + 2, dialogOffsetY + 2 + subtitleHeight
				relX := msg.X - (m.lastOffsetX + 2)
				relY := msg.Y - (m.lastOffsetY + 2 + m.subtitleHeight)

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
		switch msg.ID {
		case tui.IDSaveButton:
			m.focus = envFocusButtons
			m.btnIdx = 0
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
		case tui.IDBackButton:
			m.focus = envFocusButtons
			m.btnIdx = 1
			if m.hasChanges() {
				return m, m.promptUnsavedChanges(m.onClose)
			}
			return m, m.onClose
		case tui.IDExitButton:
			m.focus = envFocusButtons
			m.btnIdx = 2
			if m.hasChanges() {
				return m, m.promptUnsavedChanges(tui.ConfirmExitAction())
			}
			return m, tui.ConfirmExitAction()
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
			if m.hasChanges() {
				return m, m.promptUnsavedChanges(m.onClose)
			}
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
		case "ctrl+ ", "shift+F10": // Keyboard equiv of right-click: open context menu at current cursor
			if m.focus == envFocusEditor && len(m.tabs) > 0 {
				editor := m.tabs[m.activeTab].editor
				editorTopY := m.lastOffsetY + 2 + m.subtitleHeight
				y := editorTopY + editor.Line() - editor.YOffset()
				x := m.lastOffsetX + 2
				return m, m.showContextMenuForClick(x, y)
			}
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
					if m.hasChanges() {
						return m, m.promptUnsavedChanges(m.onClose)
					}
					return m, m.onClose
				case 2: // Exit
					if m.hasChanges() {
						return m, m.promptUnsavedChanges(tui.ConfirmExitAction())
					}
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
				return m, m.showAddVarDialog()
			case "f2":
				return m, m.showSetValueDialog()
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
				relY := msg.Y - (m.lastOffsetY + 2 + m.subtitleHeight)
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
			relY := msg.Y - (m.lastOffsetY + 2 + m.subtitleHeight)
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
	case envAddVarTemplateMsg:
		prefix := msg.prefix
		return m, func() tea.Msg {
			keyName, err := tui.PromptText("Add Variable", "Enter variable name:", false)
			if err == nil && keyName != "" {
				keyName = strings.TrimSpace(strings.ToUpper(keyName))
				if !strings.HasPrefix(keyName, prefix) {
					keyName = prefix + keyName
				}
				return envAddVarMsg{key: keyName}
			}
			return nil
		}
	case envAddAllStockMsg:
		if len(m.tabs) > 0 {
			for _, key := range msg.vars {
				m.tabs[m.activeTab].editor.AddVariable(key, msg.defaults[key])
			}
		}
		return m, nil
	case ApplyVarValueMsg:
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.SetVariableValue(msg.VarName, msg.Value)
		}
		return m, nil
	case envLoadDoneMsg:
		// Recalculate layout now that heading data (niceName, description) is available.
		// Clear undo history — content has been reloaded so prior edits are irrelevant.
		for i := range m.tabs {
			m.tabs[i].editor.ClearUndo()
		}
		m.SetSize(m.width, m.height)
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
	if m.width == 0 || m.height == 0 {
		return ""
	}

	if len(m.tabs) == 0 {
		return "No tabs loaded"
	}

	tab := m.tabs[m.activeTab]
	editor := tab.editor
	editorView := editor.View()

	ctx := tui.GetActiveContext()
	focused := m.focus == envFocusEditor

	// 1. Render the tabs row (to be used as the title of the inner border)
	tabRow := m.renderTabs()

	// 2. Wrap the editor in its own inner border (matching MenuModel list style)
	// We use m.contentWidth for the inner box width.
	// Height includes editor + 2 lines for borders.
	innerBox := tui.RenderBorderedBoxCtx(
		tabRow,
		editorView,
		m.contentWidth-2,
		m.editorHeight+2,
		focused,
		false, // No focus indicators here
		true,  // Rounded corners to match submenu style
		ctx.SubmenuTitleAlign,
		"RAW", // Use the pre-rendered tabRow exactly
		ctx,
	)

	// 3. Add scroll percentage to the bottom border of the inner box if needed
	if editor.LineCount() > editor.Height() {
		lines := strings.Split(innerBox, "\n")
		if len(lines) > 0 {
			bottomLine := tui.BuildScrollPercentBottomBorder(m.contentWidth, editor.ScrollPercent(), focused, ctx)
			lines[len(lines)-1] = bottomLine
			innerBox = strings.Join(lines, "\n")
		}
	}

	// 4. Render buttons
	buttons := m.renderButtons(m.contentWidth)

	// 5. Join components vertically: subtitle (if any) → editor box → buttons
	var parts []string
	if subtitle := m.renderSubtitle(); subtitle != "" {
		parts = append(parts, subtitle)
	}
	parts = append(parts, innerBox, buttons)

	// Standardize TrimRight to prevent implicit gaps
	for i, part := range parts {
		parts[i] = strings.TrimRight(part, "\n")
	}
	fullContent := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// 6. Wrap in the outer dialog border
	// We pass m.contentWidth to RenderBorderedBoxCtx which will add borders (+2)
	// resulting in a total width of m.width.
	return tui.RenderBorderedBoxCtx(
		m.title,
		fullContent,
		m.contentWidth,
		m.height,
		m.focused,
		true, // Show indicators in the main title
		false,
		ctx.DialogTitleAlign,
		"Theme_Title",
		ctx,
	)
}

func (m *TabbedVarsEditorModel) View() tea.View {
	return tea.View{Content: m.ViewString()}
}

func (m *TabbedVarsEditorModel) getButtonSpecs() []tui.ButtonSpec {
	zoneIDs := []string{tui.IDSaveButton, tui.IDBackButton, tui.IDExitButton}
	var specs []tui.ButtonSpec
	for i, btn := range m.buttons {
		zoneID := ""
		if i < len(zoneIDs) {
			zoneID = zoneIDs[i]
		}
		specs = append(specs, tui.ButtonSpec{
			Text:   btn,
			Active: m.focus == envFocusButtons && m.btnIdx == i,
			ZoneID: zoneID,
		})
	}
	return specs
}

func (m *TabbedVarsEditorModel) renderButtons(width int) string {
	specs := m.getButtonSpecs()
	return tui.RenderCenteredButtonsExplicit(width, m.buttonHeight == tui.DialogButtonHeight, tui.GetActiveContext(), specs...)
}

func (m *TabbedVarsEditorModel) renderTabs() string {
	ctx := tui.GetActiveContext()
	focused := m.focus == envFocusEditor
	var tabSegments []string
	for i, tab := range m.tabs {
		title := tab.spec.Title
		styleTag := "Theme_TitleSubMenu"
		if i == m.activeTab && focused {
			styleTag = "Theme_TitleSubMenuFocused"
		}
		seg := tui.RenderTitleSegmentCtx(title, focused, i == m.activeTab && focused, true, styleTag, ctx)
		tabSegments = append(tabSegments, seg)
	}
	return strings.Join(tabSegments, "")
}

func (m *TabbedVarsEditorModel) Title() string {
	return m.title
}

func (m *TabbedVarsEditorModel) ShortHelp() []key.Binding {
	if m.focus == envFocusEditor {
		b := []key.Binding{tui.Keys.EnvRefresh, tui.Keys.EnvAddVar, tui.Keys.EnvDelete, tui.Keys.Esc, tui.Keys.Help}
		if len(m.tabs) > 1 {
			b = append(b, tui.Keys.CycleTab)
		}
		return b
	}
	return []key.Binding{tui.Keys.Left, tui.Keys.Right, tui.Keys.Enter, tui.Keys.CycleTab, tui.Keys.Esc}
}

func (m *TabbedVarsEditorModel) FullHelp() [][]key.Binding {
	nav := []key.Binding{
		key.NewBinding(key.WithKeys("up"), key.WithHelp("↑/↓/←/→", "navigate")),
		key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp/PgDn", "page up/down")),
		key.NewBinding(key.WithKeys("home"), key.WithHelp("Home/End", "top/bottom")),
	}
	if len(m.tabs) > 1 {
		nav = append(nav, tui.Keys.CycleTab, tui.Keys.CycleShiftTab)
	}

	return [][]key.Binding{
		nav,
		{
			tui.Keys.EnvRefresh,
			tui.Keys.EnvAddVar,
			tui.Keys.EnvInsert,
			tui.Keys.EnvDelete,
			key.NewBinding(key.WithKeys("ctrl+up"), key.WithHelp("Ctrl+↑/↓", "reorder row")),
			key.NewBinding(key.WithKeys("ctrl+z"), key.WithHelp("Ctrl+Z/Y", "undo/redo")),
			key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("Ctrl+C", "copy value/selection")),
			key.NewBinding(key.WithKeys("shift+left"), key.WithHelp("Shift+←/→/Home/End", "select text")),
			tui.Keys.EnvEditValue,
			key.NewBinding(key.WithKeys("ctrl+ "), key.WithHelp("right-click/Ctrl+Space", "value options")),
		},
		{tui.Keys.Tab, tui.Keys.Enter, tui.Keys.Esc, tui.Keys.ToggleLog, tui.Keys.Help, tui.Keys.ForceQuit},
	}
}

func (m *TabbedVarsEditorModel) HelpText() string {
	if m.focus != envFocusEditor || len(m.tabs) == 0 {
		return ""
	}
	meta, ok := m.tabs[m.activeTab].editor.CurrentLineMeta()
	if !ok || !meta.IsVariable {
		return ""
	}
	varName := meta.Text
	if idx := strings.Index(varName, "="); idx > 0 {
		varName = strings.TrimSpace(varName[:idx])
	}
	return appenv.GetVarHelpText(varName)
}

func (m *TabbedVarsEditorModel) SetSize(width, height int) {
	m.width = width
	m.height = height

	// width and height are the already-computed content area dimensions passed by AppModel.
	// Use them directly as dialog bounds, just like MenuModel does.
	// Budget for our internal components using standardized inner width.
	// Padding = 1 on each side (matches MenuModel's marginStyle).
	m.contentWidth = m.width - 2
	if m.contentWidth < 1 {
		m.contentWidth = 1
	}

	specs := m.getButtonSpecs()
	// Determine button height based on width availability (bordered=3, flat=1)
	m.buttonHeight = tui.ButtonRowHeight(m.contentWidth, 0, specs...)

	// Calculate subtitle height based on active tab heading content
	m.subtitleHeight = m.calcSubtitleHeight()

	// Inner vertical space inside dialog borders (dialogHeight - 2)
	innerH := m.height - 2

	// Available for the editor: total inner height minus button row, subtitle, and editor borders
	m.editorHeight = innerH - m.buttonHeight - m.subtitleHeight - 2
	if m.editorHeight < 1 {
		m.editorHeight = 1
	}
	if m.editorHeight < 3 && m.buttonHeight == 3 {
		// Fallback: force buttons flat to save 2 lines if editor would be too small
		m.buttonHeight = 1
		m.editorHeight = innerH - 1 - m.subtitleHeight - 2
	}

	if m.editorHeight < 1 {
		m.editorHeight = 1
	}

	editorWidth := m.contentWidth - 2 // Editor content width accounts for inner box borders (+2)
	if editorWidth < 10 {
		editorWidth = 10
	}

	for i := range m.tabs {
		m.tabs[i].editor.SetWidth(editorWidth)
		m.tabs[i].editor.SetHeight(m.editorHeight)
	}
}

func (m *TabbedVarsEditorModel) SetFocused(f bool) {
	m.focused = f
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


func (m *TabbedVarsEditorModel) MenuName() string {
	return "tabbed_vars"
}

// calcSubtitleHeight returns the number of subtitle lines for the active tab.
// Global tabs: 1 line (file path). App tabs: 1 line (app name) + wrapped description lines.
func (m *TabbedVarsEditorModel) calcSubtitleHeight() int {
	if len(m.tabs) == 0 || m.contentWidth < 4 {
		return 0
	}
	tab := m.tabs[m.activeTab]
	if tab.niceName == "" {
		// Global tab: just the file path, 1 line
		return 1
	}
	// App tab: "Application: AppName" (1 line) + word-wrapped description
	h := 1
	if tab.description != "" {
		valueW := m.contentWidth - headingLabelW
		if valueW < 10 {
			valueW = 10
		}
		h += subtitleWrapLines(tab.description, valueW)
	}
	return h
}

// subtitleWrapLines returns how many lines the text occupies when word-wrapped to maxWidth.
func subtitleWrapLines(text string, maxWidth int) int {
	if maxWidth <= 0 || text == "" {
		return 0
	}
	words := strings.Fields(text)
	lines, lineLen := 1, 0
	for _, w := range words {
		wl := len(w)
		if lineLen == 0 {
			lineLen = wl
		} else if lineLen+1+wl > maxWidth {
			lines++
			lineLen = wl
		} else {
			lineLen += 1 + wl
		}
	}
	return lines
}

// renderSubtitle renders the heading subtitle for the active tab.
// Returns a slice of styled lines (each already padded to contentWidth).
func (m *TabbedVarsEditorModel) renderSubtitle() string {
	if m.subtitleHeight == 0 || len(m.tabs) == 0 {
		return ""
	}
	tab := m.tabs[m.activeTab]
	ctx := tui.GetActiveContext()
	bgStyle := ctx.Dialog

	renderLine := func(raw string) string {
		processed := console.ToANSI(raw)
		w := lipgloss.Width(processed)
		padded := processed + strings.Repeat(" ", m.contentWidth-w)
		return tui.MaintainBackground(bgStyle.Render(padded), bgStyle)
	}

	var lines []string

	if tab.niceName == "" {
		// Global: show file path
		lines = append(lines, renderLine(headingLabel("File: ")+"{{|Theme_HeadingValue|}}"+tab.envFilePath+"{{[-]}}"))
	} else {
		// App: "Application: AppName" on first line
		appLine := headingLabel("Application: ") + "{{|Theme_HeadingValue|}}" + tab.niceName + "{{[-]}}"
		lines = append(lines, renderLine(appLine))

		// Word-wrap description onto continuation lines, indented to align with value
		if tab.description != "" {
			indent := strings.Repeat(" ", headingLabelW)
			valueW := m.contentWidth - headingLabelW
			if valueW < 10 {
				valueW = 10
			}
			for _, dl := range subtitleWrapText(tab.description, valueW) {
				lines = append(lines, renderLine(indent+"{{|Theme_HeadingAppDescription|}}"+dl+"{{[-]}}"))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// subtitleWrapText word-wraps text to maxWidth, returning a slice of lines.
func subtitleWrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 || text == "" {
		return nil
	}
	words := strings.Fields(text)
	var lines []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
		} else if cur.Len()+1+len(w) > maxWidth {
			lines = append(lines, cur.String())
			cur.Reset()
			cur.WriteString(w)
		} else {
			cur.WriteByte(' ')
			cur.WriteString(w)
		}
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}


func (m *TabbedVarsEditorModel) GetHitRegions(offsetX, offsetY int) []tui.HitRegion {
	var regions []tui.HitRegion

	// The dialog itself has a border of 1 on each side.
	// The content area starts at offsetX+1, offsetY+1.
	// The content area has width m.contentWidth and height m.height - 2.

	m.lastOffsetX = offsetX
	m.lastOffsetY = offsetY

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
		// Inner box content width = m.contentWidth - 2 (accounts for inner box borders).
		innerContentW := m.contentWidth - 2
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
		// Inner box starts at offsetX+1 (inside outer border), tabs start after inner TopLeft corner.
		tabX := offsetX + 2 + leftPad // outer border(1) + inner border TopLeft(1) + leftPad
		for i, tabWidth := range tabWidths {
			regions = append(regions, tui.HitRegion{
				ID:     "tabbed_vars.tab-" + strconv.Itoa(i),
				X:      tabX,
				Y:      offsetY + 1 + m.subtitleHeight, // outer border + subtitle + inner border line with tabs
				Width:  tabWidth,
				Height: 1,
				ZOrder: tui.ZDialog + 10,
			})
			tabX += tabWidth
		}
	}

	// Editor hit region
	// Editor content is inside both outer border and inner border.
	regions = append(regions, tui.HitRegion{
		ID:     "tabbed_vars.editor",
		X:      offsetX + 2,                         // outer border(1) + inner border(1)
		Y:      offsetY + 1 + m.subtitleHeight + 1,  // outer border + subtitle + inner border/tabs
		Width:  m.contentWidth - 2,                  // inner box content width
		Height: m.editorHeight,
		ZOrder: tui.ZDialog + 5,
	})

	// Button regions (standardized width)
	btnY := m.height - m.buttonHeight - 1
	regions = append(regions, tui.GetButtonHitRegions("", offsetX+1, offsetY+btnY, m.contentWidth, tui.ZDialog+10, m.getButtonSpecs()...)...)

	return regions
}


// showContextMenuForClick builds and shows a right-click context menu for the variable row
// at screen position (x, y).  Returns nil (no-op) when the click is outside a variable row.
func (m *TabbedVarsEditorModel) showContextMenuForClick(x, y int) tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	tab := &m.tabs[m.activeTab]
	editor := tab.editor

	// Compute which editor row was clicked.
	// Editor content starts at: outer border (1) + subtitle + inner border/tab row (1) = lastOffsetY + 2 + subtitleHeight
	editorTopY := m.lastOffsetY + 2 + m.subtitleHeight
	clickedRow := (y - editorTopY) + editor.YOffset()

	meta, ok := editor.LineMetaAt(clickedRow)
	if !ok || !meta.IsVariable {
		return nil
	}

	// Extract the variable name (everything before '=').
	varName := meta.Text
	if idx := strings.Index(varName, "="); idx > 0 {
		varName = strings.TrimSpace(varName[:idx])
	}
	if varName == "" {
		return nil
	}

	// Get current value first so we can offer "Original Value" in the submenu.
	currentVal := editor.GetVariableValue(varName)

	// Build "Set Value" submenu: "Original Value" first, then GetVarOptions presets.
	opts := appenv.GetVarOptions(varName, strings.ToUpper(tab.spec.App), tab.builtinDefaults[varName])
	var subItems []tui.ContextMenuItem
	// Prepend "Original Value" so the user can always revert.
	origVn := varName
	origV := currentVal
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

	// Main menu: Edit Value | Set Value ▶ | Copy | Paste Value
	var items []tui.ContextMenuItem
	{
		evVarName := varName
		evOrigVal := currentVal
		evOpts := make([]appenv.VarOption, len(opts)+1)
		evOpts[0] = appenv.VarOption{Display: "Original Value", Value: evOrigVal, Help: "Restore the value from before editing."}
		copy(evOpts[1:], opts)
		evTab := tab
		items = append(items, tui.ContextMenuItem{
			Label: "Edit Value",
			Help:  "Open the value editor for this variable.",
			Action: func() tea.Msg {
				dlg := newSetValueDialog(evVarName, evTab.niceName, evTab.description, evOrigVal, evOpts)
				return tui.CloseDialogMsg{Result: tui.ShowDialogMsg{Dialog: dlg}}
			},
		})
	}
	items = append(items, tui.ContextMenuItem{
		Label:    "Set Value",
		Help:     "Choose or reset this variable's value.",
		SubItems: subItems,
	})
	selectedText := editor.GetSelectedText()
	copyText := selectedText
	if copyText == "" {
		copyText = currentVal
	}
	copyLabel := "Copy Value"
	if selectedText != "" {
		copyLabel = "Copy Selection"
	}
	ct := copyText
	items = append(items, tui.ContextMenuItem{
		Label: copyLabel,
		Help:  "Copy to clipboard.",
		Action: func() tea.Msg {
			_ = clipboard.WriteAll(ct)
			return tui.CloseDialogMsg{}
		},
	})
	vn2 := varName
	items = append(items, tui.ContextMenuItem{
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

	return func() tea.Msg {
		return tui.ShowDialogMsg{Dialog: tui.NewContextMenuModel(x, y, m.width, m.height, items)}
	}
}

// showAddVarMenu builds and shows an "Add Variable" context menu for the active tab.
// For app tabs (IsGlobal, non-empty App) it offers template prefixes, stock variables
// not yet in the editor, and an "Add All" option. For other tabs it falls back to a
// free-form PromptText dialog.
func (m *TabbedVarsEditorModel) showAddVarMenu() tea.Cmd {
	if len(m.tabs) == 0 {
		return nil
	}
	tab := &m.tabs[m.activeTab]

	// Non-app tabs: free-form name prompt.
	if tab.spec.App == "" || !tab.spec.IsGlobal {
		return func() tea.Msg {
			keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
			if err == nil && keyName != "" {
				return envAddVarMsg{key: strings.TrimSpace(strings.ToUpper(keyName))}
			}
			return nil
		}
	}

	appUpper := strings.ToUpper(tab.spec.App)
	editor := tab.editor

	// --- Template items (prefix + user-completed suffix via PromptText) ---
	type tpl struct {
		prefix string
		label  string
		help   string
	}
	templates := []tpl{
		{appUpper + "__", appUpper + "__…", "Complete this with any variable name you want."},
		{appUpper + "__ENVIRONMENT_", appUpper + "__ENVIRONMENT_…", "Complete with a var for the environment: section of your override."},
		{appUpper + "__PORT_", appUpper + "__PORT_…", "Complete with a port number for the ports: section of your override."},
		{appUpper + "__VOLUME_", appUpper + "__VOLUME_…", "Complete with a path for the volumes: section of your override."},
	}

	var items []tui.ContextMenuItem
	for _, t := range templates {
		t := t
		items = append(items, tui.ContextMenuItem{
			Label: t.label,
			Help:  t.help,
			Action: func() tea.Msg {
				return tui.CloseDialogMsg{Result: envAddVarTemplateMsg{prefix: t.prefix}}
			},
		})
	}

	// --- Stock variables (only those not already present in the editor) ---
	type stock struct {
		name string
		help string
	}
	allStock := []stock{
		{appUpper + "__CONTAINER_NAME", "Used in the container_name: section of your override."},
		{appUpper + "__HOSTNAME", "Used in the hostname: section of your override."},
		{appUpper + "__NETWORK_MODE", "Used in the network_mode: section of your override."},
		{appUpper + "__RESTART", "Used in the restart: section of your override."},
		{appUpper + "__TAG", "Used in the image: tag section of your override."},
	}

	// Include ENABLED for builtin apps that don't yet have it.
	if appenv.IsAppBuiltIn(appUpper) && !editor.HasVariable(appUpper+"__ENABLED") {
		allStock = append([]stock{{
			appUpper + "__ENABLED",
			"Creating this variable causes DockSTARTer to manage this app with no override needed.",
		}}, allStock...)
	}

	var missingStock []stock
	for _, s := range allStock {
		if !editor.HasVariable(s.name) {
			missingStock = append(missingStock, s)
		}
	}

	if len(missingStock) > 0 {
		items = append(items, tui.ContextMenuItem{IsSeparator: true})

		// "Add All" option
		addAllVars := make([]string, 0, len(missingStock))
		addAllDefaults := make(map[string]string, len(missingStock))
		for _, s := range missingStock {
			addAllVars = append(addAllVars, s.name)
			addAllDefaults[s.name] = tab.builtinDefaults[s.name]
		}
		av := addAllVars
		ad := addAllDefaults
		items = append(items, tui.ContextMenuItem{
			Label: "Add All Stock Variables",
			Help:  "Add all stock variables listed below with their default values.",
			Action: func() tea.Msg {
				return tui.CloseDialogMsg{Result: envAddAllStockMsg{vars: av, defaults: ad}}
			},
		})

		// Individual stock items
		for _, s := range missingStock {
			s := s
			defVal := tab.builtinDefaults[s.name]
			items = append(items, tui.ContextMenuItem{
				Label:    s.name,
				SubLabel: defVal,
				Help:     s.help,
				Action: func() tea.Msg {
					return tui.CloseDialogMsg{Result: envAddVarMsg{key: s.name}}
				},
			})
		}
	}

	// Position menu near the editor top-left
	x := m.lastOffsetX + 2
	y := m.lastOffsetY + 2 + m.subtitleHeight
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
				return envAddVarMsg{key: strings.TrimSpace(strings.ToUpper(keyName))}
			}
			return nil
		}
	}

	appUpper := strings.ToUpper(tab.spec.App)
	editor := tab.editor

	templates := []struct{ prefix, label, help string }{
		{appUpper + "__", appUpper + "__…", "Complete this with any variable name you want."},
		{appUpper + "__ENVIRONMENT_", appUpper + "__ENVIRONMENT_…", "Complete with a var for the environment: section of your override."},
		{appUpper + "__PORT_", appUpper + "__PORT_…", "Complete with a port number for the ports: section of your override."},
		{appUpper + "__VOLUME_", appUpper + "__VOLUME_…", "Complete with a path for the volumes: section of your override."},
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
		allStock = append([]stockDef{{appUpper + "__ENABLED", "Creating this variable causes DockSTARTer to manage this app with no override needed."}}, allStock...)
	}

	var stockItems []addVarItem
	addAllVars := make([]string, 0)
	addAllDefaults := make(map[string]string)
	for _, s := range allStock {
		if !editor.HasVariable(s.name) {
			defVal := tab.builtinDefaults[s.name]
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
	opts := appenv.GetVarOptions(varName, appUpper, tab.builtinDefaults[varName])
	// Always offer "Original Value" first so the user can revert.
	opts = append([]appenv.VarOption{{
		Display: "Original Value",
		Value:   origVal,
		Help:    "Restore the value from before editing.",
	}}, opts...)
	dlg := newSetValueDialog(varName, tab.niceName, tab.description, origVal, opts)
	return func() tea.Msg {
		return tui.ShowDialogMsg{Dialog: dlg}
	}
}

// IsScrollbarDragging returns true if the current editor is dragging a line or a scrollbar.
func (m *TabbedVarsEditorModel) IsScrollbarDragging() bool {
	if len(m.tabs) > 0 {
		return m.tabs[m.activeTab].editor.IsDragging()
	}
	return false
}

func (m *TabbedVarsEditorModel) hasChanges() bool {
	for _, tab := range m.tabs {
		currentVars, _ := appenv.ListVarsLiteralsData(tab.editor.GetContent())
		if len(currentVars) != len(tab.initialVars) {
			return true
		}
		for k, v := range currentVars {
			if initialV, ok := tab.initialVars[k]; !ok || initialV != v {
				return true
			}
		}
	}
	return false
}

func (m *TabbedVarsEditorModel) promptUnsavedChanges(onConfirm tea.Cmd) tea.Cmd {
	return func() tea.Msg {
		if tui.Confirm("Unsaved Changes", "You have unsaved changes. Do you want to leave without saving?", false) {
			return onConfirm()
		}
		return nil
	}
}

func (m *TabbedVarsEditorModel) HasDialog() bool {
	return false
}

// HelpContext implements tui.HelpContextProvider.
// Returns heading-style info about the variable under the cursor shown at the top of the help dialog.
// contentWidth is the available display width (used to word-wrap descriptions).
func (m *TabbedVarsEditorModel) HelpContext(contentWidth int) string {
	if m.focus != envFocusEditor || len(m.tabs) == 0 {
		return ""
	}
	tab := m.tabs[m.activeTab]
	meta, ok := tab.editor.CurrentLineMeta()
	if !ok || !meta.IsVariable {
		return ""
	}

	varName := meta.Text
	if idx := strings.Index(varName, "="); idx > 0 {
		varName = strings.TrimSpace(varName[:idx])
	}
	if varName == "" {
		return ""
	}

	var currentValue string
	if idx := strings.Index(meta.Text, "="); idx > 0 {
		currentValue = meta.Text[idx+1:]
	}

	// VarIsUserDefined: for app vars the IsUserDefined flag covers the var level;
	// for global vars it means the var itself is user-defined (not in defaults).
	varIsUserDefined := meta.IsUserDefined && tab.niceName == ""

	params := MenuHeadingParams{
		AppName:          tab.niceName,
		AppDescription:   tab.description,
		AppIsUserDefined: meta.IsUserDefined && tab.niceName != "",
		FilePath:         tab.envFilePath,
		VarName:          varName,
		VarIsUserDefined: varIsUserDefined,
		CurrentValue:     currentValue,
	}

	heading := FormatMenuHeading(params, contentWidth)

	// Append variable description (plain text — word-wrapped at source by FormatMenuHeading
	// for AppDescription; variable help text is appended as-is and word-wrapped in the
	// help dialog since it may come from an external source).
	if desc := appenv.GetVarHelpText(varName); desc != "" {
		heading += "\n\n" + desc
	}

	return heading
}
