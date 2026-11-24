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

// TestRecordBacklogSample_SkipsWhilePendingThenClears guards the core
// state machine added chasing a live "kicked for floating too long" kick
// (2026-11-24, docs/bugs): zero pending writes never skips, any nonzero
// backlog does, and a return to zero clears the streak rather than
// leaving it stuck.
func TestRecordBacklogSample_SkipsWhilePendingThenClears(t *testing.T) {
	me := newTestMovementPacketSenderForBacklog()

	if me.recordBacklogSample(0) {
		t.Fatal("pending=0 must never skip")
	}
	if me.backlog.consecutiveSkips != 0 {
		t.Fatalf("consecutiveSkips = %d, want 0 when nothing has ever backed up", me.backlog.consecutiveSkips)
	}

	if !me.recordBacklogSample(3) {
		t.Fatal("pending>0 must skip")
	}
	if me.backlog.consecutiveSkips != 1 {
		t.Fatalf("consecutiveSkips = %d, want 1 after the first backlogged sample", me.backlog.consecutiveSkips)
	}

	if !me.recordBacklogSample(2) {
		t.Fatal("a still-nonzero pending count must keep skipping")
	}
	if me.backlog.consecutiveSkips != 2 {
		t.Fatalf("consecutiveSkips = %d, want 2 after a second consecutive backlogged sample", me.backlog.consecutiveSkips)
	}

	if me.recordBacklogSample(0) {
		t.Fatal("pending returning to 0 must stop skipping")
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
		me.recordBacklogSample(1)
	}
	if me.backlog.consecutiveSkips != 5 {
		t.Fatalf("consecutiveSkips = %d, want 5 after five consecutive backlogged samples", me.backlog.consecutiveSkips)
	}

	me.recordBacklogSample(0) // clears the streak

	if !me.recordBacklogSample(1) {
		t.Fatal("pending>0 must skip again after a clear")
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
