package tui

import (
	"context"
	"time"

	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"

	"charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// startConfigWatcher watches the config file for external changes (e.g. from
// `ds2 --theme-no-shadows` run in a separate terminal or via the web server).
// On a valid change it sends ConfigChangedMsg to the running program.
// Invalid TOML files are silently ignored so a mid-write partial file does not
// flash bad state into the UI.
func startConfigWatcher(ctx context.Context, p *tea.Program) {
	cfgPath := paths.GetConfigFilePath()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Debug(ctx, "config watcher unavailable: %v", err)
		return
	}

	if err := watcher.Add(cfgPath); err != nil {
		// File may not exist yet; not fatal.
		logger.Debug(ctx, "config watcher: cannot watch %s: %v", cfgPath, err)
		watcher.Close()
		return
	}

	go func() {
		defer watcher.Close()

		const debounce = 500 * time.Millisecond
		var timer *time.Timer

		for {
			select {
			case <-ctx.Done():
				return

			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					if timer != nil {
						timer.Stop()
					}
					timer = time.AfterFunc(debounce, func() {
						conf, err := config.TryLoadAppConfig()
						if err != nil {
							// Invalid or mid-write file; ignore.
							return
						}
						p.Send(ConfigChangedMsg{Config: conf})
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Debug(ctx, "config watcher error: %v", err)
			}
		}
	}()
}
