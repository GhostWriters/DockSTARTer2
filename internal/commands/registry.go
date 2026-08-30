// Package commands provides the CLI command registry shared between the cmd
// executor and the TUI console panel.
package commands

import "strings"

// BaseCommand strips a "=value" shorthand suffix (e.g. "--env-get=VAR") off
// a raw flag argument, returning just the flag name -- every Registry
// lookup and top-level dispatch switch must compare against this, not the
// raw argument, since command flags supporting that shorthand (--env-get,
// --env-set, and their variants) are stored with the value still attached
// in CommandGroup.Command for their own handlers to split back out.
func BaseCommand(s string) string {
	if idx := strings.Index(s, "="); idx != -1 {
		return s[:idx]
	}
	return s
}

// Def holds metadata for a single CLI command flag.
// SessionLocked: blocks the command when a TUI session is active.
// ConsoleSafe: the command can be run from the restricted console panel
// input bar (unenforced in System Console, which allows any ds2 command).
// ConsoleBlocked: the command can never be run from EITHER console panel
// mode, System Console included, regardless of sudo -- for commands that
// would disrupt or restart the very process serving the console session
// (re-exec, launching another daemon, etc.), not just ones that need
// elevated trust. Currently these have no commands.Execute dispatch case at
// all (they're handled elsewhere, e.g. cmd/executor.go's startup-flag path),
// so this is a deliberate guard against a future case being added there
// without anyone reconsidering console-safety.
// RequiresSudo: the command requires a fresh sudo re-verification when run
// from a remote System Console session (local sessions and the restricted
// Console mode are unaffected -- ConsoleSafe already gates the latter). For
// ConsoleSafe commands whose action isn't already reachable unrestricted
// through the normal TUI menus for any authenticated remote user -- e.g. one
// that redirects a trusted path (config/compose folder) to somewhere an
// attacker controls, rather than a routine app/config operation the menus
// already permit freely.
// ConfigChanging: after running, the TUI should reload config/styles (ConfigChangedMsg).
// AppsChanging: after running, the TUI should refresh the app list (RefreshAppsListMsg).
type Def struct {
	Title          string
	SessionLocked  bool
	ConsoleSafe    bool
	ConsoleBlocked bool
	RequiresSudo   bool
	ConfigChanging bool
	AppsChanging   bool
}

// Registry maps CLI flag strings to their definitions.
// Modelled after the bash version's associative arrays in
// DockSTARTer/includes/cmdline.sh.
var Registry = map[string]Def{
	// ── Read-only ──────────────────────────────────────────────────────────────
	"-h":                        {Title: "Help", ConsoleSafe: true},
	"--help":                    {Title: "Help", ConsoleSafe: true},
	"-V":                        {Title: "Version", ConsoleSafe: true},
	"--version":                 {Title: "Version", ConsoleSafe: true},
	"--sysinfo":                 {Title: "System Info", ConsoleSafe: true},
	"--print-version":           {Title: "Print Version", ConsoleSafe: true},
	"--print-templates-version": {Title: "Print Templates Version", ConsoleSafe: true},
	"--man":                     {Title: "Application Documentation", ConsoleSafe: true},
	"--env-appfiles":            {Title: "App Var Files", ConsoleSafe: true},
	"-l":                        {Title: "List All Applications", ConsoleSafe: true},
	"--list":                    {Title: "List All Applications", ConsoleSafe: true},
	"--list-builtin":            {Title: "List Builtin Applications", ConsoleSafe: true},
	"--list-deprecated":         {Title: "List Deprecated Applications", ConsoleSafe: true},
	"--list-nondeprecated":      {Title: "List Non-Deprecated Applications", ConsoleSafe: true},
	"--list-added":              {Title: "List Added Applications", ConsoleSafe: true},
	"--list-enabled":            {Title: "List Enabled Applications", ConsoleSafe: true},
	"--list-disabled":           {Title: "List Disabled Applications", ConsoleSafe: true},
	"--list-referenced":         {Title: "List Referenced Applications", ConsoleSafe: true},
	"-s":                        {Title: "Application Status", ConsoleSafe: true},
	"--status":                  {Title: "Application Status", ConsoleSafe: true},
	"--env-appvars":             {Title: "Variables for Application", ConsoleSafe: true},
	"--env-appvars-lines":       {Title: "Variable Lines for Application", ConsoleSafe: true},
	"--env-get":                 {Title: "Get Value of Variable", ConsoleSafe: true},
	"--env-get-lower":           {Title: "Get Value of Variable", ConsoleSafe: true},
	"--env-get-line":            {Title: "Get Line of Variable", ConsoleSafe: true},
	"--env-get-lower-line":      {Title: "Get Line of Variable", ConsoleSafe: true},
	"--env-get-literal":         {Title: "Get Literal Value of Variable", ConsoleSafe: true},
	"--env-get-lower-literal":   {Title: "Get Literal Value of Variable", ConsoleSafe: true},
	"--config-show":             {Title: "Show Configuration", ConsoleSafe: true},
	"--show-config":             {Title: "Show Configuration", ConsoleSafe: true},
	"--theme-list":              {Title: "List Themes", ConsoleSafe: true},
	"--theme-table":             {Title: "List Themes", ConsoleSafe: true},
	"--theme-extract":           {Title: "Extract Theme", ConsoleSafe: true},
	"--theme-extract-all":       {Title: "Extract All Themes", ConsoleSafe: true},
	"--app-template-extract":    {Title: "Extract App Template", ConsoleSafe: true},
	"--app-template-new":        {Title: "New App Template", ConsoleSafe: true},
	"--server":                  {Title: "Server Management", ConsoleBlocked: true},
	"--server-daemon":           {Title: "Server Daemon", ConsoleBlocked: true},
	"--disconnect":              {Title: "Disconnect Session", ConsoleBlocked: true},

	// ── Session-locked (modifies env files / shared state) ────────────────────
	"-a":                      {Title: "Add Application", SessionLocked: true, ConsoleSafe: true, AppsChanging: true},
	"--add":                   {Title: "Add Application", SessionLocked: true, ConsoleSafe: true, AppsChanging: true},
	"-r":                      {Title: "Remove Application", SessionLocked: true, ConsoleSafe: true, AppsChanging: true},
	"--remove":                {Title: "Remove Application", SessionLocked: true, ConsoleSafe: true, AppsChanging: true},
	"-e":                      {Title: "Creating Environment Variables", SessionLocked: true, ConsoleSafe: true},
	"--env":                   {Title: "Creating Environment Variables", SessionLocked: true, ConsoleSafe: true},
	"--env-set":               {Title: "Set Value of Variable", SessionLocked: true, ConsoleSafe: true},
	"--env-set-lower":         {Title: "Set Value of Variable", SessionLocked: true, ConsoleSafe: true},
	"--env-set-literal":       {Title: "Set Value of Variable", SessionLocked: true, ConsoleSafe: true},
	"--env-set-lower-literal": {Title: "Set Value of Variable", SessionLocked: true, ConsoleSafe: true},
	"--env-edit":              {Title: "Edit Variable", SessionLocked: true, ConsoleSafe: true}, // launches TUI editor
	"--env-edit-lower":        {Title: "Edit Variable", SessionLocked: true, ConsoleSafe: true}, // launches TUI editor
	"--status-enable":         {Title: "Enable Application", SessionLocked: true, ConsoleSafe: true, AppsChanging: true},
	"--status-disable":        {Title: "Disable Application", SessionLocked: true, ConsoleSafe: true, AppsChanging: true},
	"-c":                      {Title: "Docker Compose", SessionLocked: true, ConsoleSafe: true},
	"--compose":               {Title: "Docker Compose", SessionLocked: true, ConsoleSafe: true},
	"-p":                      {Title: "Docker Prune", SessionLocked: true, ConsoleSafe: true},
	"--prune":                 {Title: "Docker Prune", SessionLocked: true, ConsoleSafe: true},
	"--start":                 {Title: "Start Container", SessionLocked: true, ConsoleSafe: true},
	"--stop":                  {Title: "Stop Container", SessionLocked: true, ConsoleSafe: true},
	"--restart":               {Title: "Restart Container", SessionLocked: true, ConsoleSafe: true},
	"--start-all":             {Title: "Start All Containers", SessionLocked: true, ConsoleSafe: true},
	"--stop-all":              {Title: "Stop All Containers", SessionLocked: true, ConsoleSafe: true},
	"--restart-all":           {Title: "Restart All Containers", SessionLocked: true, ConsoleSafe: true},
	"--start-stopped":         {Title: "Start Stopped Containers", SessionLocked: true, ConsoleSafe: true},
	"--stop-started":          {Title: "Stop Started Containers", SessionLocked: true, ConsoleSafe: true},
	"--restart-started":       {Title: "Restart Started Containers", SessionLocked: true, ConsoleSafe: true},
	"--logs":                  {Title: "Container Logs", ConsoleSafe: true},
	"-i":                      {Title: "Install", ConsoleSafe: true},
	"--install":               {Title: "Install", ConsoleSafe: true},
	// RequiresSudo: an update target can be redirected to an arbitrary
	// "owner/repo" (see update.ParseRepoAndRef) -- --update-app replaces the
	// running executable outright, and --update-templates' fetched content
	// feeds back into compose files, so a low-trust remote Console session
	// must not be able to point either at attacker-controlled content
	// without a fresh sudo re-check.
	"-u":                         {Title: "Update", ConsoleSafe: true, AppsChanging: true, RequiresSudo: true},
	"--update":                   {Title: "Update", ConsoleSafe: true, AppsChanging: true, RequiresSudo: true},
	"--update-app":               {Title: "Update App", ConsoleSafe: true, AppsChanging: true, RequiresSudo: true},
	"--update-templates":         {Title: "Update Templates", ConsoleSafe: true, AppsChanging: true, RequiresSudo: true},
	"-R":                         {Title: "Reset Actions", SessionLocked: true, ConsoleSafe: true, AppsChanging: true, ConfigChanging: true},
	"--reset":                    {Title: "Reset Actions", SessionLocked: true, ConsoleSafe: true, AppsChanging: true, ConfigChanging: true},
	"--uninstall":                {Title: "Uninstall", ConsoleBlocked: true},
	"-S":                         {Title: "Select Applications", ConsoleSafe: true},   // launches TUI; edit lock handles conflicts
	"--select":                   {Title: "Select Applications", ConsoleSafe: true},   // launches TUI; edit lock handles conflicts
	"-M":                         {Title: "Menu", ConsoleSafe: true},                  // launches TUI; edit lock handles conflicts
	"--menu":                     {Title: "Menu", ConsoleSafe: true},                  // launches TUI; edit lock handles conflicts
	"--edit-global":              {Title: "Edit Global Variables", ConsoleSafe: true}, // launches TUI; edit lock handles conflicts
	"--start-edit-global":        {Title: "Edit Global Variables", ConsoleSafe: true}, // launches TUI; edit lock handles conflicts
	"--edit-app":                 {Title: "Edit App Variables", ConsoleSafe: true},    // launches TUI; edit lock handles conflicts
	"--start-edit-app":           {Title: "Edit App Variables", ConsoleSafe: true},    // launches TUI; edit lock handles conflicts
	"--setcap":                   {Title: "Grant File Capabilities", ConsoleBlocked: true},
	"--config-setcap":            {Title: "Enable File Capabilities", ConsoleBlocked: true},
	"--config-no-setcap":         {Title: "Disable File Capabilities", ConsoleBlocked: true},
	"--config-pm":                {Title: "Select Package Manager", ConsoleSafe: true},
	"--config-pm-auto":           {Title: "Select Package Manager", ConsoleSafe: true},
	"--config-pm-list":           {Title: "List Known Package Managers", ConsoleSafe: true},
	"--config-pm-table":          {Title: "List Known Package Managers", ConsoleSafe: true},
	"--config-pm-existing-list":  {Title: "List Existing Package Managers", ConsoleSafe: true},
	"--config-pm-existing-table": {Title: "List Existing Package Managers", ConsoleSafe: true},
	"--config-folder":            {Title: "Set Config Folder", SessionLocked: true, ConsoleSafe: true, ConfigChanging: true, RequiresSudo: true},
	"--config-compose-folder":    {Title: "Set Compose Folder", SessionLocked: true, ConsoleSafe: true, ConfigChanging: true, RequiresSudo: true},
	"-T":                         {Title: "Set Theme", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme":                    {Title: "Set Theme", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-shadows":            {Title: "Turning on shadows.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-shadows":         {Title: "Turning off shadows.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-shadow":             {Title: "Turning on shadows.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-shadow":          {Title: "Turning off shadows.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-shadow-level":       {Title: "Set Shadow Level", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-scrollbar":          {Title: "Turning on scrollbars.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-scrollbar":       {Title: "Turning off scrollbars.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-scrollbars":         {Title: "Turning on scrollbars.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-scrollbars":      {Title: "Turning off scrollbars.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-spinner":            {Title: "Turning on spinners.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-spinner":         {Title: "Turning off spinners.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-spinners":           {Title: "Turning on spinners.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-spinners":        {Title: "Turning off spinners.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-spinner-speed":      {Title: "Set Spinner Speed", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-refresh-rate":       {Title: "Set Refresh Rate", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-lines":              {Title: "Turning on line drawing characters.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-lines":           {Title: "Turning off line drawing characters.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-line":               {Title: "Turning on line drawing characters.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-line":            {Title: "Turning off line drawing characters.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-borders":            {Title: "Turning on borders.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-borders":         {Title: "Turning off borders.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-border":             {Title: "Turning on borders.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-border":          {Title: "Turning off borders.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-large-buttons":      {Title: "Turning on large buttons.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-large-buttons":   {Title: "Turning off large buttons.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-large-titlebars":    {Title: "Turning on large title bars.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-large-titlebars": {Title: "Turning off large title bars.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-border-color":       {Title: "Set Border Color", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-dialog-title":       {Title: "Set Dialog Title Align", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-submenu-title":      {Title: "Set Submenu Title Align", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-panel-title":        {Title: "Set Panel Title Align", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-checkbox-brackets":  {Title: "Set Checkbox Brackets Mode", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-radio-brackets":     {Title: "Set Radio Brackets Mode", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-menu-brackets":      {Title: "Turning on menu brackets.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-show-preview":       {Title: "Showing the Appearance Settings preview panel by default.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-show-preview":    {Title: "Hiding the Appearance Settings preview panel by default.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-no-menu-brackets":   {Title: "Turning off menu brackets.", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--theme-tab-layout":         {Title: "Set Tab Layout", SessionLocked: false, ConsoleSafe: true, ConfigChanging: true},
	"--config-panel":             {Title: "Set Panel Mode", ConfigChanging: true, ConsoleBlocked: true},
}

// IsConsoleSafe reports whether a command flag is safe to run from the console panel.
func IsConsoleSafe(flag string) bool {
	return Registry[BaseCommand(flag)].ConsoleSafe
}

// IsRequiresSudo reports whether a command flag needs a fresh sudo
// re-verification when run from a remote System Console session -- see
// Def's RequiresSudo doc comment.
func IsRequiresSudo(flag string) bool {
	return Registry[BaseCommand(flag)].RequiresSudo
}

// IsConsoleBlocked reports whether a command flag can never be run from
// either console panel mode (restricted Console or System Console),
// regardless of sudo verification -- see Def's ConsoleBlocked doc comment.
func IsConsoleBlocked(flag string) bool {
	return Registry[BaseCommand(flag)].ConsoleBlocked
}

// IsSessionLocked reports whether a command flag requires an inactive TUI session.
func IsSessionLocked(flag string) bool {
	return Registry[BaseCommand(flag)].SessionLocked
}

// GroupsNeedConfigReload reports whether any group in groups has ConfigChanging set,
// meaning the TUI should reload config/styles after execution.
func GroupsNeedConfigReload(groups []CommandGroup) bool {
	for _, g := range groups {
		if Registry[BaseCommand(g.Command)].ConfigChanging {
			return true
		}
	}
	return false
}

// GroupsNeedAppsRefresh reports whether any group in groups has AppsChanging set,
// meaning the TUI should refresh the app list after execution.
func GroupsNeedAppsRefresh(groups []CommandGroup) bool {
	for _, g := range groups {
		if Registry[BaseCommand(g.Command)].AppsChanging {
			return true
		}
	}
	return false
}
