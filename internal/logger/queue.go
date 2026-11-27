package logger

import (
	"fmt"
	"sync"
)

// maxQueuedLines caps a line queue: hundreds of times the largest real burst (a
// compose log is a few hundred lines), so it's only reached by a bug logging in a
// loop or a reader that has stopped reading, and memory stays bounded either way.
const maxQueuedLines = 100_000

// lineQueue is a log line queue that never blocks the writer and loses nothing
// short of maxQueuedLines: past that it drops the oldest lines, and its reader
// gets one line saying how many first.
type lineQueue struct {
	mu      sync.Mutex
	lines   []LogLine
	dropped int
	closed  bool
	ready   chan struct{} // gets a value when a line is pushed or the queue closed
}

func newLineQueue() *lineQueue {
	return &lineQueue{ready: make(chan struct{}, 1)}
}

// push adds line, dropping the oldest when the queue is full. A closed queue
// ignores it.
func (q *lineQueue) push(line LogLine) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	if len(q.lines) >= maxQueuedLines {
		q.lines = q.lines[1:]
		q.dropped++
	}
	q.lines = append(q.lines, line)
	q.mu.Unlock()
	q.signal()
}

// close stops the queue taking lines; its reader still gets those already queued.
func (q *lineQueue) close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
	q.signal()
}

func (q *lineQueue) signal() {
	select {
	case q.ready <- struct{}{}:
	default:
	}
}

// take returns the queued lines, led by a line counting any dropped, and whether
// the queue is closed with nothing left.
func (q *lineQueue) take() (lines []LogLine, finished bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	lines, q.lines = q.lines, nil
	if q.dropped > 0 {
		lines = append([]LogLine{droppedLine(q.dropped)}, lines...)
		q.dropped = 0
	}
	return lines, q.closed && len(lines) == 0
}

// droppedLine is the line standing in for n dropped log lines.
func droppedLine(n int) LogLine {
	noun := "lines"
	if n == 1 {
		noun = "line"
	}
	return LogLine{Text: fmt.Sprintf("[%d log %s dropped]", n, noun)}
}
