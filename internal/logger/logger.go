package logger

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type Level slog.Level

const (
	LevelDebug = Level(slog.LevelDebug)
	LevelInfo  = Level(slog.LevelInfo)
	LevelWarn  = Level(slog.LevelWarn)
	LevelError = Level(slog.LevelError)
)

type Logger struct {
	*slog.Logger
	file *os.File
}

func New(path string, debug bool) (*Logger, error) {
	var w io.Writer = io.Discard
	var f *os.File

	if debug && path != "" {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
		var err error
		f, err = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		w = f
	}

	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	if debug {
		opts.ReplaceAttr = func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.SourceKey {
				if source, ok := a.Value.Any().(*slog.Source); ok {
					source.File = filepath.Base(source.File)
				}
			}
			return a
		}
	}

	var handler slog.Handler
	if debug {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	slog.SetDefault(slog.New(handler))

	log.SetOutput(io.Discard)
	log.SetPrefix("netpulse: ")

	l := &Logger{
		Logger: slog.New(handler),
		file:   f,
	}

	return l, nil
}

func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) Debugf(format string, args ...any) {
	if !l.Logger.Enabled(context.Background(), slog.LevelDebug) {
		return
	}
	l.LogAttrs(context.Background(), slog.LevelDebug, format, slog.Any("args", args))
}

func (l *Logger) Infof(format string, args ...any) {
	l.LogAttrs(context.Background(), slog.LevelInfo, format, slog.Any("args", args))
}

func (l *Logger) Warnf(format string, args ...any) {
	l.LogAttrs(context.Background(), slog.LevelWarn, format, slog.Any("args", args))
}

func (l *Logger) Errorf(format string, args ...any) {
	l.LogAttrs(context.Background(), slog.LevelError, format, slog.Any("args", args))
}

func (l *Logger) Fatal(msg string, err error) {
	l.LogAttrs(context.Background(), slog.LevelError, msg, slog.String("error", err.Error()))
	os.Exit(1)
}

func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger: l.Logger.With(slog.String("component", component)),
	}
}

func (l *Logger) WithError(err error) *Logger {
	return &Logger{
		Logger: l.Logger.With(slog.String("error", err.Error())),
	}
}

func StartMemoryLogger(ctx context.Context) *slog.Logger {
	memHandler := newMemoryHandler()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				memHandler.logger.LogAttrs(ctx, slog.LevelDebug, "memory",
					slog.Uint64("alloc_mb", m.Alloc/1024/1024),
					slog.Uint64("total_alloc_mb", m.TotalAlloc/1024/1024),
					slog.Uint64("sys_mb", m.Sys/1024/1024),
					slog.Int("goroutines", runtime.NumGoroutine()),
				)
			case <-ctx.Done():
				return
			}
		}
	}()
	return slog.New(memHandler)
}

type memoryHandler struct {
	slog.Handler
	logger *slog.Logger
}

func newMemoryHandler() *memoryHandler {
	h := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug})
	return &memoryHandler{Handler: h}
}
