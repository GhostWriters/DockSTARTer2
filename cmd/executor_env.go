package cmd

import (
	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/commands"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/tui"
	"context"
	"errors"
	"fmt"
	"strings"
)

func handleEnvEdit(ctx context.Context, group *CommandGroup) error {
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires a variable name.", group.Command)
		return fmt.Errorf("no variable name provided")
	}

	upperCase := !strings.Contains(group.Command, "-lower")
	arg := group.Args[0]
	if upperCase && !strings.Contains(arg, ":") {
		arg = strings.ToUpper(arg)
	} else if upperCase && strings.Contains(arg, ":") {
		parts := strings.SplitN(arg, ":", 2)
		arg = parts[0] + ":" + strings.ToUpper(parts[1])
	}

	conf := config.LoadAppConfig()
	varName, file := commands.ResolveEnvVar(arg, conf)

	if !appenv.VarNameIsValid(varName, "") {
		logger.Error(ctx, "'{{|Var|}}%s{{[-]}}' is an invalid variable name.", varName)
		return fmt.Errorf("invalid variable name: %s", varName)
	}

	// appName is only set when APP:VAR syntax was used (variable lives in app file)
	appName := ""
	if strings.Contains(arg, ":") {
		appName = strings.ToUpper(strings.SplitN(arg, ":", 2)[0])
	}

	if err := tui.StartVarEditor(ctx, appName, varName, file); err != nil {
		if !errors.Is(err, tui.ErrUserAborted) {
			logger.Error(ctx, "TUI Error: %v", err)
		}
		return err
	}
	return nil
}
