package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"

	"github.com/reallyoldfogie/mc-agent/utils"
)

// packetLogger returns a generic packet handler that writes JSON logs to a.logw.
func (a *agent) packetLogger() bot.PacketHandler {
	ver := a.cfg.Version
	proto := a.cfg.ProtocolVersion
	return bot.PacketHandler{
		Priority: 0,
		F: func(p pk.Packet) error {
			if a.packetLogWriter != nil {
				name := ""
				if a.packetMgr != nil {
					name = a.packetMgr.ClientboundToString(protocol_models.ClientboundPacketID(p.ID))
				}
				pl := protocol_models.PacketLog{
					ID:              p.ID,
					Data:            make([]byte, len(p.Data)),
					Name:            name,
					Timestamp:       time.Now(),
					Version:         ver,
					ProtocolVersion: proto,
					Direction:       "clientbound",
					State:           "play",
					Source:          "agent",
				}
				copy(pl.Data, p.Data)
				if err := json.NewEncoder(a.packetLogWriter).Encode(pl); err != nil {
					a.log().Error("failed to write packet log", "error", err)
					return err
				}
			}
			return nil
		},
	}
}

// syncWriter is a mutex-guarded io.Writer whose target can be swapped at
// runtime. Each *agent owns exactly one, created once in New alongside its
// *slog.Logger, so the logger itself never needs to be reassigned (and can
// safely be read from any goroutine without a lock) — only the underlying
// writer moves between stdout-only and file+stdout as setupLogging and
// closeLogging run.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func newSyncWriter(w io.Writer) *syncWriter {
	return &syncWriter{w: w}
}

func (s *syncWriter) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(p)
}

func (s *syncWriter) setTarget(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.w = w
}

// newAgentLogger builds the per-agent *slog.Logger and its backing
// syncWriter, defaulting to stdout until setupLogging opens this instance's
// own log file. The "agent" field is attached once here so every line this
// logger (or anything derived from it) ever writes is attributable, even
// before setupLogging runs.
//
// The handler's level is Debug when utils.VerboseLoggingEnabled(), Info
// (the slog default) otherwise — this is what makes Debug-gated call sites
// like GetPosition/setPosition (tracking.go) and logLineOfSightFailure
// (actions.go) actually controllable by MC_AGENT_VERBOSE_LOG rather than
// permanently silent, since a bare nil *slog.HandlerOptions defaults to
// Info and filters Debug out unconditionally.
func newAgentLogger(name string) (*slog.Logger, *syncWriter) {
	w := newSyncWriter(os.Stdout)
	opts := &slog.HandlerOptions{}
	if utils.VerboseLoggingEnabled() {
		opts.Level = slog.LevelDebug
	}
	handler := slog.NewTextHandler(w, opts)
	return slog.New(handler).With("agent", name), w
}

// safeLogger falls back to slog.Default() for a nil logger. Package-level
// log.Printf never had a nil-receiver failure mode; this preserves that for
// the (mostly test-only) code paths that build an *agent, or one of the
// small handful of types that borrow its logger, as a bare struct literal
// rather than through New().
func safeLogger(l *slog.Logger) *slog.Logger {
	if l == nil {
		return slog.Default()
	}
	return l
}

// log returns a.logger, or slog.Default() if this *agent was constructed
// without going through New() (as some tests do) and so never got one.
func (a *agent) log() *slog.Logger {
	return safeLogger(a.logger)
}

// setupLogging opens this agent's own timestamped log file and points its
// logger at file+stdout. This is called automatically from Init() and
// should not be called by clients.
//
// Unlike the previous package-global log.SetOutput approach, this only ever
// touches this *agent's own syncWriter, so two agents running concurrently
// in the same process each get their own complete, attributable log file
// instead of racing to reconfigure one shared logger.
func (a *agent) setupLogging() error {
	a.loggingMu.Lock()
	defer a.loggingMu.Unlock()

	// Already setup?
	if a.logFile != nil {
		return nil
	}

	// Find or create cache directory
	cacheDir, err := utils.FindOrCreateCacheDir()
	if err != nil {
		return fmt.Errorf("find cache directory: %w", err)
	}

	// Create log directory
	agentLogsDir := filepath.Join(cacheDir, "logs", "agents")
	if err := os.MkdirAll(agentLogsDir, 0760); err != nil {
		return fmt.Errorf("create agent logs directory: %w", err)
	}

	// Create log file with agent name + timestamp, so concurrent agents in
	// the same process don't collide on one file.
	logFileName := filepath.Join(agentLogsDir, "agent_"+sanitizeLogFileName(a.cfg.Name)+"_"+time.Now().Format("20060102_150405")+".log")
	file, err := os.Create(logFileName)
	if err != nil {
		return fmt.Errorf("create agent log file: %w", err)
	}

	// Write to both the file and stdout.
	a.logWriter.setTarget(io.MultiWriter(file, os.Stdout))
	a.logFile = file

	fmt.Printf("Agent logging enabled: %s (also to console)\n", logFileName)
	return nil
}

// closeLogging closes this agent's log file and points its logger back at
// stdout only. This is called automatically from Close() and should not be
// called by clients.
//
// Because the writer swap is scoped to this *agent (via a.logWriter),
// closing one agent's log can never redirect a different, still-running
// agent's output — the failure mode the old global closeAgentLog() had.
func (a *agent) closeLogging() {
	a.loggingMu.Lock()
	defer a.loggingMu.Unlock()

	if a.logFile != nil {
		a.logWriter.setTarget(os.Stdout)
		a.logFile.Close()
		a.logFile = nil
	}
}

// sanitizeLogFileName strips characters that would be awkward in a path
// segment. Player names are typically alphanumeric/underscore, but this
// guards offline-mode or non-vanilla names that might not be.
func sanitizeLogFileName(name string) string {
	if name == "" {
		return "agent"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '_'
		}
	}, name)
}

// logf mirrors log.Printf's signature. It exists so the package's many
// pre-existing call sites could move off the global "log" package onto
// this agent's own fielded slog.Logger mechanically; new call sites should
// prefer a.logger.Info/Warn/Error directly with structured key/value fields
// instead of a formatted message.
func (a *agent) logf(format string, args ...any) {
	a.log().Info(fmt.Sprintf(format, args...))
}

// logln mirrors log.Println's signature: operands are space-separated, and
// (unlike fmt.Sprintln) no trailing newline is kept since the handler adds
// its own record terminator.
func (a *agent) logln(args ...any) {
	a.log().Info(strings.TrimSuffix(fmt.Sprintln(args...), "\n"))
}

// logp mirrors log.Print's signature.
func (a *agent) logp(args ...any) {
	a.log().Info(fmt.Sprint(args...))
}
