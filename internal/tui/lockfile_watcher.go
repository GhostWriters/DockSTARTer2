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

// startLockFileWatcher watches for lock file events in the locks directory
// and sends ConsoleLockMsg to the running program so the console input bar
// locks/unlocks in response to activity (SSH sessions or Edit modes).
func startLockFileWatcher(ctx context.Context, p *tea.Program) {
	locksDir := paths.GetLocksDir()
	remoteLockBase := filepath.Base(paths.GetRemoteLockPath())
	editLockBase := "edit.lock"

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Debug(ctx, "lockfile watcher unavailable: %v", err)
		return
	}

	if err := watcher.Add(locksDir); err != nil {
		logger.Debug(ctx, "lockfile watcher: cannot watch %s: %v", locksDir, err)
		watcher.Close()
		return
	}

	// Initial state check
	if lockfile.IsLocked(paths.GetRemoteLockPath()) {
		p.Send(ConsoleLockMsg{ID: "remote.lock", Locked: true})
	}
	if lockfile.IsLocked(filepath.Join(locksDir, editLockBase)) {
		p.Send(ConsoleLockMsg{ID: "edit.lock", Locked: true})
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
				base := filepath.Base(event.Name)
				isRemote := base == remoteLockBase
				isEdit := base == editLockBase
				if !isRemote && !isEdit {
					continue
				}

				lockID := base
				locked := lockfile.IsLocked(event.Name)
				p.Send(ConsoleLockMsg{ID: lockID, Locked: locked})

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Debug(ctx, "lockfile watcher error: %v", err)
			}
		}
	}()
}
