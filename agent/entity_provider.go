package agent

import (
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/movement"
)

// Ensure agent implements movement.EntityProvider for dependency injection
var _ movement.EntityProvider = (*agent)(nil)

// GetEntitiesSnapshot returns a snapshot of all tracked entities for physics collision queries.
// Returns position and velocity for each entity, excluding the agent itself and removed entities.
func (a *agent) GetEntitiesSnapshot() map[int32]movement.EntitySnapshot {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	out := make(map[int32]movement.EntitySnapshot, len(a.entities))
	agentID := a.entID // Read under existing lock (could add RLock for entIDMu, but agent's lifecycle is stable)

	for id, e := range a.entities {
		// Skip removed entities and self
		if e.Removed || id == agentID {
			continue
		}

		// Determine entity type from metadata if available
		var entityType models.EntityType
		if a.entityRegistry != nil {
			entityType = a.entityRegistry.GetEntityType(id)
		}
		if entityType == models.EntityTypeUnknown || entityType == "" {
			// Fallback: use entity type from tracked entity if available
			entityType = models.EntityTypeUnknown
		}

		out[id] = movement.EntitySnapshot{
			Pos:        models.V3{X: e.X, Y: e.Y, Z: e.Z},
			Vel:        models.V3{X: e.VelX, Y: e.VelY, Z: e.VelZ},
			EntityType: entityType,
		}
	}

	return out
}
