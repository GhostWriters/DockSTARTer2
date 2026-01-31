package appenv

import (
	"DockSTARTer2/internal/config"
	"fmt"
	"os"
	"path/filepath"
)

// Create ensures the environment file exists.
// If not, it copies from the default template.
func Create(file, defaultFile string) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create folder %s: %w", dir, err)
	}

	if _, err := os.Stat(file); err == nil {
		return nil // File exists
	}

	// Copy from default
	input, err := os.ReadFile(defaultFile)
	if err != nil {
		// If default doesn't exist, create empty? Or error?
		// Bash says: warn ... Copying example template. cp ...
		// If template missing, maybe just create empty.
		if os.IsNotExist(err) {
			return os.WriteFile(file, []byte{}, 0644)
		}
		return fmt.Errorf("failed to read default env %s: %w", defaultFile, err)
	}

	// Expand variables
	content := config.ExpandVariables(string(input))

	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create env file %s: %w", file, err)
	}
	return nil
}
