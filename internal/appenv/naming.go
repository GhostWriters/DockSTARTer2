package appenv

import (
	"context"
	"os"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"DockSTARTer2/internal/console"

	"go.yaml.in/yaml/v4"
)

// appURLCache caches AppURL results: app name → url string, or "" if not built-in.
var appURLCache sync.Map

// AppURL returns the dockstarter.com docs page for an app resolved from the
// bundled DockSTARTer-Templates repo, or "" if appName isn't a known
// built-in app (e.g. user-defined) or is a user app template override --
// dockstarter.com only documents the repo's own copy, which may not match
// (or may not even exist for) a user's local override/addition, so linking
// there would be misleading. Shared by compose/prune output styling and
// any UI that wants to link an app name to its docs.
func AppURL(appName string) string {
	if v, ok := appURLCache.Load(appName); ok {
		return v.(string)
	}
	url := ""
	if IsRepoTemplate(appName) {
		base := strings.ToLower(AppNameToBaseAppName(appName))
		url = "https://dockstarter.com/apps/" + base + "/"
	}
	appURLCache.Store(appName, url)
	return url
}

// LabelsFile structure for unmarshaling labels.yml
type LabelsFile struct {
	Services map[string]struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"services"`
}

// stripServiceSuffix removes a multi-service var-file marker from appName,
// if present, before any instance splitting: either a "___service" marker
// (a real per-service file, e.g. "immich___postgres") or a "-suffix" marker
// (a shared/virtual file, e.g. "immich-database"). Neither marker is ever
// part of a real base app name (app names are restricted to [A-Za-z0-9_],
// see IsAppNameValid), so stripping from the first occurrence of either is
// safe. The two forms are never combined in one name -- see
// project_multiservice_appvar_naming session notes.
func stripServiceSuffix(appName string) string {
	if idx := strings.Index(appName, "___"); idx >= 0 {
		return appName[:idx]
	}
	if idx := strings.Index(appName, "-"); idx >= 0 {
		return appName[:idx]
	}
	return appName
}

// AppNameToBaseAppName extracts the base application name.
func AppNameToBaseAppName(appName string) string {
	appName = stripServiceSuffix(appName)
	if strings.Contains(appName, "__") {
		parts := strings.Split(appName, "__")
		return parts[0]
	}
	return appName
}

// AppNameToInstanceName extracts the instance suffix from an app name.
func AppNameToInstanceName(appName string) string {
	appName = stripServiceSuffix(appName)
	if strings.Contains(appName, "__") {
		parts := strings.SplitN(appName, "__", 2)
		return parts[1]
	}
	return ""
}

// AppNameToServiceName extracts the service/shared-file suffix from an app
// name, if present -- either a "___service" marker (a real per-service
// file) or a "-suffix" marker (a shared/virtual file, e.g. "-database").
// Returns "" if appName has neither.
func AppNameToServiceName(appName string) string {
	if idx := strings.Index(appName, "___"); idx >= 0 {
		return appName[idx+3:]
	}
	if idx := strings.Index(appName, "-"); idx >= 0 {
		return appName[idx+1:]
	}
	return ""
}

// VarNameToAppName returns the DS application name based on the variable name
// passed -- always the bare app[__instance] name, with any service/shared
// qualifier (see stripServiceSuffix) stripped. Use this for anything that
// looks up or groups by the app itself: template/meta lookups, nice
// name/description, referenced-apps detection, etc. Use
// VarNameToAppNameService instead when you specifically need to preserve the
// service qualifier (e.g. to strip an "APPNAME[__INST]___SERVICE__" prefix
// back off the same var name).
func VarNameToAppName(varName string) string {
	return stripServiceSuffix(VarNameToAppNameService(varName))
}

// VarNameToAppNameService returns the DS application name based on the
// variable name passed, preserving any service/shared qualifier (e.g.
// "IMMICH___ML" for "IMMICH___ML__CONTAINER_NAME"). Mirrors
// varname_to_appname.sh: if the name contains ":", the part before the colon
// is the app name; otherwise the app name is extracted via the
// double-underscore pattern. Most callers want the bare app name instead --
// see VarNameToAppName.
func VarNameToAppNameService(varName string) string {
	// APPNAME:VARNAME format (used for .env.app.* vars)
	if idx := strings.Index(varName, ":"); idx > 0 {
		return strings.ToUpper(varName[:idx])
	}
	if !strings.Contains(varName, "__") {
		return ""
	}
	// Regex matches:
	// Group 1: The App Name (can be builtin or instance)
	// __: The separator
	// Group 2: The starting character of the variable name (can be _ or alphanumeric)
	// followed by the rest.
	// 0. Try to match APP[__INST]___SERVICE__VAR first (most specific --
	// must be tried before re3, which would otherwise partially match a
	// service-bearing name and silently drop the service segment: its
	// instance group excludes "_", so a leading "_" from a triple-underscore
	// marker gets absorbed into re3's var-name group instead of being
	// recognized as a service marker).
	reService := regexp.MustCompile(`^([A-Z][A-Z0-9]*)(?:__([A-Z0-9]+))?___([A-Z0-9]+)__([A-Za-z0-9_].*)`)
	mService := reService.FindStringSubmatch(varName)
	if len(mService) > 4 {
		result := mService[1]
		if mService[2] != "" {
			result += "__" + mService[2]
		}
		result += "___" + mService[3]
		return result
	}

	// 1. Try to match APP__INST__VAR
	re3 := regexp.MustCompile(`^([A-Z][A-Z0-9]*)__([A-Z0-9]+)__([A-Za-z0-9_].*)`)
	m3 := re3.FindStringSubmatch(varName)
	if len(m3) > 3 {
		return m3[1] + "__" + m3[2]
	}

	// 2. Try to match APP__VAR
	re2 := regexp.MustCompile(`^([A-Z][A-Z0-9]*)__([A-Za-z0-9_].*)`)
	m2 := re2.FindStringSubmatch(varName)
	if len(m2) > 2 {
		return m2[1]
	}

	return ""
}

// CapitalizeFirstLetter lowercases s then capitalizes the first Unicode letter,
// skipping any leading digits or non-letter characters.
// Examples: "4K" → "4K", "4k" → "4K", "23kkk" → "23Kkk", "abc" → "Abc".
func CapitalizeFirstLetter(s string) string {
	s = strings.ToLower(s)
	for i, r := range s {
		if unicode.IsLetter(r) {
			return s[:i] + string(unicode.ToUpper(r)) + s[i+utf8.RuneLen(r):]
		}
	}
	return s
}

// InstanceDisplayName returns the display label for an instance sub-row.
// If appName has no instance suffix it returns baseNiceName unchanged;
// otherwise it appends "__<TitleCasedSuffix>" (e.g. "Radarr__4k").
func InstanceDisplayName(baseNiceName, appName string) string {
	suffix := AppNameToInstanceName(appName)
	if suffix == "" {
		return baseNiceName
	}
	return baseNiceName + "__" + CapitalizeFirstLetter(suffix)
}

// AppStyleTag returns "UserApp" for a user-defined app and "App" for a
// built-in one, so callers can pick the right style without each
// duplicating the IsAppBuiltIn check.
func AppStyleTag(appName string) string {
	if IsAppBuiltIn(appName) {
		return "App"
	}
	return "UserApp"
}

// StyledAppName returns appName's nice name in the app's standard {{|App|}}
// (or {{|UserApp|}} for a user-defined app) style, hyperlinked to its docs
// page (AppURL) when one exists, ready to drop into any themed message.
// appName may be instance-qualified (e.g. "radarr__4k") -- both the nice
// name and the docs link are always resolved against the base app, since
// instances don't have their own.
func StyledAppName(ctx context.Context, appName string) string {
	base := AppNameToBaseAppName(appName)
	return console.FormatLink(AppStyleTag(base), GetNiceName(ctx, base), AppURL(base))
}

// StyledInstanceName is StyledAppName but the visible label keeps the
// instance suffix (e.g. "Radarr__4k" instead of just "Radarr") -- the
// hyperlink still resolves to the base app's docs page, since instances
// don't have their own. Falls back to the plain base name when appName has
// no instance suffix.
func StyledInstanceName(ctx context.Context, appName string) string {
	base := AppNameToBaseAppName(appName)
	label := InstanceDisplayName(GetNiceName(ctx, base), appName)
	return console.FormatLink(AppStyleTag(base), label, AppURL(base))
}

// GetNiceName returns a nicely formatted app name.
// Checks template labels first, then falls back to title casing.
func GetNiceName(ctx context.Context, appName string) string {
	// 1. Try to get from labels
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err == nil && labelsFile != "" {
		content, err := os.ReadFile(labelsFile)
		if err == nil {
			var labels LabelsFile
			if err := yaml.Unmarshal(content, &labels); err == nil {
				for _, service := range labels.Services {
					if name, ok := service.Labels["com.dockstarter.appinfo.nicename"]; ok {
						return strings.Trim(name, `"' `)
					}
				}
			}
		}
	}

	// 2. Fallback
	appUpper := strings.ToUpper(appName)
	parts := strings.Split(appUpper, "__")
	var niceParts []string
	for _, part := range parts {
		niceParts = append(niceParts, CapitalizeFirstLetter(part))
	}
	return strings.Join(niceParts, " ")
}

// GetDescription returns the description of an application.
func GetDescription(ctx context.Context, appName string, envFile string) string {
	// Check if user defined (not built-in OR missing ENABLED var)
	if IsAppUserDefined(ctx, appName, envFile) {
		return "{{|UserApp|}}" + GetNiceName(ctx, appName) + "{{[-]}} is a user defined application"
	}

	// Prefer description from .meta.toml (supports style tags) over labels.yml
	if appMeta, err := LoadAppMeta(ctx, appName); err == nil && appMeta != nil && appMeta.App.Description != "" {
		return appMeta.App.Description
	}

	// Try to get from labels
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil || labelsFile == "" {
		return "! Missing description !"
	}

	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return "! Missing description !"
	}

	var labels LabelsFile
	if err := yaml.Unmarshal(content, &labels); err != nil {
		return "! Missing description !"
	}

	for _, service := range labels.Services {
		if desc, ok := service.Labels["com.dockstarter.appinfo.description"]; ok {
			return strings.Trim(desc, `"' `)
		}
	}

	return "! Missing description !"
}

// GetDescriptionFromLines returns the description using staged env lines to determine
// user-defined status, instead of reading from disk.
func GetDescriptionFromLines(ctx context.Context, appName string, lines []string) string {
	if IsAppUserDefinedFromLines(ctx, appName, lines) {
		return "{{|UserApp|}}" + GetNiceName(ctx, appName) + "{{[-]}} is a user defined application"
	}
	// Fall through to the same metadata/labels lookup as GetDescription.
	if appMeta, err := LoadAppMeta(ctx, appName); err == nil && appMeta != nil && appMeta.App.Description != "" {
		return appMeta.App.Description
	}
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil || labelsFile == "" {
		return "! Missing description !"
	}
	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return "! Missing description !"
	}
	var labels LabelsFile
	if err := yaml.Unmarshal(content, &labels); err != nil {
		return "! Missing description !"
	}
	for _, service := range labels.Services {
		if desc, ok := service.Labels["com.dockstarter.appinfo.description"]; ok {
			return strings.Trim(desc, `"' `)
		}
	}
	return "! Missing description !"
}

// GetDescriptionFromTemplate returns the description of an application.
func GetDescriptionFromTemplate(ctx context.Context, appName string, envFile string) string {
	// Check if user defined (not built-in OR missing ENABLED var)
	if !IsAppBuiltIn(appName) {
		return "{{|UserApp|}}" + GetNiceName(ctx, appName) + "{{[-]}} is a user defined application"
	}

	// Prefer description from .meta.toml (supports style tags) over labels.yml
	if appMeta, err := LoadAppMeta(ctx, appName); err == nil && appMeta != nil && appMeta.App.Description != "" {
		return appMeta.App.Description
	}

	// Try to get from labels
	labelsFile, err := AppInstanceFile(ctx, appName, "*.labels.yml")
	if err != nil || labelsFile == "" {
		return "! Missing description !"
	}

	content, err := os.ReadFile(labelsFile)
	if err != nil {
		return "! Missing description !"
	}

	var labels LabelsFile
	if err := yaml.Unmarshal(content, &labels); err != nil {
		return "! Missing description !"
	}

	for _, service := range labels.Services {
		if desc, ok := service.Labels["com.dockstarter.appinfo.description"]; ok {
			return strings.Trim(desc, `"' `)
		}
	}

	return "! Missing description !"
}
