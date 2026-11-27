package logger

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/process"
)

// Every process that shows a log panel (the daemon, a local TUI) listens on a
// Unix socket in logSocketDir, and every DockSTARTer2 process sends its log lines
// to each one it finds, so a panel shows other processes' runs live, such as a
// CLI run from a shell. A received line is only published to the receiver's own
// panels, never sent on, so nothing loops or arrives twice. The log file stays the
// panels' starting history.

const (
	// forwardScanEvery is how often a sender looks for listeners that started since.
	forwardScanEvery = 2 * time.Second
	// forwardDialTimeout caps connecting to a listener.
	forwardDialTimeout = 250 * time.Millisecond
	// forwardWriteTimeout is how long a listener may stall before its connection is
	// closed: it has hung, and its lines are dropped.
	forwardWriteTimeout = 5 * time.Second
	// maxForwardedLineBytes caps one received line, so one huge line can't do what
	// maxQueuedLines stops many from doing.
	maxForwardedLineBytes = 1 << 20
)

// logSocketDir returns the folder of the listeners' sockets, one per process,
// <pid>.sock. A variable so tests use a folder of their own.
var logSocketDir = func() string {
	return filepath.Join(paths.GetLocksDir(), "logs")
}

func ownLogSocket() string {
	return filepath.Join(logSocketDir(), strconv.Itoa(os.Getpid())+".sock")
}

// forwardedLine is a log line on the wire, one JSON object per line.
type forwardedLine struct {
	Text  string `json:"text"`
	ASCII string `json:"ascii,omitempty"`
}

// ─── Sending ─────────────────────────────────────────────────────────────────

// forwardHandler sends each log line to the other processes' listeners, at the
// level the panels get (the global TUIHandler's).
type forwardHandler struct{ level slog.Leveler }

func (h *forwardHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *forwardHandler) Handle(_ context.Context, r slog.Record) error {
	fwd.send(tuiLogLine(r))
	return nil
}

func (h *forwardHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *forwardHandler) WithGroup(string) slog.Handler      { return h }

// forwarder holds a connection and a queue per listener. Logging only queues a
// line (send), so it never waits; each connection's goroutine writes its queue.
type forwarder struct {
	mu      sync.Mutex
	started bool
	stopped bool
	peers   map[string]*peer // by socket path
	stop    chan struct{}
	wg      sync.WaitGroup
}

type peer struct {
	conn net.Conn
	q    *lineQueue
}

var fwd = forwarder{peers: map[string]*peer{}}

// StartForwarding makes this process send its log lines to every listener, now
// and as they start, until FlushForwarding.
func StartForwarding() { fwd.start() }

// FlushForwarding stops forwarding, waiting up to timeout for the lines already
// queued to reach their listeners, so a run's last lines aren't lost at exit.
func FlushForwarding(timeout time.Duration) { fwd.flush(timeout) }

func (f *forwarder) start() {
	f.mu.Lock()
	if f.started {
		f.mu.Unlock()
		return
	}
	f.started, f.stop = true, make(chan struct{})
	f.mu.Unlock()
	f.scan()
	go func() {
		ticker := time.NewTicker(forwardScanEvery)
		defer ticker.Stop()
		for {
			select {
			case <-f.stop:
				return
			case <-ticker.C:
				f.scan()
			}
		}
	}()
}

func (f *forwarder) flush(timeout time.Duration) {
	f.mu.Lock()
	if !f.started || f.stopped {
		f.mu.Unlock()
		return
	}
	f.stopped = true
	close(f.stop)
	for _, p := range f.peers {
		p.q.close()
	}
	f.mu.Unlock()
	done := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

func (f *forwarder) send(line LogLine) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.peers {
		p.q.push(line)
	}
}

// scan connects to each listener not yet connected. A socket whose process is gone
// is removed; one that doesn't answer yet is tried again on the next scan.
func (f *forwarder) scan() {
	socks, _ := filepath.Glob(filepath.Join(logSocketDir(), "*.sock"))
	own := ownLogSocket()
	for _, path := range socks {
		f.mu.Lock()
		_, known := f.peers[path]
		stopped := f.stopped
		f.mu.Unlock()
		if path == own || known || stopped {
			continue
		}
		conn, err := net.DialTimeout("unix", path, forwardDialTimeout)
		if err != nil {
			if pid, perr := strconv.Atoi(strings.TrimSuffix(filepath.Base(path), ".sock")); perr == nil && !process.Exists(pid) {
				_ = os.Remove(path)
			}
			continue
		}
		p := &peer{conn: conn, q: newLineQueue()}
		f.mu.Lock()
		if f.stopped {
			f.mu.Unlock()
			conn.Close()
			return
		}
		f.peers[path] = p
		f.wg.Add(1)
		f.mu.Unlock()
		go f.run(path, p)
	}
}

// run writes p's queue to its listener until the queue closes and empties, or the
// listener stops reading; then the connection goes, with any lines left.
func (f *forwarder) run(path string, p *peer) {
	defer f.wg.Done()
	defer func() {
		p.conn.Close()
		p.q.close()
		f.mu.Lock()
		if f.peers[path] == p {
			delete(f.peers, path)
		}
		f.mu.Unlock()
	}()
	w := bufio.NewWriter(p.conn)
	enc := json.NewEncoder(w)
	for {
		lines, finished := p.q.take()
		if len(lines) > 0 {
			_ = p.conn.SetWriteDeadline(time.Now().Add(forwardWriteTimeout))
			for _, line := range lines {
				if enc.Encode(forwardedLine(line)) != nil {
					return
				}
			}
			if w.Flush() != nil {
				return
			}
			continue
		}
		if finished {
			return
		}
		<-p.q.ready
	}
}

// ─── Listening ───────────────────────────────────────────────────────────────

var (
	listenOnce sync.Once
	stopListen = func() {} // closes the listener and removes its socket; set by ListenForLogs
)

// ListenForLogs publishes the log lines other processes send to this process's
// log panels, until ctx ends or Cleanup runs. Only the first call in a process
// listens.
func ListenForLogs(ctx context.Context) {
	listenOnce.Do(func() {
		stop, err := listenAt(ctx, ownLogSocket())
		if err != nil {
			Debug(ctx, "Can't listen for other processes' log lines: %v", err)
			return
		}
		stopListen = stop
	})
}

// listenAt listens on the socket at path until ctx ends or the returned stop runs,
// which also closes the senders' connections, so they stop sending, and removes
// the socket.
func listenAt(ctx context.Context, path string) (stop func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	_ = os.Remove(path) // a leftover of an earlier process with this PID
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	var (
		mu      sync.Mutex
		conns   = map[net.Conn]struct{}{}
		stopped bool
		once    sync.Once
	)
	stop = func() {
		once.Do(func() {
			ln.Close()
			_ = os.Remove(path)
			mu.Lock()
			stopped = true
			for c := range conns {
				c.Close()
			}
			mu.Unlock()
		})
	}
	go func() {
		<-ctx.Done()
		stop()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if stopped {
				mu.Unlock()
				conn.Close()
				return
			}
			conns[conn] = struct{}{}
			mu.Unlock()
			go func() {
				receiveLogLines(conn)
				mu.Lock()
				delete(conns, conn)
				mu.Unlock()
			}()
		}
	}()
	return stop, nil
}

// receiveLogLines publishes each line read from conn until it closes. A line over
// maxForwardedLineBytes is dropped, and a line saying so published in its place.
func receiveLogLines(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		data, tooLong, err := readLimitedLine(r, maxForwardedLineBytes)
		if tooLong {
			publishLogLine(droppedLine(1))
		} else if len(data) > 0 {
			var line forwardedLine
			if json.Unmarshal(data, &line) == nil {
				publishLogLine(LogLine(line))
			}
		}
		if err != nil {
			return
		}
	}
}

// readLimitedLine reads one line from r, without its newline. A line longer than
// limit is read to its end and reported as tooLong, with no data.
func readLimitedLine(r *bufio.Reader, limit int) (data []byte, tooLong bool, err error) {
	for {
		chunk, isPrefix, rerr := r.ReadLine()
		if !tooLong {
			if len(data)+len(chunk) > limit {
				tooLong, data = true, nil
			} else {
				data = append(data, chunk...)
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) && (len(data) > 0 || tooLong) {
				return data, tooLong, nil
			}
			return data, tooLong, rerr
		}
		if !isPrefix {
			return data, tooLong, nil
		}
	}
}
