package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/compose-spec/compose-go/v2/types"
	"go.yaml.in/yaml/v4"

	"DockSTARTer2/internal/appenv"
	"DockSTARTer2/internal/config"
	"DockSTARTer2/internal/constants"
)

// previewDependencies returns each service's depends_on as the next merge will produce
// them, read from the enabled apps' template files and the override file without
// merging, interpolating or writing anything, so a command's question can list the
// services it affects even before the first merge. Only service names and depends_on
// are filled in. It returns nil when no app is enabled.
func previewDependencies(ctx context.Context) (*types.Project, error) {
	conf := config.LoadAppConfig()
	envFile := filepath.Join(conf.ComposeDir, constants.EnvFileName)
	enabledApps, err := appenv.ListEnabledApps(conf)
	if err != nil || len(enabledApps) == 0 {
		return nil, err
	}
	var files []string
	for _, appName := range enabledApps {
		appFiles, err := appTemplateFiles(ctx, conf.Arch, appName, envFile, false)
		if err != nil {
			return nil, err
		}
		files = append(files, appFiles...)
	}
	if override := filepath.Join(conf.ComposeDir, constants.ComposeOverrideFileName); fileExists(override) {
		files = append(files, override)
	}

	services := types.Services{}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if err := mergeDependsOn(services, data); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
	}
	return &types.Project{Services: services}, nil
}

// mergeDependsOn adds the services and depends_on entries in one compose file's YAML to
// services, a later file's entry for a dependency replacing an earlier one's, as
// compose merges them.
func mergeDependsOn(services types.Services, data []byte) error {
	var doc struct {
		Services map[string]struct {
			DependsOn yaml.Node `yaml:"depends_on"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	for name, svc := range doc.Services {
		cfg := services[name]
		cfg.Name = name
		if cfg.DependsOn == nil {
			cfg.DependsOn = types.DependsOnConfig{}
		}
		switch svc.DependsOn.Kind {
		case yaml.SequenceNode: // short syntax: a list of names
			for _, item := range svc.DependsOn.Content {
				cfg.DependsOn[item.Value] = types.ServiceDependency{Condition: types.ServiceConditionStarted, Required: true}
			}
		case yaml.MappingNode: // long syntax: name → condition, restart, required
			for i := 0; i+1 < len(svc.DependsOn.Content); i += 2 {
				var entry struct {
					Condition string `yaml:"condition"`
					Restart   bool   `yaml:"restart"`
					Required  *bool  `yaml:"required"`
				}
				if err := svc.DependsOn.Content[i+1].Decode(&entry); err != nil {
					return err
				}
				if entry.Condition == "" {
					entry.Condition = types.ServiceConditionStarted
				}
				cfg.DependsOn[svc.DependsOn.Content[i].Value] = types.ServiceDependency{
					Condition: entry.Condition,
					Restart:   entry.Restart,
					Required:  entry.Required == nil || *entry.Required,
				}
			}
		}
		services[name] = cfg
	}
	return nil
}
