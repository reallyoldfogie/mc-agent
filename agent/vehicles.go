package agent

import (
	"context"
	"fmt"
	"log"
	"time"

	versions_common "github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/movement"
)

// manualModeChecker is implemented by executors that can report whether they are
// currently in manual mode. Used by JumpVehicle to avoid entering/exiting manual
// mode when the caller is already controlling the agent manually.
type manualModeChecker interface {
	IsManualMode() bool
}

// MountEntity mounts the agent on a vehicle entity by its ID.
// The mounting is initiated by sending a right-click interaction packet (UseItemOnEntity),
// and the server will respond with ClientboundSetPassengers when the mount is successful.
func (a *agent) MountEntity(ctx context.Context, entityID int32) error {
	// Verify the entity exists
	a.entitiesMu.RLock()
	entity, exists := a.entities[entityID]
	a.entitiesMu.RUnlock()

	if !exists {
		return fmt.Errorf("entity %d not found", entityID)
	}

	if entity.Removed {
		return fmt.Errorf("entity %d is marked as removed", entityID)
	}

	log.Printf("[MountEntity] Attempting to mount entity %d (type %d at %.1f, %.1f, %.1f)",
		entityID, entity.EntityType, entity.X, entity.Y, entity.Z)

	// Use the existing UseItemOnEntity action to interact with the entity (right-click)
	// The hand parameter (false = main hand) is not used for mount interactions
	err := a.UseItemOnEntity(ctx, entityID, 0, false)
	if err != nil {
		return fmt.Errorf("failed to send use entity packet: %v", err)
	}

	return nil
}

// MountNearest resolves entityTypeName (e.g. "horse", "minecraft:boat") to
// the nearest matching entity within perception range and mounts it,
// covering the common case a raw "mount <entityID>" can't: a chat user
// rarely already knows a target's numeric entity ID, but does know what
// kind of thing they want to ride.
func (a *agent) MountNearest(ctx context.Context, entityTypeName string) error {
	pos, ok := a.GetPositionSimple()
	if !ok {
		return fmt.Errorf("agent position not initialized")
	}

	typeID, ok := a.GetEntityTypeID(normalizeItemName(entityTypeName))
	if !ok {
		return fmt.Errorf("unknown entity type %q", entityTypeName)
	}

	entityID, distance, found := a.FindNearestEntityByType(typeID, pos.X, pos.Y, pos.Z, true)
	if !found {
		return fmt.Errorf("no nearby %s found", entityTypeName)
	}

	if err := a.MountEntity(ctx, entityID); err != nil {
		return fmt.Errorf("mount nearest %s (entity %d, %.1f blocks away): %w", entityTypeName, entityID, distance, err)
	}
	return nil
}

// DismountEntity dismounts the agent from the current vehicle.
// This is typically called in response to a /dismount command.
// The dismounting is done by sending a sneak action while mounted.
func (a *agent) DismountEntity() error {
	// Check if the agent is actually mounted
	currentMount := a.getMountedEntityID()
	if currentMount == -1 {
		return fmt.Errorf("agent is not currently mounted")
	}

	log.Printf("[DismountEntity] Dismounting from entity %d", currentMount)

	// Send a sneak action to dismount
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	// Notify the physics executor that a dismount is pending. This forces
	// sneak=true in every subsequent SendVehicleInput until the server
	// acknowledges via SetPassengers, preventing the race where updateInput()
	// clears the sneaking flag before tickRiding() can process the dismount.
	a.movementMu.RLock()
	if physicsExec, ok := a.moveExec.(*movement.PhysicsMovementExecutor); ok {
		physicsExec.NotifyDismountRequested()
	}
	a.movementMu.RUnlock()

	entityID := a.GetEntityID()
	// Use ActionStartSneaking to initiate the dismount - the server will handle the actual dismount
	// and send ClientboundSetPassengers to remove the agent from the vehicle's passengers
	err := a.versionHandler.Play().Movement().SendPlayerCommand(
		a.client.Conn(),
		entityID,
		versions_common.ActionStartSneaking,
	)

	if err != nil {
		return fmt.Errorf("failed to send dismount packet: %v", err)
	}

	return nil
}

// JumpVehicle makes the mounted horse/vehicle jump with the given power.
//
// Power is 0-100 (0 = no jump, 100 = maximum jump) and is clamped to that range.
// It represents the charge the jump bar would reach if the jump key were held:
// 100 maps to full strength (1.0), 50 to ~half, etc.
//
// This is a thin wrapper around the SetManualJump hold/release workflow used by
// the physics executor's horse handler, so there is a single jump execution
// path (the client-authoritative charge ramp from vanilla
// ClientPlayerEntity.tickMovement). The wrapper holds the jump key for the tick
// count the MountJumpStrength ramp maps to the requested power, then releases to
// fire the jump, and waits for the horse to lift off.
//
// If the executor is already in manual mode, JumpVehicle only triggers the jump
// and leaves manual mode active afterwards (so a jump issued during manual
// control does not end manual mode). If the executor is not in manual mode,
// JumpVehicle enters manual mode for the jump and exits it when done.
//
// Returns an error if the agent is not mounted or the executor does not support
// manual movement.
func (a *agent) JumpVehicle(ctx context.Context, power int32) error {
	// Check if the agent is actually mounted
	if a.getMountedEntityID() == -1 {
		return fmt.Errorf("agent is not mounted on a vehicle")
	}

	// Clamp power to valid range [0, 100]
	if power < 0 {
		power = 0
	} else if power > 100 {
		power = 100
	}

	a.movementMu.RLock()
	moveExec := a.moveExec
	a.movementMu.RUnlock()

	manual, ok := moveExec.(models.ManualMovementExecutor)
	if !ok {
		return fmt.Errorf("movement executor does not support manual jump control")
	}

	// Detect whether the executor is already in manual mode so we only
	// enter/exit it when we entered it ourselves. A jump triggered while the
	// caller is already in manual mode must not end manual mode.
	alreadyManual := false
	if checker, ok := moveExec.(manualModeChecker); ok {
		alreadyManual = checker.IsManualMode()
	}

	if !alreadyManual {
		if err := manual.EnterManualMode(); err != nil {
			return fmt.Errorf("enter manual mode for jump: %w", err)
		}
		defer func() {
			if err := manual.ExitManualMode(); err != nil {
				log.Printf("[JumpVehicle] Failed to exit manual mode after jump: %v", err)
			}
		}()
	}

	// Hold the jump key for the tick count that the MountJumpStrength ramp maps to
	// the requested power. The ramp reaches 1.0 (power 100) at tick 10, then
	// decays toward 0.8; we want the rising side, so hold for power/10 ticks (max
	// 10). power 0 means no hold → release immediately → minimum-strength jump
	// (still fires because the release edge arms the strength).
	holdTicks := int(power) / 10
	if holdTicks > 10 {
		holdTicks = 10
	}

	if err := manual.SetManualJump(true); err != nil {
		return fmt.Errorf("press jump: %w", err)
	}

	// Hold for holdTicks ticks (50ms each at 20 TPS). At least one tick so the
	// rising edge registers even for power 0.
	holdDuration := time.Duration(holdTicks) * physicsTick
	if holdDuration < physicsTick {
		holdDuration = physicsTick
	}
	select {
	case <-ctx.Done():
		_ = manual.SetManualJump(false)
		return ctx.Err()
	case <-time.After(holdDuration):
	}

	// Release to fire the jump (the handler arms the strength on the release edge
	// and applies the velocity on the next on-ground tick).
	if err := manual.SetManualJump(false); err != nil {
		return fmt.Errorf("release jump: %w", err)
	}

	// Wait for the jump to lift the horse. The handler applies the velocity on
	// the next on-ground tick after release, so a short settle covers the armed
	// → fire → rise. Use the context to allow early cancellation.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(jumpSettleTimeout):
	}

	log.Printf("[JumpVehicle] Horse jump fired with power %d (holdTicks=%d, alreadyManual=%v)", power, holdTicks, alreadyManual)
	return nil
}

// physicsTick is the Minecraft server tick interval (50ms at 20 TPS).
const physicsTick = 50 * time.Millisecond

// jumpSettleTimeout is how long JumpVehicle waits after releasing the jump key
// for the horse handler to arm the strength, apply it on the next on-ground
// tick, and lift off. Generous enough to cover the armed→fire→rise latency.
const jumpSettleTimeout = 600 * time.Millisecond
