package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"DockSTARTer2/internal/console"
)

// captureHandler records each record's message and attributes.
type captureHandler struct{ got []string }

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	s := r.Message
	r.Attrs(func(a slog.Attr) bool { s += " " + a.Key + "=" + a.Value.String(); return true })
	h.got = append(h.got, s)
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// withHandlers makes handlers the default logger's for the test.
func withHandlers(t *testing.T, handlers ...slog.Handler) {
	t.Helper()
	prev := slog.Default()
	slog.SetDefault(slog.New(&FanoutHandler{handlers: handlers}))
	t.Cleanup(func() { slog.SetDefault(prev) })
}

func TestLinesGiveTheFileTheirASCIIVersions(t *testing.T) {
	file := &captureHandler{}
	withHandlers(t, &TagProcessorHandler{base: file, mode: "strip"})
	Lines(context.Background(), LevelNotice, []string{"┌─┐", "│✓│", "plain"}, []string{"+-+", "|+|", "plain"})
	if want := "+-+,|+|,plain"; strings.Join(file.got, ",") != want {
		t.Errorf("file got %q, want the ASCII versions and no %s attribute", file.got, asciiAttr)
	}

	file.got = nil
	Lines(context.Background(), LevelNotice, []string{"┌─┐", "│✓│"}, []string{"+-+"})
	if strings.Join(file.got, ",") != "┌─┐,│✓│" {
		t.Errorf("with ASCII versions of the wrong length, file got %q, want the lines", file.got)
	}

	file.got = nil
	Lines(context.Background(), LevelNotice, []string{"100% done"}, nil)
	if strings.Join(file.got, ",") != "100% done" {
		t.Errorf("file got %q, want the line as written", file.got)
	}
}

// The log panels get both versions, each to pick by its session's setting; a
// program box's writer gets the version its session's setting picks.
func TestLinesGiveEachOutputItsVersion(t *testing.T) {
	console.SetConnTypeDisplay("test-lines", console.ConnTypeDisplay{LineCharacters: true})
	console.SetConnTypeDisplay("test-ascii", console.ConnTypeDisplay{LineCharacters: false})
	ch, unsubscribe := SubscribeLogLines()
	defer unsubscribe()
	withHandlers(t, &TUIHandler{level: LevelDebug, global: true}, &TUIHandler{level: LevelDebug})

	for _, tc := range []struct{ connType, want string }{{"test-lines", "│✓│"}, {"test-ascii", "|+|"}} {
		var w bytes.Buffer
		ctx := console.WithTUIWriter(console.WithConnType(context.Background(), tc.connType), &w)
		Lines(ctx, LevelNotice, []string{"│✓│"}, []string{"|+|"})
		if got := w.String(); !strings.HasSuffix(got, tc.want+"\n") {
			t.Errorf("%s writer got %q, want it to end %q", tc.connType, got, tc.want)
		}
		line := receive(t, ch)
		if !strings.HasSuffix(line.For(true), "│✓│") || !strings.HasSuffix(line.For(false), "|+|") {
			t.Errorf("%s: published %+v, want both versions", tc.connType, line)
		}
	}
	Lines(context.Background(), LevelNotice, []string{"no ASCII version"}, nil)
	if line := receive(t, ch); line.ASCII != "" || !strings.HasSuffix(line.For(false), "no ASCII version") {
		t.Errorf("published %+v, want the line as written, with line characters on or off", line)
	}
}

// receive returns the next line published to ch, failing when none comes.
func receive(t *testing.T, ch <-chan LogLine) LogLine {
	t.Helper()
	select {
	case line := <-ch:
		return line
	case <-time.After(time.Second):
		t.Fatal("no line published")
		return LogLine{}
	}
}

func TestLinesSkipDisabledLevels(t *testing.T) {
	file := &captureHandler{}
	withHandlers(t, &TagProcessorHandler{base: slogLevel{file, LevelWarn}, mode: "strip"})
	Lines(context.Background(), LevelNotice, []string{"quiet"}, nil)
	if len(file.got) != 0 {
		t.Errorf("file got %q below its level", file.got)
	}
}

// slogLevel is a handler enabled from its level up.
type slogLevel struct {
	*captureHandler
	level slog.Level
}

func (h slogLevel) Enabled(_ context.Context, l slog.Level) bool { return l >= h.level }
