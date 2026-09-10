package agent

import (
	"context"
	"testing"
	"time"
)

// TestFindVisibleBlockTimedOutDistinguishesOwnTimeoutFromCallerCancellation
// verifies the fix for a real live-run hang (2026-09-10): FindVisibleBlock's
// own findVisibleBlockTimeout expiring must read as an ordinary "not found"
// (findVisibleBlockTimedOut == true), while the caller's own context dying
// must still be treated as a real error (findVisibleBlockTimedOut == false)
// — see findVisibleBlockTimedOut's own doc comment.
func TestFindVisibleBlockTimedOutDistinguishesOwnTimeoutFromCallerCancellation(t *testing.T) {
	callerCtx := context.Background()
	ctx, cancel := context.WithTimeout(callerCtx, 0) // already expired
	defer cancel()
	<-ctx.Done()

	if !findVisibleBlockTimedOut(ctx, callerCtx) {
		t.Fatalf("findVisibleBlockTimedOut = false, want true (ctx expired, callerCtx untouched)")
	}

	callerCtx2, cancelCaller := context.WithCancel(context.Background())
	cancelCaller()
	ctx2, cancel2 := context.WithTimeout(callerCtx2, time.Hour)
	defer cancel2()

	if findVisibleBlockTimedOut(ctx2, callerCtx2) {
		t.Fatalf("findVisibleBlockTimedOut = true, want false (callerCtx itself was cancelled — a real error)")
	}

	if findVisibleBlockTimedOut(callerCtx, callerCtx) {
		t.Fatalf("findVisibleBlockTimedOut = true, want false (neither context is cancelled)")
	}
}
