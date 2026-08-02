package movement

import (
	"github.com/reallyoldfogie/mc-agent/models"
)

// handleRidingModeLlama runs one tick for a ridden llama or trader llama.
//
// A llama is rideable but NOT steerable. It takes no saddle, so Java's
// LlamaEntity.getControllingPassenger() never returns the rider and the llama
// keeps running its own mob AI. That makes the rider a passive passenger:
// no steering, no thrust, no jump, no position prediction and no VehicleMove.
// See handleRidingModePassiveRider for the full rationale.
//
// Before llamas were registered as their own vehicle type, a mounted llama fell
// through the riding dispatch to the horse handler, which applied all four of
// those and shipped a predicted position for a server-controlled entity.
//
// The llama's inventory remains accessible while mounted, and caravans (leads
// chaining up to ten llamas) are entirely server-side, so neither needs
// anything from this handler.
func (pe *PhysicsMovementExecutor) handleRidingModeLlama(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	return pe.handleRidingModePassiveRider(
		versionHandler, mountedEntityID, "llama (no saddle, not steerable)",
		forward, backward, left, right, jump, sneak, entityGetter,
	)
}
