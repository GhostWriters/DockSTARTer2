package classic

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui/components/sinput"
	"DockSTARTer2/internal/version"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// preloadPanelLog reads the last 200 lines of the log file.
func preloadPanelLog() tea.Msg {
	path := logger.GetLogFilePath()
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		all = append(all, sc.Text())
	}

	const maxLines = 200
	if len(all) > maxLines {
		all = all[len(all)-maxLines:]
	}
	if len(all) == 0 {
		return nil
	}
	return PanelLineMsg(strings.Join(all, "\n"))
}

// waitForPanelLine blocks until the panel's own log subscription channel
// (ch, from logger.SubscribeLogLines -- see PanelModel.logSub) sends a line,
// then returns it as a message.
func waitForPanelLine(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return PanelLineMsg(line)
	}
}

// ─── Command execution ────────────────────────────────────────────────────────

// runShellCmd runs cmdStr as a shell command, streaming output to w.
// If stdinContent is provided, it is piped to the command's stdin.
//
// On Unix, cmdStr is parsed and expanded ourselves (see parseShellArgs) and
// exec'd directly -- never handed to sh -c, so there's no shell expansion
// step left for a blocked word to hide behind. Windows keeps the simpler
// cmd /c form (Windows is test-only for this project, not a hardening
// priority).
func runShellCmd(ctx context.Context, cmdStr string, w io.Writer, stdinContent string) error {
	if runtime.GOOS == "windows" {
		shellCmd := exec.CommandContext(ctx, "cmd", "/c", cmdStr)
		shellCmd.Stdout = w
		shellCmd.Stderr = w
		if stdinContent != "" {
			shellCmd.Stdin = strings.NewReader(stdinContent + "\n")
		}
		return shellCmd.Run()
	}

	argv, err := parseShellArgs(cmdStr)
	if err != nil {
		return err
	}
	if bad := findBlockedArgvWord(argv); bad != "" {
		return fmt.Errorf("'%s' is not on the console's allowed command list", bad)
	}
	if bad := findSensitivePathArg(argv); bad != "" {
		return fmt.Errorf("'%s' refers to a file the console won't touch", styleBlockedPathArg(bad))
	}

	shellCmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	shellCmd.Stdout = w
	shellCmd.Stderr = w
	if stdinContent != "" {
		shellCmd.Stdin = strings.NewReader(stdinContent + "\n")
	}
	return shellCmd.Run()
}

// readConsoleBatch reads one line from the scanner and returns it as a
// ConsoleLinesMsg, or ConsoleDoneMsg on EOF.
func readConsoleBatch(sc *bufio.Scanner, cancel context.CancelFunc) tea.Cmd {
	return readConsoleBatchWithFlag(sc, cancel, false, false)
}

// readConsoleBatchWithFlag is like readConsoleBatch but carries post-execution
// flags so AppModel can trigger config reload and/or app list refresh.
func readConsoleBatchWithFlag(sc *bufio.Scanner, cancel context.CancelFunc, configChanged, appsChanged bool) tea.Cmd {
	if sc == nil || cancel == nil {
		return nil
	}
	return func() tea.Msg {
		if !sc.Scan() {
			cancel()
			return ConsoleDoneMsg{Err: sc.Err(), ConfigChanged: configChanged, AppsChanged: appsChanged}
		}
		return ConsoleLinesMsg{Lines: []string{sc.Text()}}
	}
}

// isDS2Prefix reports whether tok is a recognized ds2 command prefix —
// the detected binary name (e.g. "dockstarter2"), "ds2", or "ds". Used only
// to optionally strip a redundant leading token from plain (non "!"/"!!")
// input for convenience (people type or paste it out of habit) -- it no
// longer decides shell vs. ds2 routing, which is now the "!"/"!!" prefix's
// job (see submitConsoleCommand).
func isDS2Prefix(tok string) bool {
	lower := strings.ToLower(tok)
	cmdName := strings.ToLower(version.CommandName)
	return lower == cmdName || lower == "ds2" || lower == "ds"
}

// allowedShellWords are the only basenames "!"/"!!" shell commands may
// execute as argv[0]. This is default-deny by design: an unlisted binary --
// a shell we didn't think to name, a future interpreter, a renamed/symlinked
// executable -- is rejected automatically instead of requiring a blacklist
// entry to catch it after the fact. Deliberately excluded despite being
// common utilities, because their own flags can hand off to arbitrary
// further commands: find (-exec), tar (--checkpoint-action/--to-command),
// rsync (-e), xargs, env, nice, timeout, watch, general-purpose interpreters
// (awk/perl/python/ruby/lua/tclsh), pagers/editors that can shell out
// (less/more/man/vi/vim/nano), remote/arbitrary-command channels
// (ssh/telnet/ftp/socat/nc), git (-c core.pager=/-c diff.external= run an
// arbitrary shell command via CLI config override), docker/docker-compose
// (docker run/exec with a host bind mount is root-equivalent code execution
// regardless of sudo -- ds2's own docker-management commands cover this
// panel's actual docker needs instead), ip (netns exec runs an arbitrary
// command in a network namespace), and systemctl (combined with the
// file-write commands below, lets a unit file with an arbitrary ExecStart=
// be planted and started -- the ExecStart command is never checked against
// this whitelist either). "sudo" and any ds2 command-name spelling are
// handled separately in findBlockedShellWord/findBlockedArgvWord with their
// own guidance messages, not via this list.
func allowedShellWords() map[string]struct{} {
	words := []string{
		"ls", "cat", "head", "tail", "grep", "wc", "du", "df", "ps", "free",
		"uptime", "uname", "hostname", "whoami", "id", "date", "stat", "file",
		"which", "pwd", "top",
		"ping", "curl", "wget", "ss", "dig", "nslookup", "traceroute",
		"cp", "mv", "mkdir", "rmdir", "rm", "touch", "chmod", "chown", "ln",
		"echo",
	}
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

// firstField returns cmd's first whitespace-split token, or ok=false if
// cmd has none.
func firstField(cmd string) (tok string, ok bool) {
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

// findBlockedShellWord reports whether cmd's first word is disallowed as a
// "!"/"!!" shell command, returning that word or "" if it's fine. "sudo"
// and any ds2 command-name spelling are always rejected (with their own
// guidance messages at the call site) regardless of session trust -- that's
// about routing through the right mechanism ("!!" for sudo, dispatchDS2Command
// for ds2 commands), not privilege. The allowedShellWords whitelist itself
// only applies when console.RequiresRemoteSudoGate() is true: it exists to
// limit what's reachable through DS2's own multi-tenant SSH/web server,
// where a session's DS2 auth doesn't prove real OS access. A local session
// or a real external SSH shell running the TUI directly already has an
// unrestricted shell available outside DS2, so the whitelist adds no
// security value there -- only friction -- and is skipped. Only the first
// word matters: parseShellArgs rejects chaining/pipes outright, so there's
// no way for a second command to reach position 0 by any other route, and a
// disallowed word appearing later is just a harmless argument (e.g. "grep
// sudo /var/log/auth.log" never executes sudo).
//
// This is a cheap early rejection on the raw, pre-expansion text only --
// findBlockedArgvWord (run on parseShellArgs's expanded output) is the
// authoritative check, since raw text alone can't see what a glob or
// variable expands to.
func findBlockedShellWord(cmd string) string {
	tok, ok := firstField(cmd)
	if !ok {
		return ""
	}
	lower := strings.ToLower(tok)
	if lower == "sudo" || isDS2Prefix(tok) {
		return tok
	}
	if !console.RequiresRemoteSudoGate() {
		return ""
	}
	if _, allowed := allowedShellWords()[lower]; !allowed {
		return tok
	}
	return ""
}

// findBlockedArgvWord is findBlockedShellWord's counterpart for an already
// parsed and expanded argv (see parseShellArgs): checks only argv[0] --
// the one word that's actually executed -- via filepath.Base, so a
// wildcard that expands to e.g. "/usr/bin/sudo" is still caught, not just
// a bare literal word.
func findBlockedArgvWord(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	base := filepath.Base(argv[0])
	lower := strings.ToLower(base)
	if lower == "sudo" || isDS2Prefix(base) {
		return argv[0]
	}
	if !console.RequiresRemoteSudoGate() {
		return ""
	}
	if _, allowed := allowedShellWords()[lower]; !allowed {
		return argv[0]
	}
	return ""
}

// verifySudo invalidates any cached sudo credential (sudo -k) so a stale
// timestamp can't silently satisfy this check, then prompts for and
// verifies a fresh sudo password (sudo -S -v). Returns console.ErrUserAborted
// if the user cancels the prompt, or another error if authentication fails.
func (m *PanelModel) verifySudo(reason string) error {
	_ = exec.Command("sudo", "-k").Run()

	var pass string
	var err error
	if prompt := m.PromptFunc(); prompt != nil {
		pass, err = prompt("Sudo Password", reason, true)
	} else {
		pass, err = PromptTextHook("Sudo Password", reason, true)
	}
	if err != nil {
		return err
	}

	primeCmd := exec.Command("sudo", "-S", "-v")
	primeCmd.Stdin = strings.NewReader(pass + "\n")
	if err := primeCmd.Run(); err != nil {
		return fmt.Errorf("sudo: authentication failed")
	}
	return nil
}

// submitConsoleCommand parses and runs cmdStr. A leading "!!" runs the rest
// as a shell command under sudo; a leading "!" runs it as a plain shell
// command (crush-style); anything else is always a ds2 command -- there is
// no more "starts with -" guessing, since "!"/"!!" now fully own the
// shell-vs-ds2 distinction.
func (m *PanelModel) submitConsoleCommand(cmdStr string) tea.Cmd {
	trimmed := strings.TrimSpace(cmdStr)
	if trimmed == "" {
		return nil
	}

	switch {
	case strings.HasPrefix(trimmed, "!!"):
		return m.dispatchShellCommand(cmdStr, strings.TrimSpace(trimmed[2:]), true)
	case strings.HasPrefix(trimmed, "!"):
		return m.dispatchShellCommand(cmdStr, strings.TrimSpace(trimmed[1:]), false)
	default:
		return m.dispatchDS2Command(cmdStr, trimmed)
	}
}

// dispatchDS2Command strips an optional leading ds2/ds/<binary name> token
// (kept purely for convenience -- people type or paste it out of habit; it's
// never required) and routes the rest through commands.Parse/Execute,
// applying the same ConsoleBlocked/RequiresSudo/ConsoleSafe checks as
// before "!"/"!!" existed.
func (m *PanelModel) dispatchDS2Command(cmdStr, trimmed string) tea.Cmd {
	tokens := strings.Fields(trimmed)
	args := tokens
	if len(tokens) > 0 && isDS2Prefix(tokens[0]) {
		args = tokens[1:]
	}

	groups, err := commands.Parse(args)
	if err != nil {
		logger.Error(context.Background(), "%s", err.Error())
		return func() tea.Msg { return ConsoleDoneMsg{} }
	}

	// In restricted console mode, enforce ConsoleSafe flag from the command registry.
	// This blocks privileged commands like --config-panel, --server, etc. —
	// preventing a remote user from self-upgrading their access.
	if m.PanelMode == "console" {
		for _, g := range groups {
			if !commands.IsConsoleSafe(g.Command) {
				logger.Error(context.Background(),
					"Command '{{|UserCommand|}}%s{{[-]}}' is not permitted in Console mode.", g.Command)
				return func() tea.Msg { return ConsoleDoneMsg{} }
			}
		}
	}

	// Blocked in EVERY console mode, System Console included, regardless
	// of sudo -- see Def.ConsoleBlocked's doc comment.
	requiresSudo := false
	for _, g := range groups {
		if commands.IsConsoleBlocked(g.Command) {
			logger.Error(context.Background(),
				"Command '{{|UserCommand|}}%s{{[-]}}' cannot be run from the console.", g.Command)
			return func() tea.Msg { return ConsoleDoneMsg{} }
		}
		if commands.IsRequiresSudo(g.Command) {
			requiresSudo = true
		}
	}

	// A handful of ConsoleSafe commands (e.g. --config-folder) still need
	// a fresh sudo re-verification when remote -- ConsoleSafe permits
	// them in the restricted Console mode too, not just System Console,
	// so this check isn't limited to m.PanelMode == "system" the way the
	// shell-command gates are.
	if requiresSudo && console.RequiresRemoteSudoGate() {
		run := func() tea.Cmd { return m.runDS2Groups(cmdStr, groups) }
		return func() tea.Msg {
			if err := m.verifySudo("Confirm sudo access to run:\n  {{|UserCommand|}}" + cmdStr + "{{[-]}}"); err != nil {
				if err == console.ErrUserAborted {
					return ConsoleDoneMsg{}
				}
				return ConsoleDoneMsg{Err: err}
			}
			cmd := run()
			if cmd == nil {
				return nil
			}
			return cmd()
		}
	}

	return m.runDS2Groups(cmdStr, groups)
}

// dispatchShellCommand runs rawCmd (already stripped of its "!"/"!!"
// prefix) as a shell command, under sudo if sudo is true. Shell access is
// System Console only -- blocked entirely in restricted Console mode.
// Blocked in both modes if rawCmd contains "sudo", "ds", "ds2", or the
// detected binary name as a standalone word anywhere in the line (even
// split across shell chaining like "true && sudo ls"): "!!" is the only
// sanctioned way to escalate, and ds2 commands belong on the ds2-native
// path (dispatchDS2Command), not re-invoked as a subprocess.
func (m *PanelModel) dispatchShellCommand(cmdStr, rawCmd string, sudo bool) tea.Cmd {
	if m.PanelMode != "system" {
		logger.Error(context.Background(), "Shell commands ('!'/'!!') are only allowed in System Console. Switch to 'System Console' for shell access.")
		return func() tea.Msg { return ConsoleDoneMsg{} }
	}
	if rawCmd == "" {
		return nil
	}
	if bad := findBlockedShellWord(rawCmd); bad != "" {
		switch {
		case strings.EqualFold(bad, "sudo"):
			logger.Error(context.Background(), "'{{|UserCommand|}}sudo{{[-]}}' isn't allowed in '!' commands — use '!!' to run a command with sudo.")
		case isDS2Prefix(bad):
			logger.Error(context.Background(), "'{{|UserCommand|}}%s{{[-]}}' isn't allowed in '!'/'!!' commands — ds2 commands don't need a shell prefix.", bad)
		default:
			logger.Error(context.Background(), "'{{|UserCommand|}}%s{{[-]}}' is not on the console's allowed command list.", bad)
		}
		return func() tea.Msg { return ConsoleDoneMsg{} }
	}

	if sudo {
		return m.runSudoShellCommand(cmdStr, rawCmd)
	}
	return m.runShellConsoleCommand(cmdStr, rawCmd)
}

// runDS2Groups executes already-parsed, already sudo/block-checked ds2
// command groups, streaming output through the panel writer/scanner path.
func (m *PanelModel) runDS2Groups(cmdStr string, groups []commands.CommandGroup) tea.Cmd {
	// The edit-lock check happens per-group inside commands.Execute (matching
	// CLI behavior exactly), not here -- a pre-check across all groups
	// upfront would abort the entire command line at the first locked group,
	// silently skipping (and never logging) any earlier group that could
	// have run successfully, e.g. "ds2 --list-enabled -yp" should still run
	// --list-enabled even if -p is currently locked.
	configChanged := commands.GroupsNeedConfigReload(groups)
	appsChanged := commands.GroupsNeedAppsRefresh(groups)

	ctx, cancel := context.WithCancel(context.Background())
	m.ConsoleCancel = cancel
	m.replaceHeaderCount = -1
	pr, pw := io.Pipe()
	cmdCtx := console.WithPanelWriter(ctx, pw)
	if fn := m.ConfirmFunc(); fn != nil {
		cmdCtx = console.WithConfirmFunc(cmdCtx, fn)
	}
	if fn := m.PromptFunc(); fn != nil {
		cmdCtx = console.WithPromptFunc(cmdCtx, fn)
	}
	if fn := m.SendFunc(); fn != nil {
		cmdCtx = console.WithSendFunc(cmdCtx, fn)
	}

	go func() {
		// Log the command header into the pipe first
		if m.PanelMode == "system" {
			logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
		} else {
			logger.Notice(cmdCtx, "Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
		}

		commands.Execute(cmdCtx, groups, m.clientIP, m.connType, m.sessionKey, m.graphicsSupported)
		pw.Close()
	}()

	sc := bufio.NewScanner(pr)
	m.consoleScanner = sc
	m.consoleConfigChanged = configChanged
	m.consoleAppsChanged = appsChanged
	m.titleSpinner.Start()
	lockCmd := m.lockSession("console.command", true)
	return tea.Batch(lockCmd, readConsoleBatchWithFlag(sc, cancel, configChanged, appsChanged))
}

// runShellConsoleCommand runs rawCmd (already validated by
// dispatchShellCommand: System Console only, no sudo/ds/ds2 words) as a
// plain shell command.
func (m *PanelModel) runShellConsoleCommand(cmdStr, rawCmd string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.ConsoleCancel = cancel
	m.titleSpinner.Start()

	// Remote System Console: every shell command requires a fresh sudo
	// re-verification first -- a second DS2 user (different pubkey/password)
	// could otherwise ride an earlier remote session's already-enabled
	// System Console toggle without ever proving they have sudo rights on
	// this machine. ds2-native commands (dispatchDS2Command) are never
	// gated this way -- only real shell access needs it. Local sessions are
	// exempt: DS2's own connType can't be spoofed to "local" over the
	// network (see stripDS2TrustEnv), so this only ever gates real remote
	// access.
	if console.RequiresRemoteSudoGate() {
		return func() tea.Msg {
			if err := m.verifySudo("Confirm sudo access to run:\n  {{|UserCommand|}}" + rawCmd + "{{[-]}}"); err != nil {
				if err == console.ErrUserAborted {
					return ConsoleDoneMsg{}
				}
				return ConsoleDoneMsg{Err: err}
			}

			pr, pw := io.Pipe()
			cmdCtx := console.WithPanelWriter(ctx, pw)
			go func() {
				logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
				err := runShellCmd(ctx, rawCmd, pw, "")
				pw.CloseWithError(err)
			}()

			sc := bufio.NewScanner(pr)
			return ConsoleScannerReadyMsg{Scanner: sc, Cancel: cancel}
		}
	}

	pr, pw := io.Pipe()
	cmdCtx := console.WithPanelWriter(ctx, pw)
	go func() {
		logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
		err := runShellCmd(ctx, rawCmd, pw, "")
		pw.CloseWithError(err)
	}()

	sc := bufio.NewScanner(pr)
	m.consoleScanner = sc
	lockCmd := m.lockSession("console.command", true)
	return tea.Batch(lockCmd, readConsoleBatch(sc, cancel))
}

// runSudoShellCommand runs rawCmd (already validated by
// dispatchShellCommand: System Console only, no sudo/ds/ds2 words) under
// sudo. Always prompts for the password rather than trusting a "sudo -n
// true" cache-check: that check can succeed (no password needed) even when
// the real "-S" run moments later still requires one, since sudo's cache
// can go stale between the two checks. Re-entering an already-valid
// password via -S is harmless, so always asking is simpler and reliably
// correct.
func (m *PanelModel) runSudoShellCommand(cmdStr, rawCmd string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.ConsoleCancel = cancel
	m.titleSpinner.Start()

	// Returned as a bare closure, not wrapped in tea.Batch: every other
	// working path here either returns a bare closure or batches two
	// genuinely distinct commands. Wrapping a single closure in tea.Batch
	// leaves the console panel's read side never re-polling after the
	// first line, stalling the command permanently.
	return func() tea.Msg {
		question := "Password required to run:\n  {{|UserCommand|}}sudo " + rawCmd + "{{[-]}}"
		var pass string
		var err error
		if prompt := m.PromptFunc(); prompt != nil {
			pass, err = prompt("Sudo Password", question, true)
		} else {
			pass, err = PromptTextHook("Sudo Password", question, true)
		}
		if err != nil {
			if err == console.ErrUserAborted {
				return ConsoleDoneMsg{}
			}
			return ConsoleDoneMsg{Err: err}
		}

		pr, pw := io.Pipe()
		cmdCtx := console.WithPanelWriter(ctx, pw)
		go func() {
			// Log the command header into the pipe first
			logger.Notice(cmdCtx, "System Console command: '{{|UserCommand|}}%s{{[-]}}'", cmdStr)
			// sudo prints its own "Sorry, try again"/exit reason as part of
			// its output on a bad password, so no special-case error
			// relabeling is needed here.
			err := runSudoWithPassword(ctx, rawCmd, pass, pw)
			pw.CloseWithError(err)
		}()

		sc := bufio.NewScanner(pr)
		return ConsoleScannerReadyMsg{Scanner: sc, Cancel: cancel}
	}
}


// ─── Update ───────────────────────────────────────────────────────────────────

func (m PanelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case sinput.PasteMsg, sinput.CutMsg, sinput.SelectAllMsg:
		if m.InputFocused {
			var cmd tea.Cmd
			m.Input, cmd = m.Input.Update(msg)
			return m, cmd
		}
		return m, nil

	case ConsoleLockMsg:
		return m, m.lockSession(msg.ID, msg.Locked)

	case ConfigChangedMsg:
		m.PanelMode = EffectivePanelMode(msg.Config, m.connType)
		if m.PanelMode == "none" {
			m.Expanded = false
		}
		m.SetSize(m.width, m.totalHeight)
		m.applyInputStyles()
		return m, nil

	case PanelLineMsg:
		m.lastLineTime = time.Now()
		if !m.Expanded {
			m.panelChanged = true
		}
		m.Sv.AppendLines(strings.Split(string(msg), "\n"), panelRenderFn())
		return m, waitForPanelLine(m.logSub)

	case ConsoleScannerReadyMsg:
		m.consoleScanner = msg.Scanner
		m.ConsoleCancel = msg.Cancel
		return m, readConsoleBatch(msg.Scanner, msg.Cancel)

	case ConsoleLinesMsg:
		m.lastLineTime = time.Now()
		if !m.Expanded {
			m.panelChanged = true
		}
		m.Sv.CommandRunning = true
		m.Sv.AppendLines(msg.Lines, panelRenderFn())
		if m.consoleScanner == nil {
			return m, nil
		}
		return m, readConsoleBatchWithFlag(m.consoleScanner, m.ConsoleCancel, m.consoleConfigChanged, m.consoleAppsChanged)

	case ReplaceOutputMsg:
		SetActiveOutputWidth(m.Sv.Width())
		m.lastLineTime = time.Now()
		if !m.Expanded {
			m.panelChanged = true
		}
		m.Sv.CommandRunning = false
		if m.replaceHeaderCount < 0 {
			m.replaceHeaderCount = m.Sv.TotalLineCount()
		}
		m.Sv.ReplaceTailLines(m.replaceHeaderCount, msg.Lines, panelRenderFn())
		return m, nil

	case ConsoleDoneMsg:
		m.consoleScanner = nil
		m.ConsoleCancel = nil
		m.replaceHeaderCount = -1
		m.Sv.CommandRunning = false
		m.Sv.ClearSpinner()
		if msg.Err != nil {
			logger.Error(context.Background(), "%s", msg.Err.Error())
		}
		unlockCmd := m.lockSession("console.command", false)
		if !m.SessionActive() {
			m.InputFocused = true
			cmd := m.Input.Focus()
			return m, tea.Batch(unlockCmd, cmd, sinput.Blink)
		}
		return m, unlockCmd

	case TogglePanelMsg:
		m.Expanded = !m.Expanded
		if m.Expanded {
			m.panelChanged = false
			m.SetSize(m.width, m.totalHeight)
			m.Sv.GotoBottom()
		}
		return m, nil

	case LayerHitMsg:
		if strings.HasSuffix(msg.ID, ".sb.up") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.ScrollUp(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.down") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.ScrollDown(1)
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.above") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.HalfPageUp()
			}
			return m, nil
		}
		if strings.HasSuffix(msg.ID, ".sb.below") {
			if m.Expanded && msg.Button != HoverButton {
				m.Sv.HalfPageDown()
			}
			return m, nil
		}
		if msg.ID == IDConsoleInput && msg.Button == tea.MouseRight {
			return m, ShowInputContextMenu(m.Input, msg.X, msg.Y, m.width, m.totalHeight)
		}
		if msg.ID == panelZoneID {
			return m, func() tea.Msg { return TogglePanelMsg{} }
		}

	case DragDoneMsg:
		if msg.ID == ResizeZoneID {
			m.ResizeDrag.DragPending = false
			if m.ResizeDrag.PendingDragY != m.ResizeDrag.LastDragY {
				m.ResizeDrag.LastDragY = m.ResizeDrag.PendingDragY
				m.applyDragY(m.ResizeDrag.PendingDragY)
				m.ResizeDrag.DragPending = true
				return m, DragDoneCmd(ResizeZoneID)
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft {
			m.ResizeDrag.StartDrag(msg.Y, m.PanelHeight, ScrollbarInfo{})
			if !m.Expanded {
				m.Expanded = true
				m.PanelHeight = 1
				m.SetSize(m.width, m.totalHeight)
				m.ResizeDrag.StartThumbTop = 1
				m.Sv.GotoBottom()
			}
			return m, nil
		}

	case tea.MouseReleaseMsg:
		if m.ResizeDrag.Dragging {
			m.ResizeDrag.StopDrag()
			m.SetSize(m.width, m.totalHeight)
			return m, nil
		}

	case tea.MouseMotionMsg:
		// Applied unconditionally on every event -- the render itself (not
		// this state update) is what gets coalesced during a fast drag, at
		// the AppModel level.
		if m.ResizeDrag.Dragging {
			m.applyDragY(msg.Y)
			return m, nil
		}

	case tea.MouseWheelMsg:
		if m.Expanded {
			if msg.Button == tea.MouseWheelUp {
				m.Sv.ScrollUp(3)
				return m, nil
			}
			if msg.Button == tea.MouseWheelDown {
				m.Sv.ScrollDown(3)
				return m, nil
			}
		}

	case tea.KeyPressMsg:
		if m.Focused && m.InputFocused {
			return m.updateInputFocused(msg)
		}
		if m.Expanded {
			switch {
			case key.Matches(msg, Keys.Home):
				m.Sv.GotoTop()
				return m, nil
			case key.Matches(msg, Keys.End):
				m.Sv.GotoBottom()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	if m.Expanded {
		cmd = m.Sv.ViewUpdate(msg)
	}
	return m, cmd
}

// updateInputFocused handles key events when the input bar has focus.
func (m PanelModel) updateInputFocused(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, Keys.Esc):
		m.Input.Blur()
		m.InputFocused = false
		return m, nil

	case key.Matches(msg, Keys.Up):
		if len(m.history) == 0 {
			return m, nil
		}
		if m.historyIdx == -1 {
			m.historyDraft = m.Input.Value()
			m.historyIdx = len(m.history) - 1
		} else if m.historyIdx > 0 {
			m.historyIdx--
		}
		m.Input.SetValue(m.history[m.historyIdx])
		m.Input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Down):
		if m.historyIdx == -1 {
			return m, nil
		}
		m.historyIdx++
		if m.historyIdx >= len(m.history) {
			m.historyIdx = -1
			m.Input.SetValue(m.historyDraft)
		} else {
			m.Input.SetValue(m.history[m.historyIdx])
		}
		m.Input.CursorEnd()
		return m, nil

	case key.Matches(msg, Keys.Enter):
		cmdStr := strings.TrimSpace(m.Input.Value())
		if cmdStr == "" {
			return m, nil
		}
		m.history = append(m.history, cmdStr)
		m.historyIdx = -1
		m.historyDraft = ""
		m.Input.SetValue("")
		m.Input.Blur()
		m.InputFocused = false

		// Show the submitted command in the scrollback, matching the input
		// prompt's own "text starts immediately after >" spacing.
		m.Sv.AppendLines([]string{">{{|RunningCommand|}}" + cmdStr + "{{[-]}}"}, panelRenderFn())

		return m, m.submitConsoleCommand(cmdStr)
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}
