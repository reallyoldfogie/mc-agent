package movement

import (
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// handleRidingModePig runs one tick for a ridden pig.
//
// TODO: Pigs use carrot_on_a_stick for speed boost and direction control.
// The pig's movement speed is multiplied when the rider holds a carrot_on_a_stick.
// For now, this delegates to the generic horse handler.
func (pe *PhysicsMovementExecutor) handleRidingModePig(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) {
	log.Printf("[handleRidingModePig] Pig riding not fully implemented — using horse physics as fallback")
	pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, entityGetter)
}
