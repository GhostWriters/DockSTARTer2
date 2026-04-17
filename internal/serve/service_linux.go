//go:build linux

package serve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const systemdUnitName = "dockstarter2.service"

var systemdUnitTemplate = template.Must(template.New("unit").Parse(`[Unit]
Description=DockSTARTer2 SSH Server
After=network.target

[Service]
Type=simple
ExecStart={{.ExecPath}} --server-daemon
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
`))

func systemdUnitPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("finding user config dir: %w", err)
	}
	return filepath.Join(cfgDir, "systemd", "user", systemdUnitName), nil
}

func InstallService(execPath string) error {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(unitPath), 0755); err != nil {
		return fmt.Errorf("creating systemd user unit directory: %w", err)
	}
	f, err := os.Create(unitPath)
	if err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}
	defer f.Close()
	if err := systemdUnitTemplate.Execute(f, struct{ ExecPath string }{execPath}); err != nil {
		return fmt.Errorf("rendering unit file: %w", err)
	}
	return systemctlUser("daemon-reload")
}

func UninstallService() error {
	_ = systemctlUser("disable", "--now", systemdUnitName)
	unitPath, err := systemdUnitPath()
	if err != nil {
		return err
	}
	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}
	return systemctlUser("daemon-reload")
}

func EnableService() error {
	return systemctlUser("enable", "--now", systemdUnitName)
}

func DisableService() error {
	return systemctlUser("disable", "--now", systemdUnitName)
}

func ServiceInstalled() (bool, error) {
	unitPath, err := systemdUnitPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(unitPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func systemctlUser(args ...string) error {
	args = append([]string{"--user"}, args...)
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %v: %w\n%s", args, err, out)
	}
	return nil
}
