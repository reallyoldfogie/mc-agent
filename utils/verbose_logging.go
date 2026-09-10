package utils

import (
	"os"
	"strings"
	"sync"
)

// verboseLoggingOnce/verboseLoggingValue cache MC_AGENT_VERBOSE_LOG for the
// life of the process — read once via sync.Once, not once per call, since
// every call site VerboseLoggingEnabled gates is a hot path (once per
// physics tick or more often).
var (
	verboseLoggingOnce  sync.Once
	verboseLoggingValue bool
)

// VerboseLoggingEnabled reports whether MC_AGENT_VERBOSE_LOG opts into
// high-frequency, per-tick/per-packet diagnostic logging (packet sends,
// physics ticks, position queries, block-shape lookups, line-of-sight
// failures, ...) that's silent by default.
//
// Found necessary, not merely tidy: several of these call sites used to log
// unconditionally at Info level. Over any real movement-heavy session — a
// live RL training run in particular, which calls FindVisibleBlock/
// GetPosition/collision queries far more often than an interactive chat
// session ever would — that compounded into gigabytes of log output that
// dominated wall-clock time and drowned out the actually useful log lines
// (2026-09-09). Set MC_AGENT_VERBOSE_LOG=1 to get all of it back for
// hands-on debugging of movement/physics/packet issues specifically; leave
// it unset for normal operation, including normal training runs.
func VerboseLoggingEnabled() bool {
	verboseLoggingOnce.Do(func() {
		verboseLoggingValue = envBool("MC_AGENT_VERBOSE_LOG")
	})
	return verboseLoggingValue
}

// envBool reports whether the named environment variable is set to a
// truthy value — "1", "true", "yes", "on" (case-insensitive). Mirrors
// pathfinding's own (now-retired in favor of this) identically-behaved
// helper, and agent's MC_AGENT_MEM_STATS precedent.
func envBool(key string) bool {
	val := strings.TrimSpace(os.Getenv(key))
	switch strings.ToLower(val) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
