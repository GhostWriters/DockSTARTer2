package tui

import (
	"context"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/displayengine"
	"DockSTARTer2/internal/sessionlocks"

	tea "charm.land/bubbletea/v2"
)

// StartForSession builds and registers a Bubble Tea program for a single
// remote session whose caller -- not this function -- calls Run() and owns
// the goroutine it runs in. This is for transports like sip's
// ServeWithProgram, which take an already-constructed *tea.Program and run
// it themselves; Start (used by SSH sessions and the local terminal) covers
// the same bootstrap plus Run() and post-run cleanup in one call because it
// owns that goroutine directly.
//
// The returned finish func must be called exactly once, after the caller's
// Run() call returns, to release the session's resources (mirrors the second
// half of Start, after its own p.Run() call). Since this function's caller
// observes Run()'s return, not this one, finish carries no error/final-model
// handling -- log any Run() error at the call site if needed.
//
// Unlike Start, this does not install its own panic recovery or ctx-done
// shutdown watcher: the caller's Run() goroutine is outside this package's
// control, and DS2-level disconnect requests should call the returned
// program's own Quit() directly rather than through ctx cancellation.
func StartForSession(ctx context.Context, startMenu string, opts ProgramOptions) (*tea.Program, func(), error) {
	clientIP, connType, viaOwnServer := parseClientInfo(opts.Environ)
	sessionKey := parseSessionKey(opts.Environ)
	console.SetViaOwnServer(viaOwnServer)
	console.SetClientIP(clientIP)

	// See Start's matching comment (internal/tui/tui.go): activate
	// connType's tint for the Initialize/screen-construction/NewAppModel
	// span below, released before returning p to the caller -- sip's
	// ServeWithProgram calls p.Run() itself, outside this function, and
	// AppModel.Update/View activate this same tint again per render via
	// ActivateSessionRenderContext, which would deadlock on semstyle's
	// non-reentrant tint mutex if this scope were still held when that
	// starts.
	ctx = console.WithConnType(ctx, connType)
	restoreStartupTint := BeginTintFor(connType)
	endStartupTintScope := func() {
		if restoreStartupTint != nil {
			restoreStartupTint()
			restoreStartupTint = nil
		}
	}
	defer endStartupTintScope()
	RegisterConnTypeTints(ctx, connType, config.LoadAppConfig().Appearance.ForConnType(connType).AnsiColors)

	if err := Initialize(ctx); err != nil {
		return nil, nil, err
	}

	pageName, isRoot := resolveMenuTarget(startMenu)
	isRootSession = isRoot
	activeConnType = connType
	webOutbound = opts.WebOutbound
	webToken = opts.WebToken
	opts.RefreshRate = resolveRefreshRate(connType, opts.WebToken)

	entry, ok := screenRegistry[pageName]
	if !ok {
		entry = screenRegistry["main"]
	}
	startScreen := entry.create(isRoot, connType)

	var initialStack []ScreenModel
	if !isRoot {
		for _, parentName := range entry.parents {
			if parentEntry, ok := screenRegistry[parentName]; ok {
				initialStack = append(initialStack, parentEntry.create(false, connType))
			}
		}
	}

	model := NewAppModel(ctx, displayengine.CurrentConfig(), clientIP, connType, sessionKey, opts.Environ, startScreen, initialStack...)
	endStartupTintScope()

	p := NewProgram(model, opts)
	model.panel.SetConfirmFunc(sessionConfirmFunc(p))
	model.panel.SetPromptFunc(sessionPromptFunc(p))
	model.SetProgram(p)

	exited := registerSession(p)

	startWindowSizeForwarder(ctx, p, opts)
	go startUpdateChecker(ctx, model.Send)
	startConfigWatcher(ctx, p)
	startLockFileWatcher(ctx, p)
	startRestartWatcher(ctx)

	finish := func() {
		close(exited)
		unregisterSession(p)
		model.Cleanup()
		// Scoped to this session's own key -- a server daemon runs many
		// sessions in one process, so an unconditional release would clear
		// a lock a different, still-running session legitimately holds
		// (see sessionlocks.ReleaseEditLockAs's doc comment). Without this,
		// a session that ends while holding the edit lock (e.g. a browser
		// reconnect triggered from sip's own settings panel while the env
		// editor is open) left it stuck until the whole daemon restarted.
		sessionlocks.Sessions.ReleaseEditLockAs(sessionKey)
		// Must be last: WaitForActiveSessions (see registerSession's doc
		// comment) depends on this session's cleanup, edit-lock release
		// included, having fully run before it's marked done.
		sessionDone()
	}

	return p, finish, nil
}
