package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/constants"
	"DockSTARTer2/internal/logger"
)

// Helper to resolve VAR and FILE from argument
// Arg can be "VAR" (uses default file) or "APP:VAR" (uses app file)
func resolveEnvVar(arg string, conf config.AppConfig) (string, string) {
	if strings.Contains(arg, ":") {
		parts := strings.SplitN(arg, ":", 2)
		appName := strings.ToLower(parts[0])
		varName := parts[1]
		filename := fmt.Sprintf("%s%s", constants.AppEnvFileNamePrefix, appName)
		return varName, filepath.Join(conf.ComposeDir, filename)
	}
	// Default to main env file in ComposeDir
	return arg, filepath.Join(conf.ComposeDir, constants.EnvFileName)
}

func handleEnvGet(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()

	// 1. Determine variables to process
	var args []string
	baseCmd := group.Command
	if idx := strings.Index(baseCmd, "="); idx != -1 {
		// Single parameter version: --env-get=VAR
		args = []string{baseCmd[idx+1:]}
		baseCmd = baseCmd[:idx]
	} else {
		// Multiple parameter version: --env-get VAR1 VAR2 ...
		args = group.Args
	}

	upperCase := !strings.Contains(baseCmd, "-lower")

	for _, arg := range args {
		if !appenv.VarNameIsValid(arg, "") {
			logger.Error(ctx, "'{{|Var|}}%s{{[-]}}' is an invalid variable name.", arg)
			continue
		}
		key, file := resolveEnvVar(arg, conf)
		if upperCase && !strings.Contains(arg, ":") {
			key = strings.ToUpper(key)
		}

		var val string
		var err error

		// Determine operation based on command
		switch {
		case strings.HasPrefix(baseCmd, "--env-get-literal"):
			val, err = appenv.GetLiteral(key, file)
		case strings.HasPrefix(baseCmd, "--env-get-line"):
			val, err = appenv.GetLine(key, file)
		case strings.HasPrefix(baseCmd, "--env-get-line-regex"):
			var lines []string
			lines, err = appenv.GetLineRegex(key, file)
			if err == nil {
				val = strings.Join(lines, "\n")
			}
		case strings.HasPrefix(baseCmd, "--env-get"):
			val, err = appenv.Get(key, file)
		}

		if err != nil {
			logger.Error(ctx, "Error getting %s: %v", arg, err)
			continue
		}

		if val != "" {
			console.Println(val)
		}
	}
	return nil
}

func handleEnvSet(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()

	type kv struct {
		key string
		val string
	}
	var pairs []kv
	var retErr error

	baseCmd := group.Command
	if idx := strings.Index(baseCmd, "="); idx != -1 {
		// Single parameter version: --env-set=VAR,VAL
		param := baseCmd[idx+1:]
		baseCmd = baseCmd[:idx]
		parts := strings.Split(param, ",")
		if len(parts) >= 2 {
			pairs = append(pairs, kv{parts[0], strings.Join(parts[1:], ",")})
		} else {
			logger.Error(ctx, "Command %s requires a variable name and a value (separated by comma).", group.Command)
			return fmt.Errorf("invalid command format")
		}
	} else {
		// Argument version: --env-set VAR=VAL
		for _, arg := range group.Args {
			if strings.Contains(arg, "=") {
				parts := strings.SplitN(arg, "=", 2)
				pairs = append(pairs, kv{parts[0], parts[1]})
			} else {
				// We don't support separate VAR VAL here to match Bash's check for '=' in the arg
				logger.Error(ctx, "Argument %s missing '='", arg)
			}
		}
	}

	upperCase := !strings.Contains(baseCmd, "-lower")
	isLiteral := strings.Contains(baseCmd, "-literal")

	for _, p := range pairs {
		if !appenv.VarNameIsValid(p.key, "") {
			logger.Error(ctx, "'{{|Var|}}%s{{[-]}}' is an invalid variable name.", p.key)
			continue
		}
		varName, file := resolveEnvVar(p.key, conf)
		if upperCase && !strings.Contains(p.key, ":") {
			varName = strings.ToUpper(varName)
		}

		// Ensure env file exists (create if needed)
		if err := appenv.Create(ctx, file); err != nil {
			logger.Debug(ctx, "Ensure env file error: %v", err)
		}

		var err error
		if isLiteral {
			err = appenv.SetLiteral(ctx, varName, p.val, file)
		} else {
			err = appenv.Set(ctx, varName, p.val, file)
		}

		if err != nil {
			logger.Error(ctx, "Error setting %s: %v", p.key, err)
			retErr = err
		} else {
			logger.Debug(ctx, "Set %s=%s in %s", varName, p.val, file)
		}
	}
	return retErr
}

func handleAppVarsCreateAll(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()
	if err := appenv.CreateAll(ctx, state.Force, conf); err != nil {
		logger.Error(ctx, "Failed to create app variables: %v", err)
		return err
	}
	return nil
}

func handleAppVarsCreate(ctx context.Context, group *CommandGroup, state *CmdState) error {
	conf := config.LoadAppConfig()
	if len(group.Args) == 0 {
		logger.Error(ctx, "The '{{|UserCommand|}}%s{{[-]}}' command requires at least one application name.", group.Command)
		return fmt.Errorf("no application name provided")
	}

	// Ensure env file exists (create if needed)
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	if err := appenv.Create(ctx, envFile); err != nil {
		logger.Debug(ctx, "Ensure env file error: %v", err)
	}

	// Migrate old-style APPNAME_ENABLED vars before creating app vars
	appenv.MigrateEnabledLines(ctx, conf)

	// Enable the apps first
	if err := appenv.Enable(ctx, group.Args, conf); err != nil {
		logger.Error(ctx, "Failed to enable apps: %v", err)
		return err
	}

	for _, arg := range group.Args {
		if err := appenv.CreateApp(ctx, arg, state.Force, conf); err != nil {
			logger.Error(ctx, "%v", err)
			return err
		}
	}

	if err := appenv.Update(ctx, state.Force, envFile); err != nil {
		logger.Warn(ctx, "Failed to update env usage: %v", err)
	} else {
		// Only unset on success as per user feedback
		for _, arg := range group.Args {
			appenv.UnsetNeedsCreateApp(ctx, strings.ToUpper(arg), conf)
		}
	}
	return nil
}

func handleEnvAppVars(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	args := group.Args
	if len(args) == 0 {
		logger.Fatal(ctx, "Command --env-appvars requires one or more application names.")
		return fmt.Errorf("no application name provided")
	}

	for _, appName := range args {
		vars, err := appenv.ListAppVars(ctx, appName, conf)
		if err != nil {
			logger.Error(ctx, "Failed to list variables for %s: %v", appName, err)
			return err
		}
		for _, v := range vars {
			logger.Display(ctx, v)
		}
	}
	return nil
}

func handleEnvAppVarsLines(ctx context.Context, group *CommandGroup) error {
	conf := config.LoadAppConfig()
	args := group.Args
	if len(args) == 0 {
		logger.Fatal(ctx, "Command --env-appvars-lines requires one or more application names.")
		return fmt.Errorf("no application name provided")
	}

	for _, appName := range args {
		lines, err := appenv.ListAppVarLines(ctx, appName, conf)
		if err != nil {
			logger.Error(ctx, "Failed to list variable lines for %s: %v", appName, err)
			return err
		}
		for _, l := range lines {
			logger.Display(ctx, l)
		}
	}
	return nil
}
