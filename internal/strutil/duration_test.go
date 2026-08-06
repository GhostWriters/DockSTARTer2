package strutil

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d      time.Duration
		layout string
		want   string
	}{
		// Tenths only
		{1*time.Second + 200*time.Millisecond, "5.0s", "1.2s"},
		{9*time.Second + 100*time.Millisecond, "5.0s", "9.1s"},
		{42*time.Second + 700*time.Millisecond, "5.0s", "42.7s"},
		{59*time.Second + 900*time.Millisecond, "5.0s", "59.9s"},
		// Hundredths
		{1*time.Second + 230*time.Millisecond, "5.00s", "1.23s"},
		// Milliseconds
		{1*time.Second + 234*time.Millisecond, "5.000s", "1.234s"},
		// Minutes + zero-padded seconds
		{62*time.Second + 300*time.Millisecond, "4m05.0s", "1m02.3s"},
		{600*time.Second + 500*time.Millisecond, "4m05.0s", "10m00.5s"},
		// No minutes token — total seconds
		{62*time.Second + 300*time.Millisecond, "5.0s", "62.3s"},
		// Edge: just under a minute
		{59*time.Second + 990*time.Millisecond, "5.0s", "59.9s"},
		// Zero
		{0, "5.0s", "0.0s"},
		// Negative clamps to zero
		{-1 * time.Second, "5.0s", "0.0s"},
		// Space-padded seconds
		{9*time.Second + 100*time.Millisecond, "_5.0s", " 9.1s"},
		{42*time.Second + 700*time.Millisecond, "_5.0s", "42.7s"},
		// Zero-padded seconds
		{9*time.Second + 100*time.Millisecond, "05.0s", "09.1s"},
		// Zero-padded minutes
		{62*time.Second + 300*time.Millisecond, "04m05.0s", "01m02.3s"},
	}

	for _, tc := range cases {
		got := FormatDuration(tc.d, tc.layout)
		if got != tc.want {
			t.Errorf("FormatDuration(%v, %q) = %q, want %q", tc.d, tc.layout, got, tc.want)
		}
	}
}
