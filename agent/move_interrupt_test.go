package agent

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/stretchr/testify/require"
)

// fakeManyStepPF returns a long, already-found path of plain Traverse
// steps - long enough that followPath's per-step fallback loop (100ms per
// step, since fakeMoveExec doesn't implement ExecutePathWithContext) would
// take clearly longer than one moveInterruptPollInterval if run to
// completion, so a test can tell "cut short by the interrupt" apart from
// "ran the whole path" just by counting recorded SendPosition calls.
type fakeManyStepPF struct{ steps int }

func (f fakeManyStepPF) FindPath(_ context.Context, s, _ models.V3, _ int) (*models.Path, error) {
	path := &models.Path{Found: true, StartPos: s}
	for i := 0; i < f.steps; i++ {
		path.Steps = append(path.Steps, models.PathStep{
			Position: models.V3{X: s.X + float64(i) + 1, Y: s.Y, Z: s.Z},
			Movement: models.Traverse,
		})
	}
	return path, nil
}
func (f fakeManyStepPF) FindGroundBelow(_, _ float64, startY float64, _ float64) float64 {
	return startY
}

// TestMoveInterruptCheckCutsPathShort confirms SetMoveInterruptCheck's own
// documented mechanics end-to-end: a check that starts false (nothing
// visible yet, matching how a dig actually plays out) and later flips true
// cancels the in-flight MoveTo well before its (otherwise much longer)
// path would have finished on its own - the actual fix for the live gap
// found 2026-11-04 (docs: rlenv.MoveInterruptSetter's own doc comment),
// where a goto that mines through several blocks along its own path
// wasn't noticed until the whole path had already run. Uses a stateful
// check, not a constant true, because withMoveInterrupt is edge-triggered
// (see TestMoveInterruptCheckAlreadyTrueAtStartIsNoOp for why a constant
// true wouldn't exercise this path at all post-2026-11-04).
func TestMoveInterruptCheckCutsPathShort(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.SetPathFinder(fakeManyStepPF{steps: 60}) // 60 steps * 100ms = ~6s uninterrupted
	a.UpdatePosition(models.V3{}, 0, 0)

	var calls atomic.Int32
	a.SetMoveInterruptCheck(func() bool { return calls.Add(1) > 1 })

	start := time.Now()
	err = a.MoveTo(context.Background(), 60, 0, 0, false)
	elapsed := time.Since(start)

	require.NoError(t, err, "a cancelled-and-recovered MoveTo should not surface the cancellation as an error")
	if elapsed > 3*time.Second {
		t.Fatalf("MoveTo took %v, want well under the ~6s the full 60-step path would need - the interrupt check should have cut it short around the first poll (~%v in)", elapsed, moveInterruptPollInterval)
	}
	if n := len(fm.PosCalls()); n >= 60 {
		t.Fatalf("SendPosition called %d times, want fewer than the full 60 steps - the interrupt check should have cut the path short", n)
	}
}

// TestMoveInterruptCheckAlreadyTrueAtStartIsNoOp confirms the fix for the
// live regression found 2026-11-04: a check that's ALREADY true before
// this path has taken a single step (found live: common with clustered
// ore nearby, or simply because refreshMineTarget's own once-per-Step
// resolution already found the same thing) must not cancel anything -
// only a transition from false to true, something this specific dig
// actually exposed, should. Without this, v0.0.64 cancelled almost every
// dispatch near ore-dense areas almost immediately, regardless of whether
// that dispatch's own path was making fine progress toward something
// else entirely.
func TestMoveInterruptCheckAlreadyTrueAtStartIsNoOp(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.SetPathFinder(fakeManyStepPF{steps: 3})
	a.UpdatePosition(models.V3{}, 0, 0)

	a.SetMoveInterruptCheck(func() bool { return true })

	require.NoError(t, a.MoveTo(context.Background(), 3, 0, 0, false))
	if n := len(fm.PosCalls()); n != 3 {
		t.Fatalf("SendPosition called %d times, want exactly 3 (the whole path) - a check already true before the path started must not cut it short", n)
	}
}

// TestMoveInterruptCheckNilRunsToCompletion confirms the opposite: with no
// check installed (the default, and every existing MoveTo caller before
// this feature), a path still runs to completion exactly as before -
// SetMoveInterruptCheck is strictly additive.
func TestMoveInterruptCheckNilRunsToCompletion(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.SetPathFinder(fakeManyStepPF{steps: 3})
	a.UpdatePosition(models.V3{}, 0, 0)

	require.NoError(t, a.MoveTo(context.Background(), 3, 0, 0, false))
	if n := len(fm.PosCalls()); n != 3 {
		t.Fatalf("SendPosition called %d times, want exactly 3 (the whole path) with no interrupt check installed", n)
	}
}

// TestMoveInterruptCheckFalseRunsToCompletion confirms a check that's
// installed but never returns true doesn't change anything either - only
// an actual true result cuts a path short.
func TestMoveInterruptCheckFalseRunsToCompletion(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.SetPathFinder(fakeManyStepPF{steps: 3})
	a.UpdatePosition(models.V3{}, 0, 0)

	a.SetMoveInterruptCheck(func() bool { return false })

	require.NoError(t, a.MoveTo(context.Background(), 3, 0, 0, false))
	if n := len(fm.PosCalls()); n != 3 {
		t.Fatalf("SendPosition called %d times, want exactly 3 (the whole path) - a check that never returns true must never cut the path short", n)
	}
}
