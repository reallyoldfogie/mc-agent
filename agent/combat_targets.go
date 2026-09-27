package agent

import (
	"context"
	"fmt"

	"github.com/reallyoldfogie/mc-agent/combat"
	"github.com/reallyoldfogie/mc-agent/models"
)

// RankCombatTargets converts the agent's current tracked entities into the
// transport-independent combat target model and ranks them. Visibility is
// resolved against the current world, so callers get a decision-ready view.
// A non-positive radius means no distance limit.
func (a *agent) RankCombatTargets(ctx context.Context, radius float64, includeNeutral bool) ([]combat.Target, error) {
	position, _, _, initialized := a.GetPosition()
	if !initialized {
		return nil, fmt.Errorf("combat target ranking: position not initialized")
	}
	tracked := a.GetTrackedEntities()
	targets := make([]combat.Target, 0, len(tracked))
	for _, entity := range tracked {
		if entity.Removed {
			continue
		}
		entityPosition := models.V3{X: entity.X, Y: entity.Y, Z: entity.Z}
		distance := position.DistanceTo(entityPosition)
		if radius > 0 && distance > radius {
			continue
		}

		typeName := "unknown"
		if a.entityRegistry != nil {
			typeName = string(a.entityRegistry.GetEntityType(entity.EntityID))
		}
		category := combat.Classify(typeName)
		if category == combat.Unknown || (category == combat.Neutral && !includeNeutral) {
			continue
		}
		visible, err := a.HasLineOfSight(ctx, entity.X, entity.Y, entity.Z)
		if err != nil {
			return nil, fmt.Errorf("combat target %d line of sight: %w", entity.EntityID, err)
		}
		directionX, directionZ := 0.0, 0.0
		horizontalDistance := position.DistanceToXZ(entityPosition)
		if horizontalDistance > 0 {
			directionX = (entity.X - position.X) / horizontalDistance
			directionZ = (entity.Z - position.Z) / horizontalDistance
		}
		velocityX, velocityY, velocityZ, _ := a.GetEntityVelocity(entity.EntityID)
		targets = append(targets, combat.Target{
			EntityID: entity.EntityID, TypeName: typeName, Category: category,
			X: entity.X, Y: entity.Y, Z: entity.Z,
			VelocityX: velocityX, VelocityY: velocityY, VelocityZ: velocityZ,
			Distance: distance, DirectionX: directionX, DirectionZ: directionZ,
			Health: entity.Health, MaxHealth: entity.MaxHealth, Visible: visible,
		})
	}
	return combat.Rank(targets, includeNeutral), nil
}
