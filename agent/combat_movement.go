package agent

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// ApplyCombatMovement applies one already-decided combat movement tick to the
// manual physics executor. The caller owns the manual-mode lifetime: call
// EnterManualMode before the combat loop and ExitManualMode when it stops.
// Terrain validation and target/state decisions remain outside this adapter.
func (a *agent) ApplyCombatMovement(ctx context.Context, intent combat.MovementIntent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.movementMu.RLock()
	defer a.movementMu.RUnlock()
	if a.moveExec == nil {
		return fmt.Errorf("combat movement: movement executor not available")
	}
	manual, ok := a.moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("combat movement: executor does not support manual mode")
	}
	if err := manual.SetManualThrottle(intent.ThrottleX, intent.ThrottleZ); err != nil {
		return fmt.Errorf("combat movement throttle: %w", err)
	}
	if err := manual.SetManualSprint(intent.Sprint); err != nil {
		return fmt.Errorf("combat movement sprint: %w", err)
	}
	if err := manual.SetManualJump(intent.Jump); err != nil {
		return fmt.Errorf("combat movement jump: %w", err)
	}
	return nil
}
