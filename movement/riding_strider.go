package movement

import (
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// handleRidingModeStrider runs one tick for a ridden strider.
//
// TODO: Striders walk on lava and use warped_fungus_on_a_stick for speed boost.
// They also shiver and slow down when not on lava. Their movement mechanics
// differ significantly from horses. For now, this delegates to the generic
// horse handler.
func (pe *PhysicsMovementExecutor) handleRidingModeStrider(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) {
	log.Printf("[handleRidingModeStrider] Strider riding not fully implemented — using horse physics as fallback")
	pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, entityGetter)
}
