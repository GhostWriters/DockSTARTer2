package screens

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/serve"
	"DockSTARTer2/internal/tui"

	tea "charm.land/bubbletea/v2"
)

// ServerOptionsFocus defines which area of the screen has focus.
type ServerOptionsFocus int

const (
	FocusServerSettings ServerOptionsFocus = iota
	FocusServerStatus
	FocusServerButtons
)

// ServerOptionsScreen allows the user to configure the SSH (and web) server.
type ServerOptionsScreen struct {
	settingsMenu *tui.MenuModel
	statusMenu   *tui.MenuModel
	outerMenu    *tui.MenuModel

	focusedPanel  ServerOptionsFocus
	focusedButton int
	isRoot        bool

	config config.AppConfig

	width  int
	height int
	focused bool
}

// updateServerOptionMsg is sent when an option is changed in the menu.
type updateServerOptionMsg struct {
	update func(*config.AppConfig)
}

// serverStatusRefreshMsg is sent to trigger a re-read of live session state.
type serverStatusRefreshMsg struct{}

// NewServerOptionsScreen creates a new server settings screen.
func NewServerOptionsScreen(isRoot bool) *ServerOptionsScreen {
	cfg := config.LoadAppConfig()
	s := &ServerOptionsScreen{
		isRoot:  isRoot,
		config:  cfg,
		focused: true,
	}
	s.initMenus()
	return s
}

func (s *ServerOptionsScreen) initMenus() {
	s.settingsMenu = s.buildSettingsMenu()
	s.statusMenu = s.buildStatusMenu()

	var outerBack tea.Cmd
	if !s.isRoot {
		outerBack = navigateBack()
	}
	outerMenu := tui.NewMenuModel("server_outer", "Server Settings", "", nil, outerBack)
	outerMenu.SetShowExit(true)
	outerMenu.SetButtonLabels("Apply", "Back", "Exit")
	outerMenu.AddContentSection(s.settingsMenu)
	outerMenu.AddContentSection(s.statusMenu)
	s.outerMenu = outerMenu

	s.focusedPanel = FocusServerSettings
	s.updateFocusStates()
}

func (s *ServerOptionsScreen) buildSettingsMenu() *tui.MenuModel {
	authModeDesc := func() string {
		switch s.config.Server.Auth.Mode {
		case "password":
			return "Password"
		case "pubkey":
			return "Public Key"
		default:
			return "None (insecure)"
		}
	}

	items := []tui.MenuItem{
		// SSH Server section heading
		{
			Tag:        "── SSH Server ──",
			Desc:       "",
			Selectable: false,
		},
		{
			Tag:         "Enable SSH Server",
			Desc:        "Allow remote access to DS2 via SSH",
			Help:        "Toggle SSH server on/off (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Server.Enabled,
			Selectable:  true,
			SpaceAction: s.toggleSSHEnabled(),
		},
		{
			Tag:    "SSH Port",
			Desc:   fmt.Sprintf("{{|OptionValue|}}%d{{[-]}}", s.config.Server.SSH.Port),
			Help:   "TCP port the SSH server listens on (Enter to change)",
			Action: s.promptSSHPort(),
		},
		{
			Tag:    "Auth Mode",
			Desc:   s.dropdownDesc(authModeDesc()),
			Help:   "Authentication mode for incoming SSH connections (Enter for options)",
			Action: s.showAuthModeDropdown(),
		},
		{
			Tag:    "Password",
			Desc:   s.passwordDesc(),
			Help:   "Password for SSH auth (Enter to change). Stored as bcrypt hash.",
			Action: s.promptPassword(),
		},
		{
			Tag:    "Authorized Keys File",
			Desc:   s.truncatePath(s.config.Server.Auth.AuthKeysFile),
			Help:   "Path to authorized_keys file for public-key auth (Enter to change)",
			Action: s.promptAuthKeysFile(),
		},
		// Web Server section heading
		{
			Tag:        "── Web Server ──",
			Desc:       "",
			Selectable: false,
		},
		{
			Tag:         "Enable Web Server",
			Desc:        "Serve DS2 TUI in a browser via xterm.js (requires SSH)",
			Help:        "Toggle web server on/off (Space to toggle)",
			IsCheckbox:  true,
			Checked:     s.config.Server.Web.Enabled,
			Selectable:  true,
			SpaceAction: s.toggleWebEnabled(),
		},
		{
			Tag:    "Web Port",
			Desc:   fmt.Sprintf("{{|OptionValue|}}%d{{[-]}}", s.config.Server.Web.Port),
			Help:   "TCP port the web server listens on (Enter to change)",
			Action: s.promptWebPort(),
		},
	}

	menu := tui.NewMenuModel("server_settings", "Configuration", "", items, nil)
	menu.SetHelpItemPrefix("Setting")
	menu.SetHelpPageText("Configure remote access to the DS2 TUI. SSH must be enabled and configured before the web server can be used.")
	menu.SetSubMenuMode(true)
	menu.SetIsDialog(false)
	menu.SetShowExit(false)
	menu.SetFlowMode(true)
	menu.SetMaximized(true)
	return menu
}

func (s *ServerOptionsScreen) buildStatusMenu() *tui.MenuModel {
	serverInfo := serve.Sessions.ReadServerInfo()
	sessionInfo := serve.Sessions.ReadSessionInfo()

	serverStatus := "{{|TitleError|}}Not running{{[-]}}"
	if serverInfo.PID != 0 {
		serverStatus = fmt.Sprintf("{{|Yes|}}Running{{[-]}} (PID %d, port %d)", serverInfo.PID, serverInfo.Port)
	}

	sessionStatus := "None"
	disconnectEnabled := false
	if sessionInfo.PID != 0 {
		ip := sessionInfo.ClientIP
		if ip == "" {
			ip = "unknown"
		}
		sessionStatus = fmt.Sprintf("{{|TitleQuestion|}}Active{{[-]}} — %s (PID %d)", ip, sessionInfo.PID)
		disconnectEnabled = true
	}

	items := []tui.MenuItem{
		{
			Tag:        "Server",
			Desc:       serverStatus,
			Help:       "Current SSH server state",
			Selectable: false,
		},
		{
			Tag:        "Session",
			Desc:       sessionStatus,
			Help:       "Active remote session, if any",
			Selectable: false,
		},
		{
			Tag:    "Disconnect Session",
			Desc:   "Request graceful disconnect of active session",
			Help:   "Send a graceful disconnect request to the active session",
			Action: s.disconnectAction(false, disconnectEnabled),
		},
		{
			Tag:    "Force Disconnect",
			Desc:   "{{|TitleError|}}Immediately kill active session{{[-]}}",
			Help:   "Forcibly kill the active session process",
			Action: s.disconnectAction(true, disconnectEnabled),
		},
	}

	menu := tui.NewMenuModel("server_status", "Live Status", "", items, nil)
	menu.SetHelpItemPrefix("Status")
	menu.SetHelpPageText("Live status of the SSH server and any active remote session. Use Disconnect to gracefully close a session.")
	menu.SetSubMenuMode(true)
	menu.SetIsDialog(false)
	menu.SetShowExit(false)
	menu.SetFlowMode(true)
	menu.SetMaximized(true)
	return menu
}

// refreshStatus rebuilds the status menu from live session data and reconnects
// it to the outer menu (which uses a slice reference that must be replaced).
func (s *ServerOptionsScreen) refreshStatus() {
	s.statusMenu = s.buildStatusMenu()
	// Rebuild the outer container so it holds the updated section reference.
	var outerBack tea.Cmd
	if !s.isRoot {
		outerBack = navigateBack()
	}
	outer := tui.NewMenuModel("server_outer", "Server Settings", "", nil, outerBack)
	outer.SetShowExit(true)
	outer.SetButtonLabels("Apply", "Back", "Exit")
	outer.AddContentSection(s.settingsMenu)
	outer.AddContentSection(s.statusMenu)
	if s.width != 0 && s.height != 0 {
		outer.SetSize(s.width, s.height)
	}
	outer.SetFocused(s.focused)
	s.outerMenu = outer
	s.updateFocusStates()
}

// syncSettingsMenu updates item labels/states from s.config without rebuilding.
func (s *ServerOptionsScreen) syncSettingsMenu() {
	items := s.settingsMenu.GetItems()

	// indices: 0=heading, 1=enable SSH, 2=SSH port, 3=auth mode, 4=password, 5=authkeys file,
	//           6=heading, 7=enable web, 8=web port
	items[1].Checked = s.config.Server.Enabled
	items[2].Desc = fmt.Sprintf("{{|OptionValue|}}%d{{[-]}}", s.config.Server.SSH.Port)
	items[3].Desc = s.dropdownDesc(s.authModeLabel())
	items[4].Desc = s.passwordDesc()
	items[5].Desc = s.truncatePath(s.config.Server.Auth.AuthKeysFile)
	items[7].Checked = s.config.Server.Web.Enabled
	items[8].Desc = fmt.Sprintf("{{|OptionValue|}}%d{{[-]}}", s.config.Server.Web.Port)

	s.settingsMenu.SetItems(items)
}

func (s *ServerOptionsScreen) authModeLabel() string {
	switch s.config.Server.Auth.Mode {
	case "password":
		return "Password"
	case "pubkey":
		return "Public Key"
	default:
		return "None (insecure)"
	}
}

func (s *ServerOptionsScreen) dropdownDesc(val string) string {
	return fmt.Sprintf("{{|OptionValue|}}%s▼{{[-]}}", val)
}

func (s *ServerOptionsScreen) passwordDesc() string {
	if s.config.Server.Auth.Password == "" {
		return "{{|TitleError|}}(not set){{[-]}}"
	}
	return "{{|Yes|}}(set){{[-]}}"
}

func (s *ServerOptionsScreen) truncatePath(p string) string {
	if p == "" {
		return "{{|TitleError|}}(not set){{[-]}}"
	}
	const maxLen = 40
	if len(p) > maxLen {
		return "…" + p[len(p)-maxLen+1:]
	}
	return p
}

// ── Toggle Actions ──────────────────────────────────────────────────────────

func (s *ServerOptionsScreen) toggleSSHEnabled() tea.Cmd {
	return func() tea.Msg {
		return updateServerOptionMsg{func(cfg *config.AppConfig) {
			cfg.Server.Enabled = !cfg.Server.Enabled
		}}
	}
}

func (s *ServerOptionsScreen) toggleWebEnabled() tea.Cmd {
	return func() tea.Msg {
		return updateServerOptionMsg{func(cfg *config.AppConfig) {
			cfg.Server.Web.Enabled = !cfg.Server.Web.Enabled
		}}
	}
}

// ── Prompt Actions ──────────────────────────────────────────────────────────

func (s *ServerOptionsScreen) promptSSHPort() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "SSH Port", "Enter SSH port number", false)
		if err != nil {
			return nil
		}
		port, err := strconv.Atoi(strings.TrimSpace(result))
		if err != nil || port < 1 || port > 65535 {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Port",
				Message: "Port must be a number between 1 and 65535.",
				Type:    tui.MessageError,
			}
		}
		return updateServerOptionMsg{func(cfg *config.AppConfig) {
			cfg.Server.SSH.Port = port
		}}
	}
}

func (s *ServerOptionsScreen) promptWebPort() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "Web Port", "Enter web server port number", false)
		if err != nil {
			return nil
		}
		port, err := strconv.Atoi(strings.TrimSpace(result))
		if err != nil || port < 1 || port > 65535 {
			return tui.ShowMessageDialogMsg{
				Title:   "Invalid Port",
				Message: "Port must be a number between 1 and 65535.",
				Type:    tui.MessageError,
			}
		}
		return updateServerOptionMsg{func(cfg *config.AppConfig) {
			cfg.Server.Web.Port = port
		}}
	}
}

func (s *ServerOptionsScreen) promptPassword() tea.Cmd {
	return func() tea.Msg {
		pw, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "SSH Password", "Enter new password", true)
		if err != nil {
			return nil
		}
		pw = strings.TrimSpace(pw)
		if pw == "" {
			return nil
		}
		hash, err := serve.HashPassword(pw)
		if err != nil {
			return tui.ShowMessageDialogMsg{
				Title:   "Password Error",
				Message: fmt.Sprintf("Failed to hash password: %v", err),
				Type:    tui.MessageError,
			}
		}
		return updateServerOptionMsg{func(cfg *config.AppConfig) {
			cfg.Server.Auth.Password = hash
		}}
	}
}

func (s *ServerOptionsScreen) promptAuthKeysFile() tea.Cmd {
	return func() tea.Msg {
		result, err := console.TextPrompt(context.Background(),
			func(context.Context, any, ...any) {}, "Authorized Keys File",
			"Enter path to authorized_keys file", false)
		if err != nil {
			return nil
		}
		result = strings.TrimSpace(result)
		if result == "" {
			return nil
		}
		return updateServerOptionMsg{func(cfg *config.AppConfig) {
			cfg.Server.Auth.AuthKeysFile = result
		}}
	}
}

// ── Auth Mode Dropdown ───────────────────────────────────────────────────────

func (s *ServerOptionsScreen) showAuthModeDropdown() tea.Cmd {
	return func() tea.Msg {
		modes := []struct {
			value string
			label string
			help  string
		}{
			{"none", "None {{|TitleError|}}(insecure — anyone can connect){{[-]}}", "No authentication — all connections are accepted. Only use on a trusted network."},
			{"password", "Password", "Authenticate with a plain password stored as a bcrypt hash in dockstarter2.toml."},
			{"pubkey", "Public Key", "Authenticate using SSH public keys listed in an authorized_keys file."},
		}

		current := s.config.Server.Auth.Mode
		if current == "" {
			current = "none"
		}

		var items []tui.MenuItem
		for i, m := range modes {
			mode := m.value
			label := m.label
			help := m.help
			selected := i
			_ = selected
			items = append(items, tui.MenuItem{
				Tag:  label,
				Help: help,
				Action: func() tea.Msg {
					return tea.Batch(
						func() tea.Msg {
							return updateServerOptionMsg{func(cfg *config.AppConfig) {
								cfg.Server.Auth.Mode = mode
							}}
						},
						tui.CloseDialog(),
					)()
				},
			})
		}

		menu := tui.NewMenuModel("auth_mode_dropdown", "Auth Mode", "Select authentication mode", items, tui.CloseDialog())
		menu.SetShowExit(false)

		// Pre-select current mode
		for i, m := range modes {
			if m.value == current {
				menu.Select(i)
				break
			}
		}

		return tui.ShowDialogMsg{Dialog: menu}
	}
}

// ── Disconnect Action ────────────────────────────────────────────────────────

func (s *ServerOptionsScreen) disconnectAction(force bool, enabled bool) tea.Cmd {
	if !enabled {
		return func() tea.Msg {
			return tui.ShowMessageDialogMsg{
				Title:   "No Active Session",
				Message: "There is no active remote session to disconnect.",
				Type:    tui.MessageInfo,
			}
		}
	}
	return func() tea.Msg {
		go func() {
			_ = serve.Disconnect(context.Background(), force)
		}()
		return serverStatusRefreshMsg{}
	}
}

// ── Apply ────────────────────────────────────────────────────────────────────

func (s *ServerOptionsScreen) handleApply() tea.Cmd {
	return func() tea.Msg {
		if err := config.SaveAppConfig(s.config); err != nil {
			return tui.ShowMessageDialogMsg{
				Title:   "Save Failed",
				Message: fmt.Sprintf("Could not save server settings: %v", err),
				Type:    tui.MessageError,
			}
		}
		return tui.ShowMessageDialogMsg{
			Title:   "Settings Saved",
			Message: "Server settings saved. Restart the SSH server for changes to take effect.",
			Type:    tui.MessageSuccess,
		}
	}
}

// ── Focus ────────────────────────────────────────────────────────────────────

func (s *ServerOptionsScreen) maxFocusedButton() int {
	if s.isRoot {
		return 1
	}
	return 2
}

func (s *ServerOptionsScreen) execFocusedButton() (tea.Model, tea.Cmd) {
	switch s.focusedButton {
	case 0:
		return s, s.handleApply()
	case 1:
		if s.isRoot {
			return s, tui.ConfirmExitAction()
		}
		return s, navigateBack()
	case 2:
		return s, tui.ConfirmExitAction()
	}
	return s, nil
}

func (s *ServerOptionsScreen) updateFocusStates() {
	s.settingsMenu.SetSubFocused(s.focused && s.focusedPanel == FocusServerSettings)
	s.statusMenu.SetSubFocused(s.focused && s.focusedPanel == FocusServerStatus)

	if s.outerMenu == nil {
		return
	}
	s.outerMenu.SetFocused(s.focused)
	if s.focusedPanel == FocusServerButtons {
		switch s.focusedButton {
		case 0:
			s.outerMenu.SetFocusedItem(tui.FocusSelectBtn)
		case 1:
			if s.isRoot {
				s.outerMenu.SetFocusedItem(tui.FocusExitBtn)
			} else {
				s.outerMenu.SetFocusedItem(tui.FocusBackBtn)
			}
		case 2:
			s.outerMenu.SetFocusedItem(tui.FocusExitBtn)
		}
	} else {
		s.outerMenu.SetFocusedItem(tui.FocusList)
	}
	s.outerMenu.InvalidateCache()
}

func (s *ServerOptionsScreen) SetFocused(f bool) {
	s.focused = f
	s.updateFocusStates()
}

// ── ScreenModel interface ────────────────────────────────────────────────────

func (s *ServerOptionsScreen) Init() tea.Cmd {
	return tea.Batch(s.settingsMenu.Init(), s.statusMenu.Init())
}

func (s *ServerOptionsScreen) Title() string {
	return "Server Settings"
}

func (s *ServerOptionsScreen) HelpText() string {
	if s.focusedPanel == FocusServerSettings {
		return s.settingsMenu.HelpText()
	}
	if s.focusedPanel == FocusServerStatus {
		return s.statusMenu.HelpText()
	}
	return "Tab to cycle panels, Enter to Apply, Esc to Cancel"
}

func (s *ServerOptionsScreen) IsMaximized() bool {
	return true
}

func (s *ServerOptionsScreen) MenuName() string {
	return "server"
}

// MinHeight: outer border(2) + settings section(5) + status section(4) + buttons(3) = 14.
func (s *ServerOptionsScreen) MinHeight() int {
	return 14
}

func (s *ServerOptionsScreen) HasDialog() bool {
	if s.settingsMenu == nil || s.statusMenu == nil {
		return false
	}
	return s.settingsMenu.HasDialog() || s.statusMenu.HasDialog()
}

func (s *ServerOptionsScreen) IsScrollbarDragging() bool {
	return s.settingsMenu.IsScrollbarDragging() || s.statusMenu.IsScrollbarDragging()
}

func (s *ServerOptionsScreen) HelpContext(maxWidth int) tui.HelpContext {
	screenName := s.outerMenu.Title()
	pageText := "Configure remote access to the DS2 TUI. SSH must be enabled and configured before the web server can be used."

	var inner tui.HelpContext
	switch s.focusedPanel {
	case FocusServerSettings:
		inner = s.settingsMenu.HelpContext(maxWidth)
	case FocusServerStatus:
		inner = s.statusMenu.HelpContext(maxWidth)
	}

	inner.ScreenName = screenName
	inner.PageTitle = "Description"
	inner.PageText = pageText
	return inner
}
