// Package dockercheck probes the Docker daemon's reachability and API
// version so DS2 can warn early (startup) and fail clearly (before Docker
// operations) instead of surfacing a confusing low-level SDK error midway
// through a compose/prune run.
//
// It lives in its own package (rather than internal/docker) because
// internal/docker imports internal/compose for prune display, and compose
// needs to call Require -- putting the check in internal/docker would
// create an import cycle. It deliberately builds its own short-lived SDK
// client per probe instead of sharing internal/docker's singleton: probes
// must observe a daemon that appeared (or was upgraded) after startup, and
// the singleton caches its API-version negotiation for the process
// lifetime.
package dockercheck

import (
	"context"
	"strings"
	"sync"
	"time"

	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"

	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/client"
)

// MinAPIVersion is the oldest Docker daemon API version DS2 supports.
// API 1.41 = Docker Engine 20.10 (December 2020), the version that
// solidified the compose-spec era of daemon features the embedded Compose
// SDK assumes. Raise this if a newer SDK pin starts requiring newer daemon
// behavior.
const MinAPIVersion = "1.41"

// SetupURL is where users are pointed for Docker installation instructions.
const SetupURL = "https://dockstarter.com"

// Status is the outcome of one daemon probe.
type Status struct {
	// Reachable is true when the daemon answered a ping at all.
	Reachable bool
	// PermissionDenied is true when the daemon (or its socket) refused
	// access -- the classic symptom of the user not being in the docker
	// group, a different problem than Docker not being installed.
	PermissionDenied bool
	// TooOld is true when the daemon answered but its API version is below
	// MinAPIVersion.
	TooOld bool
	// APIVersion is the daemon's reported maximum API version (when
	// reachable) -- the raw ping header, not necessarily the version
	// actually in use for requests.
	APIVersion string
	// NegotiatedAPIVersion is the API version the client actually
	// negotiated down to for requests (e.g. via DOCKER_API_VERSION or a
	// client-side max lower than the daemon's). Equal to APIVersion unless
	// negotiation picked something lower.
	NegotiatedAPIVersion string
	// ServerVersion is the daemon's engine version, e.g. "27.3.1" (when
	// reachable).
	ServerVersion string
	// Err is the underlying probe error (when not reachable).
	Err error
}

// OK reports whether Docker operations can be expected to work.
func (s Status) OK() bool {
	return s.Reachable && !s.TooOld
}

var (
	lastMu   sync.Mutex
	lastStat *Status
)

// Check probes the daemon once, with a short internal timeout so a bad
// DOCKER_HOST can never hang the caller. Each call observes current
// reality -- a daemon installed or upgraded after DS2 started is picked up.
func Check(ctx context.Context) Status {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return Status{Err: err}
	}
	defer cli.Close()

	ping, err := cli.Ping(ctx)
	if err != nil {
		st := Status{Err: err}
		if strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			st.PermissionDenied = true
		}
		return st
	}

	st := Status{Reachable: true, APIVersion: ping.APIVersion}
	if sv, err := cli.ServerVersion(ctx); err == nil {
		st.ServerVersion = sv.Version
		if st.APIVersion == "" {
			st.APIVersion = sv.APIVersion
		}
	}
	cli.NegotiateAPIVersionPing(ping)
	st.NegotiatedAPIVersion = cli.ClientVersion()
	if st.NegotiatedAPIVersion == "" {
		st.NegotiatedAPIVersion = st.APIVersion
	}
	if st.APIVersion != "" && versions.LessThan(st.APIVersion, MinAPIVersion) {
		st.TooOld = true
	}
	return st
}

// StartupCheck probes the daemon and caches the result for later
// lightweight reads (Last), e.g. the -V version display.
func StartupCheck(ctx context.Context) Status {
	st := Check(ctx)
	setLast(st)
	return st
}

// Last returns the most recent probe result, or nil if no probe has run
// (e.g. the startup check was skipped this invocation).
func Last() *Status {
	lastMu.Lock()
	defer lastMu.Unlock()
	return lastStat
}

func setLast(st Status) {
	lastMu.Lock()
	defer lastMu.Unlock()
	lastStat = &st
}

// Require re-probes the daemon and, when it's unusable, logs the problem as
// an error and returns it so the calling operation aborts before doing any
// work. Called at the start of operations that genuinely need the daemon
// (compose up/down/..., prune); everything else must keep working without
// Docker, so nothing outside those paths should call this.
func Require(ctx context.Context) error {
	st := Check(ctx)
	setLast(st)
	if st.OK() {
		return nil
	}
	LogProblem(ctx, st, true)
	if st.Err != nil {
		return st.Err
	}
	return errDaemonTooOld
}

var errDaemonTooOld = &tooOldError{}

type tooOldError struct{}

func (e *tooOldError) Error() string {
	return "docker daemon API version is older than the minimum supported (" + MinAPIVersion + ")"
}

// LogProblem emits the shared explanation for a failed probe, as warnings
// (startup) or errors (aborting an operation).
func LogProblem(ctx context.Context, st Status, asError bool) {
	log := logger.Warn
	if asError {
		log = logger.Error
	}
	link := console.FormatLink("URL", SetupURL, SetupURL)
	switch {
	case st.TooOld:
		log(ctx, "The Docker daemon (API {{|Version|}}%s{{[-]}}, engine {{|Version|}}%s{{[-]}}) is older than the minimum {{|ApplicationName|}}DockSTARTer2{{[-]}} supports (API {{|Version|}}%s{{[-]}}).", st.APIVersion, st.ServerVersion, MinAPIVersion)
		log(ctx, "Please update Docker. See "+link+" for instructions.")
	case st.PermissionDenied:
		log(ctx, "Permission denied connecting to the Docker daemon socket.")
		log(ctx, "This usually means your user is not in the '{{|User|}}docker{{[-]}}' group. See "+link+" for instructions.")
	default:
		log(ctx, "The Docker daemon is not reachable (is Docker installed and running?)")
		log(ctx, "%v", st.Err)
		log(ctx, "Docker commands will not work until it is. See "+link+" for instructions on setting up Docker.")
	}
}
