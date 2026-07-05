//go:build linux

package system

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Linux capability bit positions (linux/capability.h).
const (
	capChown  = 0 // CAP_CHOWN: change file ownership without being the owner or root
	capFowner = 3 // CAP_FOWNER: bypass owner-match checks (e.g. chmod on files the process doesn't own)
)

var (
	capsOnce sync.Once
	capEff   uint64
)

// effectiveCaps returns the current process's effective capability bitmask,
// read once from /proc/self/status (the CapEff line). Capabilities are
// granted at exec() time from the binary's file capabilities (set via
// "sudo setcap cap_chown,cap_fowner+ep <binary>"), so they cannot change
// mid-run and a single read is safe to cache. Returns 0 (no capabilities)
// if the file is unreadable, which just means callers fall back to sudo.
func effectiveCaps() uint64 {
	capsOnce.Do(func() {
		data, err := os.ReadFile("/proc/self/status")
		if err != nil {
			return
		}
		capEff = parseCapEff(string(data))
	})
	return capEff
}

// parseCapEff extracts the CapEff hex bitmask from /proc/self/status
// content. Returns 0 if the line is missing or malformed.
func parseCapEff(status string) uint64 {
	for _, line := range strings.Split(status, "\n") {
		rest, ok := strings.CutPrefix(line, "CapEff:")
		if !ok {
			continue
		}
		mask, err := strconv.ParseUint(strings.TrimSpace(rest), 16, 64)
		if err != nil {
			return 0
		}
		return mask
	}
	return 0
}

// hasCapChown reports whether this process can change file ownership
// natively, without sudo.
func hasCapChown() bool { return effectiveCaps()&(1<<capChown) != 0 }

// hasCapFowner reports whether this process can chmod files it doesn't own
// natively, without sudo.
func hasCapFowner() bool { return effectiveCaps()&(1<<capFowner) != 0 }
