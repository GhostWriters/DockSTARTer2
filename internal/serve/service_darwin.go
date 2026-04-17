//go:build darwin

package serve

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const launchAgentLabel = "com.dockstarter2.server"

var launchAgentTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{.Label}}</string>
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

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding home directory: %w", err)
	}
	return filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist"), nil
}

func InstallService(execPath string) error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents directory: %w", err)
	}
	f, err := os.Create(plistPath)
	if err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}
	defer f.Close()
	return launchAgentTemplate.Execute(f, struct {
		Label    string
		ExecPath string
	}{launchAgentLabel, execPath})
}

func UninstallService() error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	_ = launchctl("unload", plistPath)
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing plist: %w", err)
	}
	return nil
}

func EnableService() error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	return launchctl("load", plistPath)
}

func DisableService() error {
	plistPath, err := launchAgentPath()
	if err != nil {
		return err
	}
	return launchctl("unload", plistPath)
}

func ServiceInstalled() (bool, error) {
	plistPath, err := launchAgentPath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(plistPath)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func launchctl(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %v: %w\n%s", args, err, out)
	}
	return nil
}
