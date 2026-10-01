package pathfinding

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/reallyoldfogie/mc-agent/utils"
	mdl "github.com/reallyoldfogie/mc-data-gen/loader"
)

// newTestLogger builds a *slog.Logger writing to a buffer at the given
// minimum level, for asserting on a blockShapeManager's own per-instance
// log output (see docs/bugs/global-log-output-not-per-agent.md) instead of
// the plain "log" package's process-global output.
func newTestLogger(level slog.Level) (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: level, ReplaceAttr: utils.ReplaceDebugVerboseLevelAttr})
	return slog.New(handler), &buf
}

// TestBlockShapeManagerGetInfoSkipsLoggingByDefault verifies the fix for a
// real live-run performance bug (2026-09-09, found immediately after fixing
// an analogous FindVisibleBlock/LOS logging issue): getInfo/
// blockInfoFromStateID used to log unconditionally, including on their
// ordinary success path, and both are called from every collision/
// passability query — hot paths invoked constantly during pathfinding,
// movement, and physics. That compounded into gigabytes of log output and
// dominated a live RL training run's wall-clock time even after the
// separate LOS fix landed. Default (Info level) must now be silent, on
// both the found and not-found paths, since these are logged at
// utils.LevelDebugVerbose.
func TestBlockShapeManagerGetInfoSkipsLoggingByDefault(t *testing.T) {
	logger, buf := newTestLogger(slog.LevelInfo)
	bsm := &blockShapeManager{
		shapeData: map[mdl.StateKey]mdl.ShapeInfo{
			{BlockID: "minecraft:stone", PropsKey: ""}: {},
		},
		logger: logger,
	}

	bsm.getInfo("minecraft:stone", nil)   // found path
	bsm.getInfo("minecraft:unknown", nil) // not-found path

	if buf.Len() != 0 {
		t.Fatalf("getInfo logged %d bytes at Info level, want 0: %s", buf.Len(), buf.String())
	}
}

// TestBlockShapeManagerGetInfoLogsWhenVerbose verifies the diagnostic is
// still available, not simply deleted, when the logger is configured down
// to utils.LevelDebugVerbose.
func TestBlockShapeManagerGetInfoLogsWhenVerbose(t *testing.T) {
	logger, buf := newTestLogger(utils.LevelDebugVerbose)
	bsm := &blockShapeManager{
		shapeData: map[mdl.StateKey]mdl.ShapeInfo{
			{BlockID: "minecraft:stone", PropsKey: ""}: {},
		},
		logger: logger,
	}

	bsm.getInfo("minecraft:stone", nil)

	if !bytes.Contains(buf.Bytes(), []byte("found StateKey")) {
		t.Fatalf("getInfo at LevelDebugVerbose wrote nothing matching the expected message: %s", buf.String())
	}
}

// TestBlockShapeManagerBlockInfoFromStateIDSkipsLoggingByDefault mirrors
// TestBlockShapeManagerGetInfoSkipsLoggingByDefault for
// blockInfoFromStateID's own nil-blockMgr early-return path (the cheapest
// way to exercise it without a real mc_versions.BlockMgr).
func TestBlockShapeManagerBlockInfoFromStateIDSkipsLoggingByDefault(t *testing.T) {
	logger, buf := newTestLogger(slog.LevelInfo)
	bsm := &blockShapeManager{logger: logger}

	bsm.blockInfoFromStateID(1)

	if buf.Len() != 0 {
		t.Fatalf("blockInfoFromStateID logged %d bytes at Info level, want 0: %s", buf.Len(), buf.String())
	}
}

// TestBlockShapeManagerBlockInfoFromStateIDMemoizes verifies the fix for a
// live-run CPU bug (2026-10-01, found via pprof on an RL training run):
// blockInfoFromStateID re-resolved its answer on every single call, and
// became the single hottest function in the whole binary (>75% of CPU
// time in one profile) once a separate pathfinding busy-wait bug was
// fixed - every passability/mining/water-flow/HPA-cluster-scan check
// funnels through it. It's cached per blockStateID now (bsm.blockInfoCache);
// the only externally-observable sign a repeat call actually hit the
// cache, rather than just recomputing an equal-looking result, is that it
// returns the exact same props map instance - so this mutates the map
// from the first call and confirms the second call's map reflects it,
// and that a different state ID gets its own independent entry rather
// than accidentally sharing one cache slot.
func TestBlockShapeManagerBlockInfoFromStateIDMemoizes(t *testing.T) {
	bsm := &blockShapeManager{}

	_, props1 := bsm.blockInfoFromStateID(5)
	props1["sentinel"] = "x"

	_, props2 := bsm.blockInfoFromStateID(5)
	if props2["sentinel"] != "x" {
		t.Fatalf("second call for state ID 5 got a different props map (got %v); blockInfoFromStateID is not memoizing", props2)
	}

	_, props3 := bsm.blockInfoFromStateID(6)
	if _, ok := props3["sentinel"]; ok {
		t.Fatalf("state ID 6 shared state ID 5's cached props map: %v", props3)
	}
}
