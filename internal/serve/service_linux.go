//go:build linux

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

const systemdUnitName = "dockstarter2.service"
const systemdUnitDir = "/etc/systemd/system"

var systemdUnitTemplate = template.Must(template.New("unit").Parse(`[Unit]
Description=DockSTARTer2 SSH Server
After=network.target

[Service]
Type=simple
User={{.Username}}
Group={{.Group}}
WorkingDirectory={{.HomeDir}}
{{- range .Env}}
Environment={{.}}
{{- end}}
ExecStart={{.ExecPath}} --server-daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`))

type systemdUnitData struct {
	Username string
	Group    string
	HomeDir  string
	Env      []string
	ExecPath string
}

func systemdUnitPath() string {
	return filepath.Join(systemdUnitDir, systemdUnitName)
}

func buildUnitData(execPath string) (systemdUnitData, error) {
	u, err := user.Current()
	if err != nil {
		return systemdUnitData{}, fmt.Errorf("getting current user: %w", err)
	}
	grp, err := user.LookupGroupId(u.Gid)
	groupName := u.Gid
	if err == nil {
		groupName = grp.Name
	}

	envVars := []string{
		"HOME=" + u.HomeDir,
		"XDG_CONFIG_HOME=" + xdg.ConfigHome,
		"XDG_DATA_HOME=" + xdg.DataHome,
		"XDG_STATE_HOME=" + xdg.StateHome,
		"XDG_CACHE_HOME=" + xdg.CacheHome,
	}
	for _, key := range []string{"PATH", "DOCKER_HOST", "XDG_RUNTIME_DIR", "LANG", "LC_ALL"} {
		if val := os.Getenv(key); val != "" {
			envVars = append(envVars, key+"="+val)
		}
	}

	return systemdUnitData{
		Username: u.Username,
		Group:    groupName,
		HomeDir:  u.HomeDir,
		Env:      envVars,
		ExecPath: execPath,
	}, nil
}

func writeUnitFile(ctx context.Context, unitPath string, data systemdUnitData) error {
	var buf bytes.Buffer
	if err := systemdUnitTemplate.Execute(&buf, data); err != nil {
		return fmt.Errorf("rendering unit file: %w", err)
	}
	// Write to a temp file then sudo mv into place.
	tmp, err := os.CreateTemp("", "ds2-unit-*.service")
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

	mvCmd, err := dsexec.SudoCommand(ctx, "mv", tmpPath, unitPath)
	if err != nil {
		return fmt.Errorf("preparing sudo mv: %w", err)
	}
	if out, err := mvCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("installing unit file: %w\n%s", err, out)
	}
	return nil
}

func InstallService(execPath string) error {
	ctx := context.Background()
	data, err := buildUnitData(execPath)
	if err != nil {
		return err
	}
	if err := writeUnitFile(ctx, systemdUnitPath(), data); err != nil {
		return err
	}
	return sudoSystemctl(ctx, "daemon-reload")
}

func UninstallService() error {
	ctx := context.Background()
	_ = sudoSystemctl(ctx, "disable", "--now", systemdUnitName)
	rmCmd, err := dsexec.SudoCommand(ctx, "rm", "-f", systemdUnitPath())
	if err != nil {
		return fmt.Errorf("preparing sudo rm: %w", err)
	}
	if out, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("removing unit file: %w\n%s", err, out)
	}
	return sudoSystemctl(ctx, "daemon-reload")
}

func EnableService() error {
	return sudoSystemctl(context.Background(), "enable", "--now", systemdUnitName)
}

func DisableService() error {
	return sudoSystemctl(context.Background(), "disable", "--now", systemdUnitName)
}

func ServiceInstalled() (bool, error) {
	_, err := os.Stat(systemdUnitPath())
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func ServiceEnabled() (bool, error) {
	out, err := dsexec.SudoCommand(context.Background(), "systemctl", "is-enabled", systemdUnitName)
	if err != nil {
		return false, nil
	}
	output, _ := out.Output()
	return strings.TrimSpace(string(output)) == "enabled", nil
}

func sudoSystemctl(ctx context.Context, args ...string) error {
	cmd, err := dsexec.SudoCommand(ctx, "systemctl", args...)
	if err != nil {
		return fmt.Errorf("preparing systemctl: %w", err)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("systemctl %v: %w\n%s", args, err, out)
	}
	return nil
}
