package classic

import (
	"fmt"
	"os"
	"os/user"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// parseShellArgs parses cmdStr as a single simple command -- no pipes,
// `&&`/`||`/`;` chaining, subshells, background execution, or redirections
// -- and returns its fully expanded argv. Wildcards are expanded against
// the real filesystem; variable references are limited to a fixed
// whitelist (HOME, USER, PWD), not the full process environment.
//
// This never invokes an actual shell: the whole point is that no string
// handed to sh -c is ever "reconstructed" by shell expansion into something
// a raw-text blacklist didn't see (command substitution, adjacent-empty-
// quote concatenation, etc.) -- there's no shell in the loop to do that
// reconstruction. Command substitution ($(...) and backticks) has no
// CmdSubst callback wired up below, so it fails expansion outright rather
// than running anything.
func parseShellArgs(cmdStr string) ([]string, error) {
	file, err := syntax.NewParser().Parse(strings.NewReader(cmdStr), "")
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	if len(file.Stmts) != 1 {
		return nil, fmt.Errorf("only a single command is supported here, not multiple statements")
	}
	stmt := file.Stmts[0]
	if stmt.Negated || stmt.Background || len(stmt.Redirs) > 0 {
		return nil, fmt.Errorf("only a single plain command is supported here, no negation/background/redirection")
	}
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok {
		return nil, fmt.Errorf("only a single plain command is supported here, no pipes/chaining/subshells")
	}
	if len(call.Assigns) > 0 {
		return nil, fmt.Errorf("inline variable assignment is not supported here")
	}

	home, _ := os.UserHomeDir()
	pwd, _ := os.Getwd()
	username := ""
	if u, err := user.Current(); err == nil {
		username = u.Username
	}
	cfg := &expand.Config{
		Env: expand.ListEnviron(
			"HOME="+home,
			"USER="+username,
			"PWD="+pwd,
		),
		ReadDir2: os.ReadDir,
	}

	argv, err := expand.Fields(cfg, call.Args...)
	if err != nil {
		return nil, fmt.Errorf("expansion error: %w", err)
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return argv, nil
}
