//go:build !linux

package system

// Linux file capabilities don't exist on other platforms (macOS/BSD have
// entirely different privilege models, and Windows never reaches the
// permission-fixing paths at all), so the native-fix eligibility checks
// always report false and callers fall back to the sudo re-exec path.
func hasCapChown() bool  { return false }
func hasCapFowner() bool { return false }
