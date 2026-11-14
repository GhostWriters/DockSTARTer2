package compose

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
)

// MergeYML merges enabled app templates into docker-compose.yml
func MergeYML(ctx context.Context, force bool) error {
	conf := config.LoadAppConfig()
	composeFile := filepath.Join(conf.ComposeDir, constants.ComposeFileName)

	// 1. Check if merge is needed (Mirror Bash which checks this BEFORE setup tasks)
	if !force && !NeedsYMLMerge(ctx, false) {
		logger.Notice(ctx, "Enabled app templates already merged to '"+console.FormatFilePath(composeFile)+"'.")
		return nil
	}

	// 2. Ensure appvars are created first
	if err := appenv.CreateAll(ctx, force, conf); err != nil {
		return err
	}

	// 3. Dual Check (CreateAll might have modified files)
	if !force && !NeedsYMLMerge(ctx, false) {
		logger.Notice(ctx, "Enabled app templates already merged to '"+console.FormatFilePath(composeFile)+"'.")
		return nil
	}

	logger.Notice(ctx, "Adding enabled app templates to merge '"+console.FormatFilePath(composeFile)+"'.")

	envFile := filepath.Join(conf.ComposeDir, ".env")
	enabledApps, err := appenv.ListEnabledApps(conf)
	if err != nil {
		return fmt.Errorf("failed to get enabled apps: %w", err)
	}

	if len(enabledApps) == 0 {
		logger.Error(ctx, "No enabled apps found.")
		return fmt.Errorf("no enabled apps found")
	}

	var composeFiles []string
	for _, appName := range enabledApps {
		niceName := appenv.GetNiceName(ctx, appName)
		appenv.LogAppTemplateOverride(ctx, appName)

		// Check if app is deprecated
		if appenv.IsAppDeprecated(ctx, appName) {
			logger.Warn(ctx,
				"'{{|App|}}%s{{[-]}}' IS DEPRECATED!",
				niceName)
			logger.Warn(ctx,
				"Please run '{{|UserCommand|}}ds2 --status-disable %s{{[-]}}' to disable it.",
				niceName)
		}

		files, err := appTemplateFiles(ctx, conf.Arch, appName, envFile, true)
		if err != nil {
			return err
		}
		composeFiles = append(composeFiles, files...)

		// Create config folders (mirrors yml_merge.sh logic)
		if err := appenv.CreateAppFolders(ctx, appName, conf); err != nil {
			logger.Warn(ctx, "Failed to create config folders for %s: %v", appName, err)
		}
	}

	// Use SDK to load and merge all compose files

	composePath := filepath.Join(conf.ComposeDir, "docker-compose.yml")

	// Load environment variables for interpolation from .env
	envMap := make(map[string]string)
	if vars, err := dotenv.GetEnvFromFile(make(map[string]string), []string{envFile}); err == nil {
		for k, v := range vars {
			envMap[strings.Clone(k)] = strings.Clone(v)
		}
	}

	configFiles := make([]types.ConfigFile, len(composeFiles))
	for i, f := range composeFiles {
		configFiles[i] = types.ConfigFile{Filename: f}
	}

	configDetails := types.ConfigDetails{
		WorkingDir:  conf.ComposeDir,
		ConfigFiles: configFiles,
		Environment: envMap,
	}

	projectName := envMap["COMPOSE_PROJECT_NAME"]
	override := true
	if projectName == "" {
		projectName = loader.NormalizeProjectName(filepath.Base(conf.ComposeDir))
		override = false
	}

	project, err := loader.LoadWithContext(ctx, configDetails, func(options *loader.Options) {
		options.SetProjectName(projectName, override)
		options.SkipInterpolation = false
		options.SkipValidation = true
		// Match `docker compose config`'s own CLI behavior (and DS1's
		// output, which shells out to that CLI): drop top-level
		// networks/volumes/secrets/configs/models that no active service
		// actually references, rather than leaving orphaned declarations
		// (e.g. a named volume block for a mount a service defines as a
		// bind mount instead) in the merged docker-compose.yml.
		options.PruneUnnecessaryResources = true
	})
	if err != nil {
		logger.Error(ctx, "Failed to load compose configurations: %v", err)
		return fmt.Errorf("failed to load compose configurations: %w", err)
	}

	for i, service := range project.Services {
		service.EnvFiles = nil
		project.Services[i] = service
	}

	marshaledProject, err := project.MarshalYAML()
	if err != nil {
		logger.Error(ctx, "Failed to marshal merged compose configuration: %v", err)
		return fmt.Errorf("failed to marshal merged compose configuration: %w", err)
	}

	existing, _ := os.ReadFile(composePath)
	if !bytes.Equal(existing, marshaledProject) {
		if err := os.WriteFile(composePath, marshaledProject, 0644); err != nil {
			logger.FatalWithStack(ctx, []string{
				"Failed to copy file.",
				"Failing command: {{|FailingCommand|}}cp -f \"<tmp>\" \"" + composePath + "\"{{[-]}}",
			})
		}
	}

	// Mark YML merge as complete
	UnsetNeedsYMLMerge(ctx)

	return nil
}

// appTemplateFiles returns the template files that make up appName in the merged
// compose file, in merge order: its arch file, its hostname and ports files or its
// netmode file, its storage and devices files, then its main file. With report, it
// checks the app's instance folder and logs what's missing, as the merge does; without
// it, it does neither, for a preview before anything is merged.
func appTemplateFiles(ctx context.Context, arch, appName, envFile string, report bool) ([]string, error) {
	appNameLower := strings.ToLower(appName)
	instanceFolder := paths.GetInstanceDir(appNameLower)
	if report && !dirExists(instanceFolder) {
		logger.Error(ctx, "Folder '"+console.FormatFolderPath(instanceFolder)+"/' does not exist.")
		return nil, fmt.Errorf("folder %s does not exist", instanceFolder)
	}
	info := func(file string) {
		if report {
			logger.Info(ctx, "File '"+console.FormatFilePath(file)+"' does not exist.")
		}
	}
	var files []string
	// optional adds the instance file for suffix when it exists.
	optional := func(suffix string) error {
		file, err := appenv.AppInstanceFile(ctx, appName, suffix)
		if err != nil {
			return err
		}
		if file != "" && fileExists(file) {
			files = append(files, file)
		} else if file != "" {
			info(file)
		}
		return nil
	}

	// Get architecture-specific file
	archFile, err := appenv.AppInstanceFile(ctx, appName, fmt.Sprintf("*.%s.yml", arch))
	if err != nil {
		return nil, fmt.Errorf("failed to get arch file for %s: %w", appName, err)
	}
	if archFile == "" || !fileExists(archFile) {
		if report {
			logger.Error(ctx, "File '"+console.FormatFilePath(archFile)+"' does not exist.")
		}
		return nil, fmt.Errorf("file %s does not exist", archFile)
	}
	files = append(files, archFile)

	// Network mode specific files
	netMode, _ := appenv.Get(fmt.Sprintf("%s__NETWORK_MODE", appName), envFile)
	if netMode == "" || netMode == "bridge" {
		if err := optional("*.hostname.yml"); err != nil {
			return nil, err
		}
		if err := optional("*.ports.yml"); err != nil {
			return nil, err
		}
	} else if err := optional("*.netmode.yml"); err != nil {
		return nil, err
	}

	// Storage files
	multipleStorage, _ := appenv.Get("DOCKER_MULTIPLE_STORAGE", envFile)
	storageNumbers := []string{""}
	if appenv.IsTrue(multipleStorage) {
		storageNumbers = append(storageNumbers, "2", "3", "4")
	}
	for _, num := range storageNumbers {
		storageOn, _ := appenv.Get(fmt.Sprintf("%s__STORAGE%s_ON", appName, num), envFile)
		if storageOn == "" {
			storageOn, _ = appenv.Get(fmt.Sprintf("DOCKER_STORAGE%s_ON", num), envFile)
		}
		if appenv.IsTrue(storageOn) {
			if storageVolume, _ := appenv.Get(fmt.Sprintf("DOCKER_VOLUME_STORAGE%s", num), envFile); storageVolume != "" {
				if err := optional(fmt.Sprintf("*.storage%s.yml", num)); err != nil {
					return nil, err
				}
			}
		}
	}

	// Devices file
	if appDevices, _ := appenv.Get(fmt.Sprintf("%s__DEVICES", appName), envFile); appenv.IsTrue(appDevices) {
		if err := optional("*.devices.yml"); err != nil {
			return nil, err
		}
	}

	// Main file (always last)
	mainFile, err := appenv.AppInstanceFile(ctx, appName, "*.yml")
	if err != nil {
		return nil, err
	}
	if mainFile == "" || !fileExists(mainFile) {
		expectedMain := filepath.Join(instanceFolder, fmt.Sprintf("%s.yml", appNameLower))
		if report {
			logger.Error(ctx, "File '"+console.FormatFilePath(expectedMain)+"' does not exist.")
		}
		return nil, fmt.Errorf("file %s does not exist", expectedMain)
	}
	return append(files, mainFile), nil
}
