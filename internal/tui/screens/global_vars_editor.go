package screens

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/envutil"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/tui"
	"DockSTARTer2/internal/tui/components/enveditor"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type focusArea int

const (
	focusEditor focusArea = iota
	focusButtons
)

type GlobalVarsEditorModel struct {
	envEditor enveditor.Model
	width     int
	height    int

	focus focusArea
	
	// Action buttons
	buttons []string
	btnIdx  int

	// Callbacks
	onClose tea.Cmd
}

type addVarMsg struct {
	key string
}

func NewGlobalVarsEditorScreen(onClose tea.Cmd) *GlobalVarsEditorModel {
	editor := enveditor.New()
	editor.ShowLineNumbers = true

	return &GlobalVarsEditorModel{
		envEditor: editor,
		buttons:   []string{"Save", "Add Variable", "Cancel"},
		btnIdx:    0,
		focus:     focusEditor,
		onClose:   onClose,
	}
}

func (m *GlobalVarsEditorModel) Init() tea.Cmd {
	// load env file logic
	return m.loadEnv
}

func (m *GlobalVarsEditorModel) loadEnv() tea.Msg {
	ctx := context.Background()
	cfg := config.LoadAppConfig()
	envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
	
	allLines, err := envutil.ReadLines(envPath)
	if err != nil {
		allLines = []string{}
	}
	
	globalVars := appenv.AppVarsLines("", allLines)
	
	currentLinesFile, _ := os.CreateTemp("", "dockstarter2.env_editor.*.tmp")
	for _, line := range globalVars {
		currentLinesFile.WriteString(line + "\n")
	}
	currentLinesFile.Close()
	defer os.Remove(currentLinesFile.Name())

	configDir := paths.GetConfigDir()
	defaultEnvFile := filepath.Join(configDir, constants.EnvExampleFileName)

	formattedGlobals, _ := appenv.FormatLines(
		ctx,
		currentLinesFile.Name(),
		defaultEnvFile,
		"", // empty appName for globals
		envPath,
	)

	content := strings.Join(formattedGlobals, "\n")

	// Collect default values for built-ins to lock their keys
	builtinDefaults := make(map[string]string)
	defaultLines, _ := envutil.ReadLines(defaultEnvFile)
	for _, line := range defaultLines {
		idx := strings.Index(line, "=")
		if idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			builtinDefaults[key] = val
		}
	}

	readOnlyVars := []string{"HOME", "DOCKER_CONFIG_FOLDER", "DOCKER_COMPOSE_FOLDER"}
	m.envEditor.ParseEnv(content, builtinDefaults, readOnlyVars)
	return nil
}

func (m *GlobalVarsEditorModel) saveEnv() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cfg := config.LoadAppConfig()
		envPath := filepath.Join(cfg.ComposeDir, constants.EnvFileName)
		
		allLines, _ := envutil.ReadLines(envPath)
		
		var appLines []string
		appList, _ := appenv.ListReferencedApps(ctx, cfg)
		for _, app := range appList {
			appLines = append(appLines, appenv.AppVarsLines(app, allLines)...)
		}
		
		globalContent := m.envEditor.GetContent()
		combinedContent := globalContent
		if len(appLines) > 0 {
			combinedContent += "\n" + strings.Join(appLines, "\n")
		}
		
		_ = os.WriteFile(envPath, []byte(combinedContent), 0644)
		
		// Run the formatting update to clean up and inject the headers correctly
		_ = appenv.Update(ctx, true, envPath)
		
		return nil
	}
}

func (m *GlobalVarsEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, m.onClose
		case "tab", "shift+tab":
			if m.focus == focusEditor {
				m.focus = focusButtons
				m.envEditor.Blur()
			} else {
				m.focus = focusEditor
				m.envEditor.Focus()
			}
			return m, nil
		}

		if m.focus == focusButtons {
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
					return m, tea.Batch(m.saveEnv(), m.onClose)
				case 1: // Add Variable
					return m, func() tea.Msg {
						keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
						if err == nil && keyName != "" {
							return addVarMsg{key: strings.TrimSpace(keyName)}
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
				m.envEditor.DeleteCurrentVariable()
				return m, nil
			case "ctrl+a":
				return m, func() tea.Msg {
					keyName, err := tui.PromptText("Add Variable", "Enter new variable name:", false)
					if err == nil && keyName != "" {
						return addVarMsg{key: strings.TrimSpace(keyName)}
					}
					return nil
				}
			case "ctrl+r":
				m.envEditor.ResetCurrentVariable()
				return m, nil
			}
		}
	case addVarMsg:
		m.envEditor.AddVariable(msg.key, "")
		return m, nil
	case tea.WindowSizeMsg:
		m.SetSize(msg.Width, msg.Height)
	}

	if m.focus == focusEditor {
		var cmd tea.Cmd
		m.envEditor, cmd = m.envEditor.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *GlobalVarsEditorModel) ViewString() string {
	ctx := tui.GetActiveContext()
	borderBG := ctx.Dialog.GetBackground()

	var sb strings.Builder

	// Editor with inner border
	editorView := m.envEditor.View()
	// Create an inner box using RenderBorderedBoxCtx, passing an empty title to get just the border
	editorWithBorder := tui.RenderBorderedBoxCtx("", editorView, m.envEditor.Width(), m.envEditor.Height()+2, m.focus == focusEditor, false, "left", "", ctx)
	
	sb.WriteString(editorWithBorder)
	sb.WriteString("\n")

	// Buttons
	var specs []tui.ButtonSpec
	for i, btn := range m.buttons {
		specs = append(specs, tui.ButtonSpec{
			Text:   btn,
			Active: m.focus == focusButtons && m.btnIdx == i,
		})
	}
	btnsRendered := tui.RenderCenteredButtonsCtx(m.envEditor.Width(), ctx, specs...)

	contentWidth := m.envEditor.Width()
	spacer := lipgloss.NewStyle().Width(contentWidth).Background(borderBG).Render("")

	fullContent := lipgloss.JoinVertical(lipgloss.Left, strings.TrimRight(sb.String(), "\n"), spacer, btnsRendered)

	// Wrap in dialog styling
	dialogBox := tui.RenderDialogWithType(" Global Variables ", fullContent, true, 0, tui.DialogTypeInfo)

	// Center on the full screen
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialogBox)
}

func (m *GlobalVarsEditorModel) View() tea.View {
	return tea.NewView(m.ViewString())
}

func (m *GlobalVarsEditorModel) Title() string {
	return "Global Variables"
}

func (m *GlobalVarsEditorModel) HelpText() string {
	return "Edit global variables for all containers"
}

func (m *GlobalVarsEditorModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	
	dialogWidth := width - 10
	if dialogWidth > 120 {
		dialogWidth = 120
	} else if dialogWidth < 60 {
		dialogWidth = 60
	}

	dialogHeight := height - 12
	if dialogHeight > 40 {
		dialogHeight = 40
	} else if dialogHeight < 10 {
		dialogHeight = 10
	}

	// Adjust for inner border (2 width, 2 height)
	editorWidth := dialogWidth - 2
	if editorWidth < 2 {
		editorWidth = 2
	}
	editorHeight := dialogHeight - 2
	if editorHeight < 2 {
		editorHeight = 2
	}

	m.envEditor.SetWidth(editorWidth)
	m.envEditor.SetHeight(editorHeight)
}

func (m *GlobalVarsEditorModel) SetFocused(f bool) {
	if f {
		if m.focus == focusEditor {
			m.envEditor.Focus()
		}
	} else {
		m.envEditor.Blur()
	}
}

func (m *GlobalVarsEditorModel) IsMaximized() bool {
	return false
}

func (m *GlobalVarsEditorModel) HasDialog() bool {
	return false
}

func (m *GlobalVarsEditorModel) MenuName() string {
	return "global_vars"
}

func (m *GlobalVarsEditorModel) ShortHelp() []key.Binding {
	if m.focus == focusEditor {
		return []key.Binding{
			key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "buttons")),
			key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete var")),
			key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "reset default")),
			key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
		}
	}
	return []key.Binding{
		key.NewBinding(key.WithKeys("left", "right"), key.WithHelp("←/→", "select")),
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
		key.NewBinding(key.WithKeys("tab"), key.WithHelp("sh+tab", "editor")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (m *GlobalVarsEditorModel) FullHelp() [][]key.Binding {
	return [][]key.Binding{m.ShortHelp()}
}
