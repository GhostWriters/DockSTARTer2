// Key concepts used throughout this package:
//
//   - Builtin apps: applications with templates in the .apps directory
//   - Added apps: applications with __ENABLED variables in .env
//   - App instances: variants of an app with a __ suffix (e.g. RADARR__4K)
//   - NiceName: human-readable app name from labels.yml
package appenv
