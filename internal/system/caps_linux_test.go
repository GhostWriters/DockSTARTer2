//go:build linux

package system

import "testing"

func TestParseCapEff(t *testing.T) {
	// 0x9 = bit 0 (CAP_CHOWN) + bit 3 (CAP_FOWNER), the exact grant from
	// "setcap cap_chown,cap_fowner+ep".
	status := "Name:\tds2\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000009\nCapEff:\t0000000000000009\nCapBnd:\t000001ffffffffff\n"
	mask := parseCapEff(status)
	if mask != 0x9 {
		t.Errorf("parseCapEff = %#x, want 0x9", mask)
	}
	if mask&(1<<capChown) == 0 {
		t.Error("expected CAP_CHOWN bit set in 0x9")
	}
	if mask&(1<<capFowner) == 0 {
		t.Error("expected CAP_FOWNER bit set in 0x9")
	}

	if got := parseCapEff("CapEff:\t0000000000000000\n"); got != 0 {
		t.Errorf("parseCapEff(zero mask) = %#x, want 0", got)
	}
	if got := parseCapEff("Name:\tds2\nno capabilities line here\n"); got != 0 {
		t.Errorf("parseCapEff(missing line) = %#x, want 0", got)
	}
	if got := parseCapEff("CapEff:\tzzzz\n"); got != 0 {
		t.Errorf("parseCapEff(malformed hex) = %#x, want 0", got)
	}
}

func TestParseVfsCaps(t *testing.T) {
	le32 := func(v uint32) []byte {
		return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
	}
	build := func(magic, permittedLow uint32) []byte {
		// revision-2 layout: magic, perm[0], inh[0], perm[1], inh[1]
		b := le32(magic)
		b = append(b, le32(permittedLow)...)
		b = append(b, le32(0)...) // inheritable low
		b = append(b, le32(0)...) // permitted high
		b = append(b, le32(0)...) // inheritable high
		return b
	}
	const rev2 = 0x02000000
	const effective = 0x000001
	const chownFowner = 1<<capChown | 1<<capFowner // 0x9

	if !parseVfsCaps(build(rev2|effective, chownFowner)) {
		t.Error("expected match: rev2, effective flag, chown+fowner permitted")
	}
	if parseVfsCaps(build(rev2, chownFowner)) {
		t.Error("expected no match without the effective flag (setcap +p, not +ep)")
	}
	if parseVfsCaps(build(rev2|effective, 1<<capChown)) {
		t.Error("expected no match with only CAP_CHOWN permitted")
	}
	if parseVfsCaps(nil) {
		t.Error("expected no match for empty xattr data")
	}
	if parseVfsCaps([]byte{0x01, 0x00}) {
		t.Error("expected no match for truncated xattr data")
	}
}
