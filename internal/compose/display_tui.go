package compose

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/compose/v5/pkg/api"
)

// tuiEventProcessor implements api.EventProcessor for TUI contexts.
// It writes each event as a plain line directly to the writer so output
// streams in real time rather than waiting for the SDK to flush progress bars.
type tuiEventProcessor struct {
	out io.Writer
}

func (p *tuiEventProcessor) Start(_ context.Context, _ string) {}
func (p *tuiEventProcessor) Done(_ string, _ bool)             {}

func (p *tuiEventProcessor) On(events ...api.Resource) {
	for _, e := range events {
		line := formatEvent(e)
		if line != "" {
			fmt.Fprintln(p.out, line)
		}
	}
}

// formatEvent converts a Resource event to a single log line.
// Skips intermediate Working events with no useful text to avoid noise.
func formatEvent(e api.Resource) string {
	id := strings.TrimSpace(e.ID)
	text := strings.TrimSpace(e.Text)
	details := strings.TrimSpace(e.Details)

	// Skip bare Working events with no text — they're progress-bar noise
	if e.Status == api.Working && text == "" {
		return ""
	}
	// Skip ResourceCompose container-level aggregates with no meaningful text
	if id == api.ResourceCompose && text == "" {
		return ""
	}

	parts := []string{}
	if id != "" && id != api.ResourceCompose {
		parts = append(parts, id)
	}
	if text != "" {
		parts = append(parts, text)
	}
	if details != "" {
		parts = append(parts, "("+details+")")
	}
	return strings.Join(parts, " ")
}
