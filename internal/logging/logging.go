package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logging configuration.
type Config struct {
	Dir        string // Directory for log files
	MaxSizeMB  int    // Max size per log file in MB (default: 10)
	MaxBackups int    // Max number of old log files to keep (default: 3)
	MaxAgeDays int    // Max age in days to keep old log files (default: 28)
	Compress   bool   // Whether to compress old log files (default: true)
	Debug      bool   // Enable debug level logging
}

// DefaultConfig returns sensible defaults.
func DefaultConfig(logDir string) Config {
	return Config{
		Dir:        logDir,
		MaxSizeMB:  10,
		MaxBackups: 3,
		MaxAgeDays: 28,
		Compress:   true,
		Debug:      false,
	}
}

// Logger wraps slog with our configuration.
type Logger struct {
	*slog.Logger
	lumberjack *lumberjack.Logger
	cmdWriter  io.Writer
}

var defaultLogger *Logger

// Init initializes the global logger.
func Init(cfg Config) error {
	if err := os.MkdirAll(cfg.Dir, 0755); err != nil {
		return err
	}

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(cfg.Dir, "coop.log"),
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAgeDays,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	level := slog.LevelInfo
	if cfg.Debug {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(lj, &slog.HandlerOptions{
		Level: level,
	})

	defaultLogger = &Logger{
		Logger:     slog.New(handler),
		lumberjack: lj,
		cmdWriter:  lj,
	}

	return nil
}

// Get returns the global logger.
func Get() *Logger {
	if defaultLogger == nil {
		// Fallback to stderr if not initialized
		return &Logger{
			Logger:    slog.Default(),
			cmdWriter: os.Stderr,
		}
	}
	return defaultLogger
}

// Close closes the log file.
func Close() error {
	if defaultLogger != nil && defaultLogger.lumberjack != nil {
		return defaultLogger.lumberjack.Close()
	}
	return nil
}

// CmdWriter returns a writer for capturing command output.
// This can be used as Stdout/Stderr for exec.Command.
func (l *Logger) CmdWriter() io.Writer {
	return l.cmdWriter
}

// MultiWriter returns a writer that writes to both the log and the provided writer.
// Useful for showing output to user while also logging it.
func (l *Logger) MultiWriter(w io.Writer) io.Writer {
	if l.cmdWriter == nil {
		return w
	}
	return io.MultiWriter(w, l.cmdWriter)
}

// Cmd logs a command execution.
func (l *Logger) Cmd(name string, args []string) {
	l.Info("executing command", "cmd", name, "args", args)
}

// CmdOutput logs command output.
func (l *Logger) CmdOutput(name string, output []byte, err error) {
	if err != nil {
		l.Error("command failed", "cmd", name, "error", err, "output", string(output))
	} else {
		l.Debug("command succeeded", "cmd", name, "output", string(output))
	}
}

// CmdStart logs the start of a command that will stream output.
func (l *Logger) CmdStart(name string, args []string) {
	l.Info("starting command", "cmd", name, "args", args)
}

// CmdEnd logs the end of a streaming command.
func (l *Logger) CmdEnd(name string, err error) {
	if err != nil {
		l.Error("command failed", "cmd", name, "error", err)
	} else {
		l.Debug("command completed", "cmd", name)
	}
}
