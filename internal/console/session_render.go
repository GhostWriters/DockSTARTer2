package console

import (
	"context"
	"sync"

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

// TintKeyForConnTypeElement returns the semstyle tint registration key for
// one connType's element ("menu" -- also what TintKeyForConnType alone
// returns, since menu is a connType's implicit/default element -- or
// "programbox"/"cli"). Kept distinct per element so e.g. a session's menu
// chrome and its ProgramBox output can carry independent tints.
func TintKeyForConnTypeElement(connType, element string) string {
	if element == "" || element == "menu" {
		return TintKeyForConnType(connType)
	}
	return TintKeyForConnType(connType) + "-" + element
}

// ActivateTintForElement makes element's tint (see TintKeyForConnTypeElement)
// active for the duration of fn, derived from whichever connType's key is
// already active (set by the enclosing session's own
// ActivateSessionRenderContext/ActivateTintFor) -- so a caller rendering one
// specific element (e.g. a ProgramBox) doesn't need to know or thread
// through its own connType, only which element it is.
//
// Deliberately uses semstyle.SetActiveTint/ActiveTintKey directly, NOT
// semstyle.RunWithTint -- this is meant to be called from inside an
// AppModel.Update/View call, which ActivateSessionRenderContext/
// ActivateTintFor has already wrapped in RunWithTint's own lock on the same
// goroutine (see model_update.go/model_view.go). RunWithTint's lock is a
// plain, non-reentrant sync.Mutex, so calling it again here would deadlock
// permanently on its own Lock() -- a goroutine blocked on itself, with no
// way to interrupt it. Safe without that lock precisely because it's
// always nested inside a call already holding it -- no other well-behaved
// caller (every other caller goes through RunWithTint/BeginTint) can be
// concurrently mutating the active tint while this goroutine holds that
// lock.
func ActivateTintForElement(element string, fn func()) {
	key := semstyle.ActiveTintKey()
	if key != "" && element != "" && element != "menu" {
		key += "-" + element
	}
	prev := semstyle.ActiveTintKey()
	semstyle.SetActiveTint(key)
	defer semstyle.SetActiveTint(prev)
	fn()
}

// ActivateTintKey makes key the active tint for the duration of fn -- for
// rendering one region (e.g. a preview) with a palette registered under its
// own key. Like ActivateTintForElement, it sets the tint directly and must
// run inside the caller's already-held render scope.
func ActivateTintKey(key string, fn func()) {
	prev := semstyle.ActiveTintKey()
	semstyle.SetActiveTint(key)
	defer semstyle.SetActiveTint(prev)
	fn()
}

// connThemePrefixes maps a connType to the semstyle theme namespace its own
// theme is registered under; a connType with no entry renders with the
// unprefixed (local) theme.
var (
	connThemePrefixesMu sync.RWMutex
	connThemePrefixes   = map[string]string{}
)

// SetThemePrefixForConnType records the semstyle theme namespace connType's
// theme is registered under ("" for the unprefixed theme).
func SetThemePrefixForConnType(connType, prefix string) {
	connThemePrefixesMu.Lock()
	defer connThemePrefixesMu.Unlock()
	if prefix == "" {
		delete(connThemePrefixes, connType)
		return
	}
	connThemePrefixes[connType] = prefix
}

// ThemePrefixForConnType returns the semstyle theme namespace connType
// renders with ("" for the unprefixed theme).
func ThemePrefixForConnType(connType string) string {
	connThemePrefixesMu.RLock()
	defer connThemePrefixesMu.RUnlock()
	return connThemePrefixes[connType]
}

// ActivateTintFor makes connType's registered tint and theme namespace, and
// connType itself (see ActiveConnType), the active ones for the duration of fn, then restores whatever was active
// before -- see semstyle.RunWithRenderScope's own doc comment for the
// locking/serialization this relies on.
func ActivateTintFor(connType string, fn func()) {
	semstyle.RunWithRenderScope(TintKeyForConnType(connType), ThemePrefixForConnType(connType), func() {
		defer SetActiveConnType(connType)()
		fn()
	})
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

// connTypeKey is the context key for WithConnType.
type connTypeKey struct{}

// WithConnType attaches the connType of the session that owns ctx, so work
// running off the session's own render pass (a ProgramBox task, say) can
// read that session's per-connType settings.
func WithConnType(ctx context.Context, connType string) context.Context {
	return context.WithValue(ctx, connTypeKey{}, connType)
}

// ConnTypeFromContext returns the connType attached to ctx by WithConnType
// or WithSessionRenderContext, or "local" if ctx carries neither (a bare CLI
// invocation).
func ConnTypeFromContext(ctx context.Context) string {
	if info, ok := ctx.Value(sessionRenderKey{}).(sessionRenderInfo); ok && info.connType != "" {
		return info.connType
	}
	if ct, ok := ctx.Value(connTypeKey{}).(string); ok && ct != "" {
		return ct
	}
	return "local"
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
