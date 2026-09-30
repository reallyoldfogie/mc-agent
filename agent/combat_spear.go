package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
)

// SpearJabAt performs a validated spear Jab against a tracked entity.
func (a *agent) SpearJabAt(ctx context.Context, targetID int32, itemName string) error {
	_, request, err := a.spearRequestForEntity(targetID, itemName, combat.SpearJab, 0, 0, 0, 0, 0)
	if err != nil {
		return err
	}
	return a.ExecuteSpearAttack(ctx, request)
}

// SpearChargeAt performs a validated held-use spear Charge. Durations and
// motion thresholds are supplied by the caller because they come from the
// item components and movement state rather than the transport.
func (a *agent) SpearChargeAt(ctx context.Context, targetID int32, itemName string, holdDuration, engagedDuration, tiredDuration time.Duration, minSpeed, minAlignment float64) error {
	_, request, err := a.spearRequestForEntity(targetID, itemName, combat.SpearCharge, holdDuration, engagedDuration, tiredDuration, minSpeed, minAlignment)
	if err != nil {
		return err
	}
	return a.ExecuteSpearAttack(ctx, request)
}

func (a *agent) spearRequestForEntity(targetID int32, itemName string, mode combat.SpearAttackMode, holdDuration, engagedDuration, tiredDuration time.Duration, minSpeed, minAlignment float64) (combat.Target, combat.SpearAttackRequest, error) {
	target, ok := a.GetTrackedEntities()[targetID]
	if !ok || target.Removed {
		return combat.Target{}, combat.SpearAttackRequest{}, fmt.Errorf("spear attack: target %d is not tracked", targetID)
	}
	position, _, _, initialized := a.GetPosition()
	if !initialized {
		return combat.Target{}, combat.SpearAttackRequest{}, fmt.Errorf("spear attack: agent position not initialized")
	}
	dx, dy, dz := target.X-position.X, target.Y-position.Y, target.Z-position.Z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	combatTarget := combat.Target{EntityID: targetID, X: target.X, Y: target.Y, Z: target.Z, Distance: distance}
	request := combat.SpearAttackRequest{
		TargetID: targetID, ItemName: itemName, Mode: mode, Distance: distance,
		MinReach: 1, MaxReach: 6, TargetPosition: models.V3{X: target.X, Y: target.Y, Z: target.Z},
		ViewAlignment: 1, RelativeSpeed: minSpeed, MinSpeed: minSpeed, MinAlignment: minAlignment,
		HoldDuration: holdDuration,
		Profile:      combat.SpearChargeProfile{EngagedDuration: engagedDuration, TiredDuration: tiredDuration},
	}
	return combatTarget, request, request.Validate()
}

func spearJabRequestForTarget(target combat.Target, itemName string) (combat.SpearAttackRequest, error) {
	request := combat.SpearAttackRequest{
		TargetID:       target.EntityID,
		ItemName:       itemName,
		Mode:           combat.SpearJab,
		Distance:       target.Distance,
		MinReach:       1,
		MaxReach:       6,
		TargetPosition: models.V3{X: target.X, Y: target.Y, Z: target.Z},
	}
	return request, request.Validate()
}

// ExecuteSpearAttack executes a validated Jab or held-use Charge for a
// 1.21.11+ spear. The request supplies version/item-component thresholds.
func (a *agent) ExecuteSpearAttack(ctx context.Context, request combat.SpearAttackRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if a.versionHandler == nil || !combat.SpearSupportedVersion(a.versionHandler.Version()) {
		return fmt.Errorf("spear attack: requires Minecraft Java 1.21.11 or newer")
	}
	if request.Mode == combat.SpearCharge && !request.ChargeEligible() {
		return fmt.Errorf("spear attack: charge thresholds are not met")
	}
	found, err := a.SwitchToItem(ctx, request.ItemName)
	if err != nil {
		return fmt.Errorf("spear attack: equip %s: %w", request.ItemName, err)
	}
	if !found {
		return fmt.Errorf("spear attack: equip %s: item not found", request.ItemName)
	}
	if request.Mode == combat.SpearCharge {
		stage := request.Profile.StageAt(request.HoldDuration)
		outcome := combat.ContactOutcome(stage)
		a.logf("[spear] charge stage=%d damage=%v knockback=%v dismount=%v", stage, outcome.Damage, outcome.Knockback, outcome.Dismount)
		return a.chargeSpearAt(ctx, request.TargetPosition, request.HoldDuration)
	}
	return a.attackEntityAtRange(ctx, request.TargetID, false, request.MaxReach)
}

func (a *agent) chargeSpearAt(ctx context.Context, target models.V3, holdDuration time.Duration) error {
	// TargetPosition is normally the entity's feet position. Aim and validate
	// visibility against the body instead of the feet, which can be occluded by
	// the target's supporting block even when the entity itself is visible.
	aimTarget := target
	aimTarget.Y += 1.0
	if err := a.TurnTowards(ctx, aimTarget.X, aimTarget.Y, aimTarget.Z); err != nil {
		return err
	}
	visible, _, _, _, err := a.hasLineOfSightForAccess(ctx, aimTarget.X, aimTarget.Y, aimTarget.Z)
	if err != nil {
		return err
	}
	if !visible {
		return fmt.Errorf("spear charge target is not visible")
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
	if err := sleepWithContext(ctx, holdDuration); err != nil {
		_ = actions.SendPlayerAction(conn, common.PlayerActionReleaseUseItem, 0, 0, 0, 0, a.getNextSequence())
		return err
	}
	return actions.SendPlayerAction(conn, common.PlayerActionReleaseUseItem, 0, 0, 0, 0, a.getNextSequence())
}
