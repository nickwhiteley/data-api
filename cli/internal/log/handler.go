package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// wdHandler is a slog.Handler that writes [wd-extract] prefixed lines to stderr.
type wdHandler struct {
	attrs []slog.Attr
}

// New returns a *slog.Logger that writes [wd-extract] lines to stderr at INFO level.
func New() *slog.Logger {
	return slog.New(&wdHandler{})
}

func (h *wdHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *wdHandler) Handle(_ context.Context, r slog.Record) error {
	level := "INFO"
	if r.Level >= slog.LevelError {
		level = "ERROR"
	} else if r.Level >= slog.LevelWarn {
		level = "WARN"
	}
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value)
		return true
	})
	for _, a := range h.attrs {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value)
	}
	_, err := fmt.Fprintf(os.Stderr, "[wd-extract] %s %s\n", level, msg)
	return err
}

func (h *wdHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(combined, h.attrs)
	copy(combined[len(h.attrs):], attrs)
	return &wdHandler{attrs: combined}
}

func (h *wdHandler) WithGroup(_ string) slog.Handler {
	return h
}
