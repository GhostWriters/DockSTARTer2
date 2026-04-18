//go:build !linux && !darwin

package serve

import "fmt"

func InstallService(_ string) error {
	return fmt.Errorf("service management is not supported on this platform")
}

func UninstallService() error {
	return fmt.Errorf("service management is not supported on this platform")
}

func EnableService() error {
	return fmt.Errorf("service management is not supported on this platform")
}

func DisableService() error {
	return fmt.Errorf("service management is not supported on this platform")
}

func ServiceInstalled() (bool, error) {
	return false, nil
}

func ServiceEnabled() (bool, error) {
	return false, nil
}
