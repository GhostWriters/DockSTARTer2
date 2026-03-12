package screens

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/enveditor"

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
	spec   EnvTabSpec
	editor enveditor.Model
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

func NewEnvEditorGlobal(onClose tea.Cmd) *TabbedVarsEditorModel {
	return NewTabbedVarsEditorScreen(onClose, " Global Variables ", []EnvTabSpec{
		{Title: ".env", App: "", IsGlobal: true},
	})
}

func NewTabbedVarsEditorScreen(onClose tea.Cmd, title string, specs []EnvTabSpec) *TabbedVarsEditorModel {
	var tabs []envTab
	for _, spec := range specs {
		editor := enveditor.New()
		editor.ShowLineNumbers = true
		tabs = append(tabs, envTab{spec: spec, editor: editor})
	}

	return &TabbedVarsEditorModel{
		tabs:      tabs,
		activeTab: 0,
		title:     title,
		buttons:   []string{"Save", "Add Variable", "Cancel"},
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

	// Parity with menu_config_vars.sh lines 31-45:
	builtinDefaults := make(map[string]string)
	if m.tabs[0].spec.App == "" {
		// Use existing appenv logic to get defaults for global variables
		// We can populate builtinDefaults from the same defaultGlobalEnvPath
		if content, err := os.ReadFile(defaultGlobalEnvPath); err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				idx := strings.Index(line, "=")
				if idx > 0 {
					key := strings.TrimSpace(line[:idx])
					if strings.HasPrefix(key, "#") {
						key = strings.TrimSpace(key[1:])
					}
					val := strings.TrimSpace(line[idx+1:])
					builtinDefaults[key] = val
				}
			}
		}
	}
	readOnlyVars := []string{"HOME", "DOCKER_CONFIG_FOLDER", "DOCKER_COMPOSE_FOLDER"}

	for i, tab := range m.tabs {
		var currentLines []string
		var defaultFilePath string

		if tab.spec.IsGlobal {
			// Get current lines using existing appenv.ListAppVarLines (mirrors appvars_lines.sh)
			currentLines, _ = appenv.ListAppVarLines(ctx, tab.spec.App, cfg)

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

		// Apply to editor with builtin defaults and read-only vars for global editor
		var tabBuiltinDefaults map[string]string
		var tabReadOnlyVars []string
		if tab.spec.IsGlobal && tab.spec.App == "" {
			tabBuiltinDefaults = builtinDefaults
			tabReadOnlyVars = readOnlyVars
		}

		m.tabs[i].editor.ParseEnv(content, tabBuiltinDefaults, tabReadOnlyVars)
	}
	
	// Ensure the active tab editor gets focus if envFocusEditor is set
	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		m.tabs[m.activeTab].editor.Focus()
	}

	return nil
}

func (m *TabbedVarsEditorModel) saveEnv() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cfg := config.LoadAppConfig()
		envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
		
		for _, tab := range m.tabs {
			if tab.spec.IsGlobal {
				// Surgical update for global .env file
				// We parse the editor content to get the literal values as seen by the user
				content := tab.editor.GetContent()
				
				// Use a temporary file to leverage appenv.ListVars for parsing
				tmpFile, err := os.CreateTemp("", "ds2_vars_edit.*.env")
				if err != nil {
					return tui.ShowMessageDialogMsg{Title: "Save Error", Message: fmt.Sprintf("Failed to create temporary file: %v", err), Type: tui.MessageError}
				}
				defer os.Remove(tmpFile.Name())
				_ = os.WriteFile(tmpFile.Name(), []byte(content), 0644)
				
				vars, err := appenv.ListVars(tmpFile.Name())
				if err != nil {
					return tui.ShowMessageDialogMsg{Title: "Save Error", Message: fmt.Sprintf("Failed to parse variables: %v", err), Type: tui.MessageError}
				}
				
				// Surgical apply
				for k, v := range vars {
					// We use SetLiteral because the editor shows things like HOME=${DETECTED_HOMEDIR}
					// and we want to preserve those literals.
					_ = appenv.SetLiteral(ctx, k, v, envPath)
				}
				// Run appenv.Update to ensure the file is sorted and formatted correctly
				// without destroying manually added orphans or comments.
				_ = appenv.Update(ctx, false, envPath)
				
			} else {
				// Save app specific file (full override is fine for .env.app.*)
				instanceDir := paths.GetInstanceDir(tab.spec.App)
				appEnvPath := filepath.Join(instanceDir, constants.AppEnvFileNamePrefix+tab.spec.App)
				_ = os.WriteFile(appEnvPath, []byte(tab.editor.GetContent()), 0644)
				_ = appenv.Update(ctx, true, appEnvPath)
			}
		}
		
		return tui.ShowMessageDialogMsg{Title: "Success", Message: "Environment variables saved successfully.", Type: tui.MessageInfo}
	}
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
		if strings.HasPrefix(msg.ID, "btn-") || strings.HasPrefix(msg.ID, "tabbed_vars.btn-") {
			btnID := msg.ID
			if strings.HasPrefix(btnID, "tabbed_vars.") {
				btnID = strings.TrimPrefix(btnID, "tabbed_vars.")
			}
			btnIdxStr := strings.TrimPrefix(btnID, "btn-")
			if idx, err := strconv.Atoi(btnIdxStr); err == nil {
				m.focus = envFocusButtons
				m.btnIdx = idx
				switch m.btnIdx {
				case 0: // Save
					return m, m.saveEnv()
				case 1: // Add Variable
					return m, func() tea.Msg {
						keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
						if err == nil && keyName != "" {
							return envAddVarMsg{key: strings.TrimSpace(keyName)}
						}
						return nil
					}
				case 2: // Cancel
					return m, m.onClose
				}
			}
		}

	case tui.LayerWheelMsg, tea.MouseWheelMsg:
		// Handle both semantic LayerWheelMsg and raw tea.MouseWheelMsg.
		// AppModel forwards raw MouseWheelMsg for non-list panels, while
		// LayerWheelMsg is used for list panels.
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
			if m.focus == envFocusEditor {
				if len(m.tabs) > 1 {
					m.tabs[m.activeTab].editor.Blur()
					m.activeTab = (m.activeTab + 1) % len(m.tabs)
					m.tabs[m.activeTab].editor.Focus()
				}
				return m, nil
			}
		case "ctrl+p", "ctrl+left": // Prev Tab
			if m.focus == envFocusEditor {
				if len(m.tabs) > 1 {
					m.tabs[m.activeTab].editor.Blur()
					m.activeTab--
					if m.activeTab < 0 {
						m.activeTab = len(m.tabs) - 1
					}
					m.tabs[m.activeTab].editor.Focus()
				}
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
					return m, m.saveEnv()
				case 1: // Add Variable
					return m, func() tea.Msg {
						keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
						if err == nil && keyName != "" {
							return envAddVarMsg{key: strings.TrimSpace(keyName)}
						}
						return nil
					}
				case 2: // Cancel
					return m, m.onClose
				}
			}
			return m, nil
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
			}
		}
	case envAddVarMsg:
		if len(m.tabs) > 0 {
			m.tabs[m.activeTab].editor.AddVariable(msg.key, "")
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}

	if m.focus == envFocusEditor && len(m.tabs) > 0 {
		var cmd tea.Cmd
		m.tabs[m.activeTab].editor, cmd = m.tabs[m.activeTab].editor.Update(msg)
		cmds = append(cmds, cmd)
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
	editorView := editor.View()
	
	// Create tab rendering for the title
	var tabTitles []string
	for i, tab := range m.tabs {
		if i == m.activeTab {
			// highlighted tab
			if m.focus == envFocusEditor {
				tabTitles = append(tabTitles, "{{|Theme_TagSelected|}}["+tab.spec.Title+"]{{[-]}}")
			} else {
				tabTitles = append(tabTitles, "{{|Theme_Item|}}["+tab.spec.Title+"]{{[-]}}")
			}
		} else {
			tabTitles = append(tabTitles, "{{|Theme_Tag|}}"+tab.spec.Title+"{{[-]}}")
		}
	}
	tabRow := " " + strings.Join(tabTitles, " | ") + " "

	// Create an inner box using RenderBorderedBoxCtx
	editorWithBorder := tui.RenderBorderedBoxCtx(tabRow, editorView, editor.Width(), editor.Height()+2, m.focus == envFocusEditor, false, "left", "", ctx)
	
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
	btnsRendered := tui.RenderCenteredButtonsCtx(editor.Width(), ctx, specs...)

	contentWidth := editor.Width()
	spacer := lipgloss.NewStyle().Width(contentWidth).Background(borderBG).Render("")

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
	// We need 2 lines for the outer border, 2 for the inner border, 
	// 3 lines for buttons, and 1 for spacer.
	editorWidth := width - 4
	editorHeight := height - 4 - layout.ButtonHeight - 1

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
		editorW = m.tabs[m.activeTab].editor.Width()
		editorH = m.tabs[m.activeTab].editor.Height()
	}

	regions = append(regions, tui.HitRegion{
		ID:     "tabbed_vars.editor",
		X:      offsetX + 2, // inner border left
		Y:      contentY + 1, // skip the tab row
		Width:  editorW,
		Height: editorH,
		ZOrder: tui.ZDialog + 5,
	})

	// Tab hit regions
	if len(m.tabs) > 0 {
		tabX := offsetX + 5 // approximation for where tabs start
		for i, tab := range m.tabs {
			tabWidth := lipgloss.Width("[" + tab.spec.Title + "]")
			regions = append(regions, tui.HitRegion{
				ID:     "tabbed_vars.tab-" + strconv.Itoa(i),
				X:      tabX,
				Y:      contentY,
				Width:  tabWidth,
				Height: 1,
				ZOrder: tui.ZDialog + 10,
			})
			tabX += tabWidth + 1 // space between tabs
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

