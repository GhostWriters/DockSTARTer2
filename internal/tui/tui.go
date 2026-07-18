package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/compose"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/docker"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/sessionlocks"
	"DockSTARTer2/internal/theme"
	"DockSTARTer2/internal/update"
	"DockSTARTer2/internal/version"
	"DockSTARTer2/internal/webmsg"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

var (
	// ErrUserAborted is returned when the user cancels an operation
	ErrUserAborted = console.ErrUserAborted

	// program holds the running Bubble Tea program
	program *tea.Program

	// CurrentPageName tracks the active menu page for re-execution
	CurrentPageName string

	// CurrentEditorApp tracks which app is being edited in the tabbed vars editor.
	// Empty string means the global vars editor is active.
	// Only meaningful when CurrentPageName == "tabbed_vars".
	CurrentEditorApp string

	// CurrentScreenHasUnsavedEdit lets a screen other than the tabbed vars
	// editor (which is instead identified by CurrentPageName == "tabbed_vars")
	// report that it's mid-edit with in-progress, unconfirmed state a forced
	// restart would silently discard -- e.g. App Select's inline instance
	// rename/add-instance text field. Checked by isRestartSafeLocally.
	CurrentScreenHasUnsavedEdit bool

	// isRootSession is true when the TUI was started with a plain -M pagename (no start- prefix).
	// Root sessions suppress the Back button on the entry screen and re-exec to the same plain
	// pagename after a self-update. Non-root sessions re-exec with the "start-" prefix so the
	// update restores the full navigation stack.
	isRootSession bool

	// activeSessions tracks every currently-running TUI program (one per
	// concurrent local/SSH/web session under --server-daemon). Registered by
	// registerSession, removed by unregisterSession. Consulted by Shutdown,
	// which quits every active session for a re-exec, as opposed to a
	// session's own panic-recovery/context-cancellation path, which only
	// quits and waits on its own program.
	sessionsMu     sync.Mutex
	activeSessions = map[*tea.Program]chan struct{}{}

	// webOutbound is a channel for sending JSON messages to the browser (web sessions only).
	webOutbound chan<- []byte

	// webToken is the session token used to look up per-session web state.
	webToken string

	// initialInputState holds the stdin terminal state for emergency restoration
	initialInputState *term.State

	// initialOutputState holds the stdout terminal state for emergency restoration
	initialOutputState *term.State
)

// registerCallbacks wires TUI prompt/shutdown handlers into the console package.
func registerCallbacks() {
	console.TUIConfirm = PromptConfirm
	console.TUIPrompt = PromptText
	console.TUIShutdown = Shutdown
	console.TUIEmergencyShutdown = EmergencyShutdown

	// classic's Esc/exit-button fallback defaults to a plain tea.Quit; override
	// it here so a menu with no explicit btn-back/btn-cancel/btn-exit shows a
	// real "confirm exit?" dialog instead. Keeps classic decoupled from
	// app-navigation message types (see displayengine.SetConfirmExitFallback's doc comment).
	displayengine.SetConfirmExitFallback(ConfirmExitAction)

	// classic's displayengine.HeaderModel only owns rendering/hit-regions; header click/wheel
	// reactions (opening Global Flags, triggering app/template updates) are
	// app-navigation concerns, so wire the real handler in here.
	displayengine.SetHeaderUpdateHook(headerUpdate)

	// classic's panel (sudo-password interception) needs the real blocking
	// prompt dialog, which is app-integrated (routes through the running
	// tea.Program or a standalone dialog).
	displayengine.SetPromptTextHook(func(title, question string, sensitive bool) (string, error) {
		return PromptText(title, question, sensitive)
	})
}

// deregisterCallbacks removes TUI prompt/shutdown handlers from the console package.
func deregisterCallbacks() {
	console.TUIConfirm = nil
	console.TUIPrompt = nil
	console.TUIShutdown = nil
}

// Initialize sets up the TUI without starting the run loop
func Initialize(ctx context.Context) error {
	registerCallbacks()

	cfg := config.LoadAppConfig()
	console.SpinnerEnabled = cfg.UI.Spinner
	console.SpinnerSpeed = console.AlignToRefreshRate(cfg.UI.SpinnerSpeed, cfg.UI.RefreshRate)
	console.LineCharacters = cfg.UI.LineCharacters
	if deflts, err := theme.Load(cfg.UI.Theme, ""); err != nil {
		if deflts == nil {
			// Default theme itself failed to parse — unrecoverable
			logger.FatalWithStack(ctx, "failed to load default theme: %v", err)
		}
		// Non-default theme fell back to default — log warning and continue
		logger.Warn(ctx, "failed to load theme '%s', fell back to default: %v", cfg.UI.Theme, err)
	}

	// Initialize styles from theme
	displayengine.InitStyles(cfg)

	return nil
}

// WindowSizeEvent carries a terminal resize notification from a remote session.
type WindowSizeEvent struct {
	Width  int
	Height int
}

// ProgramOptions controls the I/O streams used by a Bubble Tea program.
// Nil fields fall back to os.Stdin / os.Stdout (local terminal behaviour).
// Set Input and Output when running over SSH or another non-TTY transport.
// WindowSize, when non-nil, is read by a goroutine in Start/StartEditor/
// StartVarEditor that forwards resize events to the running program.
type ProgramOptions struct {
	Input      io.Reader
	Output     io.Writer
	WindowSize <-chan WindowSizeEvent

	// Environ is the remote session's environment (e.g. []string{"TERM=xterm-256color",
	// "COLORTERM=truecolor"}). Passed to tea.WithEnvironment so that color profile
	// and terminal type are correctly detected for SSH and web sessions.
	Environ []string

	// InitialWidth and InitialHeight set the starting terminal dimensions before
	// the first resize event arrives. Zero values are ignored.
	InitialWidth  int
	InitialHeight int

	// WebOutbound, when non-nil, receives JSON messages to be forwarded to the
	// browser over WebSocket (web sessions only).
	WebOutbound chan<- []byte

	// WebToken is the session token used to look up per-session web state (display settings etc).
	WebToken string

	// RefreshRate is the screen repaint interval in milliseconds, used to compute
	// the FPS passed to tea.WithFPS. Zero falls back to a 100ms default.
	RefreshRate int
}

// resolveRefreshRate returns the refresh rate (ms) to use for a new program.
// Web sessions use their per-browser-token override if the browser set one;
// otherwise (including local/SSH sessions) it falls back to the Appearance
// menu's configured refresh rate.
func resolveRefreshRate(connType, webToken string) int {
	if connType == "web" {
		if rate := webmsg.GetDisplaySettings(webToken).RefreshRate; rate > 0 {
			return clampRefreshRate(rate)
		}
	}
	if displayengine.CurrentConfig().UI.RefreshRate > 0 {
		return displayengine.CurrentConfig().UI.RefreshRate
	}
	return config.DefaultConfig().UI.RefreshRate
}

// clampRefreshRate bounds a browser-supplied refresh rate to the same
// range the Appearance menu and config validation enforce.
func clampRefreshRate(ms int) int {
	switch {
	case ms < config.MinRefreshRateMS:
		return config.MinRefreshRateMS
	case ms > config.MaxRefreshRateMS:
		return config.MaxRefreshRateMS
	default:
		return ms
	}
}

// globalTickCmd returns a tea.Cmd that fires a globalTickMsg after the
// configured refresh interval. This single ticker drives all spinner advances
// and the periodic repaint, ensuring spinners are updated before each frame.
func globalTickCmd() tea.Cmd {
	ms := resolveRefreshRate(activeConnType, webToken)
	if ms <= 0 {
		ms = 60
	}
	d := time.Duration(ms) * time.Millisecond
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return globalTickMsg{time: t}
	})
}

// SendWebMsg sends a JSON message to the browser if a web outbound channel is set.
func SendWebMsg(msg []byte) {
	if webOutbound != nil {
		select {
		case webOutbound <- msg:
		default:
		}
	}
}

// OpenAppLink opens url using whichever mechanism fits the current session.
//
// Web sessions relay the URL to the browser over the WebSocket, since a
// keyboard-triggered open (unlike a mouse click on a rendered OSC8
// hyperlink, which xterm.js intercepts client-side) has no click to catch.
//
// Local sessions open the URL directly via console.OpenURL.
//
// SSH sessions have no WebSocket to relay through, and a mouse click only
// reaches a local browser if the client terminal supports OSC8 (e.g.
// WezTerm, not MobaXterm). So keyboard-triggered opens over SSH copy the URL
// to the client's clipboard via OSC52 as a best effort, and always also show
// it in a message dialog so the user can read/copy it manually regardless.
func OpenAppLink(ctx context.Context, url string) tea.Cmd {
	if url == "" {
		return nil
	}
	switch GetConnType() {
	case "web":
		return func() tea.Msg {
			data, _ := json.Marshal(map[string]any{
				"type": "open-url",
				"url":  url,
			})
			SendWebMsg(data)
			return nil
		}
	case "local":
		return func() tea.Msg {
			_ = console.OpenURL(ctx, url)
			return nil
		}
	default: // "ssh"
		return tea.Batch(
			tea.SetClipboard(url),
			func() tea.Msg {
				return ShowMessageDialogMsg{
					Title:   "Docs Page",
					Message: "Copied to clipboard (if your terminal supports it):\n{{|URL::::|}}" + url + "{{[-]}}",
					Type:    MessageInfo,
				}
			},
		)
	}
}

// GetWebDisplaySettings returns the browser's current display settings for this session.
func GetWebDisplaySettings() webmsg.DisplaySettings {
	return webmsg.GetDisplaySettings(webToken)
}

// NewProgram creates a new Bubble Tea program with standardized options.
// It also sets the global program variable for cross-component communication.
func NewProgram(model tea.Model, opts ProgramOptions) *tea.Program {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	teaOpts := []tea.ProgramOption{tea.WithOutput(out), tea.WithoutCatchPanics()}
	if opts.Input != nil {
		teaOpts = append(teaOpts, tea.WithInput(opts.Input))
	}
	if len(opts.Environ) > 0 {
		// Remote sessions (SSH, web) are never real TTYs, so colorprofile's
		// TTY detection always returns no-color regardless of COLORTERM. Force
		// TrueColor explicitly so lipgloss renders full color over the network.
		// We still pass the environment so TERM and other vars are available.
		teaOpts = append(teaOpts,
			tea.WithEnvironment(opts.Environ),
			tea.WithColorProfile(colorprofile.TrueColor),
		)
	}
	if opts.InitialWidth > 0 && opts.InitialHeight > 0 {
		teaOpts = append(teaOpts, tea.WithWindowSize(opts.InitialWidth, opts.InitialHeight))
	}
	refreshRate := opts.RefreshRate
	if refreshRate <= 0 {
		refreshRate = 100
	}
	// Integer division truncates to 0 for refreshRate > 1000ms, and
	// tea.WithFPS treats anything below 1 as "use its 60fps default" --
	// clamp to 1 so slow refresh rates are actually honored.
	fps := 1000 / refreshRate
	if fps < 1 {
		fps = 1
	}
	teaOpts = append(teaOpts, tea.WithFPS(fps))
	p := tea.NewProgram(model, teaOpts...)
	program = p
	return p
}

// ReplaceOutputLines sends a displayengine.ReplaceOutputMsg to the running program, replacing
// all current program box output with the given lines. No-op if no program is running.
func ReplaceOutputLines(lines []string) {
	if program != nil {
		program.Send(displayengine.ReplaceOutputMsg{Lines: lines})
	}
}

// SetProgramBoxHeader updates the running ProgramBoxModel's subtitle and/or
// command-line display -- e.g. once a choice-dependent command (like
// Stop/Down) becomes known partway through the task, having started the
// dialog without one.
func SetProgramBoxHeader(subtitle, command string) {
	Send(SetProgramBoxHeaderMsg{Subtitle: subtitle, Command: command})
}

func init() {
	console.ReplaceOutputLinesFn = ReplaceOutputLines
	console.OutputContentWidthFn = displayengine.OutputContentWidth
}

// parseClientInfo extracts IP and connection type from environment strings.
// viaOwnServer reports whether the session arrived through one of DS2's own
// listeners (wish SSH or web server) -- the only cases where DS2 knows for
// certain the rendering terminal/browser is on a different machine.
// Computed before the real-shell SSH fallback below, which folds "invoked
// inside a real SSH login shell" into connType=="ssh" for a different
// purpose (OpenAppLink's browser-opening decision) but must not affect
// viaOwnServer -- a real SSH terminal may render file:// hyperlinks fine and
// DS2 has no way to know, so "did this go through our own server" callers
// should use viaOwnServer, not connType.
func parseClientInfo(environ []string) (clientIP, connType string, viaOwnServer bool) {
	clientIP = "local"
	connType = "local"
	for _, env := range environ {
		if strings.HasPrefix(env, "DS2_CLIENT_IP=") {
			clientIP = strings.TrimPrefix(env, "DS2_CLIENT_IP=")
			connType = "web"
		}
		if strings.HasPrefix(env, "SSH_CONNECTION=") {
			connType = "ssh"
		}
		if strings.HasPrefix(env, "DS2_CONN_TYPE=") {
			connType = strings.TrimPrefix(env, "DS2_CONN_TYPE=")
		}
	}
	viaOwnServer = connType != "local"
	// A plain foreground invocation (the CLI binary run directly, not through
	// DS2's own wish SSH server) never gets a synthetic environ with any of
	// the markers above, but the process's real OS environment still reflects
	// how its own shell was reached. If that shell came from an SSH login
	// (e.g. Tabby/PuTTY/OpenSSH running the binary interactively rather than
	// connecting to DS2's built-in SSH listener), treat it as ssh too --
	// there's still no local display to open a browser on. Telnet and other
	// remote-shell protocols have no equivalent de facto standard env var, so
	// they can't be detected this way.
	if connType == "local" {
		if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CLIENT") != "" {
			connType = "ssh"
		}
	}
	return clientIP, connType, viaOwnServer
}

// parseSessionKey extracts the per-connection session identifier the SSH/web
// listener registered (DS2_SESSION_ID, set by ssh_handler.go's
// RegisterSession), used for edit-lock re-entry (see sessionlocks.
// localSessionKey). Falls back to a PID-based key for a plain
// foreground/local invocation, correct since each is its own process.
func parseSessionKey(environ []string) string {
	for _, env := range environ {
		if strings.HasPrefix(env, "DS2_SESSION_ID=") {
			return strings.TrimPrefix(env, "DS2_SESSION_ID=")
		}
	}
	return fmt.Sprintf("local-%d", os.Getpid())
}

// startWindowSizeForwarder launches a goroutine that reads from opts.WindowSize
// and sends tea.WindowSizeMsg to the program. The goroutine exits when ctx is
// cancelled or the channel is closed.
func startWindowSizeForwarder(ctx context.Context, p *tea.Program, opts ProgramOptions) {
	if opts.WindowSize == nil {
		return
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-opts.WindowSize:
				if !ok {
					return
				}
				p.Send(tea.WindowSizeMsg{Width: ev.Width, Height: ev.Height})
			}
		}
	}()
}

// Start launches the TUI application
func Start(ctx context.Context, startMenu string, opts ...ProgramOptions) error {
	console.PauseSpinner()
	var pOpts ProgramOptions
	if len(opts) > 0 {
		pOpts = opts[0]
	}
	clientIP, connType, viaOwnServer := parseClientInfo(pOpts.Environ)
	sessionKey := parseSessionKey(pOpts.Environ)
	console.SetViaOwnServer(viaOwnServer)
	console.SetClientIP(clientIP)
	isSSH := pOpts.Input != nil

	// Enable Virtual Terminal Processing (ANSI) on Windows early so color detection works
	console.EnableVirtualTerminalProcessing()

	// Capture initial terminal states for emergency restoration (local terminal only)
	if !isSSH {
		initialInputState, _ = term.GetState(int(os.Stdin.Fd()))
		initialOutputState, _ = term.GetState(int(os.Stdout.Fd()))
	}

	// Ensure the TUI is active and un-frozen
	console.SetTUIDying(false)

	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	logger.Info(ctx, "TUI Starting.")
	defer sessionlocks.Sessions.ReleaseEditLock()

	captureExePath()

	// p/exited are declared here (rather than via := at their point of
	// assignment below) so the panic-recovery and context-cancellation
	// closures below -- both defined before the program exists -- close
	// over this session's own program once it's created, instead of
	// falling back to whatever the package-level Shutdown() would affect.
	var p *tea.Program
	var exited chan struct{}

	// Global panic recovery
	defer func() {
		if r := recover(); r != nil {
			shutdownSelf(p, exited)
			logger.FatalWithStack(ctx, "TUI Panic: %v", r)
		}
	}()

	if err := Initialize(ctx); err != nil {
		return err
	}

	// Resolve the menu target to a canonical page name and root-session flag.
	pageName, isRoot := resolveMenuTarget(startMenu)
	isRootSession = isRoot
	activeConnType = connType
	webOutbound = pOpts.WebOutbound
	webToken = pOpts.WebToken
	pOpts.RefreshRate = resolveRefreshRate(connType, pOpts.WebToken)

	// Look up the screen entry; fall back to "main" if unrecognised.
	entry, ok := screenRegistry[pageName]
	if !ok {
		entry = screenRegistry["main"]
	}

	startScreen := entry.create(isRoot, connType)

	// For non-root (start-*) sessions, push the canonical parent screens onto the
	// navigation stack so that Back navigates naturally rather than quitting.
	var initialStack []ScreenModel
	if !isRoot {
		for _, parentName := range entry.parents {
			if parentEntry, ok := screenRegistry[parentName]; ok {
				initialStack = append(initialStack, parentEntry.create(false, connType))
			}
		}
	}

	// Create the app model
	model := NewAppModel(ctx, displayengine.CurrentConfig(), clientIP, connType, sessionKey, startScreen, initialStack...)

	if console.IsPuTTY() {
		logger.Info(ctx, "PuTTY terminal detected")
	}

	// Create and run the Bubble Tea program
	// Note: AltScreen is set via View().AltScreen in v2
	p = NewProgram(model, pOpts)
	model.panel.SetConfirmFunc(sessionConfirmFunc(p))
	model.panel.SetPromptFunc(sessionPromptFunc(p))
	model.SetProgram(p)

	// Register this session so Shutdown (re-exec) can find it, and so
	// shutdownSelf above has something to quit/wait on.
	exited = registerSession(p)

	// Forward window resize events from remote sessions (no-op for local terminal)
	startWindowSizeForwarder(ctx, p, pOpts)

	// Start background update checker
	go startUpdateChecker(ctx, model.Send)

	// Watch config file for external changes (e.g. ds2 CLI or web server)
	startConfigWatcher(ctx, p)

	// Watch remote.lock so the console input bar locks when an SSH/web session is active
	startLockFileWatcher(ctx, p)

	// Watch for restart signal from another process that updated the binary
	startRestartWatcher(ctx)

	// Listen for context cancellation to shutdown program
	go func() {
		<-ctx.Done()
		shutdownSelf(p, exited)
	}()

	// Run the program
	finalModel, err := p.Run()

	// Signal that the program has exited
	close(exited)
	unregisterSession(p)
	model.Cleanup()

	if !isSSH {
		// Drain any buffered mouse events from stdin before disabling mouse tracking.
		// When the user clicks to confirm exit, SGR-encoded mouse motion/release events
		// may already be in the stdin buffer. If not discarded, the shell reads them as
		// raw text after the program exits (producing visible ANSI garbage).
		drainStdin()

		// Reset terminal state on exit:
		// 1. Reset colors (\x1b[0m)
		// 2. Disable all mouse modes (1000, 1002, 1003)
		// 3. Disable SGR mouse mode (1006) - prevents ANSI codes leaking to shell
		// 4. Exit AltScreen (1049) if still active
		fmt.Print("\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\n")
	}

	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		logger.FatalWithStack(ctx, "TUI Error: %v", err)
	}

	// Check if the model exited via ForceQuit (ctrl-c)
	if m, ok := finalModel.(*AppModel); errors.Is(err, tea.ErrInterrupted) || (ok && m.Fatal) {
		logger.TUIMode = false
		console.AbortHandler(ctx)
		return ErrUserAborted
	}

	return nil
}

// EditorFactory creates a tabbed vars editor screen.
// appName is empty for the global vars editor, or an app name for the app-specific editor.
// onClose is the Cmd to fire when the user navigates back/exits the editor.
// showBack controls whether the Back button is shown (false when launched as root session).
type EditorFactory func(appName string, onClose tea.Cmd, showBack bool, connType string) ScreenModel

// editorFactory is registered by the screens package to avoid a circular import.
var editorFactory EditorFactory

// RegisterEditorFactory stores the factory function for use by StartEditor.
// Called from screens/init.go during package initialization.
func RegisterEditorFactory(f EditorFactory) {
	editorFactory = f
}

// StartEditor launches the TUI with the tabbed vars editor as the entry screen.
// appName is empty for the global vars editor, or an app name for the app-specific editor.
// isRoot controls whether Back navigation exits immediately (true) or uses a pre-populated stack (false).
func StartEditor(ctx context.Context, appName string, isRoot bool, opts ...ProgramOptions) error {
	console.PauseSpinner()
	var pOpts ProgramOptions
	if len(opts) > 0 {
		pOpts = opts[0]
	}
	isSSH := pOpts.Input != nil

	// Enable Virtual Terminal Processing (ANSI) on Windows early so color detection works
	console.EnableVirtualTerminalProcessing()

	// Capture initial terminal states for emergency restoration (local terminal only)
	if !isSSH {
		initialInputState, _ = term.GetState(int(os.Stdin.Fd()))
		initialOutputState, _ = term.GetState(int(os.Stdout.Fd()))
	}

	// Ensure the TUI is active and un-frozen
	console.SetTUIDying(false)

	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	var p *tea.Program
	var exited chan struct{}
	defer func() {
		if r := recover(); r != nil {
			shutdownSelf(p, exited)
			logger.FatalWithStack(ctx, "TUI Panic: %v", r)
		}
	}()
	defer sessionlocks.Sessions.ReleaseEditLock()

	captureExePath()

	if err := Initialize(ctx); err != nil {
		return err
	}

	isRootSession = isRoot
	ip, ctype, viaOwnServer := parseClientInfo(pOpts.Environ)
	sessionKey := parseSessionKey(pOpts.Environ)
	activeConnType = ctype
	console.SetViaOwnServer(viaOwnServer)
	console.SetClientIP(ip)
	pOpts.RefreshRate = resolveRefreshRate(ctype, pOpts.WebToken)

	onClose := func() tea.Msg { return NavigateBackMsg{} }
	startScreen := editorFactory(appName, onClose, !isRoot, ctype)

	// For non-root sessions, pre-populate the navigation stack so Back returns naturally.
	// Global editor: main → config
	// App editor:    main → config → app-select
	var initialStack []ScreenModel
	if !isRoot {
		parentNames := []string{"main", "config"}
		if appName != "" {
			parentNames = append(parentNames, "app-select")
		}
		for _, name := range parentNames {
			if entry, ok := screenRegistry[name]; ok {
				initialStack = append(initialStack, entry.create(false, ctype))
			}
		}
	}

	model := NewAppModel(ctx, displayengine.CurrentConfig(), ip, ctype, sessionKey, startScreen, initialStack...)
	p = NewProgram(model, pOpts)
	model.panel.SetConfirmFunc(sessionConfirmFunc(p))
	model.panel.SetPromptFunc(sessionPromptFunc(p))
	model.SetProgram(p)
	exited = registerSession(p)

	startWindowSizeForwarder(ctx, p, pOpts)

	go startUpdateChecker(ctx, model.Send)
	startConfigWatcher(ctx, p)
	startLockFileWatcher(ctx, p)
	startRestartWatcher(ctx)
	go func() {
		<-ctx.Done()
		shutdownSelf(p, exited)
	}()

	finalModel, err := p.Run()
	close(exited)
	unregisterSession(p)
	if !isSSH {
		drainStdin()
		fmt.Print("\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\n")
	}

	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		logger.FatalWithStack(ctx, "TUI Error: %v", err)
	}

	if m, ok := finalModel.(*AppModel); errors.Is(err, tea.ErrInterrupted) || (ok && m.Fatal) {
		logger.TUIMode = false
		console.AbortHandler(ctx)
		return ErrUserAborted
	}

	return nil
}

// VarEditorFactory creates a standalone Set Value screen for a single variable.
// Called by screens/init.go to avoid a circular import.
type VarEditorFactory func(
	varName, appName, appDesc, filePath, origVal string,
	opts []appenv.VarOption,
	helpText, docMarkdown, docAppName string,
	onSave func(string) tea.Cmd,
	onCancel tea.Cmd,
) ScreenModel

var varEditorFactory VarEditorFactory

// RegisterVarEditorFactory stores the factory for use by StartVarEditor.
// Called from screens/init.go during package initialization.
func RegisterVarEditorFactory(f VarEditorFactory) {
	varEditorFactory = f
}

// StartVarEditor launches the TUI with the standalone Set Value dialog for a single variable.
// appName is "" for the global .env file, or an app name (from APP:VAR syntax) for .env.app.<appname>.
// varName is the variable to edit (upper-cased by the caller).
// file is the pre-resolved env file path (from resolveEnvVar).
func StartVarEditor(ctx context.Context, appName, varName, file string, progOpts ...ProgramOptions) error {
	console.PauseSpinner()
	var pOpts ProgramOptions
	if len(progOpts) > 0 {
		pOpts = progOpts[0]
	}
	isSSH := pOpts.Input != nil

	console.SetTUIEnabled(true)
	defer console.SetTUIEnabled(false)

	logger.TUIMode = true
	defer func() { logger.TUIMode = false }()

	var p *tea.Program
	var exited chan struct{}
	defer func() {
		if r := recover(); r != nil {
			shutdownSelf(p, exited)
			logger.FatalWithStack(ctx, "TUI Panic: %v", r)
		}
	}()
	defer sessionlocks.Sessions.ReleaseEditLock()

	captureExePath()

	if err := Initialize(ctx); err != nil {
		return err
	}

	// file is pre-resolved by the caller (resolveEnvVar).
	// Derive app name for metadata/display from APP:VAR syntax or APPNAME__ prefix.
	metaAppName := appName
	if metaAppName == "" {
		metaAppName = appenv.VarNameToAppName(varName)
	}

	// Get current value (preserve literal quotes/spacing)
	origVal, _ := appenv.GetLiteral(varName, file)

	// Load app metadata for description and preset options
	var meta *appenv.AppMeta
	if metaAppName != "" {
		if m, err := appenv.LoadAppMeta(ctx, metaAppName); err == nil {
			meta = m
		}
	}

	appDesc := ""
	if metaAppName != "" {
		if desc := appenv.GetDescription(ctx, metaAppName, file); desc != "! Missing description !" {
			appDesc = desc
		}
	}

	opts := appenv.GetVarOptions(varName, strings.ToUpper(metaAppName), origVal, meta)
	// Prepend "Original Value" so the user can revert easily
	opts = append([]appenv.VarOption{{
		Display: "Original Value",
		Value:   origVal,
		Help:    "Restore the value that was set before editing.",
	}}, opts...)

	helpText := ""
	if desc := appenv.GetVarHelpText(varName); desc != "" {
		helpText = desc
	} else if meta != nil {
		if vm, ok := meta.GetVarMeta(varName, strings.ToUpper(metaAppName)); ok && vm.HelpText != "" {
			helpText = vm.HelpText
		}
	}

	onSave := func(val string) tea.Cmd {
		if err := appenv.SetLiteral(ctx, varName, val, file); err != nil {
			logger.Error(ctx, "Failed to set %s: %v", varName, err)
		}
		return tea.Quit
	}

	// Use the display-friendly app name (e.g. "Plex" not "PLEX") to match the tabbed editor
	displayAppName := appenv.GetNiceName(ctx, metaAppName)

	// Fetch documentation if manageable
	docMarkdown, docAppName := "", ""
	if metaAppName != "" {
		if !appenv.IsAppUserDefined(ctx, metaAppName, file) {
			doc, err := appenv.GetAppMarkdown(ctx, metaAppName)
			if err == nil {
				docMarkdown = doc
				docAppName = displayAppName
			}
		}
	}

	startScreen := varEditorFactory(varName, displayAppName, appDesc, file, origVal, opts, helpText, docMarkdown, docAppName, onSave, tea.Quit)

	ip, ctype, viaOwnServer := parseClientInfo(pOpts.Environ)
	sessionKey := parseSessionKey(pOpts.Environ)
	console.SetViaOwnServer(viaOwnServer)
	console.SetClientIP(ip)
	pOpts.RefreshRate = resolveRefreshRate(ctype, pOpts.WebToken)
	model := NewAppModel(ctx, displayengine.CurrentConfig(), ip, ctype, sessionKey, startScreen)
	p = NewProgram(model, pOpts)
	model.panel.SetConfirmFunc(sessionConfirmFunc(p))
	model.panel.SetPromptFunc(sessionPromptFunc(p))
	model.SetProgram(p)
	exited = registerSession(p)

	startWindowSizeForwarder(ctx, p, pOpts)

	go startUpdateChecker(ctx, model.Send)
	startConfigWatcher(ctx, p)
	startLockFileWatcher(ctx, p)
	startRestartWatcher(ctx)
	go func() {
		<-ctx.Done()
		shutdownSelf(p, exited)
	}()

	finalModel, err := p.Run()
	close(exited)
	unregisterSession(p)
	if !isSSH {
		drainStdin()
		fmt.Print("\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\n")
	}

	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		logger.FatalWithStack(ctx, "TUI Error: %v", err)
	}

	if m, ok := finalModel.(*AppModel); errors.Is(err, tea.ErrInterrupted) || (ok && m.Fatal) {
		logger.TUIMode = false
		console.AbortHandler(ctx)
		return ErrUserAborted
	}

	return nil
}

// registerSession records a newly started program as active and returns the
// channel that will be closed once its Run() call returns. Called by
// Start/StartEditor/StartVarEditor once their *tea.Program exists.
func registerSession(p *tea.Program) chan struct{} {
	exited := make(chan struct{})
	sessionsMu.Lock()
	activeSessions[p] = exited
	sessionsMu.Unlock()
	return exited
}

// unregisterSession removes a session from the active set once its Run()
// call has returned (its exited channel is closed separately by the caller).
func unregisterSession(p *tea.Program) {
	sessionsMu.Lock()
	delete(activeSessions, p)
	sessionsMu.Unlock()
}

// shutdownSelf quits this session's own program and waits for it to actually
// exit, unaffected by any other concurrently running session. Used for a
// session's own panic recovery and context-cancellation shutdown -- p/exited
// may still be nil if the panic happened before the program was created.
func shutdownSelf(p *tea.Program, exited chan struct{}) {
	if p == nil {
		return
	}
	p.Quit()
	if exited != nil {
		<-exited
	}
	sessionlocks.Sessions.ReleaseEditLock()
}

// Shutdown quits every currently active TUI session and waits for each to
// actually exit. Registered as console.TUIShutdown, called before a re-exec
// -- the whole process (every session in it) is about to be replaced
// regardless of which session's restart-watcher triggered it, so unlike
// shutdownSelf this deliberately targets all of them, not just one.
func Shutdown() {
	sessionsMu.Lock()
	snapshot := make(map[*tea.Program]chan struct{}, len(activeSessions))
	for p, exited := range activeSessions {
		snapshot[p] = exited
	}
	sessionsMu.Unlock()
	for p, exited := range snapshot {
		p.Quit()
		<-exited
	}
	sessionlocks.Sessions.ReleaseEditLock()
}

// EmergencyShutdown forcefully restores the terminal using raw ANSI escape codes.
// This is used during panic recovery where a standard Shutdown() might deadlock.
func EmergencyShutdown() {
	sessionlocks.Sessions.ReleaseEditLock()

	// Signal that the TUI is dying to freeze the renderer goroutine
	console.SetTUIDying(true)

	// Short settle time to let the terminal's internal buffers catch up
	time.Sleep(50 * time.Millisecond)

	// Forcefully restore both Input and Output TTY states
	if initialInputState != nil {
		_ = term.Restore(int(os.Stdin.Fd()), initialInputState)
	}
	if initialOutputState != nil {
		_ = term.Restore(int(os.Stdout.Fd()), initialOutputState)
	}

	// Comprehensive ANSI reset block:
	// \x1b[!p       - DECSTR (Soft Terminal Reset)
	// \x1b[2J\x1b[H - Clear Screen and Home
	// \x1b[0m       - All Colors Reset
	// \x1b[?1000..  - All Mouse Modes Disabled (Standard Reset)
	// \x1b[?1049l   - Exit Alternate Screen
	// \x1b[?25h      - Show Cursor
	// \r\n          - Carriage Return and Newline
	os.Stdout.WriteString("\x1b[!p\x1b[2J\x1b[H\x1b[0m\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1049l\x1b[?25h\r\n")
	os.Stdout.Sync()

	// Ensure TUI flag and mode are off so we print directly to terminal
	console.SetTUIEnabled(false)
	logger.TUIMode = false
}

// startUpdateChecker runs the background update check. send delivers the
// header-refresh notification to this session's own Program (see
// AppModel.Send), not the process-wide global program var, which can point
// at a different session in a server-daemon serving several concurrently --
// each session runs its own copy of this checker.
func startUpdateChecker(ctx context.Context, send func(tea.Msg)) {
	// Initial check
	update.CheckAppUpdate(ctx)
	update.CheckTmplUpdate(ctx)
	send(UpdateHeaderMsg{})

	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			appUpdateOld := update.AppUpdateAvailable
			tmplUpdateOld := update.TmplUpdateAvailable
			appErrorOld := update.AppUpdateCheckError
			tmplErrorOld := update.TmplUpdateCheckError
			update.CheckAppUpdate(ctx)
			update.CheckTmplUpdate(ctx)
			if update.AppUpdateAvailable != appUpdateOld || update.TmplUpdateAvailable != tmplUpdateOld ||
				update.AppUpdateCheckError != appErrorOld || update.TmplUpdateCheckError != tmplErrorOld {
				send(UpdateHeaderMsg{})
			}
		}
	}
}

// RunCommand executes a task with output displayed in a TUI dialog.
// If a Bubble Tea program is already running, it shows the dialog inline.
// Otherwise, it starts a standalone program box.
func RunCommand(ctx context.Context, title, subtitle, command string, task func(context.Context) error) error {
	// Wrap the task to pass the writer
	// We use WithTUIWriter to ensure logger output is captured by the TUI
	wrappedTask := func(ctx context.Context, w io.Writer) error {
		return task(console.WithTUIWriter(ctx, w))
	}

	// If TUI is already running, show dialog within existing program
	dialog := NewProgramBoxModel(title, subtitle, command).WithDialogType(displayengine.DialogTypeSuccess)
	dialog.SetContext(ctx)
	dialog.SetTask(wrappedTask)
	dialog.SetIsDialog(true)
	dialog.SetMaximized(true)
	if bridgeSend(ctx, displayengine.ShowDialogMsg{Dialog: dialog}) {
		return nil
	}

	// Otherwise, run standalone with its own Bubble Tea program
	return RunProgramBox(ctx, title, subtitle, command, wrappedTask)
}

// screenEntry holds a screen's creator function and its canonical parent stack.
// parents is ordered outermost-first (e.g. ["main", "options"] for the appearance screen).
type screenEntry struct {
	create  func(isRoot bool, connType string) ScreenModel
	parents []string
}

// screenRegistry maps canonical page names to their screen entries.
// Populated by RegisterScreen calls from the screens package.
var screenRegistry = map[string]*screenEntry{}

// screenAliases maps alternate -M sub-command names to their canonical page name.
var screenAliases = map[string]string{
	"display":           "appearance",
	"options-display":   "appearance",
	"theme":             "appearance",
	"options-theme":     "appearance",
	"theme-select":      "appearance",
	"display-options":   "appearance",
	"display_options":   "appearance",
	"select":            "app-select",
	"config-app-select": "app-select",
}

// RegisterScreen registers a screen with its canonical page name, a creator function
// that accepts isRoot and connType, and an optional ordered list of parent page names.
// parents should be outermost-first; they are pushed onto the navigation stack
// when the screen is started via "-M start-<name>" so that Back navigates naturally.
func RegisterScreen(name string, create func(isRoot bool, connType string) ScreenModel, parents []string) {
	screenRegistry[name] = &screenEntry{create: create, parents: parents}
}

// PromptText displays a blocking text prompt dialog.
// It is used by the console package via callback.
func PromptText(title, question string, sensitive bool, initialValue ...string) (string, error) {
	iv := ""
	if len(initialValue) > 0 {
		iv = initialValue[0]
	}
	if program != nil {
		resultChan := make(chan promptResultMsg)
		program.Send(UniversalPromptMsg{
			Title:        title,
			Question:     question,
			Sensitive:    sensitive,
			InitialValue: iv,
			ResultChan:   resultChan,
			Type:         PromptTypeText,
		})
		res := <-resultChan
		if !res.confirmed {
			return "", console.ErrUserAborted
		}
		return res.result, nil
	}
	res, confirmed := ShowPromptDialog(title, question, sensitive, iv)
	if !confirmed {
		return "", console.ErrUserAborted
	}
	return res, nil
}

// resolveMenuTarget normalises a -M sub-command value into a canonical page name
// and determines whether this should be a root session (no Back button on entry screen).
//   - "" or "main" with no start- prefix → page "main", isRoot false (normal start)
//   - "config"                           → page "config", isRoot true
//   - "start-config"                     → page "config", isRoot false (pre-populated stack)
func resolveMenuTarget(startMenu string) (pageName string, isRoot bool) {
	if startMenu == "" {
		return "main", false
	}
	isRoot = true
	pageName = startMenu
	if strings.HasPrefix(startMenu, "start-") {
		isRoot = false
		pageName = strings.TrimPrefix(startMenu, "start-")
	}
	if canonical, ok := screenAliases[pageName]; ok {
		pageName = canonical
	}
	return pageName, isRoot
}

// reExecMenuArg returns the --menu sub-command value to use when re-executing after a self-update.
// Root sessions (started with "-M pagename") re-exec to the same plain pagename so they remain
// root. Non-root sessions use the "start-" prefix so the navigation stack is restored.
// Pages that are not registered as valid -M targets (e.g. transient dialogs, tabbed editors)
// fall back to "" so the re-exec lands on the main menu rather than failing to parse.
func reExecMenuArg() string {
	if CurrentPageName == "" {
		return ""
	}
	// Only pages that exist in the screen registry are valid -M targets.
	// Transient screens (e.g. tabbed vars editor) are not registered and would
	// cause a CLI parse error if passed to -M after re-exec.
	if _, ok := screenRegistry[CurrentPageName]; !ok {
		return ""
	}
	if isRootSession {
		return CurrentPageName
	}
	return "start-" + CurrentPageName
}

// GetNavArgs returns the navigation args that should be appended when spawning
// a daemon (or re-exec) to restore the current screen. Returns nil if no
// page is active or the active page has no valid nav arg.
func GetNavArgs() []string {
	if CurrentPageName == "tabbed_vars" {
		if CurrentEditorApp == "" {
			return []string{"--start-edit-global"}
		}
		return []string{"--start-edit-app", CurrentEditorApp}
	}
	if menuArg := reExecMenuArg(); menuArg != "" {
		return []string{"--menu", menuArg}
	}
	return nil
}

// CmdLine builds a styled command string for program box subtitles. Flags
// are read live from console's current state (not console.CurrentFlags,
// which is fixed at the flags this process was originally invoked with) so
// the displayed command line reflects flags the user toggled at runtime via
// the Global Flags dialog, not just how the process was started.
func CmdLine(args ...string) string {
	parts := []string{version.CommandName}
	if console.Verbose() {
		parts = append(parts, "--verbose")
	}
	if console.Debug() {
		parts = append(parts, "--debug")
	}
	if console.Force() {
		parts = append(parts, "--force")
	}
	if console.GUI() {
		parts = append(parts, "--gui")
	}
	if console.AssumeYes() {
		parts = append(parts, "--yes")
	}
	parts = append(parts, args...)
	return "{{[-]}} {{|CommandLine|}}" + strings.Join(parts, " ") + "{{[-]}}"
}

// TriggerAppUpdate returns a tea.Cmd that performs the application update.
// It detects the currently active screen to support sticky restarts (using --menu pagename).
func TriggerAppUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			// Redirect logger to the pipe so output is captured by the viewport
			ctx = console.WithTUIWriter(ctx, w)

			force := console.Force()
			yes := console.AssumeYes()

			// Re-exec args restore the active screen after update.
			// When running inside a daemon, re-exec must restart as a daemon.
			var reExecArgs []string
			if console.IsDaemon {
				reExecArgs = append(reExecArgs, "--server-daemon")
			} else {
				reExecArgs = append(reExecArgs, console.CurrentFlags...)
			}
			if CurrentPageName == "tabbed_vars" {
				// Restore the vars editor directly, preserving which app was being edited.
				if CurrentEditorApp == "" {
					reExecArgs = append(reExecArgs, "--start-edit-global")
				} else {
					reExecArgs = append(reExecArgs, "--start-edit-app", CurrentEditorApp)
				}
			} else {
				// Restore the active menu screen (root or non-root).
				reExecArgs = append(reExecArgs, "--menu")
				if menuArg := reExecMenuArg(); menuArg != "" {
					reExecArgs = append(reExecArgs, menuArg)
				}
			}
			if !console.IsDaemon {
				reExecArgs = append(reExecArgs, console.RestArgs...)
			}
			err := update.SelfUpdate(ctx, force, yes, "", reExecArgs)
			if err == nil {
				// Refresh update status and UI
				update.CheckAppUpdate(ctx)
				Send(UpdateHeaderMsg{})
			}
			return err
		}

		dialog := NewProgramBoxModel("Updating App", "Updating the application.", CmdLine("--update-app")).WithDialogType(displayengine.DialogTypeSuccess)
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return displayengine.ShowDialogMsg{Dialog: dialog}
	}
}

// TriggerTemplateUpdate returns a tea.Cmd that performs the template update.
func TriggerTemplateUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			// Redirect logger to the pipe so output is captured by the viewport
			ctx = console.WithTUIWriter(ctx, w)

			force := console.Force()
			yes := console.AssumeYes()

			err := update.UpdateTemplates(ctx, force, yes, "")
			if err == nil {
				// Refresh update status and UI
				update.CheckTmplUpdate(ctx)
				Send(UpdateHeaderMsg{})
				Send(TemplateUpdateSuccessMsg{})
			}
			return err
		}

		dialog := NewProgramBoxModel("Updating Templates", "Updating app templates.", CmdLine("--update-templates")).WithDialogType(displayengine.DialogTypeSuccess)
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return displayengine.ShowDialogMsg{Dialog: dialog}
	}
}

// TriggerUpdate returns a tea.Cmd that performs both application and template updates.
// Kept for backward compatibility with main menu update button.
func TriggerUpdate() tea.Cmd {
	return func() tea.Msg {
		task := func(ctx context.Context, w io.Writer) error {
			// Redirect logger to the pipe so output is captured by the viewport
			ctx = console.WithTUIWriter(ctx, w)

			force := console.Force()
			yes := console.AssumeYes()

			templInfo, templErr := update.CheckTemplatesUpdate(ctx, force, "")
			if templErr == nil && templInfo.HasUpdate && sessionlocks.Sessions.IsEditLocked() {
				logger.Warn(ctx, "Skipping template update from '%s' to '%s' while configuration is being edited.", update.TmplVersionLink(templInfo.CurrentDisplay), update.TmplVersionLink(templInfo.RemoteDisplay))
			} else if templErr == nil {
				if err := update.ApplyTemplatesUpdate(ctx, templInfo, yes); err != nil {
					return err
				}
			}

			// Re-exec args restore the active screen after update.
			var reExecArgs []string
			if console.IsDaemon {
				reExecArgs = append(reExecArgs, "--server-daemon")
			} else {
				reExecArgs = append(reExecArgs, console.CurrentFlags...)
			}
			reExecArgs = append(reExecArgs, "--menu")
			if menuArg := reExecMenuArg(); menuArg != "" {
				reExecArgs = append(reExecArgs, menuArg)
			}
			if !console.IsDaemon {
				reExecArgs = append(reExecArgs, console.RestArgs...)
			}
			err := update.SelfUpdate(ctx, force, yes, "", reExecArgs)
			if err == nil {
				// Refresh update status and UI
				update.CheckAppUpdate(ctx)
				update.CheckTmplUpdate(ctx)
				Send(UpdateHeaderMsg{})
				Send(TemplateUpdateSuccessMsg{})
			}
			return err
		}

		dialog := NewProgramBoxModel("Updating DockSTARTer2", "Updating DockSTARTer2 and Templates to the latest versions.", CmdLine("--update")).WithDialogType(displayengine.DialogTypeSuccess)
		dialog.SetTask(task)
		dialog.SetIsDialog(true)
		dialog.SetMaximized(true)

		return displayengine.ShowDialogMsg{Dialog: dialog}
	}
}

// editLockBusyMsg builds the "Resource Busy" dialog message from the current lock info.
// attempted is the action that was blocked (command or screen title); empty string omits it.
func editLockBusyMsg(info sessionlocks.SessionInfo, attempted string) string {
	detailLines, hint := sessionlocks.EditLockDetail(info)
	var msg string
	for i, line := range detailLines {
		if i > 0 {
			msg += "\n"
		}
		msg += line
	}
	closing := "Configuration is currently being edited."
	if attempted != "" {
		closing = fmt.Sprintf("Cannot open '{{|UserCommand|}}%s{{[-]}}' while the configuration is being edited.", attempted)
	}
	if msg != "" {
		msg += "\n\n"
	}
	msg += closing
	if hint != "" {
		msg += "\n" + hint
	}
	return msg
}

// triggerComposeUpdateMsg, triggerComposeStopMsg, and triggerDockerPruneMsg
// are requests dispatched by the Config menu's buttons; the actual edit-lock
// check and dialog construction happens in AppModel.Update (model_update.go)
// instead, since these buttons' Action closures are built once at screen
// construction time shared by every session in a server-daemon process and
// can't close over a specific session's clientIP/connType/sessionKey.
type triggerComposeUpdateMsg struct{}
type triggerComposeStopMsg struct{}
type triggerDockerPruneMsg struct{}

// TriggerComposeUpdate returns a tea.Cmd that requests starting all enabled apps via docker compose update.
func TriggerComposeUpdate() tea.Cmd {
	return func() tea.Msg { return triggerComposeUpdateMsg{} }
}

// TriggerComposeStop returns a tea.Cmd that requests prompting Stop/Down/Cancel then running the chosen compose op.
func TriggerComposeStop() tea.Cmd {
	return func() tea.Msg { return triggerComposeStopMsg{} }
}

// TriggerDockerPrune returns a tea.Cmd that requests running docker system prune.
func TriggerDockerPrune() tea.Cmd {
	return func() tea.Msg { return triggerDockerPruneMsg{} }
}

// doTriggerComposeUpdate performs the actual edit-lock check and starts all
// enabled apps via docker compose update, using the acquiring session's own identity.
func doTriggerComposeUpdate(clientIP, connType, sessionKey string) tea.Msg {
	if !sessionlocks.Sessions.AcquireEditLock(clientIP, "Start All Applications", "menu:compose-start", connType, sessionKey) {
		return ShowMessageDialogMsg{Title: "Resource Busy", Message: editLockBusyMsg(sessionlocks.Sessions.ReadEditInfo(), ""), Type: MessageError}
	}
	var dialog *ProgramBoxModel
	task := func(ctx context.Context, w io.Writer) error {
		defer sessionlocks.Sessions.ReleaseEditLock()
		ctx = console.WithTUIWriter(ctx, w)
		ctx = console.WithReplaceOutputFunc(ctx, dialog.ReplaceOutput)
		if err := compose.ExecuteCompose(ctx, console.AssumeYes(), console.Force(), "update"); err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		return nil
	}
	dialog = NewProgramBoxModel("Docker Compose", compose.YesNotice("update", ""), CmdLine("--compose", "update")).WithDialogType(displayengine.DialogTypeSuccess)
	dialog.SetTask(task)
	dialog.SetIsDialog(true)
	dialog.SetMaximized(true)
	return displayengine.ShowDialogMsg{Dialog: dialog}
}

// doTriggerComposeStop performs the actual edit-lock check and prompts
// Stop/Down/Cancel then runs the chosen compose op, using the acquiring session's own identity.
func doTriggerComposeStop(clientIP, connType, sessionKey string) tea.Msg {
	if !sessionlocks.Sessions.AcquireEditLock(clientIP, "Stop All Applications", "menu:compose-stop", connType, sessionKey) {
		return ShowMessageDialogMsg{Title: "Resource Busy", Message: editLockBusyMsg(sessionlocks.Sessions.ReadEditInfo(), ""), Type: MessageError}
	}
	question := "Would you like to {{|Highlight|}}Stop{{[-]}} all containers, or bring all containers {{|Highlight|}}Down{{[-]}}?\n\n{{|Highlight|}}Stop{{[-]}} will stop them, {{|Highlight|}}Down{{[-]}} will stop and remove them."
	var dialog *ProgramBoxModel
	task := func(ctx context.Context, w io.Writer) error {
		defer sessionlocks.Sessions.ReleaseEditLock()
		ctx = console.WithTUIWriter(ctx, w)
		ctx = console.WithReplaceOutputFunc(ctx, dialog.ReplaceOutput)
		choice := dialog.Choice("Docker Compose", question, "Stop", "Down", "Cancel")
		switch choice {
		case 0: // Stop
			SetProgramBoxHeader(compose.YesNotice("stop", ""), CmdLine("--compose", "stop"))
			if err := compose.ExecuteCompose(ctx, true, console.Force(), "stop"); err != nil {
				logger.Error(ctx, "%v", err)
				return err
			}
		case 1: // Down
			SetProgramBoxHeader(compose.YesNotice("down", ""), CmdLine("--compose", "down"))
			if err := compose.ExecuteCompose(ctx, true, console.Force(), "down"); err != nil {
				logger.Error(ctx, "%v", err)
				return err
			}
		}
		return nil
	}
	// The command line is deliberately blank at creation -- Stop vs. Down
	// isn't decided until the Choice call above resolves, and showing a
	// guessed command line before that would be misleading. The subtitle
	// itself is still fine to show generically. SetProgramBoxHeader fills
	// in both properly once the choice is known.
	dialog = NewProgramBoxModel("Docker Compose", "Stopping or removing running containers.", "").WithDialogType(displayengine.DialogTypeSuccess)
	dialog.SetTask(task)
	dialog.SetIsDialog(true)
	dialog.SetMaximized(true)
	return displayengine.ShowDialogMsg{Dialog: dialog}
}

// doTriggerDockerPrune performs the actual edit-lock check and runs docker
// system prune, using the acquiring session's own identity.
func doTriggerDockerPrune(clientIP, connType, sessionKey string) tea.Msg {
	if !sessionlocks.Sessions.AcquireEditLock(clientIP, "Prune Docker System", "menu:docker-prune", connType, sessionKey) {
		return ShowMessageDialogMsg{Title: "Resource Busy", Message: editLockBusyMsg(sessionlocks.Sessions.ReadEditInfo(), ""), Type: MessageError}
	}
	task := func(ctx context.Context, w io.Writer) error {
		defer sessionlocks.Sessions.ReleaseEditLock()
		ctx = console.WithTUIWriter(ctx, w)
		if err := docker.Prune(ctx, console.AssumeYes()); err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
		return nil
	}
	dialog := NewProgramBoxModel("Docker Prune", "Removing unused docker resources.", CmdLine("--prune")).WithDialogType(displayengine.DialogTypeSuccess)
	dialog.SetTask(task)
	dialog.SetIsDialog(true)
	dialog.SetMaximized(true)
	return displayengine.ShowDialogMsg{Dialog: dialog}
}

// Send sends a message to the running Bubble Tea program
func Send(msg tea.Msg) {
	if program != nil {
		program.Send(msg)
	}
}

// IsShadowEnabled returns whether shadow is currently enabled in the global config.
// Use this for dialog chrome that should reflect the active setting, not preview changes.
func IsShadowEnabled() bool {
	return displayengine.CurrentConfig().UI.Shadow
}

// CloseDialog returns a command to close the current modal dialog
func CloseDialog() tea.Cmd {
	return func() tea.Msg {
		return displayengine.CloseDialogMsg{}
	}
}

// CloseDialogWithResult returns a command to close the current modal dialog with a result
func CloseDialogWithResult(result any) tea.Cmd {
	return func() tea.Msg {
		return displayengine.CloseDialogMsg{Result: result}
	}
}

// GetConnType returns the connection type of the active session.
func GetConnType() string {
	if program == nil {
		return "local"
	}
	// We can't easily reach into the model from here, but we can track it globally.
	return activeConnType
}

var activeConnType = "local"
