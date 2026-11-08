package movement

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// TestGenerateSidewaysRecoveryInputs_SticksToCommittedDirectionOnATie
// reproduces the bug found live, 2026-11-07
// (docs/plans/21-buried-mining-success.md): when the bot sits on the
// direct line to the target (here, straight ahead along +Z), moving
// sideways either way is symmetric by construction, so
// rightDistToTarget and leftDistToTarget always come out equal (modulo
// floating-point noise in a live run). The old code re-derived its
// choice from that comparison on every single call, which just chased
// that noise and flip-flopped almost every tick, never committing to
// either side long enough to actually clear an obstacle - seen live as
// an ordinary 2-step jump+descend move stuck retrying forever,
// identically, across every parallel environment at once. The fixed
// code must stick to pe.sidewaysDirection (this recovery attempt's own
// already-committed choice) in that tied case instead.
func TestGenerateSidewaysRecoveryInputs_SticksToCommittedDirectionOnATie(t *testing.T) {
	exec := createTestPhysicsExecutor()
	// Straight ahead along +Z from the bot's own starting position
	// (0, 64, 0) - a perfect right/left tie by construction.
	step := pathfinding.PathStep{
		Position: models.V3{X: 0, Y: 64, Z: 10},
		Movement: pathfinding.Traverse,
	}

	exec.sidewaysDirection = 1 // committed to RIGHT for this attempt
	var want *float64
	for i := 0; i < 5; i++ {
		inputs := exec.generateSidewaysRecoveryInputs(step)
		if inputs.ThrottleX == 0 {
			t.Fatalf("call %d: ThrottleX = 0, want a nonzero sideways component", i)
		}
		if want == nil {
			x := inputs.ThrottleX
			want = &x
		} else if (inputs.ThrottleX > 0) != (*want > 0) {
			t.Fatalf("call %d: ThrottleX sign flipped (now %.4f, first call was %.4f) - direction must stay committed across calls within one recovery attempt, not flip-flop on a tied comparison", i, inputs.ThrottleX, *want)
		}
	}

	// The opposite committed direction must produce the opposite sign,
	// confirming sidewaysDirection actually drives the choice (not just
	// "whatever it happened to pick first, by luck, stays stable").
	exec.sidewaysDirection = -1 // committed to LEFT
	leftInputs := exec.generateSidewaysRecoveryInputs(step)
	if (leftInputs.ThrottleX > 0) == (*want > 0) {
		t.Fatalf("flipping sidewaysDirection to -1 did not flip the chosen side: got ThrottleX=%.4f, want opposite sign of the RIGHT case's %.4f", leftInputs.ThrottleX, *want)
	}
}

// TestGenerateSidewaysRecoveryInputs_AvoidsRetreatingTowardPathStart
// reproduces the corner-turn bug found live, 2026-11-07/08
// (docs/plans/21-buried-mining-success.md): a bot that just traversed
// several blocks along X to line up with a corner, then got stuck
// turning onto a new, almost-pure-Z leg, had its sideways recovery
// alternate blindly between left/right on the resulting tie - with even
// odds of picking the direction that retraces the X corridor just
// walked, undoing all of that progress and forcing a near-complete
// re-path. The fixed code must prefer whichever side does not move
// toward pe.pathStartPos (where this path attempt began), regardless of
// which way pe.sidewaysDirection's alternation happens to be pointing.
func TestGenerateSidewaysRecoveryInputs_AvoidsRetreatingTowardPathStart(t *testing.T) {
	exec := createTestPhysicsExecutor()
	// Bot sits at (0,64,10), having walked here from (-5,64,10) along +X
	// (pathStartPos), and must now go straight ahead along +Z - a
	// perfect right/left tie, same as the committed-direction test above,
	// but this time pathStartPos sits behind along the sideways (X) axis
	// instead of being irrelevant (the default test's pathStartPos at the
	// zero value differs from pos only in Y, which swamps the tiny X
	// offset and keeps that tie exact - deliberately not the case here).
	exec.pathStartPos = models.V3{X: -5, Y: 64, Z: 10}
	step := pathfinding.PathStep{
		Position: models.V3{X: 0, Y: 64, Z: 20},
		Movement: pathfinding.Traverse,
	}

	for _, sidewaysDirection := range []int{1, -1} {
		exec.sidewaysDirection = sidewaysDirection
		inputs := exec.generateSidewaysRecoveryInputs(step)
		if inputs.ThrottleX <= 0 {
			t.Fatalf("sidewaysDirection=%d: ThrottleX = %.4f, want > 0 (away from pathStartPos at x=-5, not back toward it)", sidewaysDirection, inputs.ThrottleX)
		}
	}
}
