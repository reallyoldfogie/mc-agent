package agent

import (
	"context"
	"fmt"
	"log"

	versions_common "github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/movement"
)

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
// Power should be 0-100 (0 = no jump, 100 = maximum jump).
// Returns an error if the agent is not mounted on a horse.
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

	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	entityID := a.GetEntityID()

	// Send ActionStartJumpHorse with the power parameter
	if err := a.versionHandler.Play().Movement().SendPlayerCommandWithParam(
		a.client.Conn(),
		entityID,
		versions_common.ActionStartJumpHorse,
		power,
	); err != nil {
		return fmt.Errorf("failed to send jump start packet: %v", err)
	}

	// Immediately send ActionStopJumpHorse to complete the jump cycle
	if err := a.versionHandler.Play().Movement().SendPlayerCommandWithParam(
		a.client.Conn(),
		entityID,
		versions_common.ActionStopJumpHorse,
		0,
	); err != nil {
		return fmt.Errorf("failed to send jump stop packet: %v", err)
	}

	log.Printf("[JumpVehicle] Horse jump sent with power %d", power)
	return nil
}
