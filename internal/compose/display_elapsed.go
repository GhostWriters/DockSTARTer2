package compose

import (
	"time"

	"github.com/docker/compose/v5/pkg/api"

	"DockSTARTer2/internal/strutil"
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

// serviceTimerTask returns a synthetic consoleTask timing the work done for a
// service: its image's time (registry check or download), plus its container
// work. The time between them is not counted, so the timer pauses while the
// service waits. Returns nil when no work was done.
func (p *consoleEventProcessor) serviceTimerTask(svcID string) *consoleTask {
	now := time.Now()
	var total time.Duration
	active, worked := false, false
	if img := p.imageForService(svcID); img != "" {
		if t := p.tasks[img]; t != nil {
			end := t.endTime
			if !t.completed() || end.IsZero() {
				end, active = now, true
			}
			total += end.Sub(t.startTime)
			worked = true
		}
	}
	if t := p.tasks[svcID]; t != nil && !t.workStart.IsZero() {
		end := t.workEnd
		if t.working || end.IsZero() {
			end, active = now, true
		}
		total += end.Sub(t.workStart)
		worked = true
	}
	if !worked {
		return nil
	}
	synthetic := &consoleTask{startTime: now.Add(-total)}
	if !active {
		synthetic.status = api.Done
		synthetic.endTime = now
	}
	return synthetic
}

// serviceSectionTask returns a synthetic consoleTask spanning the work of the
// services in svcIDs and their images, from the first work to start to the last
// to finish. Returns nil when none of them worked.
func (p *consoleEventProcessor) serviceSectionTask(svcIDs []string) *consoleTask {
	var start, end time.Time
	active := false
	add := func(s, e time.Time, running bool) {
		if start.IsZero() || s.Before(start) {
			start = s
		}
		if running {
			active = true
		} else if e.After(end) {
			end = e
		}
	}
	seenImages := make(map[string]bool)
	for _, svc := range svcIDs {
		if img := p.imageForService(svc); img != "" && !seenImages[img] {
			seenImages[img] = true
			if t := p.tasks[img]; t != nil {
				add(t.startTime, t.endTime, !t.completed() || t.endTime.IsZero())
			}
		}
		if t := p.tasks[svc]; t != nil && !t.workStart.IsZero() {
			add(t.workStart, t.workEnd, t.working || t.workEnd.IsZero())
		}
	}
	if start.IsZero() {
		return nil
	}
	synthetic := &consoleTask{startTime: start}
	if !active {
		synthetic.status = api.Done
		synthetic.endTime = end
	}
	return synthetic
}

// imageForService returns the image svcID runs, or "".
func (p *consoleEventProcessor) imageForService(svcID string) string {
	for _, imgName := range p.imageOrder {
		if contains(p.serviceIDsForImage(imgName), svcID) {
			return imgName
		}
	}
	return ""
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
