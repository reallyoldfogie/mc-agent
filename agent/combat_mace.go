package agent

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// GetCombatPhysicsState exposes the movement state needed to time a falling
// Mace smash. The returned state is owned by the movement executor and should
// be read-only by callers.
func (a *agent) GetCombatPhysicsState() models.PhysicsState {
	a.movementMu.RLock()
	executor := a.moveExec
	a.movementMu.RUnlock()
	provider, ok := executor.(interface{ GetPhysicsState() models.PhysicsState })
	if !ok {
		return nil
	}
	return provider.GetPhysicsState()
}

func (a *agent) MaceAttackAt(ctx context.Context, targetID int32, itemName string) error {
	if combat.ClassifyMeleeItem(itemName) != combat.MaceMeleeWeapon {
		return fmt.Errorf("mace attack: %q is not a mace", itemName)
	}
	if a.versionHandler == nil || !combat.MaceSupportedVersion(a.versionHandler.Version()) {
		return fmt.Errorf("mace attack: requires Minecraft Java 1.21 or newer")
	}
	target, ok := a.GetTrackedEntities()[targetID]
	if !ok || target.Removed {
		return fmt.Errorf("mace attack: target %d is not tracked", targetID)
	}
	position, _, _, initialized := a.GetPosition()
	if !initialized {
		return fmt.Errorf("mace attack: agent position not initialized")
	}
	dx, dy, dz := target.X-position.X, target.Y-position.Y, target.Z-position.Z
	request := combat.MaceAttackRequest{TargetID: targetID, ItemName: itemName, Distance: math.Sqrt(dx*dx + dy*dy + dz*dz), MinReach: combat.MaceMinReach, MaxReach: combat.MaceMaxReach}
	if err := request.Validate(); err != nil {
		return err
	}
	found, err := a.SwitchToItem(ctx, itemName)
	if err != nil {
		return fmt.Errorf("mace attack: equip %s: %w", itemName, err)
	}
	if !found {
		return fmt.Errorf("mace attack: equip %s: item not found", itemName)
	}
	// The autonomous combat loop may have entered this method just before an
	// explicit smash claimed the Mace. Recheck before dispatch so that an
	// already-running ordinary attack cannot consume the smash opportunity.
	if a.maceSmashActive.Load() {
		return nil
	}
	return a.attackEntityAtRange(ctx, targetID, false, request.MaxReach)
}

// MaceSmashAt waits for vanilla's falling attack window and sends the Mace
// attack as soon as the smash conditions are met.
func (a *agent) MaceSmashAt(ctx context.Context, targetID int32, itemName string) error {
	if ctx == nil {
		return fmt.Errorf("mace smash: nil context")
	}
	if combat.ClassifyMeleeItem(itemName) != combat.MaceMeleeWeapon {
		return fmt.Errorf("mace smash: %q is not a mace", itemName)
	}
	if !a.maceSmashActive.CompareAndSwap(false, true) {
		return fmt.Errorf("mace smash: another falling attack is already active")
	}
	defer a.maceSmashActive.Store(false)
	// Select the mace before entering the timing-sensitive part of the fall.
	// Switching items at the threshold can delay the attack packet until the
	// server has already cleared the attacker's fall distance.
	found, err := a.SwitchToItem(ctx, itemName)
	if err != nil {
		return fmt.Errorf("mace smash: equip %s: %w", itemName, err)
	}
	if !found {
		return fmt.Errorf("mace smash: equip %s: item not found", itemName)
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		state := a.GetCombatPhysicsState()
		if state == nil {
			return fmt.Errorf("mace smash: combat physics state unavailable")
		}
		target, tracked := a.GetTrackedEntities()[targetID]
		if !tracked || target.Removed {
			return fmt.Errorf("mace smash: target %d is not tracked", targetID)
		}
		position := state.Position()
		targetPosition := models.V3{X: target.X, Y: target.Y, Z: target.Z}
		falling := !state.OnGround() && state.Velocity().Y < 0
		withinReach := position.DistanceTo(targetPosition) <= combat.MaceMaxReach
		// Leave a half-block margin for movement-packet latency: the server's
		// fallDistance must also be strictly above vanilla's 1.5-block gate
		// when it handles the attack packet.
		if falling && withinReach && state.FallDistance() > combat.MaceAdditionalDamageFall+0.5 && !state.IsGliding() {
			// 1.21.1 does not have SyncEntityPosition and can otherwise process
			// the attack before the movement tick that put the player in reach.
			// Flush the physics position immediately before UseEntity so the
			// server evaluates both reach and fallDistance at the same point in
			// the fall.
			a.movementMu.RLock()
			moveExec := a.moveExec
			a.movementMu.RUnlock()
			if moveExec == nil {
				return fmt.Errorf("mace smash: movement executor unavailable")
			}
			if err := moveExec.SendPosition(position.X, position.Y, position.Z, false); err != nil {
				return fmt.Errorf("mace smash: flush falling position: %w", err)
			}
			// The mace was selected above; send the attack immediately rather
			// than re-entering the equipment path at the critical moment.
			return a.attackEntityAtRange(ctx, targetID, false, combat.MaceMaxReach)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("mace smash: falling attack window was not reached")
		case <-tick.C:
		}
	}
}
