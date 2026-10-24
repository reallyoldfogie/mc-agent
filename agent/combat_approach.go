package agent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// approachTargetWhileAttacking concurrently closes distance toward
// targetID's live tracked position (sprinting, reusing the same
// CombatMovementEnvironment/ApplyCombatMovement primitives RunCombat's own
// loop already uses — not new movement machinery) while attack runs,
// stopping the movement once attack returns — shared by
// SpearChargeAtWithApproach and MaceSmashAtWithApproach, both of which need
// the bot physically moving toward the target throughout an attack sequence
// that itself only sends interaction packets rather than orchestrating any
// movement of its own (docs/plans/RL_ACTION_SPACE_EXPANSION_III.md items
// 6/7).
//
// filterFallRisk controls whether the ordinary "don't walk off a ledge"
// safety check (CombatMovementEnvironment's FallRisk, applied via
// combat.FilterMovement) gates the approach: Spear charge wants it — nothing
// about charging requires risking a fall, so the same caution RunCombat's
// own loop already applies is appropriate here too. Mace smash must NOT
// have it: the bot deliberately walking off a seeded elevated platform's
// edge is the entire mechanic MaceSmashAt's own falling-attack window waits
// for (see SeedElevatedPlatform's own doc comment) — the ordinary filter
// would hold position at exactly that edge, making a smash impossible.
//
// Like RunCombat, the caller owns nothing about manual-mode lifetime beyond
// this call: EnterManualMode/ExitManualMode are scoped to this one
// invocation, matching every other direct (non-RunCombat-loop) entry point
// into manual movement in this package.
func (a *agent) approachTargetWhileAttacking(ctx context.Context, targetID int32, filterFallRisk bool, attack func(context.Context) error) error {
	if ctx == nil {
		return fmt.Errorf("combat approach: nil context")
	}
	if err := a.EnterManualMode(); err != nil {
		return fmt.Errorf("combat approach: enter manual mode: %w", err)
	}
	defer a.ExitManualMode()

	moveCtx, cancelMove := context.WithCancel(ctx)
	moveDone := make(chan error, 1)
	go func() {
		moveDone <- a.approachCombatTarget(moveCtx, targetID, filterFallRisk)
	}()

	err := attack(ctx)
	cancelMove()
	if moveErr := <-moveDone; err == nil && moveErr != nil && !errors.Is(moveErr, context.Canceled) {
		err = fmt.Errorf("combat approach: movement: %w", moveErr)
	}
	return err
}

// approachCombatTarget is approachTargetWhileAttacking's movement half: one
// tick per combatTickInterval, sprinting toward targetID's live tracked
// position. Returns nil once the target is no longer tracked (nothing left
// to approach) rather than treating that as an error — the attack half may
// still be mid-flight (e.g. a spear charge's hold already in progress) and
// shouldn't be aborted just because this loop found nothing further to do.
func (a *agent) approachCombatTarget(ctx context.Context, targetID int32, filterFallRisk bool) error {
	timer := time.NewTimer(a.combatTickInterval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			target, tracked := a.GetTrackedEntities()[targetID]
			if !tracked || target.Removed {
				return nil
			}
			position, _, _, initialized := a.GetPosition()
			if initialized {
				targetPos := models.V3{X: target.X, Y: target.Y, Z: target.Z}
				intent := combat.MovementIntent{Action: combat.ApproachTarget, Sprint: true}
				if dx, dz := targetPos.X-position.X, targetPos.Z-position.Z; dx != 0 || dz != 0 {
					horizontalDistance := math.Hypot(dx, dz)
					intent.ThrottleX, intent.ThrottleZ = dx/horizontalDistance, dz/horizontalDistance
				}
				if filterFallRisk {
					if environment, err := a.CombatMovementEnvironment(ctx, targetPos, false); err == nil {
						intent = combat.FilterMovement(intent, environment)
					}
				}
				if err := a.ApplyCombatMovement(ctx, intent); err != nil {
					return err
				}
			}
			timer.Reset(a.combatTickInterval())
		}
	}
}
