//go:build ignore

package compose

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	dockercommand "github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/cmd/display"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"
)

// TODO: Future enhancement - use Docker Compose SDK (github.com/compose-spec/compose-go/v2) instead of CLI commands
// Benefits: Better cross-platform compatibility, removes dependency on docker-compose being installed
// Note: Will need to handle YML generation and service orchestration programmatically

// MergeYML merges enabled app templates into docker-compose.yml
func MergeYML(ctx context.Context, force bool) error {
	if !NeedsYMLMerge(ctx, force) {
		logger.Notice(ctx, "Enabled app templates already merged to '{{|File|}}docker-compose.yml{{[-]}}'.")
		return nil
	}

	// Create all app environment variables first
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, force, conf); err != nil {
		return fmt.Errorf("failed to create environment variables: %w", err)
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
			// Bash: if [[ -f ${main_yml} ]]; then ... else error ...
			// Reconstruct path for logging if missing?
			// The caller expects it.
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

	// Run docker compose config to merge files
	logger.Info(ctx, "Running: {{|RunningCommand|}}docker compose --project-directory %s/ config{{[-]}}", conf.ComposeDir)
	logger.Info(ctx, "Running compose config to create '{{|File|}}docker-compose.yml{{[-]}}' file from enabled templates.")

	composePath := filepath.Join(conf.ComposeDir, "docker-compose.yml")

	// Load all compose files using the SDK
	configFiles := make([]types.ConfigFile, len(composeFiles))
	for i, f := range composeFiles {
		configFiles[i] = types.ConfigFile{Filename: f}
	}

	// Load environment variables for interpolation from .env
	// We MUST clone both keys and values to avoid memory sharing with volatile buffers
	envMap := make(map[string]string)
	if vars, err := dotenv.GetEnvFromFile(make(map[string]string), []string{envFile}); err == nil {
		for k, v := range vars {
			envMap[strings.Clone(k)] = strings.Clone(v)
		}
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

	// Marshaling the project using its own MarshalYAML method ensures standard Compose format
	marshaledProject, err := project.MarshalYAML()
	if err != nil {
		logger.Error(ctx, "Failed to marshal merged compose configuration: %v", err)
		return fmt.Errorf("failed to marshal merged compose configuration: %w", err)
	}

	// Write output to docker-compose.yml
	if err := os.WriteFile(composePath, marshaledProject, 0644); err != nil {
		return fmt.Errorf("failed to write docker-compose.yml: %w", err)
	}

	logger.Info(ctx, "Merging '{{|File|}}docker-compose.yml{{[-]}}' complete.")

	// Mark YML merge as complete
	UnsetNeedsYMLMerge(ctx)

	return nil
}

// ExecuteCompose executes Docker Compose commands
func ExecuteCompose(ctx context.Context, yes bool, force bool, operation string, appNames ...string) error {
	// First ensure YML is merged (skipping explicit merge/generate commands to avoid double-run/confusing logs)
	if operation != "merge" && operation != "generate" {
		if err := MergeYML(ctx, force); err != nil {
			return err
		}
	}

	conf := config.LoadAppConfig()

	var question, yesNotice, noNotice string
	var appNamesJoined string

	if len(appNames) > 0 {
		var niceNames []string
		for _, appName := range appNames {
			niceNames = append(niceNames, appenv.GetNiceName(ctx, appName))
		}
		appNamesJoined = strings.Join(niceNames, ", ")
	}

	// Build questions and notices (mirrors Bash and old logic)
	switch operation {
	case "down":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop and remove: %s?", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping and removing: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping and removing {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Stop and remove containers, networks, volumes, and images created by {{|ApplicationName|}}DockSTARTer{{[-]}}?"
			noNotice = "Not stopping and removing containers, networks, volumes, and images created by {{|ApplicationName|}}DockSTARTer{{[-]}}."
			yesNotice = "Stopping and removing containers, networks, volumes, and images created by {{|ApplicationName|}}DockSTARTer{{[-]}}."
		}
	case "pause":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Pause: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not pausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Pausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Pause all running containers?"
			noNotice = "Not pausing all running containers."
			yesNotice = "Pausing all running containers."
		}
	case "pull":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Pull the latest images for: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not pulling the latest images for: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Pulling the latest images for: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Pull the latest images for all enabled services?"
			noNotice = "Not pulling the latest images for all enabled services."
			yesNotice = "Pulling the latest images for all enabled services."
		}
	case "restart":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Restart: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not restarting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Restarting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Restart all stopped and running containers?"
			noNotice = "Not restarting all stopped and running containers."
			yesNotice = "Restarting all stopped and running containers."
		}
	case "stop":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Stop all running services?"
			noNotice = "Not stopping all running services."
			yesNotice = "Stopping all running services."
		}
	case "unpause":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Unpause: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not unpausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Unpausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Unpause all running containers?"
			noNotice = "Not unpausing all running containers."
			yesNotice = "Unpausing all running containers."
		}
	case "update":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Update and start: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not updating and starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Updating and starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Update and start containers for all enabled services?"
			noNotice = "Not updating and starting containers for all enabled services."
			yesNotice = "Updating and starting containers for all enabled services."
		}
	case "up":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Start: {{|App|}}%s{{[-]}}?", appNamesJoined)
			noNotice = fmt.Sprintf("Not starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			yesNotice = fmt.Sprintf("Starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Start containers for all enabled services?"
			noNotice = "Not starting containers for all enabled services."
			yesNotice = "Starting containers for all enabled services."
		}
	case "generate", "merge":
		question = "Merge enabled app templates to '{{|File|}}docker-compose.yml{{[-]}}'?"
		yesNotice = "Merging enabled app templates to '{{|File|}}docker-compose.yml{{[-]}}'."
		noNotice = "Not merging enabled app templates to '{{|File|}}docker-compose.yml{{[-]}}'."
	default:
		// Default to update
		question = "Update containers for all enabled services?"
		noNotice = "Not updating containers for all enabled services."
		yesNotice = "Updating containers for all enabled services."
	}

	// SDK Implementation
	// Load the merged docker-compose.yml as a Project
	composePath := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	// Load environment variables for interpolation
	// We MUST clone both keys and values to avoid memory sharing with volatile buffers
	envMap := make(map[string]string)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if vars, err := dotenv.GetEnvFromFile(make(map[string]string), []string{envFile}); err == nil {
		for k, v := range vars {
			envMap[strings.Clone(k)] = strings.Clone(v)
		}
	}

	configDetails := types.ConfigDetails{
		WorkingDir: conf.ComposeDir,
		ConfigFiles: []types.ConfigFile{
			{Filename: composePath},
		},
		Environment: envMap,
	}

	projectName := envMap["COMPOSE_PROJECT_NAME"]
	override := true
	if projectName == "" {
		projectName = loader.NormalizeProjectName(filepath.Base(conf.ComposeDir))
		override = false
	}

	logger.Notice(ctx, "Starting containers for all enabled services.")
	logger.Notice(ctx, "Project: {{|App|}}%s{{[-]}}", projectName)

	project, err := loader.LoadWithContext(ctx, configDetails, func(options *loader.Options) {
		options.SetProjectName(projectName, override)
		options.SkipInterpolation = false
	})
	if err != nil {
		logger.Error(ctx, "Failed to load merged compose file for execution: %v", err)
		return fmt.Errorf("failed to load compose file: %w", err)
	}

	// SDK filters services for metadata attachment (Infrastructure labels)
	// If we use loader.LoadWithContext, it doesn't automatically add standard labels.
	// We add them here to ensure the SDK can track and start containers correctly.
	for i, s := range project.Services {
		s.CustomLabels = s.CustomLabels.
			Add(api.ProjectLabel, project.Name).
			Add(api.ServiceLabel, s.Name).
			Add(api.VersionLabel, api.ComposeVersion).
			Add(api.WorkingDirLabel, project.WorkingDir).
			Add(api.ConfigFilesLabel, strings.Join(project.ComposeFiles, ",")).
			Add(api.OneoffLabel, "False")
		project.Services[i] = s
	}

	// Filter project services based on appNames if provided
	// Use WithSelectedServices to ensure dependencies are included (Parity with --include-deps)
	if len(appNames) > 0 {
		var err error
		project, err = project.WithSelectedServices(appNames, types.IncludeDependencies)
		if err != nil {
			return fmt.Errorf("failed to filter project services: %w", err)
		}
	}

	// Service names for commands that take a name list
	var serviceNames []string
	for name := range project.Services {
		serviceNames = append(serviceNames, name)
	}

	// Determine output streams and EventProcessor based on TUI vs CLI mode
	var outStream io.Writer = os.Stdout
	var errStream io.Writer = os.Stderr
	var bus api.EventProcessor

	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		outStream = w
		errStream = w
		bus = display.Plain(outStream)
	}

	// Initialize Docker CLI with the correct streams from the start to prevent leakage
	dockerCLI, err := dockercommand.NewDockerCli(
		dockercommand.WithInputStream(os.Stdin),
		dockercommand.WithOutputStream(outStream),
		dockercommand.WithErrorStream(errStream),
	)
	if err != nil {
		return fmt.Errorf("failed to create docker CLI: %w", err)
	}
	if err := dockerCLI.Initialize(&cliflags.ClientOptions{}); err != nil {
		return fmt.Errorf("failed to initialize docker CLI: %w", err)
	}

	// Choose EventProcessor if not already set by TUI
	if bus == nil {
		if dockerCLI.Out().IsTerminal() {
			bus = display.Full(outStream, errStream)
		} else {
			bus = display.Plain(outStream)
		}
	}

	srv, err := composev5.NewComposeService(dockerCLI,
		composev5.WithOutputStream(outStream),
		composev5.WithErrorStream(errStream),
		composev5.WithEventProcessor(bus),
	)
	if err != nil {
		return fmt.Errorf("failed to create compose service: %w", err)
	}

	answer, err := console.QuestionPrompt(ctx, logger.Notice, question, "Y", yes)
	if err != nil {
		return err
	}
	if answer {
		logger.Notice(ctx, yesNotice)

		// Helper to log "Running: ..." notice to match Bash RunAndLog
		logRunning := func(opArgs ...string) {
			fullCmd := []string{"docker", "compose", "--project-directory", conf.ComposeDir + "/"}
			fullCmd = append(fullCmd, opArgs...)
			fullCmd = append(fullCmd, appNames...)
			logger.Notice(ctx, "Running: {{|RunningCommand|}}%s{{[-]}}", strings.Join(fullCmd, " "))
		}

		switch operation {
		case "down":
			logRunning("down", "--remove-orphans")
			downOpts := api.DownOptions{
				RemoveOrphans: true,
			}
			if len(appNames) > 0 {
				downOpts.Services = serviceNames
			}
			err = srv.Down(ctx, project.Name, downOpts)
		case "pause":
			logRunning("pause")
			pauseOpts := api.PauseOptions{}
			if len(appNames) > 0 {
				pauseOpts.Services = serviceNames
			}
			err = srv.Pause(ctx, project.Name, pauseOpts)
		case "pull":
			logRunning("pull", "--include-deps")
			err = srv.Pull(ctx, project, api.PullOptions{})
		case "restart":
			logRunning("restart")
			restartOpts := api.RestartOptions{}
			if len(appNames) > 0 {
				restartOpts.Services = serviceNames
			}
			err = srv.Restart(ctx, project.Name, restartOpts)
		case "stop":
			logRunning("stop")
			stopOpts := api.StopOptions{}
			if len(appNames) > 0 {
				stopOpts.Services = serviceNames
			}
			err = srv.Stop(ctx, project.Name, stopOpts)
		case "unpause":
			logRunning("unpause")
			unpauseOpts := api.PauseOptions{}
			if len(appNames) > 0 {
				unpauseOpts.Services = serviceNames
			}
			err = srv.UnPause(ctx, project.Name, unpauseOpts)
		case "update":
			// Pull first
			logRunning("pull", "--include-deps")
			err = srv.Pull(ctx, project, api.PullOptions{})
			if err != nil {
				return err
			}
			// Then Up
			fallthrough // Continue to "up" case
		case "up":
			logRunning("up", "-d", "--remove-orphans")
			err = srv.Up(ctx, project, api.UpOptions{
				Create: api.CreateOptions{
					RemoveOrphans: true,
				},
				Start: api.StartOptions{
					Attach:  nil, // Do not attach to services
					Project: project,
				},
			})
		case "generate", "merge":
			return MergeYML(ctx, force)
		}
		return err
	} else {
		logger.Notice(ctx, noNotice)
	}
	return nil
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
