package movement

import (
	"fmt"
	"github.com/reallyoldfogie/mc-agent/utils"

	"github.com/reallyoldfogie/mc-agent/models"
)

// handleRidingModeMule runs one tick for a ridden mule.
//
// Mules have slightly different movement speed than horses (vanilla: ~0.175 vs horse ~0.225)
// and lower jump height (~0.5 vs horse variable). For now, this delegates to the horse handler
// which reads the generic.movement_speed attribute from the server, making the differentiation automatic.
func (pe *PhysicsMovementExecutor) handleRidingModeMule(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	inputs models.Inputs,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	utils.SafeLogger(pe.logger).Debug(fmt.Sprintf("[handleRidingModeMule] Mule riding not fully implemented — using horse physics as fallback"))
	return pe.handleRidingModeHorse(versionHandler, mountedEntityID, inputs, forward, backward, left, right, jump, sneak, entityGetter)
}
