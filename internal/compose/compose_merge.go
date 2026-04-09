package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// MergeYML merges enabled app templates into docker-compose.yml
func MergeYML(ctx context.Context, force bool) error {
	// 1. Check if merge is needed (Mirror Bash which checks this BEFORE setup tasks)
	if !force && !NeedsYMLMerge(ctx, false) {
		logger.Notice(ctx, "Enabled app templates already merged to '{{|File|}}docker-compose.yml{{[-]}}'.")
		return nil
	}

	conf := config.LoadAppConfig()

	// 2. Ensure appvars are created first
	if err := appenv.CreateAll(ctx, force, conf); err != nil {
		return err
	}

	// 3. Dual Check (CreateAll might have modified files)
	if !force && !NeedsYMLMerge(ctx, false) {
		logger.Notice(ctx, "Enabled app templates already merged to '{{|File|}}docker-compose.yml{{[-]}}'.")
		return nil
	}

	logger.Notice(ctx, "Adding enabled app templates to merge '{{|File|}}docker-compose.yml{{[-]}}'. Please be patient, this can take a while.")

	envFile := filepath.Join(conf.ComposeDir, ".env")
	enabledApps, err := appenv.ListEnabledApps(conf)
	if err != nil {
		return fmt.Errorf("failed to get enabled apps: %w", err)
	}

	if len(enabledApps) == 0 {
		logger.Error(ctx, "No enabled apps found.")
		return fmt.Errorf("no enabled apps found")
	}

	arch := conf.Arch
	var composeFiles []string

	for _, appName := range enabledApps {
		appNameLower := strings.ToLower(appName)
		niceName := appenv.GetNiceName(ctx, appName)

		instanceFolder := paths.GetInstanceDir(appNameLower)
		if !dirExists(instanceFolder) {
			logger.Error(ctx, "Folder '{{|Folder|}}%s/{{[-]}}' does not exist.", instanceFolder)
			return fmt.Errorf("folder %s does not exist", instanceFolder)
		}

		// Check if app is deprecated
		if appenv.IsAppDeprecated(ctx, appName) {
			logger.Warn(ctx,
				"'{{|App|}}%s{{[-]}}' IS DEPRECATED!",
				niceName)
			logger.Warn(ctx,
				"Please run '{{|UserCommand|}}ds2 --status-disable %s{{[-]}}' to disable it.",
				niceName)
		}

		// Get architecture-specific file
		archFile, err := appenv.AppInstanceFile(ctx, appName, fmt.Sprintf("*.%s.yml", arch))
		if err != nil {
			return fmt.Errorf("failed to get arch file for %s: %w", appName, err)
		}
		if archFile == "" || !fileExists(archFile) {
			logger.Error(ctx, "File '{{|File|}}%s{{[-]}}' does not exist.", archFile)
			return fmt.Errorf("file %s does not exist", archFile)
		}
		composeFiles = append(composeFiles, archFile)

		// Network mode specific files
		netMode, _ := appenv.Get(fmt.Sprintf("%s__NETWORK_MODE", appName), envFile)
		if netMode == "" || netMode == "bridge" {
			// Add hostname file if exists
			hostnameFile, err := appenv.AppInstanceFile(ctx, appName, "*.hostname.yml")
			if err != nil {
				return err
			}
			if hostnameFile != "" && fileExists(hostnameFile) {
				composeFiles = append(composeFiles, hostnameFile)
			}

			// Add ports file if exists
			portsFile, err := appenv.AppInstanceFile(ctx, appName, "*.ports.yml")
			if err != nil {
				return err
			}
			if portsFile != "" && fileExists(portsFile) {
				composeFiles = append(composeFiles, portsFile)
			}
		} else if netMode != "" {
			// Add netmode file if exists
			netmodeFile, err := appenv.AppInstanceFile(ctx, appName, "*.netmode.yml")
			if err != nil {
				return err
			}
			if netmodeFile != "" && fileExists(netmodeFile) {
				composeFiles = append(composeFiles, netmodeFile)
			}
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
				storageVolume, _ := appenv.Get(fmt.Sprintf("DOCKER_VOLUME_STORAGE%s", num), envFile)
				if storageVolume != "" {
					storageFile, err := appenv.AppInstanceFile(ctx, appName, fmt.Sprintf("*.storage%s.yml", num))
					if err != nil {
						return err
					}
					if storageFile != "" && fileExists(storageFile) {
						composeFiles = append(composeFiles, storageFile)
					} else if storageFile != "" {
						logger.Info(ctx, "File '{{|File|}}%s{{[-]}}' does not exist.", storageFile)
					}
				}
			}
		}

		// Devices file
		appDevices, _ := appenv.Get(fmt.Sprintf("%s__DEVICES", appName), envFile)
		if appenv.IsTrue(appDevices) {
			devicesFile, err := appenv.AppInstanceFile(ctx, appName, "*.devices.yml")
			if err != nil {
				return err
			}
			if devicesFile != "" && fileExists(devicesFile) {
				composeFiles = append(composeFiles, devicesFile)
			} else if devicesFile != "" {
				logger.Info(ctx, "File '{{|File|}}%s{{[-]}}' does not exist.", devicesFile)
			}
		}

		// Main file (always last)
		mainFile, err := appenv.AppInstanceFile(ctx, appName, "*.yml")
		if err != nil {
			return err
		}
		if mainFile == "" || !fileExists(mainFile) {
			expectedMain := filepath.Join(instanceFolder, fmt.Sprintf("%s.yml", appNameLower))
			logger.Error(ctx, "File '{{|File|}}%s{{[-]}}' does not exist.", expectedMain)
			return fmt.Errorf("file %s does not exist", expectedMain)
		}
		composeFiles = append(composeFiles, mainFile)

		// Create config folders (mirrors yml_merge.sh logic)
		if err := appenv.CreateAppFolders(ctx, appName, conf); err != nil {
			logger.Warn(ctx, "Failed to create config folders for %s: %v", appName, err)
		}

		logger.Info(ctx, "All configurations for '{{|App|}}%s{{[-]}}' are included.", niceName)
	}

	// Use SDK to load and merge all compose files
	logger.Info(ctx, "Running: {{|RunningCommand|}}docker compose --project-directory %s/ config{{[-]}}", conf.ComposeDir)
	logger.Info(ctx, "Running compose config to create '{{|File|}}docker-compose.yml{{[-]}}' file from enabled templates.")

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

	if err := os.WriteFile(composePath, marshaledProject, 0644); err != nil {
		return fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

	logger.Info(ctx, "Merging '{{|File|}}docker-compose.yml{{[-]}}' complete.")

	// Mark YML merge as complete
	UnsetNeedsYMLMerge(ctx)

	return nil
}
