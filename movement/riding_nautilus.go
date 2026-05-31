package movement

import (
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// handleRidingModeNautilus runs one tick for a ridden Nautilus or ZombieNautilus (1.21.11+).
//
// TODO: Nautilus (AbstractNautilusEntity) extends AbstractHorseEntity but has
// fundamentally different physics:
//   - Underwater diving and surfacing mechanics
//   - isControlledByMob() — can be ridden by mobs (e.g., ZombieNautilus by husks)
//   - Spear-charging attack while mounted (getRiderChargingSpeedMultiplier)
//   - Different water travel overrides (swims instead of sinking)
//   - Breath/air management for the rider
//
// For now, this delegates to the generic horse handler.
func (pe *PhysicsMovementExecutor) handleRidingModeNautilus(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) {
	log.Printf("[handleRidingModeNautilus] Nautilus riding not fully implemented — using horse physics as fallback")
	pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, entityGetter)
}
