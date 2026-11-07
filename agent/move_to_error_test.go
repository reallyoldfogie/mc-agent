package agent

import (
	"context"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// TestMoveTo_QuietPathfindingFailurePropagatesError reproduces the bug
// found live, 2026-11-07: MoveTo's own pathfinding-failure return used to
// live inside `if notifyChat`, so the RL dispatch path (MoveToQuiet,
// notifyChat=false) silently returned nil instead of the real error - a
// dispatched goto to a genuinely unreachable target "succeeded" every
// single time it was retried, with nothing downstream ever seeing an
// error to act on.
func TestMoveTo_QuietPathfindingFailurePropagatesError(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a := agentInt.(*agent)
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	a.UpdatePosition(models.V3{}, 0, 0)
	a.SetMovementExecutor(&fakeMoveExec{})
	a.SetPathFinder(fakePFFail{})

	err = a.MoveTo(context.Background(), 5, 0, 0, false)
	if err == nil {
		t.Fatal("MoveTo(notifyChat=false) returned nil after a pathfinding failure, want a non-nil error")
	}
}

// TestMoveTo_ChattyPathfindingFailurePropagatesErrorToo confirms the
// notifyChat=true path, already correct before this fix, is unchanged -
// it both notifies chat and returns the error.
func TestMoveTo_ChattyPathfindingFailurePropagatesErrorToo(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a := agentInt.(*agent)
	if err := a.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	a.UpdatePosition(models.V3{}, 0, 0)
	a.SetMovementExecutor(&fakeMoveExec{})
	a.SetPathFinder(fakePFFail{})
	capture := newCaptureChat()
	a.SetChat(capture)

	err = a.MoveTo(context.Background(), 5, 0, 0, true)
	if err == nil {
		t.Fatal("MoveTo(notifyChat=true) returned nil after a pathfinding failure, want a non-nil error")
	}
	time.Sleep(10 * time.Millisecond)
	if !containsMsg(capture.GetMessages(), "Pathfinding failed") {
		t.Fatalf("expected a pathfinding-failed chat message, got %v", capture.GetMessages())
	}
}
