package tui

import (
	"context"
	"path/filepath"

	"DockSTARTer2/internal/lockfile"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// startLockFileWatcher watches for remote.lock create/delete events and sends
// ConsoleLockMsg to the running program so the console input bar locks/unlocks
// in response to an active SSH/web session on the server side.
func startLockFileWatcher(ctx context.Context, p *tea.Program) {
	lockPath := paths.GetRemoteLockPath()
	stateDir := filepath.Dir(lockPath)
	lockBase := filepath.Base(lockPath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Debug(ctx, "lockfile watcher unavailable: %v", err)
		return
	}

	if err := watcher.Add(stateDir); err != nil {
		logger.Debug(ctx, "lockfile watcher: cannot watch %s: %v", stateDir, err)
		watcher.Close()
		return
	}

	// Send initial state in case remote.lock already exists when TUI starts.
	if lockfile.IsLocked(lockPath) {
		p.Send(ConsoleLockMsg{ID: "remote.lock", Locked: true})
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) != lockBase {
					continue
				}
				switch {
				case event.Has(fsnotify.Create) || event.Has(fsnotify.Write):
					p.Send(ConsoleLockMsg{ID: "remote.lock", Locked: true})
				case event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename):
					p.Send(ConsoleLockMsg{ID: "remote.lock", Locked: false})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Debug(ctx, "lockfile watcher error: %v", err)
			}
		}
	}()
}
