package compose

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
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
func MergeYML(ctx context.Context, force bool) error {
	if !NeedsYMLMerge(ctx, force) {
		logger.Notice(ctx, "Enabled app templates already merged to '{{_File_}}docker-compose.yml{{|-|}}'.")
		return nil
	}

	// Create all app environment variables first
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, force, conf); err != nil {
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
		archFile, err := appenv.AppInstanceFile(ctx, appName, fmt.Sprintf("*.%s.yml", arch))
		if err != nil {
			return fmt.Errorf("failed to get arch file for %s: %w", appName, err)
		}
		if archFile == "" || !fileExists(archFile) {
			logger.Error(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", archFile)
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
			} else {
				// Log path even if empty/missing to match legacy behavior roughly,
				// though legacy logs the *expected* path.
				// Here we just log if file logic returned something usable but missing, or standardizing logging.
				// Bash logs "does not exist" if missing.
				// We can reconstruct expected path for logging if needed, or just skip logging if no template.
				// Bash: checks app_instance_file return. If it returned valid path, and file missing -> log.
				// With update, AppInstanceFile returns path only if exists (or created).
				// So if "" returned, silence is correct (no template).
				// If path returned, it exists.
				// But let's log "does not exist" if we interpret parity strictly?
				// Bash: checks if file exists.
				// If AppInstanceFile returns "", we can't log the "path".
				// So we'll skip logging "does not exist" if template is missing, which is cleaner.
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
					storageFile, err := appenv.AppInstanceFile(ctx, appName, fmt.Sprintf("*.storage%s.yml", num))
					if err != nil {
						return err
					}
					if storageFile != "" && fileExists(storageFile) {
						composeFiles = append(composeFiles, storageFile)
					} else if storageFile != "" {
						logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", storageFile)
					}
				}
			}
		}

		// Devices file
		appDevices, _ := appenv.Get(fmt.Sprintf("%s__DEVICES", appName), envFile)
		if appDevices == "true" {
			devicesFile, err := appenv.AppInstanceFile(ctx, appName, "*.devices.yml")
			if err != nil {
				return err
			}
			if devicesFile != "" && fileExists(devicesFile) {
				composeFiles = append(composeFiles, devicesFile)
			} else if devicesFile != "" {
				logger.Info(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", devicesFile)
			}
		}

		// Main file (always last)
		mainFile, err := appenv.AppInstanceFile(ctx, appName, "*.yml")
		if err != nil {
			return err
		}
		if mainFile == "" || !fileExists(mainFile) {
			// Bash: if [[ -f ${main_yml} ]]; then ... else error ...
			// Reconstruct path for logging if missing?
			// The caller expects it.
			expectedMain := filepath.Join(instanceFolder, fmt.Sprintf("%s.yml", appNameLower))
			logger.Error(ctx, "File '{{_File_}}%s{{|-|}}' does not exist.", expectedMain)
			return fmt.Errorf("file %s does not exist", expectedMain)
		}
		composeFiles = append(composeFiles, mainFile)

		// Create config folders (mirrors yml_merge.sh logic)
		if err := appenv.CreateAppFolders(ctx, appName, conf); err != nil {
			logger.Warn(ctx, "Failed to create config folders for %s: %v", appName, err)
		}

		logger.Info(ctx, "All configurations for '{{_App_}}%s{{|-|}}' are included.", niceName)
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
func ExecuteCompose(ctx context.Context, yes bool, force bool, command string, appNames ...string) error {
	// First ensure YML is merged (skipping explicit merge/generate commands to avoid double-run/confusing logs)
	if command != "merge" && command != "generate" {
		if err := MergeYML(ctx, force); err != nil {
			return err
		}
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

	var question, yesNotice, noNotice string

	// Build command based on operation
	switch command {
	case "down":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop and remove: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping and removing {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping and removing: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Stop and remove containers, networks, volumes, and images created by {{_ApplicationName_}}DockSTARTer{{|-|}}?"
			yesNotice = "Stopping and removing containers, networks, volumes, and images created by {{_ApplicationName_}}DockSTARTer{{|-|}}."
			noNotice = "Not stopping and removing containers, networks, volumes, and images created by {{_ApplicationName_}}DockSTARTer{{|-|}}."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "down", "--remove-orphans"}
		args = append(args, appNames...)

	case "pause":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Pause: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Pausing: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not pausing: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Pause all running containers?"
			yesNotice = "Pausing all running containers."
			noNotice = "Not pausing all running containers."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "pause"}
		args = append(args, appNames...)

	case "pull":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Pull the latest images for: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Pulling the latest images for: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not pulling the latest images for: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Pull the latest images for all enabled services?"
			yesNotice = "Pulling the latest images for all enabled services."
			noNotice = "Not pulling the latest images for all enabled services."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "pull", "--include-deps"}
		args = append(args, appNames...)

	case "restart":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Restart: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Restarting: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not restarting: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Restart all stopped and running containers?"
			yesNotice = "Restarting all stopped and running containers."
			noNotice = "Not restarting all stopped and running containers."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "restart"}
		args = append(args, appNames...)

	case "stop":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Stop all running services?"
			yesNotice = "Stopping all running services."
			noNotice = "Not stopping all running services."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "stop"}
		args = append(args, appNames...)

	case "unpause":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Unpause: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Unpausing: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not unpausing: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Unpause all running containers?"
			yesNotice = "Unpausing all running containers."
			noNotice = "Not unpausing all running containers."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "unpause"}
		args = append(args, appNames...)

	case "update":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Update and start: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Updating and starting: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not updating and starting: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Update and start containers for all enabled services?"
			yesNotice = "Updating and starting containers for all enabled services."
			noNotice = "Not updating and starting containers for all enabled services."
		}
		// Update = pull + up - requires separate execution
		if console.QuestionPrompt(ctx, logger.Notice, question, "Y", yes) {
			logger.Notice(ctx, yesNotice)
			pullArgs := []string{"compose", "--project-directory", conf.ComposeDir + "/", "pull", "--include-deps"}
			pullArgs = append(pullArgs, appNames...)
			if err := runDockerCommand(ctx, pullArgs...); err != nil {
				return err
			}
			args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "up", "-d", "--remove-orphans"}
			args = append(args, appNames...)
			return runDockerCommand(ctx, args...)
		} else {
			logger.Notice(ctx, noNotice)
			return nil
		}

	case "up":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Start: {{_App_}}%s{{|-|}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Starting: {{_App_}}%s{{|-|}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not starting: {{_App_}}%s{{|-|}}.", appNamesJoined)
		} else {
			question = "Start containers for all enabled services?"
			yesNotice = "Starting containers for all enabled services."
			noNotice = "Not starting containers for all enabled services."
		}
		args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "up", "-d", "--remove-orphans"}
		args = append(args, appNames...)

	case "generate", "merge":
		question = "Merge enabled app templates to '{{_File_}}docker-compose.yml{{|-|}}'?"
		yesNotice = "Merging enabled app templates to '{{_File_}}docker-compose.yml{{|-|}}'."
		noNotice = "Not merging enabled app templates to '{{_File_}}docker-compose.yml{{|-|}}'."
		// Already merged above, just printing logic here if forced
		if console.QuestionPrompt(ctx, logger.Notice, question, "Y", yes) {
			logger.Notice(ctx, yesNotice)
			return MergeYML(ctx, force) // Re-run merge if requested explicitly
		} else {
			logger.Notice(ctx, noNotice)
			return nil
		}

	default:
		// Default to update
		question = "Update containers for all enabled services?"
		yesNotice = "Updating containers for all enabled services."
		noNotice = "Not updating containers for all enabled services."

		if console.QuestionPrompt(ctx, logger.Notice, question, "Y", yes) {
			logger.Notice(ctx, yesNotice)
			pullArgs := []string{"compose", "--project-directory", conf.ComposeDir + "/", "pull", "--include-deps"}
			if err := runDockerCommand(ctx, pullArgs...); err != nil {
				return err
			}
			args = []string{"compose", "--project-directory", conf.ComposeDir + "/", "up", "-d", "--remove-orphans"}
			return runDockerCommand(ctx, args...)
		} else {
			logger.Notice(ctx, noNotice)
			return nil
		}
	}

	// General Execution for non-custom paths
	if console.QuestionPrompt(ctx, logger.Notice, question, "Y", yes) {
		logger.Notice(ctx, yesNotice)
		return runDockerCommand(ctx, args...)
	} else {
		logger.Notice(ctx, noNotice)
		return nil
	}
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

// NeedsYMLMerge checks if YML merge is needed using timestamp comparison
func NeedsYMLMerge(ctx context.Context, force bool) bool {
	if force {
		return true
	}

	conf := config.LoadAppConfig()

	// 1. Check if docker-compose.yml is missing
	composeFile := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	if !fileExists(composeFile) {
		return true
	}

	// 3. Timestamp-based checks (Bash parity)
	// Check compose file itself
	if fileChanged(conf, composeFile) {
		return true
	}

	// Check .env
	envFile := filepath.Join(conf.ComposeDir, ".env")
	if fileChanged(conf, envFile) {
		return true
	}

	// Check enabled apps .env files
	enabledApps, _ := appenv.ListEnabledApps(conf)

	for _, appName := range enabledApps {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf(".env.app.%s", strings.ToLower(appName)))
		if fileChanged(conf, appEnvFile) {
			return true
		}
	}

	return false
}

// Mark YML merge as complete by clearing all yml_merge_* files
// and updating timestamps for current state.
func UnsetNeedsYMLMerge(ctx context.Context) {
	conf := config.LoadAppConfig()

	// Clear old yml_merge markers to ensure a clean state
	timestampsDir := paths.GetTimestampsDir()
	if entries, err := os.ReadDir(timestampsDir); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), constants.YmlMergeMarkerPrefix) {
				_ = os.Remove(filepath.Join(timestampsDir, entry.Name()))
			}
		}
	}

	// Update markers for all relevant files
	composeFile := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	updateTimestamp(ctx, conf, composeFile)

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	updateTimestamp(ctx, conf, envFile)

	enabledApps, _ := appenv.ListEnabledApps(conf)
	for _, appName := range enabledApps {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		updateTimestamp(ctx, conf, appEnvFile)
	}
}

// Helper functions

func fileChanged(conf config.AppConfig, path string) bool {
	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), constants.YmlMergeMarkerPrefix+filename)

	info, err := os.Stat(path)
	tsInfo, tsErr := os.Stat(timestampFile)

	// If neither file nor marker exists, consider it unchanged (optional file case)
	if os.IsNotExist(err) && os.IsNotExist(tsErr) {
		return false
	}

	// If one exists and other doesn't -> Changed
	if (err == nil && tsErr != nil) || (err != nil && tsErr == nil) {
		return true
	}

	// Both exist, compare modification times
	return !info.ModTime().Equal(tsInfo.ModTime())
}

func updateTimestamp(ctx context.Context, conf config.AppConfig, path string) {
	if !fileExists(path) {
		return
	}

	filename := filepath.Base(path)
	timestampDir := paths.GetTimestampsDir()
	timestampFile := filepath.Join(timestampDir, constants.YmlMergeMarkerPrefix+filename)

	// Ensure timestamps directory exists
	_ = os.MkdirAll(timestampDir, 0755)

	// Create empty timestamp file
	f, err := os.Create(timestampFile)
	if err != nil {
		return
	}
	f.Close()

	// Sync timestamp with source file
	info, err := os.Stat(path)
	if err != nil {
		return
	}

	_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
