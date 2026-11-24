package movement

import (
	"log/slog"
	"testing"
)

// newTestMovementPacketSenderForBacklog builds a movementPacketSender with
// client left nil (this package's own "test mode" convention - see
// SendPosition's own doc comment) so recordBacklogSample's state machine
// can be exercised with literal pending-write counts, without needing a
// real network connection (mc-bot-go's own
// TestConn_PendingWrites_TracksBacklogAndDrainsToZero already covers the
// counter itself).
func newTestMovementPacketSenderForBacklog() *movementPacketSender {
	return &movementPacketSender{logger: slog.Default()}
}

// TestRecordBacklogSample_BelowThresholdNeverSkips guards the specific
// noise this mechanism was found producing live (2026-11-24): a live
// trace of ~630,000 streak starts showed 99.9% began at pendingWrites=1
// - the one packet normally in transit between a tick's enqueue and
// WriteLoop's own pull, not a real backlog - so every value below
// positionSendBacklogThreshold must be a complete no-op, not just a
// skip that happens not to log.
func TestRecordBacklogSample_BelowThresholdNeverSkips(t *testing.T) {
	me := newTestMovementPacketSenderForBacklog()

	for pending := int64(0); pending < positionSendBacklogThreshold; pending++ {
		if me.recordBacklogSample(pending) {
			t.Fatalf("pending=%d must not skip (threshold is %d)", pending, positionSendBacklogThreshold)
		}
		if me.backlog.consecutiveSkips != 0 {
			t.Fatalf("pending=%d: consecutiveSkips = %d, want 0 - below-threshold samples must never start a streak", pending, me.backlog.consecutiveSkips)
		}
	}
}

// TestRecordBacklogSample_SkipsWhilePendingThenClears guards the core
// state machine added chasing a live "kicked for floating too long" kick
// (2026-11-24, docs/bugs): a backlog at or above the threshold skips,
// and a return below it clears the streak rather than leaving it stuck.
func TestRecordBacklogSample_SkipsWhilePendingThenClears(t *testing.T) {
	me := newTestMovementPacketSenderForBacklog()

	if !me.recordBacklogSample(positionSendBacklogThreshold + 1) {
		t.Fatal("pending at/above threshold must skip")
	}
	if me.backlog.consecutiveSkips != 1 {
		t.Fatalf("consecutiveSkips = %d, want 1 after the first backlogged sample", me.backlog.consecutiveSkips)
	}

	if !me.recordBacklogSample(positionSendBacklogThreshold) {
		t.Fatal("a still-at/above-threshold pending count must keep skipping")
	}
	if me.backlog.consecutiveSkips != 2 {
		t.Fatalf("consecutiveSkips = %d, want 2 after a second consecutive backlogged sample", me.backlog.consecutiveSkips)
	}

	if me.recordBacklogSample(0) {
		t.Fatal("pending dropping back below threshold must stop skipping")
	}
	if me.backlog.consecutiveSkips != 0 {
		t.Fatalf("consecutiveSkips = %d, want 0 once the backlog clears", me.backlog.consecutiveSkips)
	}
}

// TestRecordBacklogSample_NewStreakStartsCleanAfterClearing guards
// against a streak started before a clear leaking into the next one -
// each new run of backlogged samples must start its own count from 1.
func TestRecordBacklogSample_NewStreakStartsCleanAfterClearing(t *testing.T) {
	me := newTestMovementPacketSenderForBacklog()

	for i := 0; i < 5; i++ {
		me.recordBacklogSample(positionSendBacklogThreshold)
	}
	if me.backlog.consecutiveSkips != 5 {
		t.Fatalf("consecutiveSkips = %d, want 5 after five consecutive backlogged samples", me.backlog.consecutiveSkips)
	}

	me.recordBacklogSample(0) // clears the streak

	if !me.recordBacklogSample(positionSendBacklogThreshold) {
		t.Fatal("pending at/above threshold must skip again after a clear")
	}
	if me.backlog.consecutiveSkips != 1 {
		t.Fatalf("consecutiveSkips = %d, want 1 - a new streak must not continue the old count", me.backlog.consecutiveSkips)
	}
}

// TestSkipIfBacklogged_NilClientNeverSkips guards the existing "client
// nil means test mode" convention every other test in this package
// relies on (see e.g. TestMoveTowards_FlatGround): skipIfBacklogged must
// be a complete no-op whenever client is nil, regardless of whatever
// backlog state recordBacklogSample might otherwise report, so adding
// this guard to SendPosition/SendPositionAndRotation/SendRotation can't
// change behavior for any caller that constructs a sender without a real
// connection.
func TestSkipIfBacklogged_NilClientNeverSkips(t *testing.T) {
	me := newTestMovementPacketSenderForBacklog()
	me.backlog.consecutiveSkips = 7 // pretend a streak is already in progress

	if me.skipIfBacklogged() {
		t.Fatal("skipIfBacklogged must never skip when client is nil (test mode)")
	}
}
