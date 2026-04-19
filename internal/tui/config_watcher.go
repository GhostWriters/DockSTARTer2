package tui

import (
	"context"
	"path/filepath"
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
//
// We watch the parent directory rather than the file itself because many
// editors and os.WriteFile use atomic rename-based writes, which cause the
// file watcher to lose track of the inode after the first write.
func startConfigWatcher(ctx context.Context, p *tea.Program) {
	cfgPath := paths.GetConfigFilePath()
	cfgDir := filepath.Dir(cfgPath)
	cfgBase := filepath.Base(cfgPath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Debug(ctx, "config watcher unavailable: %v", err)
		return
	}

	if err := watcher.Add(cfgDir); err != nil {
		logger.Debug(ctx, "config watcher: cannot watch %s: %v", cfgDir, err)
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
				if filepath.Base(event.Name) != cfgBase {
					continue
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) {
					if timer != nil {
						timer.Stop()
					}
					timer = time.AfterFunc(debounce, func() {
						conf, err := config.TryLoadAppConfig()
						if err != nil {
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
