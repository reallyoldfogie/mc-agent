package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// RunCombat runs the first conservative autonomous combat loop. It currently
// executes melee decisions only; ranged decisions produce movement intents but
// do not send a ranged attack until projectile integration is ready.
func (a *agent) RunCombat(ctx context.Context, radius float64, includeNeutral bool) error {
	return a.RunCombatWithPolicy(ctx, radius, combat.TargetPolicy{
		IncludePlayers: false,
		IncludeNeutral: includeNeutral,
	})
}

// RunCombatWithPolicy runs autonomous combat with an explicit target policy.
// Player targeting is opt-in at this boundary to prevent accidental friendly
// fire for callers that need PvE-only behavior.
func (a *agent) RunCombatWithPolicy(ctx context.Context, radius float64, policy combat.TargetPolicy) error {
	if ctx == nil {
		return fmt.Errorf("combat loop: nil context")
	}
	if err := a.EnterManualMode(); err != nil {
		return fmt.Errorf("combat loop: enter manual mode: %w", err)
	}
	defer a.ExitManualMode()

	controller := combat.NewController()
	currentWeapon := combat.NoWeapon
	timer := time.NewTimer(a.combatTickInterval())
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case now := <-timer.C:
			targets, err := a.RankCombatTargetsWithPolicy(ctx, radius, policy)
			if err != nil {
				return fmt.Errorf("combat loop: rank targets: %w", err)
			}
			health, _, _, known := a.Health()
			if !known {
				health = 20
			}
			obs := combat.Observation{Health: health, MaxHealth: 20, EnemyCount: len(targets), CurrentWeapon: currentWeapon}
			if len(targets) > 0 {
				target := targets[0]
				obs.HasTarget, obs.TargetVisible = true, target.Visible
				obs.TargetDistance = target.Distance
				obs.TargetDirectionX, obs.TargetDirectionZ = target.DirectionX, target.DirectionZ
			}

			decision, intent := controller.Next(now, obs)
			currentWeapon = decision.Weapon
			if len(targets) > 0 {
				target := targets[0]
				environment, err := a.CombatMovementEnvironment(ctx, models.V3{X: target.X, Y: target.Y, Z: target.Z}, combatHazard(target.TypeName))
				if err != nil {
					return fmt.Errorf("combat loop: environment: %w", err)
				}
				intent = combat.FilterMovement(intent, environment)
			}
			if err := a.ApplyCombatMovement(ctx, intent); err != nil {
				return fmt.Errorf("combat loop: movement: %w", err)
			}
			if decision.Attack && decision.Weapon == combat.MeleeWeapon && len(targets) > 0 {
				if err := a.AttackEntity(ctx, targets[0].EntityID, false); err == nil {
					controller.CommitAttack(now)
				}
			}
			if decision.Attack && decision.Weapon == combat.RangedWeapon && len(targets) > 0 {
				var lastErr error
				for _, weapon := range []combat.ProjectileWeapon{combat.Bow, combat.Crossbow, combat.Trident} {
					request, err := rangedRequestForTarget(targets[0], weapon)
					if err != nil {
						lastErr = err
						continue
					}
					if err := a.ExecuteRangedAttack(ctx, request); err == nil {
						controller.CommitAttack(now)
						lastErr = nil
						break
					} else {
						lastErr = err
					}
				}
				if lastErr != nil {
					a.logf("[combat loop] ranged attack failed for all available weapons: %v", lastErr)
				}
			}
			timer.Reset(a.combatTickInterval())
		}
	}
}

// combatTickInterval returns the current server tick interval. The packet
// handler updates serverTickRateBits when the server changes its tick rate;
// an unset rate retains the existing vanilla-tick fallback.
func (a *agent) combatTickInterval() time.Duration {
	rate := float64(math.Float32frombits(a.serverTickRateBits.Load()))
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		rate = vanillaTickRate
	}
	return time.Duration(float64(time.Second) / rate)
}

func combatHazard(typeName string) bool {
	category := combat.Classify(typeName)
	if category != combat.Hostile {
		return false
	}
	return typeName == "minecraft:creeper" || typeName == "creeper" ||
		typeName == "minecraft:skeleton" || typeName == "skeleton" ||
		typeName == "minecraft:stray" || typeName == "stray"
}
