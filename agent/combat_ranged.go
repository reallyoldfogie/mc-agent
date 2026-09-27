package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/physics"
)

// ExecuteRangedAttack adapts a validated combat request to the existing
// projectile actions. Inventory selection happens here rather than in the
// controller, so the controller remains independent of protocol details.
func (a *agent) ExecuteRangedAttack(ctx context.Context, request combat.RangedAttackRequest) error {
	if ctx == nil {
		return fmt.Errorf("ranged attack: nil context")
	}
	position, err := request.LeadPosition()
	if err != nil {
		return err
	}
	if request.CancelOnTargetLoss && !a.combatTargetAvailable(request.TargetID) {
		return fmt.Errorf("ranged attack: target %d was lost before dispatch", request.TargetID)
	}

	itemName, projectile, err := projectileForCombatWeapon(request.Weapon)
	if err != nil {
		return err
	}
	found, err := a.SwitchToItem(ctx, itemName)
	if err != nil {
		return fmt.Errorf("ranged attack: equip %s: %w", itemName, err)
	}
	if !found {
		return fmt.Errorf("ranged attack: equip %s: item not found", itemName)
	}

	switch projectile {
	case models.Arrow:
		if request.Weapon == combat.Crossbow {
			if err := a.fireCrossbowAt(ctx, request.TargetID, request.CancelOnTargetLoss, position.X, position.Y, position.Z, request.ChargeDuration); err != nil {
				return fmt.Errorf("ranged attack: fire crossbow: %w", err)
			}
		} else {
			if _, err := a.FireBowAt(ctx, position.X, position.Y, position.Z); err != nil {
				return fmt.Errorf("ranged attack: fire bow: %w", err)
			}
			// FireBowAt starts the vanilla hold/release sequence asynchronously
			// for legacy callers. Autonomous combat must wait for that sequence
			// to finish before its controller cooldown is committed, otherwise
			// the next tick can overlap an active draw.
			if err := sleepWithContext(ctx, maxBowHoldDuration); err != nil {
				return fmt.Errorf("ranged attack: wait for bow release: %w", err)
			}
		}
	case models.Trident:
		if _, err := a.ThrowProjectileAt(ctx, models.Trident, position.X, position.Y, position.Z); err != nil {
			return fmt.Errorf("ranged attack: throw trident: %w", err)
		}
	default:
		return fmt.Errorf("ranged attack: unsupported projectile %s", projectile)
	}
	return nil
}

func projectileForCombatWeapon(weapon combat.ProjectileWeapon) (string, models.ProjectileType, error) {
	switch weapon {
	case combat.Bow:
		return "minecraft:bow", models.Arrow, nil
	case combat.Trident:
		return "minecraft:trident", models.Trident, nil
	case combat.Crossbow:
		return "minecraft:crossbow", models.Arrow, nil
	default:
		return "", 0, fmt.Errorf("ranged attack: unknown projectile weapon %d", weapon)
	}
}

func filterCombatProjectileWeapons(available map[string]bool) []combat.ProjectileWeapon {
	weapons := make([]combat.ProjectileWeapon, 0, 3)
	for _, weapon := range []combat.ProjectileWeapon{combat.Bow, combat.Crossbow, combat.Trident} {
		itemName, _, err := projectileForCombatWeapon(weapon)
		if err == nil && available[itemName] {
			weapons = append(weapons, weapon)
		}
	}
	return weapons
}

func rangedRequestForTarget(target combat.Target, weapon combat.ProjectileWeapon) (combat.RangedAttackRequest, error) {
	if target.Distance <= 0 {
		return combat.RangedAttackRequest{}, fmt.Errorf("ranged attack: target %d has invalid distance %.2f", target.EntityID, target.Distance)
	}
	projectile := models.Arrow
	if weapon == combat.Trident {
		projectile = models.Trident
	}
	speed := physics.GetProjectilePhysics(projectile).InitialSpeed
	return combat.RangedAttackRequest{
		TargetID:           target.EntityID,
		Weapon:             weapon,
		TargetPosition:     models.V3{X: target.X, Y: target.Y, Z: target.Z},
		TargetVelocity:     models.V3{X: target.VelocityX, Y: target.VelocityY, Z: target.VelocityZ},
		ProjectileSpeed:    speed,
		FlightTime:         target.Distance / speed,
		ChargeDuration:     crossbowChargeDuration(weapon),
		Priority:           0,
		CancelOnTargetLoss: true,
	}, nil
}

func crossbowChargeDuration(weapon combat.ProjectileWeapon) time.Duration {
	if weapon == combat.Crossbow {
		return combat.CrossbowChargeDuration
	}
	return 0
}

func (a *agent) fireCrossbowAt(ctx context.Context, targetID int32, cancelOnTargetLoss bool, x, y, z float64, chargeDuration time.Duration) error {
	if chargeDuration <= 0 {
		return fmt.Errorf("invalid crossbow charge duration %v", chargeDuration)
	}
	if err := a.TurnTowards(ctx, x, y, z); err != nil {
		return err
	}
	visible, _, _, _, err := a.hasLineOfSightForAccess(ctx, x, y, z)
	if err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("target is not visible")
	}
	botPos, _, _, initialized := a.GetPosition()
	if !initialized {
		return fmt.Errorf("agent position not initialized")
	}
	origin := models.V3{X: botPos.X, Y: botPos.Y + a.getEyeHeight() - 0.1, Z: botPos.Z}
	if solution, err := a.FindValidTrajectory(models.Arrow, origin, models.V3{X: x, Y: y, Z: z}); err != nil {
		return err
	} else if len(solution.Trajectory) == 0 {
		return fmt.Errorf("no unobstructed crossbow trajectory to target")
	}
	actions := a.versionHandler.Play().Actions()
	if actions == nil {
		return fmt.Errorf("action handler not available")
	}
	conn, err := a.getPacketWriter()
	if err != nil {
		return err
	}
	_, yaw, pitch, ok := a.GetPosition()
	if !ok {
		return fmt.Errorf("agent position not initialized")
	}
	if err := actions.SendUseItem(conn, models.MainHand, a.getNextSequence(), yaw, pitch); err != nil {
		return err
	}
	if err := a.sleepRangedCharge(ctx, targetID, cancelOnTargetLoss, chargeDuration); err != nil {
		_ = actions.SendPlayerAction(conn, common.PlayerActionReleaseUseItem, 0, 0, 0, 0, a.getNextSequence())
		return err
	}
	return actions.SendPlayerAction(conn, common.PlayerActionReleaseUseItem, 0, 0, 0, 0, a.getNextSequence())
}

func (a *agent) combatTargetAvailable(entityID int32) bool {
	target, ok := a.GetTrackedEntities()[entityID]
	return ok && !target.Removed
}

func (a *agent) sleepRangedCharge(ctx context.Context, targetID int32, cancelOnTargetLoss bool, duration time.Duration) error {
	if !cancelOnTargetLoss {
		return sleepWithContext(ctx, duration)
	}
	if !a.combatTargetAvailable(targetID) {
		return fmt.Errorf("ranged attack: target %d was lost while charging", targetID)
	}
	const pollInterval = 50 * time.Millisecond
	timer := time.NewTimer(duration)
	poll := time.NewTicker(pollInterval)
	defer timer.Stop()
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		case <-poll.C:
			if !a.combatTargetAvailable(targetID) {
				return fmt.Errorf("ranged attack: target %d was lost while charging", targetID)
			}
		}
	}
}
