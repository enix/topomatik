package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/lmittmann/tint"
	"golang.org/x/term"
)

type contextKey struct{}

// NewContext returns a context with the given logger attached.
func NewContext(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext returns the logger stored in ctx, or slog.Default() if none.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

type Format int

const (
	FormatJSON Format = iota
	FormatText
)

func (f Format) String() string {
	switch f {
	case FormatJSON:
		return "json"
	case FormatText:
		return "text"
	default:
		return "json"
	}
}

func (f *Format) Set(s string) error {
	switch strings.ToLower(s) {
	case "json":
		*f = FormatJSON
	case "text":
		*f = FormatText
	default:
		return fmt.Errorf("unknown log format: %q (expected \"json\" or \"text\")", s)
	}
	return nil
}

// Level wraps slog.Level to implement flag.Value.
type Level struct {
	slog.Level
}

func (l *Level) Set(s string) error {
	return l.UnmarshalText([]byte(s))
}

func Setup(format Format, level Level) {
	opts := &slog.HandlerOptions{Level: level.Level}

	var handler slog.Handler
	switch format {
	case FormatJSON:
		handler = slog.NewJSONHandler(os.Stderr, opts)
	case FormatText:
		if term.IsTerminal(int(os.Stderr.Fd())) {
			handler = tint.NewHandler(os.Stderr, &tint.Options{Level: level.Level})
		} else {
			handler = slog.NewTextHandler(os.Stderr, opts)
		}
	}

	slog.SetDefault(slog.New(handler))
}
