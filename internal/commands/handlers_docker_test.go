package commands

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// A log stream cut by canceling its context (Ctrl+C on -F) ends cleanly whatever the
// read failed with; one that fails while still wanted is an error.
func TestLogStreamEnd(t *testing.T) {
	live := context.Background()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	cut := errors.New("net/http: request canceled")

	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want error
	}{
		{"ended", live, nil, nil},
		{"canceled, no error", canceled, nil, nil},
		{"canceled, cut read", canceled, cut, nil},
		{"wrapped context.Canceled", live, fmt.Errorf("read: %w", context.Canceled), nil},
		{"failed while wanted", live, cut, cut},
	} {
		if got := logStreamEnd(tc.ctx, tc.err); !errors.Is(got, tc.want) || (got == nil) != (tc.want == nil) {
			t.Errorf("%s: logStreamEnd = %v, want %v", tc.name, got, tc.want)
		}
	}
}
