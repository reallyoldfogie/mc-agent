package utils

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// LevelDebugVerbose is a custom slog level below slog.LevelDebug (-4), for
// extra-fine, per-tick/per-packet diagnostic logging (packet sends, physics
// ticks, position queries, block-shape lookups, line-of-sight failures,
// ...) that would otherwise flood normal debug output.
//
// Replaces the old MC_AGENT_VERBOSE_LOG env-var gate (a bare bool checked
// at every hot-path call site via the now-removed VerboseLoggingEnabled):
// with a per-agent *slog.Logger threaded through every package (see
// docs/bugs/global-log-output-not-per-agent.md Phase 2/3), the handler's
// own configured level already decides whether a line is emitted, the same
// way it already does for Debug/Info/Warn/Error — so call sites just log
// at this level instead of wrapping themselves in an env-var check. The
// level itself is configured via config.LoggingSettings.Level (JSON/ENV/CLI,
// same pattern as every other setting in the config package), not read
// directly from an environment variable by each call site.
const LevelDebugVerbose = slog.Level(-8)

// levelNames maps this package's level vocabulary to slog.Level, used by
// both ParseLevel (config -> slog.Level) and ReplaceDebugVerboseLevelAttr
// (slog.Level -> display string). Kept as one table so the two stay in
// sync by construction.
var levelNames = map[string]slog.Level{
	"debugverbose": LevelDebugVerbose,
	"debug":        slog.LevelDebug,
	"info":         slog.LevelInfo,
	"warn":         slog.LevelWarn,
	"warning":      slog.LevelWarn,
	"error":        slog.LevelError,
}

// ParseLevel parses a config.LoggingSettings.Level string (case-insensitive;
// "debugverbose", "debug", "info", "warn"/"warning", "error") into a
// slog.Level. An empty string returns slog.LevelInfo (slog's own default),
// matching config.Default()'s zero-value contract for an unset section.
func ParseLevel(level string) (slog.Level, error) {
	level = strings.ToLower(strings.TrimSpace(level))
	if level == "" {
		return slog.LevelInfo, nil
	}
	if l, ok := levelNames[level]; ok {
		return l, nil
	}
	return 0, fmt.Errorf("unknown log level %q (want debugverbose, debug, info, warn, or error)", level)
}

// ReplaceDebugVerboseLevelAttr is a slog.HandlerOptions.ReplaceAttr function
// that renders LevelDebugVerbose as "DEBUGVERBOSE" instead of slog's default
// "DEBUG-4". Every slog handler in this codebase that might be configured
// down to LevelDebugVerbose should set this, so a mixed-level log stream
// reads clearly.
func ReplaceDebugVerboseLevelAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.LevelKey {
		if level, ok := a.Value.Any().(slog.Level); ok && level == LevelDebugVerbose {
			a.Value = slog.StringValue("DEBUGVERBOSE")
		}
	}
	return a
}

// DebugVerbose logs msg at LevelDebugVerbose through logger — the
// structured-logging call sites throughout the codebase use instead of the
// former "if VerboseLoggingEnabled() { log.Printf(...) }" pattern. A nil
// logger (a struct built as a bare literal rather than through its own
// New* constructor, mostly in tests) falls back to slog.Default() rather
// than panicking — see SafeLogger.
func DebugVerbose(logger *slog.Logger, msg string, args ...any) {
	SafeLogger(logger).Log(context.Background(), LevelDebugVerbose, msg, args...)
}

// DebugVerboseEnabled reports whether logger is currently configured to
// emit LevelDebugVerbose records. Call sites that build an expensive
// diagnostic payload (a full state dump, a formatted table) rather than a
// cheap message should guard the work with this first, the same way they
// used to guard it with VerboseLoggingEnabled() — a nil logger (the
// zero-value/uninitialized case some constructors still allow) is treated
// as not-enabled rather than panicking.
func DebugVerboseEnabled(logger *slog.Logger) bool {
	if logger == nil {
		return false
	}
	return logger.Enabled(context.Background(), LevelDebugVerbose)
}

// SafeLogger returns logger, or slog.Default() if logger is nil — the
// common fallback for constructors across the codebase that accept an
// injected *slog.Logger but may still be built as a bare struct literal
// (mostly in tests) rather than through their own New* constructor.
func SafeLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
