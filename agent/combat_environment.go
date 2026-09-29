package agent

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// CombatMovementEnvironment derives conservative movement facts from the
// agent's currently loaded world. targetHazard is supplied by the combat
// classifier because block data cannot identify whether a tracked entity is a
// creeper, skeleton, or other tactical hazard.
func (a *agent) CombatMovementEnvironment(ctx context.Context, target models.V3, targetHazard bool) (combat.MovementEnvironment, error) {
	position, _, _, initialized := a.GetPosition()
	if !initialized {
		return combat.MovementEnvironment{}, fmt.Errorf("combat environment: position not initialized")
	}
	world := a.GetWorld()
	shapeMgr := a.BlockShapeManager()
	if world == nil || shapeMgr == nil {
		return combat.MovementEnvironment{}, fmt.Errorf("combat environment: world or block shapes unavailable")
	}

	feetID, feetLoaded := world.GetBlockAt(position.X, position.Y, position.Z)
	headID, headLoaded := world.GetBlockAt(position.X, position.Y+1, position.Z)
	groundID, groundLoaded := world.GetBlockAt(position.X, position.Y-1, position.Z)
	groundStable := feetLoaded && headLoaded && groundLoaded &&
		shapeMgr.IsPassable(feetID) && shapeMgr.IsPassable(headID) && !shapeMgr.IsPassable(groundID)
	inWater := (feetLoaded && shapeMgr.IsWater(feetID)) || (headLoaded && shapeMgr.IsWater(headID))

	// Probe one block toward the target. If the next support block is absent
	// or passable, moving in that direction risks a drop. This is intentionally
	// conservative; a future terrain adapter can inspect a wider footprint.
	dx, dz := target.X-position.X, target.Z-position.Z
	horizontalDistance := position.DistanceToXZ(target)
	fallRisk := false
	if horizontalDistance > 0 {
		dx /= horizontalDistance
		dz /= horizontalDistance
		nextGround, loaded := world.GetBlockAt(position.X+dx, position.Y-1, position.Z+dz)
		fallRisk = !loaded || shapeMgr.IsPassable(nextGround)
	}

	visible, err := a.HasLineOfSight(ctx, target.X, target.Y, target.Z)
	if err != nil {
		return combat.MovementEnvironment{}, fmt.Errorf("combat environment line of sight: %w", err)
	}
	return combat.MovementEnvironment{
		GroundStable:   groundStable,
		InWater:        inWater,
		FallRisk:       fallRisk,
		CanJump:        groundStable && !inWater,
		HasCover:       !visible,
		TargetIsHazard: targetHazard,
	}, nil
}
