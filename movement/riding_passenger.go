package movement

import (
	"fmt"
	"log"

	"github.com/reallyoldfogie/mc-agent/models"
)

// handleRidingModePassiveRider runs one tick for a rider that is NOT the
// vehicle's controlling passenger, and is therefore not the movement authority
// for it.
//
// Vanilla only sends VehicleMoveC2SPacket from the client that owns the root
// vehicle's movement (ClientPlayerEntity.tick → isLogicalSideForUpdatingMovement,
// which requires getControllingPassenger() == this player). Two situations make
// that false:
//
//   - The vehicle is not steerable at all, so it has no controlling passenger.
//     A llama takes no saddle; an unsaddled horse fails AbstractHorseEntity's
//     isSaddled() check.
//   - The vehicle is steerable but somebody else is driving. Boats and camels
//     seat two and a happy ghast seats four; only the passenger at index 0
//     controls it.
//
// In both cases the correct client behaviour is identical: apply no steering,
// thrust or jump; take the position from the entity tracker rather than
// predicting one; and suppress VehicleMove so we do not contradict the server
// (or the real driver). The rider can still look around freely and can still
// sneak to dismount, so PlayerInput is still sent each tick.
//
// reason is used only for logging, to make it obvious in a trace why a tick
// took the passive path.
func (pe *PhysicsMovementExecutor) handleRidingModePassiveRider(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	reason string,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	// Send PlayerInput. The steering bits are ignored by the server because we
	// are not the controlling passenger, but sneak still drives dismount.
	sendRidingInput(pe, versionHandler, forward, backward, left, right, jump, sneak)

	pe.mountedEntityMu.Lock()
	defer pe.mountedEntityMu.Unlock()

	currentPos, yaw, pitch, prevOnGround := pe.physicsState.GetPosition()

	// Follow the server's position for the vehicle rather than predicting one.
	newPos := currentPos
	if entityGetter != nil {
		if entityX, entityY, entityZ, found := entityGetter.GetMountedEntityPosition(mountedEntityID); found {
			newPos = models.V3{X: entityX, Y: entityY, Z: entityZ}
		}
	}

	// A passive rider accumulates no local velocity. Clearing it prevents a
	// stale value from a previous mount leaking into the next tick, and keeps
	// GetRidingVelocity honest for diagnostics.
	pe.ridingVelX = 0
	pe.ridingVelY = 0
	pe.ridingVelZ = 0
	pe.lastVelMultiplier = 0
	pe.lastRidingJumpState = jump

	// The server owns ground state for the vehicle; carry the previous value
	// forward rather than inventing one from a physics step we did not run.
	onGround := prevOnGround

	log.Printf("[handleRidingModePassiveRider] %s: entity=%d yaw=%.1f pitch=%.1f sneak=%v pos=(%.2f,%.2f,%.2f)",
		reason, mountedEntityID, yaw, pitch, sneak, newPos.X, newPos.Y, newPos.Z)

	// Update physicsState inside the lock so a concurrent TurnTowards cannot
	// race with this write-back. The rider keeps their own look direction.
	applyRidingTickState(pe, newPos, yaw, pitch, onGround)
	return ridingTickResult{
		NewPos:              newPos,
		OnGround:            onGround,
		Sneak:               sneak,
		StateYaw:            yaw,
		StatePitch:          pitch,
		PacketYaw:           yaw,
		PacketPitch:         pitch,
		SuppressVehicleMove: true,
	}
}

// handleRidingModePassenger runs one tick for a rider occupying a non-driver
// seat on a multi-passenger vehicle (boat, camel, happy ghast).
//
// Seat ordering comes from ClientboundSetPassengers: index 0 is the controlling
// passenger and every later index is a passenger along for the ride. Note the
// seat can change without a mount or dismount — when the driver leaves, the
// remaining riders rotate up and one of them becomes the driver — so this
// decision is re-evaluated every tick from the tracked index rather than
// latched at mount time.
func (pe *PhysicsMovementExecutor) handleRidingModePassenger(
	versionHandler models.VersionHandler,
	mountedEntityID int32,
	passengerIndex int,
	forward, backward, left, right, jump, sneak bool,
	entityGetter models.MountedEntityPositionGetter,
) ridingTickResult {
	reason := fmt.Sprintf("passenger seat %d (not the driver)", passengerIndex)
	return pe.handleRidingModePassiveRider(
		versionHandler, mountedEntityID, reason,
		forward, backward, left, right, jump, sneak, entityGetter,
	)
}
