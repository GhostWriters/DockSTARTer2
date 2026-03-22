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

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	dockercommand "github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/cmd/display"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"
)

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

	configFiles := []types.ConfigFile{{Filename: composePath}}
	overridePath := filepath.Join(conf.ComposeDir, constants.ComposeOverrideFileName)
	if _, err := os.Stat(overridePath); err == nil {
		configFiles = append(configFiles, types.ConfigFile{Filename: overridePath})
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
