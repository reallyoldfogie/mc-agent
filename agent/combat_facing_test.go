package agent

import (
	"context"
	"testing"

	"github.com/reallyoldfogie/mc-agent/models"
)

type combatFacingExecutor struct {
	fakeMoveExec
	rotations [][2]float64
}

func (e *combatFacingExecutor) SendPositionAndRotation(_ float64, _ float64, _ float64, yaw, pitch float64, _ bool) error {
	e.rotations = append(e.rotations, [2]float64{yaw, pitch})
	return nil
}

func TestTurnTowardsDispatchesBodyRotationForCombatTarget(t *testing.T) {
	executor := &combatFacingExecutor{}
	a := &agent{
		moveExec:       executor,
		posX:           10,
		posY:           64,
		posZ:           10,
		posInitialized: true,
	}

	if err := a.TurnTowards(context.Background(), 10, 65, 20); err != nil {
		t.Fatalf("TurnTowards: %v", err)
	}
	if len(executor.rotations) != 1 {
		t.Fatalf("rotation dispatch count = %d, want 1", len(executor.rotations))
	}
	rotation := executor.rotations[0]
	if rotation[0] != 0 {
		t.Fatalf("target directly along +Z should use yaw 0, got %.2f", rotation[0])
	}
	if rotation[1] == 0 {
		t.Fatal("target above the agent should produce a non-zero pitch")
	}
}

var _ models.MovementExecutor = (*combatFacingExecutor)(nil)
