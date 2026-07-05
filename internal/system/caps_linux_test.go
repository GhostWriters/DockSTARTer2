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
