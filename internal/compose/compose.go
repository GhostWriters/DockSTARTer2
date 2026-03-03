package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	dockercommand "github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/cmd/display"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"
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

// ExecuteCompose executes Docker Compose commands
func ExecuteCompose(ctx context.Context, yes bool, force bool, command string, appNames ...string) error {
	conf := config.LoadAppConfig()

	// For merge/generate: do an upfront read-only check before prompting
	if command == "merge" || command == "generate" {
		if !force && !NeedsYMLMerge(ctx, false) {
			logger.Notice(ctx, "Enabled app templates already merged to '{{|File|}}docker-compose.yml{{[-]}}'.")
			return nil
		}
	}

	// Build nice names for display
	var appNamesJoined string
	if len(appNames) > 0 {
		var niceNames []string
		for _, appName := range appNames {
			niceNames = append(niceNames, appenv.GetNiceName(ctx, appName))
		}
		appNamesJoined = strings.Join(niceNames, ", ")
	}

	// Build question and notice strings
	var question, yesNotice, noNotice string

	switch command {
	case "down":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop and remove: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping and removing {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping and removing: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Stop and remove containers, networks, volumes, and images created by {{|ApplicationName|}}DockSTARTer{{[-]}}?"
			yesNotice = "Stopping and removing containers, networks, volumes, and images created by {{|ApplicationName|}}DockSTARTer{{[-]}}."
			noNotice = "Not stopping and removing containers, networks, volumes, and images created by {{|ApplicationName|}}DockSTARTer{{[-]}}."
		}
	case "pause":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Pause: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Pausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not pausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Pause all running containers?"
			yesNotice = "Pausing all running containers."
			noNotice = "Not pausing all running containers."
		}
	case "pull":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Pull the latest images for: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Pulling the latest images for: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not pulling the latest images for: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Pull the latest images for all enabled services?"
			yesNotice = "Pulling the latest images for all enabled services."
			noNotice = "Not pulling the latest images for all enabled services."
		}
	case "restart":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Restart: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Restarting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not restarting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Restart all stopped and running containers?"
			yesNotice = "Restarting all stopped and running containers."
			noNotice = "Not restarting all stopped and running containers."
		}
	case "stop":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Stop all running services?"
			yesNotice = "Stopping all running services."
			noNotice = "Not stopping all running services."
		}
	case "unpause":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Unpause: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Unpausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not unpausing: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Unpause all running containers?"
			yesNotice = "Unpausing all running containers."
			noNotice = "Not unpausing all running containers."
		}
	case "update":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Update and start: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Updating and starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not updating and starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Update and start containers for all enabled services?"
			yesNotice = "Updating and starting containers for all enabled services."
			noNotice = "Not updating and starting containers for all enabled services."
		}
	case "up":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Start: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Start containers for all enabled services?"
			yesNotice = "Starting containers for all enabled services."
			noNotice = "Not starting containers for all enabled services."
		}
	case "generate", "merge":
		question = "Merge enabled app templates to '{{|File|}}docker-compose.yml{{[-]}}'?"
		yesNotice = "Merging enabled app templates to '{{|File|}}docker-compose.yml{{[-]}}'."
		noNotice = "Not merging enabled app templates to '{{|File|}}docker-compose.yml{{[-]}}'."
	default:
		question = "Update and start containers for all enabled services?"
		yesNotice = "Updating and starting containers for all enabled services."
		noNotice = "Not updating and starting containers for all enabled services."
	}

	// --- PROMPT FIRST — no file modifications until user confirms ---
	answer, err := console.QuestionPrompt(ctx, logger.Notice, "Docker Compose", question, "Y", yes)
	if err != nil {
		return err
	}
	if !answer {
		logger.Notice(ctx, noNotice)
		return nil
	}

	logger.Notice(ctx, yesNotice)

	// For merge/generate: just run MergeYML and return
	if command == "merge" || command == "generate" {
		return MergeYML(ctx, force)
	}

	// For all other operations: merge first (now that user confirmed), then run SDK operation
	if command != "down" && command != "stop" && command != "pause" && command != "unpause" {
		// Operations that need an up-to-date compose file first
		if err := MergeYML(ctx, force); err != nil {
			return err
		}
	}

	// Load the merged docker-compose.yml as a Project
	composePath := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)

	envMap := make(map[string]string)
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

	project, err := loader.LoadWithContext(ctx, configDetails, func(options *loader.Options) {
		options.SetProjectName(projectName, override)
		options.SkipInterpolation = false
	})
	if err != nil {
		logger.Error(ctx, "Failed to load merged compose file for execution: %v", err)
		return fmt.Errorf("failed to load compose file: %w", err)
	}

	// Add infrastructure labels required by the compose SDK
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

	// Filter to requested services if specified
	if len(appNames) > 0 {
		project, err = project.WithSelectedServices(appNames, types.IncludeDependencies)
		if err != nil {
			return fmt.Errorf("failed to filter project services: %w", err)
		}
	}

	var serviceNames []string
	for name := range project.Services {
		serviceNames = append(serviceNames, name)
	}

	// Set up output streams
	var outStream io.Writer = os.Stdout
	var errStream io.Writer = os.Stderr
	var bus api.EventProcessor

	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		outStream = w
		errStream = w
		bus = display.Plain(outStream)
	}

	// Initialize Docker CLI
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

	if bus == nil {
		if dockerCLI.Out().IsTerminal() {
			bus = display.Full(outStream, errStream, true)
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

	// Log the equivalent CLI command for transparency
	logRunning := func(opArgs ...string) {
		fullCmd := []string{"docker", "compose", "--project-directory", conf.ComposeDir + "/"}
		fullCmd = append(fullCmd, opArgs...)
		fullCmd = append(fullCmd, appNames...)
		logger.Notice(ctx, "Running: {{|RunningCommand|}}%s{{[-]}}", strings.Join(fullCmd, " "))
	}

	// Execute the operation via SDK
	switch command {
	case "down":
		logRunning("down", "--remove-orphans")
		downOpts := api.DownOptions{RemoveOrphans: true}
		if len(appNames) > 0 {
			downOpts.Services = serviceNames
		}
		return srv.Down(ctx, project.Name, downOpts)
	case "pause":
		logRunning("pause")
		pauseOpts := api.PauseOptions{}
		if len(appNames) > 0 {
			pauseOpts.Services = serviceNames
		}
		return srv.Pause(ctx, project.Name, pauseOpts)
	case "pull":
		logRunning("pull", "--include-deps")
		return srv.Pull(ctx, project, api.PullOptions{})
	case "restart":
		logRunning("restart")
		restartOpts := api.RestartOptions{}
		if len(appNames) > 0 {
			restartOpts.Services = serviceNames
		}
		return srv.Restart(ctx, project.Name, restartOpts)
	case "stop":
		logRunning("stop")
		stopOpts := api.StopOptions{}
		if len(appNames) > 0 {
			stopOpts.Services = serviceNames
		}
		return srv.Stop(ctx, project.Name, stopOpts)
	case "unpause":
		logRunning("unpause")
		unpauseOpts := api.PauseOptions{}
		if len(appNames) > 0 {
			unpauseOpts.Services = serviceNames
		}
		return srv.UnPause(ctx, project.Name, unpauseOpts)
	case "update":
		// Pull first, then up
		logRunning("pull", "--include-deps")
		if err := srv.Pull(ctx, project, api.PullOptions{}); err != nil {
			return err
		}
		fallthrough
	default: // "up" and unknown commands
		logRunning("up", "-d", "--remove-orphans")
		return srv.Up(ctx, project, api.UpOptions{
			Create: api.CreateOptions{
				RemoveOrphans: true,
			},
			Start: api.StartOptions{
				Attach:  nil,
				Project: project,
			},
		})
	}
}

// NeedsYMLMerge checks if YML merge is needed using timestamp comparison
func NeedsYMLMerge(ctx context.Context, force bool) bool {
	if force {
		return true
	}

	conf := config.LoadAppConfig()

	// Check main files
	dockerCompose := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	if fileChanged(conf, dockerCompose) {
		return true
	}

	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if fileChanged(conf, envFile) {
		return true
	}

	// Check enabled apps .env files
	enabledApps, _ := appenv.ListEnabledApps(conf)
	for _, appName := range enabledApps {
		appEnvFile := filepath.Join(conf.ComposeDir, fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, strings.ToLower(appName)))
		if fileChanged(conf, appEnvFile) {
			return true
		}
	}

	return false
}

// UnsetNeedsYMLMerge marks YML merge as complete by clearing all yml_merge_* files
// and updating timestamps for current state.
func UnsetNeedsYMLMerge(ctx context.Context) {
	conf := config.LoadAppConfig()
	timestampsFolder := filepath.Join(paths.GetTimestampsDir(), "yml_merge")

	// Clear existing yml_merge markers
	_ = os.RemoveAll(timestampsFolder)
	_ = os.MkdirAll(timestampsFolder, 0755)

	dockerCompose := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
	updateTimestamp(ctx, conf, dockerCompose)

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
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return true
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "yml_merge", filename)

	info, err := os.Stat(path)
	tsInfo, tsErr := os.Stat(timestampFile)

	if os.IsNotExist(tsErr) {
		return true
	}

	if err != nil {
		return false
	}

	if !info.ModTime().Equal(tsInfo.ModTime()) {
		if appenv.CompareFiles(path, timestampFile) {
			_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
			return false
		}
		return true
	}

	return false
}

func updateTimestamp(ctx context.Context, conf config.AppConfig, path string) {
	if !fileExists(path) {
		return
	}

	filename := filepath.Base(path)
	timestampFile := filepath.Join(paths.GetTimestampsDir(), "yml_merge", filename)

	_ = os.MkdirAll(filepath.Dir(timestampFile), 0755)

	_ = appenv.CopyFile(path, timestampFile)

	info, err := os.Stat(path)
	if err == nil {
		_ = os.Chtimes(timestampFile, info.ModTime(), info.ModTime())
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
