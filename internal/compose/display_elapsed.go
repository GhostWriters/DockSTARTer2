package compose

import (
	"time"

	"DockSTARTer2/internal/strutil"

	"github.com/docker/compose/v5/pkg/api"
)

const elapsedShortFmt = "5.0s" // "9.1s" / "42.7s"

// const elapsedLongFmt = "4m05.0s" // "1m02.3s" (minutes + zero-padded seconds)
const elapsedLongFmt = "5.0s" // total seconds: "62.3s" / "200.3s"

// formatElapsed formats a duration for display.
// < 60s → elapsedShortFmt, >= 60s → elapsedLongFmt.
// Truncates to 100ms precision before branching to avoid e.g. 59.95s → "60.0s" with short format.
func formatElapsed(d time.Duration) string {
	d = (d / (100 * time.Millisecond)) * (100 * time.Millisecond)
	if d < 60*time.Second {
		return strutil.FormatDuration(d, elapsedShortFmt)
	}
	return strutil.FormatDuration(d, elapsedLongFmt)
}

func elapsedFromTime(start time.Time) string {
	return formatElapsed(time.Since(start))
}

func elapsedStr(t *consoleTask) string {
	end := time.Now()
	if t.completed() && !t.endTime.IsZero() {
		end = t.endTime
	}
	return formatElapsed(end.Sub(t.startTime))
}

// serviceTimerTask returns a synthetic consoleTask for a service timer:
// startTime = earliest recorded start (image pull or container start),
// endTime/status = from the service container task once it completes.
// Returns nil if work hasn't started yet.
func (p *consoleEventProcessor) serviceTimerTask(svcID string) *consoleTask {
	start, ok := p.serviceStartTimes[svcID]
	if !ok || start.IsZero() {
		return nil
	}
	synthetic := &consoleTask{startTime: start}
	if t := p.tasks[svcID]; t != nil && t.completed() && !t.endTime.IsZero() {
		synthetic.status = api.Done
		synthetic.endTime = t.endTime
	}
	return synthetic
}

// sectionTaskFor returns a synthetic consoleTask whose startTime/endTime span all tasks in ids.
// Returns nil if no tasks have started yet.
func (p *consoleEventProcessor) sectionTaskFor(ids []string) *consoleTask {
	var minStart, maxEnd time.Time
	allDone := true
	for _, id := range ids {
		t := p.tasks[id]
		if t == nil || t.startTime.IsZero() {
			allDone = false
			continue
		}
		if minStart.IsZero() || t.startTime.Before(minStart) {
			minStart = t.startTime
		}
		if !t.completed() {
			allDone = false
		} else if !t.endTime.IsZero() && t.endTime.After(maxEnd) {
			maxEnd = t.endTime
		}
	}
	if minStart.IsZero() {
		return nil
	}
	synthetic := &consoleTask{startTime: minStart}
	if allDone && !maxEnd.IsZero() {
		synthetic.status = api.Done
		synthetic.endTime = maxEnd
	}
	return synthetic
}
