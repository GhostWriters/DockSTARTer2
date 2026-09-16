package console

import (
	"context"

	"github.com/GhostWriters/semstyle"
	"github.com/charmbracelet/colorprofile"
)

// TintKeyForConnType returns the semstyle tint registration key for connType
// ("local", "ssh", or "web") -- kept separate per connType so local/SSH/web
// sessions never clobber each other's tint. Canonical definition; internal/tui
// re-exports this (and ActivateTintFor/ActivateSessionRenderContext below)
// under the same names for its own callers, since those predate this package
// having a use for them.
func TintKeyForConnType(connType string) string {
	return "ds2-tint-" + connType
}

// ActivateTintFor makes connType's registered tint the active one for the
// duration of fn, then restores whatever was active before -- see
// semstyle.RunWithTint's own doc comment for the locking/serialization this
// relies on.
func ActivateTintFor(connType string, fn func()) {
	semstyle.RunWithTint(TintKeyForConnType(connType), fn)
}

// ActivateSessionRenderContext nests ActivateTintFor(connType, ...) inside
// semstyle.RunWithProfile(profile, ...) for the duration of fn, so it renders
// with both connType's tint and profile -- one call instead of nesting both
// wrappers at every call site. fn should be quick (build/format a bounded
// batch of already-known output) -- semstyle.RunWithProfile's lock is meant
// to serialize short render passes against other sessions' own short render
// passes, not to hold a session's tint for the duration of unrelated slow
// work (a Docker API call, a subprocess, etc.).
func ActivateSessionRenderContext(connType string, profile colorprofile.Profile, fn func()) {
	semstyle.RunWithProfile(profile, func() {
		ActivateTintFor(connType, fn)
	})
}

// sessionRenderKey is the context key for WithSessionRenderContext.
type sessionRenderKey struct{}

type sessionRenderInfo struct {
	connType string
	profile  colorprofile.Profile
}

// WithSessionRenderContext attaches the connType/profile of the session that
// owns ctx's TUI writer (see WithTUIWriter), so a package that builds colored
// output for that writer from outside the session's own Update/View render
// pass (e.g. a report formatted once and written directly, rather than
// streamed as raw tags for the TUI to render later) can render it correctly
// scoped -- see RenderWithSessionContext -- instead of reading semstyle's
// process-global profile/tint unscoped, racing against every other
// concurrently rendering session.
func WithSessionRenderContext(ctx context.Context, connType string, profile colorprofile.Profile) context.Context {
	return context.WithValue(ctx, sessionRenderKey{}, sessionRenderInfo{connType: connType, profile: profile})
}

// RenderWithSessionContext runs fn with ctx's attached session render
// context active (see WithSessionRenderContext), or runs fn unscoped if ctx
// carries none -- e.g. a bare CLI invocation, already pinned for its whole
// duration via BeginTintFor, has no need to attach one. fn should be quick,
// same caveat as ActivateSessionRenderContext.
func RenderWithSessionContext(ctx context.Context, fn func()) {
	info, ok := ctx.Value(sessionRenderKey{}).(sessionRenderInfo)
	if !ok {
		fn()
		return
	}
	ActivateSessionRenderContext(info.connType, info.profile, fn)
}
