package update

var (
	// AppUpdateAvailable is true if an application update is available.
	AppUpdateAvailable bool
	// TmplUpdateAvailable is true if a template update is available.
	TmplUpdateAvailable bool
	// UpdateCheckError is true if the last update check failed due to network/timeout errors.
	UpdateCheckError bool
	// LatestAppVersion is the tag name of the latest application release.
	LatestAppVersion string
	// LatestTmplVersion is the short hash of the latest template commit.
	LatestTmplVersion string

	// PendingReExec stores the command to run after the TUI shuts down.
	// The actual exec is performed by the main thread after return.
	PendingReExec []string

	// RestartPending is true when another process updated the binary and wrote
	// a restart signal file for this process. The TUI header displays
	// "(Updated: Restart Pending)" and re-exec fires when a safe screen is reached.
	RestartPending bool
)
