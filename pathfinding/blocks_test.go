package pathfinding

import (
	"bytes"
	"log"
	"testing"

	mdl "github.com/reallyoldfogie/mc-data-gen/loader"
)

// captureLogOutput redirects the standard library's shared log output to a
// buffer for the duration of the test, restoring it on cleanup — needed
// because blockShapeManager logs via the plain "log" package (see its own
// verbose field's doc comment), not a per-instance logger.
func captureLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// TestBlockShapeManagerGetInfoSkipsLoggingByDefault verifies the fix for a
// real live-run performance bug (2026-09-09, found immediately after fixing
// an analogous FindVisibleBlock/LOS logging issue): getInfo/
// blockInfoFromStateID used to log unconditionally, including on their
// ordinary success path, and both are called from every collision/
// passability query — hot paths invoked constantly during pathfinding,
// movement, and physics. That compounded into gigabytes of log output and
// dominated a live RL training run's wall-clock time even after the
// separate LOS fix landed. Default (verbose=false) must now be silent, on
// both the found and not-found paths.
func TestBlockShapeManagerGetInfoSkipsLoggingByDefault(t *testing.T) {
	buf := captureLogOutput(t)
	bsm := &blockShapeManager{
		shapeData: map[mdl.StateKey]mdl.ShapeInfo{
			{BlockID: "minecraft:stone", PropsKey: ""}: {},
		},
		verbose: false,
	}

	bsm.getInfo("minecraft:stone", nil)   // found path
	bsm.getInfo("minecraft:unknown", nil) // not-found path

	if buf.Len() != 0 {
		t.Fatalf("getInfo logged %d bytes with verbose=false, want 0: %s", buf.Len(), buf.String())
	}
}

// TestBlockShapeManagerGetInfoLogsWhenVerbose verifies the diagnostic is
// still available, not simply deleted, when explicitly enabled
// (MC_AGENT_BLOCKSHAPE_DEBUG at construction — see NewBlockShapeManager).
func TestBlockShapeManagerGetInfoLogsWhenVerbose(t *testing.T) {
	buf := captureLogOutput(t)
	bsm := &blockShapeManager{
		shapeData: map[mdl.StateKey]mdl.ShapeInfo{
			{BlockID: "minecraft:stone", PropsKey: ""}: {},
		},
		verbose: true,
	}

	bsm.getInfo("minecraft:stone", nil)

	if !bytes.Contains(buf.Bytes(), []byte("[BlockShapeManager.getInfo] Found StateKey")) {
		t.Fatalf("getInfo with verbose=true wrote nothing matching the expected message: %s", buf.String())
	}
}

// TestBlockShapeManagerBlockInfoFromStateIDSkipsLoggingByDefault mirrors
// TestBlockShapeManagerGetInfoSkipsLoggingByDefault for
// blockInfoFromStateID's own nil-blockMgr early-return path (the cheapest
// way to exercise it without a real mc_versions.BlockMgr).
func TestBlockShapeManagerBlockInfoFromStateIDSkipsLoggingByDefault(t *testing.T) {
	buf := captureLogOutput(t)
	bsm := &blockShapeManager{verbose: false}

	bsm.blockInfoFromStateID(1)

	if buf.Len() != 0 {
		t.Fatalf("blockInfoFromStateID logged %d bytes with verbose=false, want 0: %s", buf.Len(), buf.String())
	}
}
