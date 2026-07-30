//go:build !windows

package classic

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// runSudoWithPassword runs cmd under sudo, feeding the password to that
// single process via stdin, in a new session (Setsid) with no controlling
// terminal (so it can't reach for the TUI's own /dev/tty, which some sudo
// implementations -- notably sudo-rs, now default on newer Ubuntu -- will
// still do despite -S). DS2 always owns the sudo invocation now (cmd is a
// raw shell command with no "sudo" of its own -- dispatchShellCommand's
// blacklist guarantees that), so there's no need to find-and-inject -S into
// a user-typed sudo; -p '' also suppresses sudo's own redundant password
// prompt banner, since DS2 already showed its own password dialog.
//
// Stdout/Stderr are wired through a raw *os.File pipe we manage ourselves,
// not a plain io.Writer. When Cmd.Stdout/Stderr are a plain io.Writer, Go
// creates its own internal pipe and a background copy goroutine, and
// Cmd.Wait() blocks until *that* goroutine sees EOF -- not just until the
// process exits. A short-lived helper sudo itself forks can hold that
// pipe's write end open after the tracked process has already exited,
// starving Go's copy goroutine and hanging Wait() indefinitely. Using our
// own *os.File pipe decouples process-reaping (Wait, driven by wait4() on
// the specific pid) from output-draining entirely, so a stray pipe-holder
// can only leak our copy goroutine, not hang the command.
func runSudoWithPassword(ctx context.Context, cmd, pass string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	withSudo := "sudo -S -p '' " + cmd

	pr, pw, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("sudo: failed to create output pipe: %w", err)
	}

	sudoCmd := exec.CommandContext(ctx, "sh", "-c", withSudo)
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
	// immediately after Wait() would race the still-draining copy goroutine
	// when the command finishes very fast (e.g. an already-cached sudo
	// credential completing near-instantly), discarding buffered output
	// that hadn't been copied out yet.
	<-copyDone
	pr.Close()

	if ctx.Err() != nil {
		return fmt.Errorf("sudo: timed out waiting for the command to finish (%w)", ctx.Err())
	}
	return waitErr
}
