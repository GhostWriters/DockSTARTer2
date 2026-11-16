package compose

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"DockSTARTer2/internal/logger"
)

// sdkLog routes the compose SDK's own log lines (logrus, e.g. its warning that an
// optional dependency failed) to DS2's logger instead of raw to stderr. logrus has one
// global logger, so a hook installed once forwards each line to the session of the
// latest compose run still going.
var sdkLog struct {
	once sync.Once
	mu   sync.Mutex
	ctxs []context.Context // the running compose runs' contexts, oldest first
}

// routeSDKLogs sends the compose SDK's log lines to ctx's session until the returned
// func is called.
func routeSDKLogs(ctx context.Context) func() {
	sdkLog.once.Do(func() {
		logrus.SetOutput(io.Discard)
		logrus.AddHook(sdkLogHook{})
	})
	sdkLog.mu.Lock()
	sdkLog.ctxs = append(sdkLog.ctxs, ctx)
	sdkLog.mu.Unlock()
	return func() {
		sdkLog.mu.Lock()
		defer sdkLog.mu.Unlock()
		if i := slices.Index(sdkLog.ctxs, ctx); i >= 0 {
			sdkLog.ctxs = slices.Delete(sdkLog.ctxs, i, i+1)
		}
	}
}

// sdkLogHook forwards logrus entries to DS2's logger at the matching level.
type sdkLogHook struct{}

func (sdkLogHook) Levels() []logrus.Level { return logrus.AllLevels }

func (sdkLogHook) Fire(e *logrus.Entry) error {
	ctx := context.Background()
	sdkLog.mu.Lock()
	if n := len(sdkLog.ctxs); n > 0 {
		ctx = sdkLog.ctxs[n-1]
	}
	sdkLog.mu.Unlock()

	msg := strings.TrimSpace(e.Message)
	if err, ok := e.Data[logrus.ErrorKey].(error); ok {
		msg += ": " + err.Error()
	}
	switch e.Level {
	case logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel:
		logger.Error(ctx, "%s", msg)
	case logrus.WarnLevel:
		logger.Warn(ctx, "%s", msg)
	case logrus.InfoLevel:
		logger.Info(ctx, "%s", msg)
	default:
		logger.Debug(ctx, "%s", msg)
	}
	return nil
}
