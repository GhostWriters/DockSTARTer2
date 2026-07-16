package system

import (
	dsexec "DockSTARTer2/internal/exec"
	"DockSTARTer2/internal/console"
	"DockSTARTer2/internal/logger"
	"DockSTARTer2/internal/paths"
	"DockSTARTer2/internal/version"
	"bytes"
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// AutoSetcapStartup manages the optional CAP_CHOWN/CAP_FOWNER grant on the
// DS2 binary at startup, which lets permission fixes (see applyPermissionFix)
// run natively without sudo. Strictly opt-in via two persisted config values
// (config.SystemConfig), passed in and returned for the caller to save back:
//
//	enabled (auto_setcap) -- keep the capabilities applied: re-run
//	        "sudo setcap" whenever the binary lacks them (a self-update
//	        replacing the binary strips file capabilities). Applies
//	        regardless of asked, so setting it true by hand skips the prompt.
//	asked (setcap_asked)  -- the one-time question has been answered; only
//	        set after the user actually answers (an aborted prompt leaves it
//	        false so a later startup asks again).
//
// Linux-only, and skipped on non-interactive startups (daemons, cron,
// piped) since applying needs sudo (possibly a password prompt) and no user
// is present to ask. Capabilities granted this run only bind at the next
// exec (a kernel property), so applied reports whether that just happened,
// letting the caller re-exec with the original command line instead of
// continuing without them.
func AutoSetcapStartup(ctx context.Context, asked, enabled, interactive bool) (newAsked, newEnabled bool, applied bool) {
	if runtime.GOOS != "linux" || !interactive {
		return asked, enabled, false
	}
	if os.Geteuid() == 0 {
		// Already root (a genuine root login, since sudo invocations are
		// demoted before this runs): CAP_CHOWN/CAP_FOWNER would be strictly
		// redundant, and asking to grant them is meaningless noise.
		return asked, enabled, false
	}
	if binaryHasFixCaps() {
		return asked, enabled, false
	}

	if enabled {
		// Opted in; the binary lost the capabilities (most likely replaced
		// by a self-update). Re-apply without asking.
		announceGrant(ctx)
		if err := applySelfCaps(ctx); err != nil {
			logger.Warn(ctx, "Failed to re-apply file capabilities: %v", err)
			logger.Warn(ctx, "Set {{|Var|}}auto_setcap = false{{[-]}} in '"+console.FormatFilePath(paths.GetConfigFilePath())+"' to stop these attempts.")
			return asked, enabled, false
		}
		return asked, enabled, true
	}
	if asked {
		// Declined previously; never ask again.
		return asked, enabled, false
	}

	// Never asked before: offer once.
	exe := selfExePath()
	if exe == "" {
		return asked, enabled, false
	}
	yes, err := promptGrant(ctx, exe)
	if err != nil {
		// Aborted (e.g. Ctrl-C): leave unasked so a future startup asks again.
		return asked, enabled, false
	}
	if !yes {
		logger.Notice(ctx, "Not granting capabilities; '{{|UserCommand|}}sudo{{[-]}}' will be used when needed. To change this later, edit {{|Var|}}auto_setcap{{[-]}} in '"+console.FormatFilePath(paths.GetConfigFilePath())+"' or run '{{|UserCommand|}}ds2 --setcap{{[-]}}'.")
		return true, false, false
	}
	announceGrant(ctx)
	if err := applySelfCaps(ctx); err != nil {
		logger.Warn(ctx, "Failed to apply file capabilities: %v", err)
		logger.Warn(ctx, "Will retry at the next interactive startup; set {{|Var|}}auto_setcap = false{{[-]}} in '"+console.FormatFilePath(paths.GetConfigFilePath())+"' to stop.")
		return true, true, false
	}
	return true, true, true
}

// RunSetcapCommand implements the explicit "--setcap" CLI command: ask the
// grant question (auto-accepted by -y/--yes via QuestionPrompt's GlobalYes
// handling, making "ds2 -y --setcap" fully scriptable) and apply on yes.
// Unlike the startup offer, this always asks -- an explicit --setcap is a
// request to (re)decide, even if the question was answered before. Returns
// the values to persist (setcap_asked/auto_setcap) plus whether the
// capabilities were newly applied to the binary in this run -- callers use
// that to re-exec so any REMAINING command-line commands run with the
// capabilities active (file capabilities only bind at exec time).
func RunSetcapCommand(ctx context.Context) (asked, enabled, applied bool, err error) {
	if runtime.GOOS != "linux" {
		return false, false, false, fmt.Errorf("file capabilities are only supported on Linux")
	}
	if binaryHasFixCaps() {
		logger.Notice(ctx, "Capabilities {{|Var|}}CAP_CHOWN{{[-]}} and {{|Var|}}CAP_FOWNER{{[-]}} are already granted; keeping them maintained automatically.")
		return true, true, false, nil
	}
	exe := selfExePath()
	if exe == "" {
		return false, false, false, fmt.Errorf("cannot locate own executable")
	}
	yes, err := promptGrant(ctx, exe)
	if err != nil {
		return false, false, false, err
	}
	if !yes {
		logger.Notice(ctx, "Not granting capabilities; '{{|UserCommand|}}sudo{{[-]}}' will be used when needed.")
		return true, false, false, nil
	}
	announceGrant(ctx)
	if err := applySelfCaps(ctx); err != nil {
		// Keep the opt-in: startup will retry, and the failure cause is
		// reported to the caller for a proper non-zero exit.
		return true, true, false, fmt.Errorf("failed to apply file capabilities: %w", err)
	}
	return true, true, true, nil
}

// EnableSetcap applies the CAP_CHOWN/CAP_FOWNER grant directly, with no
// question -- the "--config-setcap" command. Returns whether the
// capabilities were newly applied (the current process lacked them), which
// callers use to decide whether a re-exec is worthwhile for any remaining
// command-line commands.
func EnableSetcap(ctx context.Context) (applied bool, err error) {
	if runtime.GOOS != "linux" {
		return false, fmt.Errorf("file capabilities are only supported on Linux")
	}
	hadCaps := hasCapChown() && hasCapFowner()
	announceGrant(ctx)
	if err := applySelfCaps(ctx); err != nil {
		return false, fmt.Errorf("failed to apply file capabilities: %w", err)
	}
	return !hadCaps, nil
}

// announceGrant logs the two-line notice shown immediately before every
// applySelfCaps call, regardless of which path triggered it (auto-startup
// grant/re-apply, --setcap, --config-setcap): what's about to happen, and
// how to turn it back off. applySelfCaps itself follows up with the actual
// "Running: sudo setcap ..." command line.
func announceGrant(ctx context.Context) {
	logger.Notice(ctx, "Granting {{|Var|}}CAP_CHOWN{{[-]}} and {{|Var|}}CAP_FOWNER{{[-]}} capabilities to {{|ApplicationName|}}%s{{[-]}}.", version.ApplicationName)
	logger.Notice(ctx, "Reapplies automatically after updates; disable with '{{|UserCommand|}}ds2 --config-no-setcap{{[-]}}'.")
}

// DisableSetcap removes the capability grant from the binary -- the
// "--config-no-setcap" command. Removal is skipped when the on-disk binary
// doesn't carry the grant (nothing to remove, no pointless sudo prompt).
// A no-op outside Linux.
func DisableSetcap(ctx context.Context) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if !binaryHasFixCaps() {
		logger.Notice(ctx, "No file capabilities to remove.")
		return nil
	}
	exe := selfExePath()
	if exe == "" {
		return fmt.Errorf("cannot locate own executable")
	}
	cmd, err := dsexec.SudoCommand(ctx, "setcap", "-r", exe)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	logger.Notice(ctx, "File capabilities removed from '"+console.FormatFilePath(exe)+"'.")
	return nil
}

// binaryHasFixCaps reports whether the on-disk binary currently carries the
// CAP_CHOWN+CAP_FOWNER grant -- the state auto_setcap maintains. Reads the
// file's security.capability xattr rather than the process's capabilities:
// the two diverge when DS2 was launched via sudo and demoted (the setuid
// drop clears all process caps even though the file still has its grant),
// and process caps also lag when the grant was applied or removed after
// this process exec'd. Falls back to the process view only when the xattr
// can't be read at all (e.g. a filesystem without xattr support).
func binaryHasFixCaps() bool {
	exe := selfExePath()
	if exe == "" {
		return hasCapChown() && hasCapFowner()
	}
	has, err := fileHasFixCaps(exe)
	if err != nil {
		return hasCapChown() && hasCapFowner()
	}
	return has
}

// promptGrant explains the capability grant and asks for confirmation.
// QuestionPrompt auto-accepts when -y/--yes is in effect.
func promptGrant(ctx context.Context, exe string) (bool, error) {
	logger.Notice(ctx, "{{|ApplicationName|}}DockSTARTer2{{[-]}} can be granted the Linux capabilities {{|Var|}}CAP_CHOWN{{[-]}} and {{|Var|}}CAP_FOWNER{{[-]}}, letting it fix file ownership and permissions without any '{{|UserCommand|}}sudo{{[-]}}' password prompts.")
	logger.Notice(ctx, "This runs '{{|UserCommand|}}sudo setcap cap_chown,cap_fowner+ep %s{{[-]}}' once now, and again automatically whenever an update replaces the binary.", exe)
	return console.QuestionPrompt(ctx, logger.Notice, "Grant Capabilities", "Grant these capabilities now?", "Y", false)
}

// applySelfCaps grants CAP_CHOWN/CAP_FOWNER to the running binary's on-disk
// file via "sudo setcap". The current process does NOT gain them (file
// capabilities are read at exec time); they apply from the next run.
func applySelfCaps(ctx context.Context) error {
	exe := selfExePath()
	if exe == "" {
		return fmt.Errorf("cannot locate own executable")
	}
	cmd, err := dsexec.SudoCommand(ctx, "setcap", "cap_chown,cap_fowner+ep", exe)
	if err != nil {
		return err
	}
	logger.Notice(ctx, "Running: {{|RunningCommand|}}sudo setcap cap_chown,cap_fowner+ep %s{{[-]}}", exe)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

func selfExePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}
