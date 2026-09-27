package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
)

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
