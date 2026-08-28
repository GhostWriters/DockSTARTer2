package commands

import (
	"context"
	"fmt"
	"sort"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
)

// HandleEnvAppFiles lists every .env.app.* var file belonging to an
// application -- the plain file, and (for a multi-service app) any
// per-service or shared/virtual files alongside it. Mostly a debugging/
// verification aid for AppVarFileNames itself, and for confirming a
// multi-service app's files are all correctly discovered.
func HandleEnvAppFiles(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires an application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	appName := group.Args[0]
	conf := config.LoadAppConfig()
	names := appenv.AppVarFileNames(appName, conf)

	if len(names) == 0 {
		logger.Notice(ctx, "No .env.app.* files found for '{{|App|}}%s{{[-]}}'.", appName)
		return nil
	}

	sort.Strings(names)
	logger.Notice(ctx, "'{{|App|}}%s{{[-]}}' var files:", appName)
	for _, name := range names {
		logger.Notice(ctx, "  {{|File|}}%s{{[-]}}", name)
	}
	return nil
}
