//go:build !windows

package classic

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// sudoWordRe matches the first standalone "sudo" word in a command string,
// word-bounded so it doesn't match inside an unrelated word like "pseudo".
var sudoWordRe = regexp.MustCompile(`\bsudo\b`)

// runSudoWithPassword runs cmdStr with -S injected right after its own
// leading "sudo", feeding the password to that single process via stdin, in
// a new session (Setsid) with no controlling terminal (so it can't reach
// for the TUI's own /dev/tty, which some sudo implementations -- notably
// sudo-rs, now default on newer Ubuntu -- will still do despite -S).
//
// Stdout/Stderr are wired through a raw *os.File pipe we manage ourselves,
// not a plain io.Writer. When Cmd.Stdout/Stderr are a plain io.Writer, Go
// creates its own internal pipe and a background copy goroutine, and
// Cmd.Wait() blocks until *that* goroutine sees EOF -- not just until the
// process exits. Confirmed live (repeated, connection-independent process
// watches spanning the exact moment Start() reported success): the sudo
// process itself was never observable on the host at all, yet Wait() still
// blocked for the full timeout regardless -- consistent with something
// (e.g. a short-lived internal helper sudo forks, gone before any polling
// could catch it) holding that pipe's write end open just long enough to
// starve Go's copy goroutine, even after the tracked process is long gone.
// Using our own *os.File pipe decouples process-reaping (Wait, driven by
// wait4() on the specific pid) from output-draining entirely, so a stray
// pipe-holder can only leak our copy goroutine, not hang the command.
func runSudoWithPassword(ctx context.Context, cmdStr, pass string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	loc := sudoWordRe.FindStringIndex(cmdStr)
	if loc == nil {
		return fmt.Errorf("sudo: %q does not contain a standalone \"sudo\"", cmdStr)
	}
	// -p '' overrides sudo's own "[sudo] password for user:"-style prompt
	// with an empty one -- DS2 already showed its own password dialog, so
	// sudo's redundant prompt banner would otherwise still appear in the
	// console panel's output.
	withDashS := cmdStr[:loc[1]] + " -S -p ''" + cmdStr[loc[1]:]

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("sudo: failed to create output pipe: %w", err)
	}

	sudoCmd := exec.CommandContext(ctx, "sh", "-c", withDashS)
	sudoCmd.Stdout = pw
	sudoCmd.Stderr = pw
	sudoCmd.Stdin = strings.NewReader(pass + "\n")
	sudoCmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := sudoCmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return fmt.Errorf("sudo: failed to start: %w", err)
	}
	// Close our copy of the write end now that the child has its own --
	// otherwise our own held-open fd would keep the read side from ever
	// seeing EOF, regardless of what the child does.
	pw.Close()

	copyDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(w, pr)
		close(copyDone)
	}()

	waitErr := sudoCmd.Wait()
	// Wait for the copy to finish naturally (EOF once every write end is
	// closed -- our own was already closed above, and the child's closes as
	// part of process exit) *before* closing pr ourselves. Closing pr
	// immediately after Wait() raced the still-draining copy goroutine when
	// the command finished very fast (e.g. an already-cached sudo
	// credential completing near-instantly): confirmed live, the second of
	// two consecutive sudo commands in the same session -- cache still
	// warm from the first -- produced no output at all even though
	// runSudoWithPassword returned a nil error, meaning Wait() outraced the
	// pipe still holding buffered output that pr.Close() then discarded.
	<-copyDone
	pr.Close()

	if ctx.Err() != nil {
		return fmt.Errorf("sudo: timed out waiting for the command to finish (%w)", ctx.Err())
	}
	return waitErr
}
