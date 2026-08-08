package commands

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
)

// HandleUninstall removes DockSTARTer2's state and cache folders and
// deletes the running binary. The config folder (settings, themes) and the
// compose folder (the user's actual app configuration) are always kept, so
// a later reinstall picks up right where the user left off. The shared
// templates repository is kept only if a legacy DockSTARTer install is
// still present; otherwise it's removed along with the rest of that
// install's shared state folder.
func HandleUninstall(ctx context.Context, state *CmdState) error {
	if runtime.GOOS == "windows" {
		logger.Warn(ctx, "Uninstalling {{|ApplicationName|}}%s{{[-]}} is not supported on Windows.", version.ApplicationName)
		return nil
	}

	question := "Do you want to uninstall {{|ApplicationName|}}" + version.ApplicationName + "{{[-]}}?"
	answer, err := console.QuestionPrompt(ctx, logger.Notice, "Uninstall", question, "N", state.Yes)
	if err != nil || !answer {
		logger.Notice(ctx, "Not uninstalling {{|ApplicationName|}}%s{{[-]}}", version.ApplicationName)
		return nil
	}

	logger.Notice(ctx, []string{
		"",
		"Uninstalling {{|ApplicationName|}}" + version.ApplicationName + "{{[-]}}",
		"",
	})

	conf := config.LoadAppConfig()
	ds1Installed := paths.GetBashScriptFolder() != ""

	keeping := []string{paths.GetConfigDir(), conf.ComposeDir}
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if volumeConfig, err := appenv.ResolveDockerVolumeConfig(ctx, envFile, conf); err == nil {
		keeping = append(keeping, volumeConfig)
	}
	if ds1Installed {
		templatesDir := filepath.Dir(paths.GetTemplatesDir())
		keeping = append(keeping, paths.GetLegacyConfigDir(), templatesDir)
		logger.Notice(ctx, []string{
			"{{|ApplicationName|}}DockSTARTer{{[-]}} install detected. Keeping downloaded templates in '" + console.FormatFolderPath(templatesDir) + "'.",
			"",
		})
	}

	keepLines := []string{"Keeping:"}
	for _, p := range keeping {
		keepLines = append(keepLines, "\t'"+console.FormatFolderPath(p)+"'")
	}
	keepLines = append(keepLines, "")
	logger.Notice(ctx, keepLines)

	cacheDir := paths.GetCacheDir()
	if info, err := os.Stat(cacheDir); err == nil && info.IsDir() {
		logger.Notice(ctx, "Removing {{|ApplicationName|}}%s{{[-]}} cache folder '"+console.FormatFolderPath(cacheDir)+"'.", version.ApplicationName)
		removePath(ctx, cacheDir)
	}

	stateDir := paths.GetStateDir()
	logger.Notice(ctx, "Removing {{|ApplicationName|}}%s{{[-]}} state folder '"+console.FormatFolderPath(stateDir)+"'.", version.ApplicationName)
	removePath(ctx, stateDir)

	if !ds1Installed {
		legacyStateDir := paths.GetLegacyStateDir()
		logger.Notice(ctx, "No {{|ApplicationName|}}DockSTARTer{{[-]}} install detected. Removing downloaded templates in '"+console.FormatFolderPath(legacyStateDir)+"'.")
		removePath(ctx, legacyStateDir)
	}

	if exePath, err := os.Executable(); err == nil {
		logger.Notice(ctx, "Removing {{|ApplicationName|}}%s{{[-]}} executable file: '"+console.FormatFilePath(exePath)+"'.", version.ApplicationName)
		removePath(ctx, exePath)
	}

	logger.Notice(ctx, "{{|ApplicationName|}}%s{{[-]}} has been uninstalled.", version.ApplicationName)
	return nil
}

// removePath deletes path, falling back to sudo if a plain removal fails.
func removePath(ctx context.Context, path string) {
	if path == "" {
		return
	}
	if err := os.RemoveAll(path); err == nil {
		return
	}
	cmd, err := dsexec.SudoCommand(ctx, "rm", "-rf", path)
	if err != nil {
		logger.Warn(ctx, "Failed to remove '"+console.FormatFolderPath(path)+"': %v", err)
		return
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		logger.Warn(ctx, "Failed to remove '"+console.FormatFolderPath(path)+"': %s: %v", string(out), err)
	}
}
