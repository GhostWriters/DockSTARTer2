//go:build darwin

package serve

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"

	dsexec "DockSTARTer2/internal/exec"
	"github.com/adrg/xdg"
)

const launchDaemonLabel = "com.dockstarter2.server"
const launchDaemonDir = "/Library/LaunchDaemons"

var launchDaemonTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
	<key>UserName</key>
	<string>{{.Username}}</string>
	<key>GroupName</key>
	<string>{{.Group}}</string>
	<key>WorkingDirectory</key>
	<string>{{.HomeDir}}</string>
	<key>EnvironmentVariables</key>
	<dict>
		{{- range .Env}}
		<key>{{.Key}}</key>
		<string>{{.Value}}</string>
		{{- end}}
	</dict>
	<key>ProgramArguments</key>
	<array>
		<string>{{.ExecPath}}</string>
		<string>--server-daemon</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<false/>
</dict>
</plist>
`))

type envPair struct {
	Key   string
	Value string
}

type launchDaemonData struct {
	Label    string
	Username string
	Group    string
	HomeDir  string
	Env      []envPair
	ExecPath string
}

func launchDaemonPath() string {
	return filepath.Join(launchDaemonDir, launchDaemonLabel+".plist")
}

func buildDaemonData(execPath string) (launchDaemonData, error) {
	u, err := user.Current()
	if err != nil {
		return launchDaemonData{}, fmt.Errorf("getting current user: %w", err)
	}
	grp, err := user.LookupGroupId(u.Gid)
	groupName := u.Gid
	if err == nil {
		groupName = grp.Name
	}

	envPairs := []envPair{
		{"HOME", u.HomeDir},
		{"XDG_CONFIG_HOME", xdg.ConfigHome},
		{"XDG_DATA_HOME", xdg.DataHome},
		{"XDG_STATE_HOME", xdg.StateHome},
		{"XDG_CACHE_HOME", xdg.CacheHome},
	}
	for _, key := range []string{"PATH", "DOCKER_HOST", "XDG_RUNTIME_DIR", "LANG", "LC_ALL"} {
		if val := os.Getenv(key); val != "" {
			envPairs = append(envPairs, envPair{key, val})
		}
	}

	return launchDaemonData{
		Label:    launchDaemonLabel,
		Username: u.Username,
		Group:    groupName,
		HomeDir:  u.HomeDir,
		Env:      envPairs,
		ExecPath: execPath,
	}, nil
}

func writePlistFile(ctx context.Context, plistPath string, data launchDaemonData) error {
	var buf bytes.Buffer
	if err := launchDaemonTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering plist: %w", err)
	}
	tmp, err := os.CreateTemp("", "ds2-plist-*.plist")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	tmp.Close()

	mvCmd, err := dsexec.SudoCommand(ctx, "mv", tmpPath, plistPath)
	if err != nil {
		return fmt.Errorf("preparing sudo mv: %w", err)
	}
	if out, err := mvCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("installing plist: %w\n%s", err, out)
	}
	return nil
}

func InstallService(execPath string) error {
	ctx := context.Background()
	data, err := buildDaemonData(execPath)
	if err != nil {
		return err
	}
	mkdirCmd, err := dsexec.SudoCommand(ctx, "mkdir", "-p", launchDaemonDir)
	if err != nil {
		return fmt.Errorf("preparing sudo mkdir: %w", err)
	}
	if out, err := mkdirCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating LaunchDaemons directory: %w\n%s", err, out)
	}
	return writePlistFile(ctx, launchDaemonPath(), data)
}

func UninstallService() error {
	ctx := context.Background()
	_ = sudoLaunchctl(ctx, "unload", launchDaemonPath())
	rmCmd, err := dsexec.SudoCommand(ctx, "rm", "-f", launchDaemonPath())
	if err != nil {
		return fmt.Errorf("preparing sudo rm: %w", err)
	}
	if out, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("removing plist: %w\n%s", err, out)
	}
	return nil
}

func EnableService() error {
	return sudoLaunchctl(context.Background(), "load", launchDaemonPath())
}

func DisableService() error {
	return sudoLaunchctl(context.Background(), "unload", launchDaemonPath())
}

func ServiceInstalled() (bool, error) {
	_, err := os.Stat(launchDaemonPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func ServiceEnabled() (bool, error) {
	ctx := context.Background()
	cmd, err := dsexec.SudoCommand(ctx, "launchctl", "list", launchDaemonLabel)
	if err != nil {
		return false, nil
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(out), launchDaemonLabel), nil
}

func sudoLaunchctl(ctx context.Context, args ...string) error {
	cmd, err := dsexec.SudoCommand(ctx, "launchctl", args...)
	if err != nil {
		return fmt.Errorf("preparing launchctl: %w", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl %v: %w\n%s", args, err, out)
	}
	return nil
}
