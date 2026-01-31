package compose

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	execpkg "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TODO: Future enhancement - use Docker Compose SDK (github.com/compose-spec/compose-go/v2) instead of CLI commands
// Benefits: Better cross-platform compatibility, removes dependency on docker-compose being installed
// Note: Will need to handle YML generation and service orchestration programmatically

// MergeYML merges enabled app templates into docker-compose.yml
func MergeYML(ctx context.Context) error {
	if !NeedsYMLMerge(ctx) {
		logger.Notice(ctx, "Enabled app templates already merged to '{{_File_}}docker-compose.yml{{|-|}}'.")
		return nil
	}

	// Create all app environment variables first
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, conf); err != nil {
		return fmt.Errorf("failed to create environment variables: %w", err)
	}

	logger.Notice(ctx, "Adding enabled app templates to merge '{{_File_}}docker-compose.yml{{|-|}}'.  Please be patient, this can take a while.")

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
			logger.Error(ctx, "Folder '{{_Folder_}}%s/{{|-|}}' does not exist.", instanceFolder)
			return fmt.Errorf("folder %s does not exist", instanceFolder)
		}

		// Check if app is deprecated
		if appenv.IsAppDeprecated(ctx, appName) {
			logger.Warn(ctx,
				"'{{_App_}}%s{{|-|}}' IS DEPRECATED!",
				niceName)
			logger.Warn(ctx,
				"Please run '{{_UserCommand_}}ds2 --status-disable %s{{|-|}}' to disable it.",
				niceName)
		}

		// Get architecture-specific file
		archFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.%s.yml", appNameLower, arch))
		if !fileExists(archFile) {
			logger.Error(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", archFile)
			return fmt.Errorf("file %s does not exist", archFile)
		}
		composeFiles = append(composeFiles, archFile)

		// Network mode specific files
		netMode, _ := appenv.Get(fmt.Sprintf("%s__NETWORK_MODE", appName), envFile)
		if netMode == "" || netMode == "bridge" {
			// Add hostname file if exists
			hostnameFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.hostname.yml", appNameLower))
			if fileExists(hostnameFile) {
				composeFiles = append(composeFiles, hostnameFile)
			} else {
				logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", hostnameFile)
			}

			// Add ports file if exists
			portsFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.ports.yml", appNameLower))
			if fileExists(portsFile) {
				composeFiles = append(composeFiles, portsFile)
			} else {
				logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", portsFile)
			}
		} else if netMode != "" {
			// Add netmode file if exists
			netmodeFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.netmode.yml", appNameLower))
			if fileExists(netmodeFile) {
				composeFiles = append(composeFiles, netmodeFile)
			} else {
				logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", netmodeFile)
			}
		}

		// Storage files
		multipleStorage, _ := appenv.Get("DOCKER_MULTIPLE_STORAGE", envFile)
		storageNumbers := []string{""}
		if multipleStorage == "true" {
			storageNumbers = append(storageNumbers, "2", "3", "4")
		}

		for _, num := range storageNumbers {
			storageOn, _ := appenv.Get(fmt.Sprintf("%s__STORAGE%s_ON", appName, num), envFile)
			if storageOn == "" {
				storageOn, _ = appenv.Get(fmt.Sprintf("DOCKER_STORAGE%s_ON", num), envFile)
			}
			if storageOn == "true" {
				storageVolume, _ := appenv.Get(fmt.Sprintf("DOCKER_VOLUME_STORAGE%s", num), envFile)
				if storageVolume != "" {
					storageFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.storage%s.yml", appNameLower, num))
					if fileExists(storageFile) {
						composeFiles = append(composeFiles, storageFile)
					} else {
						logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", storageFile)
					}
				}
			}
		}

		// Devices file
		appDevices, _ := appenv.Get(fmt.Sprintf("%s__DEVICES", appName), envFile)
		if appDevices == "true" {
			devicesFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.devices.yml", appNameLower))
			if fileExists(devicesFile) {
				composeFiles = append(composeFiles, devicesFile)
			} else {
				logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", devicesFile)
			}
		}

		// Main file (always last)
		mainFile := filepath.Join(instanceFolder, fmt.Sprintf("%s.yml", appNameLower))
		if !fileExists(mainFile) {
			logger.Error(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", mainFile)
			return fmt.Errorf("file %s does not exist", mainFile)
		}
		composeFiles = append(composeFiles, mainFile)

		logger.Info(ctx, "All configurations for '{{_App_}}%s{{|-|}}' are included.", niceName)

		// Create app folders
		if err := appenv.CreateApp(ctx, appName, conf); err != nil {
			logger.Warn(ctx, "Failed to create folders for %s: %v", appName, err)
		}
	}

	// Run docker compose config to merge files
	logger.Info(ctx, "Running compose config to create '{{_File_}}docker-compose.yml{{|-|}}' file from enabled templates.")

	composePath := filepath.Join(conf.ComposeDir, "docker-compose.yml")

	// Set COMPOSE_FILE environment variable
	os.Setenv("COMPOSE_FILE", strings.Join(composeFiles, string(os.PathListSeparator)))

	cmd := exec.CommandContext(ctx, "docker", "compose", "--project-directory", conf.ComposeDir+"/", "config")
	output, err := cmd.Output()
	if err != nil {
		logger.Error(ctx, "Failed to output compose config.")
		logger.Error(ctx, "Failing command: {{_FailingCommand_}}docker compose --project-directory %s/ config > \"%s\"{{|-|}}", conf.ComposeDir, composePath)
		if exitErr, ok := err.(*exec.ExitError); ok {
			logger.Error(ctx, "Error output: %s", string(exitErr.Stderr))
		}
		return fmt.Errorf("failed to run docker compose config: %w", err)
	}

	// Write output to docker-compose.yml
	if err := os.WriteFile(composePath, output, 0644); err != nil {
		return fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

	logger.Info(ctx, "Merging '{{_File_}}docker-compose.yml{{|-|}}' complete.")

	// Mark YML merge as complete
	UnsetNeedsYMLMerge(ctx)

	return nil
}

// ExecuteCompose executes Docker Compose commands
func ExecuteCompose(ctx context.Context, command string, appNames ...string) error {
	// First ensure YML is merged
	if err := MergeYML(ctx); err != nil {
		return err
	}

	conf := config.LoadAppConfig()

	// Build compose command arguments
	var args []string
	var appNamesJoined string

	if len(appNames) > 0 {
		var niceNames []string
		for _, appName := range appNames {
			niceNames = append(niceNames, appenv.GetNiceName(ctx, appName))
		}
		appNamesJoined = strings.Join(niceNames, ", ")
	}

	// Build command based on operation
	switch command {
	case "down":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Stopping and removing {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Stopping and removing containers, networks, volumes, and images created by {{_ApplicationName_}}DockSTARTer{{|-|}}.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "down", "--remove-orphans"}
		args = append(args, appNames...)

	case "pause":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Pausing: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Pausing all running containers.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "pause"}
		args = append(args, appNames...)

	case "pull":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Pulling the latest images for: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Pulling the latest images for all enabled services.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "pull", "--include-deps"}
		args = append(args, appNames...)

	case "restart":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Restarting: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Restarting all stopped and running containers.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "restart"}
		args = append(args, appNames...)

	case "stop":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Stopping: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Stopping all running services.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "stop"}
		args = append(args, appNames...)

	case "unpause":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Unpausing: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Unpausing all running containers.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "unpause"}
		args = append(args, appNames...)

	case "update":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Updating and starting: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Updating and starting containers for all enabled services.")
		}
		// Update = pull + up
		pullArgs := []string{"compose", "--project-directory", conf.ComposeDir + "/", "pull", "--include-deps"}
		pullArgs = append(pullArgs, appNames...)
		if err := runDockerCommand(ctx, pullArgs...); err != nil {
			return err
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "up", "-d", "--remove-orphans"}
		args = append(args, appNames...)

	case "up":
		if appNamesJoined != "" {
			logger.Notice(ctx, "Starting: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			logger.Notice(ctx, "Starting containers for all enabled services.")
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "up", "-d", "--remove-orphans"}
		args = append(args, appNames...)

	case "generate", "merge":
		// Already merged above
		return nil

	default:
		// Default to update
		logger.Notice(ctx, "Updating containers for all enabled services.")
		pullArgs := []string{"compose", "--project-directory", conf.ComposeDir + "/", "pull", "--include-deps"}
		if err := runDockerCommand(ctx, pullArgs...); err != nil {
			return err
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "up", "-d", "--remove-orphans"}
	}

	return runDockerCommand(ctx, args...)
}

// runDockerCommand runs a docker command using RunAndLog pattern
// Bash: RunAndLog notice "" error "Failed to run compose." "${Command[@]}"
func runDockerCommand(ctx context.Context, args ...string) error {
	return execpkg.RunAndLog(ctx,
		"notice",                 // runningNoticeType
		"",                       // outputNoticeType (no prefix, let docker compose show its output directly)
		"error",                  // errorNoticeType
		"Failed to run compose.", // errorMessage
		"docker",                 // command
		args...,                  // docker args
	)
}

// NeedsYMLMerge checks if YML merge is needed
func NeedsYMLMerge(ctx context.Context) bool {
	// Check if the needs file exists
	conf := config.LoadAppConfig()
	needsFile := filepath.Join(conf.ConfigDir, "needs", "yml_merge")
	return fileExists(needsFile)
}

// UnsetNeedsYMLMerge marks YML merge as complete
func UnsetNeedsYMLMerge(ctx context.Context) {
	conf := config.LoadAppConfig()
	needsFile := filepath.Join(conf.ConfigDir, "needs", "yml_merge")
	os.Remove(needsFile)
}

// SetNeedsYMLMerge marks that YML merge is needed
func SetNeedsYMLMerge(ctx context.Context) error {
	conf := config.LoadAppConfig()
	needsDir := filepath.Join(conf.ConfigDir, "needs")
	if err := os.MkdirAll(needsDir, 0755); err != nil {
		return err
	}
	needsFile := filepath.Join(needsDir, "yml_merge")
	return os.WriteFile(needsFile, []byte(""), 0644)
}

// Helper functions
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
