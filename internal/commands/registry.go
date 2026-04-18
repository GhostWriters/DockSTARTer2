// Package commands provides the CLI command registry shared between the cmd
// executor and the TUI console panel.
package commands

// Def holds metadata for a single CLI command flag.
// SessionLocked: blocks the command when a TUI session is active.
// ConsoleSafe: the command can be run from the console panel input bar.
type Def struct {
	Title         string
	SessionLocked bool
	ConsoleSafe   bool
}

// Registry maps CLI flag strings to their definitions.
// Modelled after the bash version's associative arrays in
// DockSTARTer/includes/cmdline.sh.
var Registry = map[string]Def{
	// ── Read-only ──────────────────────────────────────────────────────────────
	"-h":                         {Title: "Help",                             ConsoleSafe: true},
	"--help":                     {Title: "Help",                             ConsoleSafe: true},
	"-V":                         {Title: "Version",                          ConsoleSafe: true},
	"--version":                  {Title: "Version",                          ConsoleSafe: true},
	"--man":                      {Title: "Application Documentation",        ConsoleSafe: true},
	"-l":                         {Title: "List All Applications",            ConsoleSafe: true},
	"--list":                     {Title: "List All Applications",            ConsoleSafe: true},
	"--list-builtin":             {Title: "List Builtin Applications",        ConsoleSafe: true},
	"--list-deprecated":          {Title: "List Deprecated Applications",     ConsoleSafe: true},
	"--list-nondeprecated":       {Title: "List Non-Deprecated Applications", ConsoleSafe: true},
	"--list-added":               {Title: "List Added Applications",          ConsoleSafe: true},
	"--list-enabled":             {Title: "List Enabled Applications",        ConsoleSafe: true},
	"--list-disabled":            {Title: "List Disabled Applications",       ConsoleSafe: true},
	"--list-referenced":          {Title: "List Referenced Applications",     ConsoleSafe: true},
	"-s":                         {Title: "Application Status",               ConsoleSafe: true},
	"--status":                   {Title: "Application Status",               ConsoleSafe: true},
	"--env-appvars":              {Title: "Variables for Application",        ConsoleSafe: true},
	"--env-appvars-lines":        {Title: "Variable Lines for Application",   ConsoleSafe: true},
	"--env-get":                  {Title: "Get Value of Variable",            ConsoleSafe: true},
	"--env-get-lower":            {Title: "Get Value of Variable",            ConsoleSafe: true},
	"--env-get-line":             {Title: "Get Line of Variable",             ConsoleSafe: true},
	"--env-get-lower-line":       {Title: "Get Line of Variable",             ConsoleSafe: true},
	"--env-get-literal":          {Title: "Get Literal Value of Variable",    ConsoleSafe: true},
	"--env-get-lower-literal":    {Title: "Get Literal Value of Variable",    ConsoleSafe: true},
	"--config-show":              {Title: "Show Configuration",               ConsoleSafe: true},
	"--show-config":              {Title: "Show Configuration",               ConsoleSafe: true},
	"--theme-list":               {Title: "List Themes",                      ConsoleSafe: true},
	"--theme-table":              {Title: "List Themes",                      ConsoleSafe: true},
	"--theme-extract":            {Title: "Extract Theme",                    ConsoleSafe: true},
	"--theme-extract-all":        {Title: "Extract All Themes",               ConsoleSafe: true},
	"--server":                   {Title: "Server Management"},      // needs serve package — not console-safe
	"--server-daemon":            {Title: "Server Daemon"},           // launches daemon — not console-safe

	// ── Session-locked (modifies env files / shared state) ────────────────────
	"-a":                         {Title: "Add Application",              SessionLocked: true, ConsoleSafe: true},
	"--add":                      {Title: "Add Application",              SessionLocked: true, ConsoleSafe: true},
	"-r":                         {Title: "Remove Application",           SessionLocked: true, ConsoleSafe: true},
	"--remove":                   {Title: "Remove Application",           SessionLocked: true, ConsoleSafe: true},
	"-e":                         {Title: "Creating Environment Variables", SessionLocked: true}, // launches TUI editor
	"--env":                      {Title: "Creating Environment Variables", SessionLocked: true}, // launches TUI editor
	"--env-set":                  {Title: "Set Value of Variable",        SessionLocked: true, ConsoleSafe: true},
	"--env-set-lower":            {Title: "Set Value of Variable",        SessionLocked: true, ConsoleSafe: true},
	"--env-set-literal":          {Title: "Set Value of Variable",        SessionLocked: true, ConsoleSafe: true},
	"--env-set-lower-literal":    {Title: "Set Value of Variable",        SessionLocked: true, ConsoleSafe: true},
	"--env-edit":                 {Title: "Edit Variable",                SessionLocked: true}, // launches TUI editor
	"--env-edit-lower":           {Title: "Edit Variable",                SessionLocked: true}, // launches TUI editor
	"--status-enable":            {Title: "Enable Application",           SessionLocked: true, ConsoleSafe: true},
	"--status-disable":           {Title: "Disable Application",          SessionLocked: true, ConsoleSafe: true},
	"-c":                         {Title: "Docker Compose",               SessionLocked: true, ConsoleSafe: true},
	"--compose":                  {Title: "Docker Compose",               SessionLocked: true, ConsoleSafe: true},
	"-p":                         {Title: "Docker Prune",                 SessionLocked: true, ConsoleSafe: true},
	"--prune":                    {Title: "Docker Prune",                 SessionLocked: true, ConsoleSafe: true},
	"-i":                         {Title: "Install",                      SessionLocked: true, ConsoleSafe: true},
	"--install":                  {Title: "Install",                      SessionLocked: true, ConsoleSafe: true},
	"-u":                         {Title: "Update",                       SessionLocked: true, ConsoleSafe: true},
	"--update":                   {Title: "Update",                       SessionLocked: true, ConsoleSafe: true},
	"--update-app":               {Title: "Update App",                   SessionLocked: true, ConsoleSafe: true},
	"--update-templates":         {Title: "Update Templates",             SessionLocked: true, ConsoleSafe: true},
	"-R":                         {Title: "Reset Actions",                SessionLocked: true, ConsoleSafe: true},
	"--reset":                    {Title: "Reset Actions",                SessionLocked: true, ConsoleSafe: true},
	"-S":                         {Title: "Select Applications",          SessionLocked: true}, // launches TUI
	"--select":                   {Title: "Select Applications",          SessionLocked: true}, // launches TUI
	"-M":                         {Title: "Menu",                         SessionLocked: true}, // launches TUI
	"--menu":                     {Title: "Menu",                         SessionLocked: true}, // launches TUI
	"--edit-global":              {Title: "Edit Global Variables",        SessionLocked: true}, // launches TUI
	"--start-edit-global":        {Title: "Edit Global Variables",        SessionLocked: true}, // launches TUI
	"--edit-app":                 {Title: "Edit App Variables",           SessionLocked: true}, // launches TUI
	"--start-edit-app":           {Title: "Edit App Variables",           SessionLocked: true}, // launches TUI
	"--config-pm":                {Title: "Select Package Manager",       SessionLocked: true}, // launches TUI
	"--config-pm-auto":           {Title: "Select Package Manager",       SessionLocked: true, ConsoleSafe: true},
	"--config-pm-list":           {Title: "List Known Package Managers",  SessionLocked: true, ConsoleSafe: true},
	"--config-pm-table":          {Title: "List Known Package Managers",  SessionLocked: true, ConsoleSafe: true},
	"--config-pm-existing-list":  {Title: "List Existing Package Managers", SessionLocked: true, ConsoleSafe: true},
	"--config-pm-existing-table": {Title: "List Existing Package Managers", SessionLocked: true, ConsoleSafe: true},
	"--config-folder":            {Title: "Set Config Folder",            SessionLocked: true, ConsoleSafe: true},
	"--config-compose-folder":    {Title: "Set Compose Folder",           SessionLocked: true, ConsoleSafe: true},
	"-T":                         {Title: "Set Theme",                    SessionLocked: true, ConsoleSafe: true},
	"--theme":                    {Title: "Set Theme",                    SessionLocked: true, ConsoleSafe: true},
	"--theme-shadows":            {Title: "Turned On Shadows",            SessionLocked: true, ConsoleSafe: true},
	"--theme-no-shadows":         {Title: "Turned Off Shadows",           SessionLocked: true, ConsoleSafe: true},
	"--theme-shadow":             {Title: "Turned On Shadows",            SessionLocked: true, ConsoleSafe: true},
	"--theme-no-shadow":          {Title: "Turned Off Shadows",           SessionLocked: true, ConsoleSafe: true},
	"--theme-shadow-level":       {Title: "Set Shadow Level",             SessionLocked: true, ConsoleSafe: true},
	"--theme-scrollbar":          {Title: "Turned On Scrollbars",         SessionLocked: true, ConsoleSafe: true},
	"--theme-no-scrollbar":       {Title: "Turned Off Scrollbars",        SessionLocked: true, ConsoleSafe: true},
	"--theme-lines":              {Title: "Turned On Line Drawing",       SessionLocked: true, ConsoleSafe: true},
	"--theme-no-lines":           {Title: "Turned Off Line Drawing",      SessionLocked: true, ConsoleSafe: true},
	"--theme-line":               {Title: "Turned On Line Drawing",       SessionLocked: true, ConsoleSafe: true},
	"--theme-no-line":            {Title: "Turned Off Line Drawing",      SessionLocked: true, ConsoleSafe: true},
	"--theme-borders":            {Title: "Turned On Borders",            SessionLocked: true, ConsoleSafe: true},
	"--theme-no-borders":         {Title: "Turned Off Borders",           SessionLocked: true, ConsoleSafe: true},
	"--theme-border":             {Title: "Turned On Borders",            SessionLocked: true, ConsoleSafe: true},
	"--theme-no-border":          {Title: "Turned Off Borders",           SessionLocked: true, ConsoleSafe: true},
	"--theme-button-borders":     {Title: "Turned On Button Borders",     SessionLocked: true, ConsoleSafe: true},
	"--theme-no-button-borders":  {Title: "Turned Off Button Borders",    SessionLocked: true, ConsoleSafe: true},
	"--theme-border-color":       {Title: "Set Border Color",             SessionLocked: true, ConsoleSafe: true},
	"--theme-dialog-title":       {Title: "Set Dialog Title Align",       SessionLocked: true, ConsoleSafe: true},
	"--theme-submenu-title":      {Title: "Set Submenu Title Align",      SessionLocked: true, ConsoleSafe: true},
	"--theme-log-title":          {Title: "Set Log Title Align",          SessionLocked: true, ConsoleSafe: true},
}

// IsConsoleSafe reports whether a command flag is safe to run from the console panel.
func IsConsoleSafe(flag string) bool {
	return Registry[flag].ConsoleSafe
}

// IsSessionLocked reports whether a command flag requires an inactive TUI session.
func IsSessionLocked(flag string) bool {
	return Registry[flag].SessionLocked
}
