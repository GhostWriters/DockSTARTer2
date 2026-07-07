//go:build linux

package system

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
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

// fileHasFixCaps reports whether the file at path carries the
// CAP_CHOWN+CAP_FOWNER grant in its security.capability xattr -- the
// on-disk state that setcap manages. This is deliberately NOT the same
// question as hasCapChown/hasCapFowner: those read the current PROCESS's
// capabilities, which diverge from the file's when DS2 was launched via
// sudo and demoted (root gets full caps at exec regardless of the file,
// and the setuid drop then clears them all) -- using process caps there
// made auto_setcap maintenance re-apply a grant the binary still had.
func fileHasFixCaps(path string) (bool, error) {
	buf := make([]byte, 64)
	n, err := unix.Getxattr(path, "security.capability", buf)
	if err != nil {
		if err == unix.ENODATA {
			return false, nil // no capabilities set at all
		}
		return false, err
	}
	return parseVfsCaps(buf[:n]), nil
}

// parseVfsCaps decodes a security.capability xattr value (struct
// vfs_cap_data) and reports whether CAP_CHOWN and CAP_FOWNER are in the
// permitted set with the effective flag raised (the "+ep" setcap applies).
// Both capability bits live in the low 32-bit word. Layout (all
// little-endian u32s): magic_etc, then permitted/inheritable pairs --
// revision 1 has one pair, revisions 2 and 3 have two (revision 3 appends
// a rootid, which doesn't matter here).
func parseVfsCaps(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	magic := binary.LittleEndian.Uint32(data[0:4])
	const vfsCapFlagsEffective = 0x000001
	if magic&vfsCapFlagsEffective == 0 {
		return false
	}
	permittedLow := binary.LittleEndian.Uint32(data[4:8])
	const want = uint32(1<<capChown | 1<<capFowner)
	return permittedLow&want == want
}
