package agent

import (
	"context"
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
// documented mechanics end-to-end: a check that reports true cancels the
// in-flight MoveTo well before its (otherwise much longer) path would have
// finished on its own - the actual fix for the live gap found 2026-11-04
// (docs: rlenv.MoveInterruptSetter's own doc comment), where a goto that
// mines through several blocks along its own path wasn't noticed until
// the whole path had already run.
func TestMoveInterruptCheckCutsPathShort(t *testing.T) {
	agentInt, err := New(models.AgentConfig{Version: "1.21.5", Address: "x"})
	require.NoError(t, err)
	a := agentInt.(*agent)
	require.NoError(t, a.Init(context.Background()))

	fm := &fakeMoveExec{}
	a.SetMovementExecutor(fm)
	a.SetPathFinder(fakeManyStepPF{steps: 20}) // 20 steps * 100ms = ~2s uninterrupted
	a.UpdatePosition(models.V3{}, 0, 0)

	a.SetMoveInterruptCheck(func() bool { return true })

	start := time.Now()
	err = a.MoveTo(context.Background(), 20, 0, 0, false)
	elapsed := time.Since(start)

	require.NoError(t, err, "a cancelled-and-recovered MoveTo should not surface the cancellation as an error")
	if elapsed > 1*time.Second {
		t.Fatalf("MoveTo took %v, want well under the ~2s the full 20-step path would need - the interrupt check should have cut it short", elapsed)
	}
	if n := len(fm.PosCalls()); n >= 20 {
		t.Fatalf("SendPosition called %d times, want fewer than the full 20 steps - the interrupt check should have cut the path short", n)
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
