package compose

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/dockercheck"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/version"

	"github.com/compose-spec/compose-go/v2/dotenv"
	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	dockercommand "github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	composev5 "github.com/docker/compose/v5/pkg/compose"
	mobyclient "github.com/moby/moby/client"
)

// setPullPolicy sets the pull policy on all services in the project.
func setPullPolicy(project *types.Project, policy string) {
	for name, svc := range project.Services {
		svc.PullPolicy = policy
		project.Services[name] = svc
	}
}

// YesNotice returns the human-readable notice string for a compose action,
// given the sub-command and an already-joined, display-ready app name string.
// Matches the yesNotice values used inside ExecuteCompose.
func YesNotice(command, appNamesJoined string) string {
	switch command {
	case "create":
		if appNamesJoined != "" {
			return fmt.Sprintf("Creating containers for: %s.", appNamesJoined)
		}
		return "Creating containers for all enabled services."
	case "rm":
		if appNamesJoined != "" {
			return fmt.Sprintf("Removing stopped containers for: %s.", appNamesJoined)
		}
		return "Removing stopped containers."
	case "down":
		if appNamesJoined != "" {
			return fmt.Sprintf("Stopping and removing %s.", appNamesJoined)
		}
		return "Stopping and removing containers, networks, volumes, and images created by " + version.ApplicationName + "."
	case "kill":
		if appNamesJoined != "" {
			return fmt.Sprintf("Force stopping: %s.", appNamesJoined)
		}
		return "Force stopping all running containers."
	case "pause":
		if appNamesJoined != "" {
			return fmt.Sprintf("Pausing: %s.", appNamesJoined)
		}
		return "Pausing all running containers."
	case "pull":
		if appNamesJoined != "" {
			return fmt.Sprintf("Pulling the latest images for: %s.", appNamesJoined)
		}
		return "Pulling the latest images for all enabled services."
	case "restart":
		if appNamesJoined != "" {
			return fmt.Sprintf("Restarting: %s.", appNamesJoined)
		}
		return "Restarting all stopped and running containers."
	case "stop":
		if appNamesJoined != "" {
			return fmt.Sprintf("Stopping: %s.", appNamesJoined)
		}
		return "Stopping all running services."
	case "start":
		if appNamesJoined != "" {
			return fmt.Sprintf("Starting: %s.", appNamesJoined)
		}
		return "Starting all stopped containers."
	case "unpause":
		if appNamesJoined != "" {
			return fmt.Sprintf("Unpausing: %s.", appNamesJoined)
		}
		return "Unpausing all running containers."
	case "up":
		if appNamesJoined != "" {
			return fmt.Sprintf("Starting: %s.", appNamesJoined)
		}
		return "Starting containers for all enabled services."
	case "generate", "merge":
		return "Merging enabled app templates to 'docker-compose.yml'."
	default: // update
		if appNamesJoined != "" {
			return fmt.Sprintf("Updating and starting: %s.", appNamesJoined)
		}
		return "Updating and starting containers for all enabled services."
	}
}

// ExecuteCompose executes Docker Compose commands
func ExecuteCompose(ctx context.Context, yes bool, force bool, command string, appNames ...string) error {
	conf := config.LoadAppConfig()

	// For merge/generate: do an upfront read-only check before prompting
	if command == "merge" || command == "generate" {
		if !force && !NeedsYMLMerge(ctx, false) {
			composeFile := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
			logger.Notice(ctx, "Enabled app templates already merged to '"+console.FormatFilePath(composeFile)+"'.")
			return nil
		}
	} else {
		// Every other subcommand talks to the daemon -- fail clearly up
		// front (before any prompt) if it's missing or too old, instead of
		// a low-level SDK error partway through. merge/generate are pure
		// file generation and must keep working without Docker.
		if err := dockercheck.Require(ctx); err != nil {
			return err
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
	case "create":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Create containers for: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Creating containers for: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not creating containers for: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Create containers for all enabled services?"
			yesNotice = "Creating containers for all enabled services."
			noNotice = "Not creating containers for all enabled services."
		}
	case "rm":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Remove stopped containers for: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Removing stopped containers for: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not removing stopped containers for: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Remove stopped containers?"
			yesNotice = "Removing stopped containers."
			noNotice = "Not removing stopped containers."
		}
	case "kill":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Force stop: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Force stopping: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not force stopping: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Force stop all running containers?"
			yesNotice = "Force stopping all running containers."
			noNotice = "Not force stopping all running containers."
		}
	case "start":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Start: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not starting: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Start all stopped containers?"
			yesNotice = "Starting all stopped containers."
			noNotice = "Not starting all stopped containers."
		}
	case "down":
		if appNamesJoined != "" {
			question = fmt.Sprintf("Stop and remove: {{|App|}}%s{{[-]}}?", appNamesJoined)
			yesNotice = fmt.Sprintf("Stopping and removing {{|App|}}%s{{[-]}}.", appNamesJoined)
			noNotice = fmt.Sprintf("Not stopping and removing: {{|App|}}%s{{[-]}}.", appNamesJoined)
		} else {
			question = "Stop and remove containers, networks, volumes, and images created by {{|ApplicationName|}}" + version.ApplicationName + "{{[-]}}?"
			yesNotice = "Stopping and removing containers, networks, volumes, and images created by {{|ApplicationName|}}" + version.ApplicationName + "{{[-]}}."
			noNotice = "Not stopping and removing containers, networks, volumes, and images created by {{|ApplicationName|}}" + version.ApplicationName + "{{[-]}}."
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
		composeFile := filepath.Join(conf.ComposeDir, constants.ComposeFileName)
		question = "Merge enabled app templates to '" + console.FormatFilePath(composeFile) + "'?"
		yesNotice = "Merging enabled app templates to '" + console.FormatFilePath(composeFile) + "'."
		noNotice = "Not merging enabled app templates to '" + console.FormatFilePath(composeFile) + "'."
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
	if command != "down" && command != "stop" && command != "start" && command != "kill" &&
		command != "pause" && command != "unpause" && command != "rm" {
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

	// outStream is where the themed processor writes its final output (real stdout, or the
	// TUI writer in GUI mode). The processor owns all output, so the SDK streams below are
	// always discarded and there is no separate error stream to manage.
	var outStream io.Writer = os.Stdout
	var bus api.EventProcessor

	// Detect TUI writer (GUI program box or console panel). The live CLI viewport draws on
	// stderr and dumps its final content to stdout, so it's only usable when BOTH stdout and
	// stderr are terminals. If either is redirected (and there's no TUI), run in static mode:
	// render silently and write the final output once to stdout.
	tuiWriter, hasTUIWriter := ctx.Value(console.TUIWriterKey).(io.Writer)
	staticOut := !hasTUIWriter && (!console.IsStdoutTTY() || !console.IsTTY())

	if hasTUIWriter {
		outStream = tuiWriter
	}

	// The themed event processor owns all output in every mode (TTY, TUI, and redirected),
	// so the SDK's own CLI streams are always discarded.
	cliOut := io.Discard
	cliErr := io.Discard

	// Initialize Docker CLI
	dockerCLI, err := dockercommand.NewDockerCli(
		dockercommand.WithInputStream(os.Stdin),
		dockercommand.WithOutputStream(cliOut),
		dockercommand.WithErrorStream(cliErr),
	)
	if err != nil {
		return fmt.Errorf("failed to create docker CLI: %w", err)
	}
	if err := dockerCLI.Initialize(&cliflags.ClientOptions{}); err != nil {
		return fmt.Errorf("failed to initialize docker CLI: %w", err)
	}

	{
		imageServices := make(map[string][]string)
		containerToService := make(map[string]string)
		for name, svc := range project.Services {
			if svc.Image != "" {
				imageServices[svc.Image] = append(imageServices[svc.Image], name)
			}
			// Map container_name back to service name so SDK events normalize correctly.
			if svc.ContainerName != "" && svc.ContainerName != name {
				containerToService[svc.ContainerName] = name
			}
		}
		// Also query running containers to map their actual names (which may include
		// random suffixes like "<project>-<service>-<random>") to service names via label.
		f := mobyclient.Filters{}
		f.Add("label", api.ProjectLabel+"="+project.Name)
		if result, err := dockerCLI.Client().ContainerList(ctx, mobyclient.ContainerListOptions{
			All:     true,
			Filters: f,
		}); err == nil {
			for _, ctr := range result.Items {
				svcName := ctr.Labels[api.ServiceLabel]
				if svcName == "" {
					continue
				}
				for _, rawName := range ctr.Names {
					cname := strings.TrimPrefix(rawName, "/")
					if cname != svcName {
						containerToService[cname] = svcName
					}
				}
			}
		}
		imageOrder := make([]string, 0, len(imageServices))
		for img, svcs := range imageServices {
			sort.Strings(svcs)
			imageServices[img] = svcs
			imageOrder = append(imageOrder, img)
		}
		sort.Slice(imageOrder, func(i, j int) bool {
			return imageBaseName(imageOrder[i]) < imageBaseName(imageOrder[j])
		})
		var updateFn func([]string)
		if hasTUIWriter {
			updateFn = console.ReplaceOutputFuncFromContext(ctx)
			if updateFn == nil {
				updateFn = console.ReplaceOutputLinesFn
			}
		}
		bus = NewConsoleEventProcessor(ctx, outStream, command, imageServices, imageOrder, containerToService, project.Name, !conf.UI.LineCharacters, console.GlobalVerbose, staticOut, updateFn, dockerCLI.Client())
	}

	srv, err := composev5.NewComposeService(dockerCLI,
		composev5.WithOutputStream(io.Discard),
		composev5.WithErrorStream(io.Discard),
		composev5.WithEventProcessor(bus),
	)
	if err != nil {
		return fmt.Errorf("failed to create compose service: %w", err)
	}

	logRunning := func(opArgs ...string) {
		fullCmd := []string{"docker", "compose", "--project-directory", conf.ComposeDir + "/"}
		fullCmd = append(fullCmd, opArgs...)
		fullCmd = append(fullCmd, appNames...)
		logger.Notice(ctx, "Running: {{|RunningCommand|}}%s{{[-]}}", strings.Join(fullCmd, " "))
	}

	// Suppress logger output during the SDK operation so log lines don't
	// interleave with the compose display. logRunning above uses ctx (unsuppressed)
	// so the "Running:" line still appears before the display takes over.
	sdkCtx := ctx
	if w, ok := ctx.Value(console.TUIWriterKey).(io.Writer); ok {
		sdkCtx = logger.WithSuppressWriter(ctx, w)
	}

	// Execute the operation via SDK.
	// Pause our spinner just before calling the SDK — it has its own progress display.
	console.PauseSpinner()
	defer console.ResumeSpinner()
	switch command {
	case "create":
		logRunning("create", "--remove-orphans")
		createOpts := api.CreateOptions{RemoveOrphans: true}
		if len(appNames) > 0 {
			createOpts.Services = serviceNames
		}
		return srv.Create(sdkCtx, project, createOpts)
	case "kill":
		logRunning("kill")
		killOpts := api.KillOptions{Project: project}
		if len(appNames) > 0 {
			killOpts.Services = serviceNames
		}
		return srv.Kill(sdkCtx, project.Name, killOpts)
	case "start":
		logRunning("start")
		startOpts := api.StartOptions{Project: project}
		if len(appNames) > 0 {
			startOpts.AttachTo = serviceNames
		}
		return srv.Start(sdkCtx, project.Name, startOpts)
	case "rm":
		logRunning("rm")
		rmOpts := api.RemoveOptions{Project: project}
		if len(appNames) > 0 {
			rmOpts.Services = serviceNames
		}
		return srv.Remove(sdkCtx, project.Name, rmOpts)
	case "down":
		logRunning("down", "--remove-orphans")
		downOpts := api.DownOptions{RemoveOrphans: true}
		if len(appNames) > 0 {
			downOpts.Services = serviceNames
		}
		return srv.Down(sdkCtx, project.Name, downOpts)
	case "pause":
		logRunning("pause")
		pauseOpts := api.PauseOptions{}
		if len(appNames) > 0 {
			pauseOpts.Services = serviceNames
		}
		return srv.Pause(sdkCtx, project.Name, pauseOpts)
	case "pull":
		logRunning("pull", "--include-deps")
		return srv.Pull(sdkCtx, project, api.PullOptions{})
	case "restart":
		logRunning("restart")
		restartOpts := api.RestartOptions{}
		if len(appNames) > 0 {
			restartOpts.Services = serviceNames
		}
		return srv.Restart(sdkCtx, project.Name, restartOpts)
	case "stop":
		logRunning("stop")
		stopOpts := api.StopOptions{}
		if len(appNames) > 0 {
			stopOpts.Services = serviceNames
		}
		return srv.Stop(sdkCtx, project.Name, stopOpts)
	case "unpause":
		logRunning("unpause")
		unpauseOpts := api.PauseOptions{}
		if len(appNames) > 0 {
			unpauseOpts.Services = serviceNames
		}
		return srv.UnPause(sdkCtx, project.Name, unpauseOpts)
	case "update":
		logRunning("up", "-d", "--remove-orphans", "--pull", "always")
		setPullPolicy(project, types.PullPolicyAlways)
		return srv.Up(sdkCtx, project, api.UpOptions{
			Create: api.CreateOptions{RemoveOrphans: true},
			Start:  api.StartOptions{Attach: nil, Project: project},
		})
	case "up":
		logRunning("up", "-d", "--remove-orphans", "--pull", "missing")
		setPullPolicy(project, types.PullPolicyMissing)
		return srv.Up(sdkCtx, project, api.UpOptions{
			Create: api.CreateOptions{RemoveOrphans: true},
			Start:  api.StartOptions{Attach: nil, Project: project},
		})
	default: // unknown commands
		logRunning("up", "-d", "--remove-orphans", "--pull", "always")
		setPullPolicy(project, types.PullPolicyAlways)
		return srv.Up(sdkCtx, project, api.UpOptions{
			Create: api.CreateOptions{RemoveOrphans: true},
			Start:  api.StartOptions{Attach: nil, Project: project},
		})
	}
}

// LoadImageServices loads the compose project and returns a map of image ref → []service names.
// Returns an empty map (not an error) if the compose file cannot be read.
// ProjectName returns the compose project name (from COMPOSE_PROJECT_NAME or the
// normalized compose dir basename), matching what the SDK labels containers with.
func ProjectName() string {
	conf := config.LoadAppConfig()
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	envMap := make(map[string]string)
	if vars, err := dotenv.GetEnvFromFile(make(map[string]string), []string{envFile}); err == nil {
		for k, v := range vars {
			envMap[strings.Clone(k)] = strings.Clone(v)
		}
	}
	if pn := envMap["COMPOSE_PROJECT_NAME"]; pn != "" {
		return pn
	}
	return loader.NormalizeProjectName(filepath.Base(conf.ComposeDir))
}

func LoadImageServices(ctx context.Context) map[string][]string {
	conf := config.LoadAppConfig()
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
		return map[string][]string{}
	}

	result := make(map[string][]string)
	for name, svc := range project.Services {
		if svc.Image != "" {
			result[svc.Image] = append(result[svc.Image], name)
		}
	}
	for img := range result {
		sort.Strings(result[img])
	}
	return result
}
