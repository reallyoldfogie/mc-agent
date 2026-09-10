package agent

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

// TestLogLineOfSightFailureSkipsWorkByDefault verifies the fix for a real
// live-run performance bug (2026-09-09): logLineOfSightFailure used to log
// unconditionally at Info level, and FindVisibleBlock calls it once per
// candidate block that fails line of sight — for a common target block
// name, re-resolved every rlenv.Environment.Step, that compounded into
// gigabytes of log output and dominated a live RL training run's
// wall-clock time. It must now be silent (and skip collectLineOfSightBlocks'
// own real raycast cost) at the default log level.
func TestLogLineOfSightFailureSkipsWorkByDefault(t *testing.T) {
	var buf bytes.Buffer
	a := &agent{logger: slog.New(slog.NewTextHandler(&buf, nil))} // default level: Info

	a.logLineOfSightFailure(context.Background(), 0, 0, 0, 5, 5, 5)

	if buf.Len() != 0 {
		t.Fatalf("logLineOfSightFailure wrote %d bytes at the default log level, want 0 (silent unless Debug is enabled): %s", buf.Len(), buf.String())
	}
}

// TestLogLineOfSightFailureLogsWhenDebugEnabled verifies the diagnostic is
// still available, not simply deleted, for anyone who explicitly opts into
// Debug-level logging.
func TestLogLineOfSightFailureLogsWhenDebugEnabled(t *testing.T) {
	var buf bytes.Buffer
	a := &agent{logger: slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))}

	a.logLineOfSightFailure(context.Background(), 0, 0, 0, 5, 5, 5)

	if !bytes.Contains(buf.Bytes(), []byte("[LOS] failed")) {
		t.Fatalf("logLineOfSightFailure with Debug enabled wrote nothing matching \"[LOS] failed\": %s", buf.String())
	}
}
