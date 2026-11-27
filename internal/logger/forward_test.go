package logger

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withSocketDir points the log sockets at a folder of the test's own.
func withSocketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ds2logs") // short, for the socket path limit
	if err != nil {
		t.Fatal(err)
	}
	prev := logSocketDir
	logSocketDir = func() string { return dir }
	t.Cleanup(func() {
		logSocketDir = prev
		os.RemoveAll(dir)
	})
	return dir
}

// otherProcessSocket is a listener's socket as another process would name it.
func otherProcessSocket(dir string) string {
	return filepath.Join(dir, fmt.Sprintf("%d.sock", os.Getpid()+1))
}

func newTestForwarder() *forwarder {
	return &forwarder{peers: map[string]*peer{}, started: true, stop: make(chan struct{})}
}

func TestForwardedLinesReachTheListenersPanels(t *testing.T) {
	dir := withSocketDir(t)
	stop, err := listenAt(context.Background(), otherProcessSocket(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ch, unsubscribe := SubscribeLogLines()
	defer unsubscribe()

	f := newTestForwarder()
	f.scan()
	f.send(LogLine{Text: "│✓│ one", ASCII: "|+| one"})
	f.send(LogLine{Text: "two"})
	f.flush(time.Second) // the last lines arrive before it returns

	if got := receive(t, ch); got.Text != "│✓│ one" || got.ASCII != "|+| one" {
		t.Errorf("first line = %+v, want both versions", got)
	}
	if got := receive(t, ch); got.Text != "two" || got.ASCII != "" {
		t.Errorf("second line = %+v", got)
	}
	select {
	case extra := <-ch:
		t.Errorf("got %+v as well, want each line once", extra)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestForwarderSkipsItsOwnSocketAndRemovesDeadOnes(t *testing.T) {
	dir := withSocketDir(t)
	own, err := listenAt(context.Background(), ownLogSocket())
	if err != nil {
		t.Fatal(err)
	}
	defer own()
	dead := filepath.Join(dir, "2147483646.sock") // no such process, nothing listening
	if err := os.WriteFile(dead, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	f := newTestForwarder()
	f.scan()
	if len(f.peers) != 0 {
		t.Errorf("connected to %d listeners, want none: its own socket is skipped", len(f.peers))
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Error("a dead process's socket was left behind")
	}
}

func TestForwarderDropsAListenerThatGoes(t *testing.T) {
	dir := withSocketDir(t)
	stop, err := listenAt(context.Background(), otherProcessSocket(dir))
	if err != nil {
		t.Fatal(err)
	}
	f := newTestForwarder()
	f.scan()
	stop()
	// Writing to the closed listener fails, which removes its connection.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		f.send(LogLine{Text: "after"})
		f.mu.Lock()
		n := len(f.peers)
		f.mu.Unlock()
		if n == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("the gone listener's connection is still kept")
}

func TestLineQueueDropsTheOldestPastItsCap(t *testing.T) {
	q := newLineQueue()
	for i := range maxQueuedLines + 2 {
		q.push(LogLine{Text: fmt.Sprint(i)})
	}
	lines, finished := q.take()
	if finished || len(lines) != maxQueuedLines+1 {
		t.Fatalf("took %d lines (finished %v), want the cap plus the dropped line", len(lines), finished)
	}
	if lines[0].Text != "[2 log lines dropped]" || lines[1].Text != "2" || lines[len(lines)-1].Text != fmt.Sprint(maxQueuedLines+1) {
		t.Errorf("lines start %q, %q and end %q", lines[0].Text, lines[1].Text, lines[len(lines)-1].Text)
	}
	q.close()
	q.push(LogLine{Text: "late"})
	if lines, finished := q.take(); len(lines) != 0 || !finished {
		t.Errorf("closed queue took %v (finished %v), want nothing more", lines, finished)
	}
}

func TestSubscribersGetABurstWhole(t *testing.T) {
	ch, unsubscribe := SubscribeLogLines()
	defer unsubscribe()
	const n = 1000 // well past the old 200-line buffer
	for i := range n {
		publishLogLine(LogLine{Text: fmt.Sprint(i)})
	}
	for i := range n {
		if got := receive(t, ch); got.Text != fmt.Sprint(i) {
			t.Fatalf("line %d = %q", i, got.Text)
		}
	}
	unsubscribe()
	if _, ok := <-ch; ok {
		t.Error("channel still open after unsubscribing")
	}
}

func TestReadLimitedLine(t *testing.T) {
	r := bufio.NewReaderSize(strings.NewReader("short\n"+strings.Repeat("x", 100)+"\nnext"), 16)
	for _, want := range []struct {
		data    string
		tooLong bool
	}{{"short", false}, {"", true}, {"next", false}} {
		data, tooLong, err := readLimitedLine(r, 50)
		if string(data) != want.data || tooLong != want.tooLong || err != nil {
			t.Errorf("readLimitedLine = %q, %v, %v, want %q, %v", data, tooLong, err, want.data, want.tooLong)
		}
	}
	if _, _, err := readLimitedLine(r, 50); err == nil {
		t.Error("want an error at the end")
	}
}

func TestAnOversizedLineIsCountedAsDropped(t *testing.T) {
	dir := withSocketDir(t)
	path := otherProcessSocket(dir)
	stop, err := listenAt(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ch, unsubscribe := SubscribeLogLines()
	defer unsubscribe()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(conn, "%s\n{\"text\":\"after\"}\n", strings.Repeat("x", maxForwardedLineBytes+1))
	conn.Close()
	if got := receive(t, ch); got.Text != "[1 log line dropped]" {
		t.Errorf("first = %q, want the dropped line", got.Text)
	}
	if got := receive(t, ch); got.Text != "after" {
		t.Errorf("then %q, want the next line", got.Text)
	}
}
