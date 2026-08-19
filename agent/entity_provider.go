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
			Pos:                models.V3{X: e.X, Y: e.Y, Z: e.Z},
			Vel:                models.V3{X: e.VelX, Y: e.VelY, Z: e.VelZ},
			EntityType:         entityType,
			IsStandableSurface: isStandableSurface(entityType, e),
		}
	}

	return out
}

// isStandableSurface reports whether the given tracked entity currently
// behaves as a solid, walkable surface for a nearby (not necessarily
// mounted) player — the client-relevant half of Java
// HappyGhastEntity.isCollidable(Entity):
//
//	!isBaby() && isAlive() && ... : true when method_72227() (staying still)
//
// The entity-type check here is defense-in-depth, not strictly load-bearing
// today: HappyGhastStayingStill/IsBaby are only ever set true by the metadata
// handler when the entity is already known to be a happy ghast (see
// agent/handlers.go's onSetEntityMetadata gating), so this check can never
// currently fire for a different type — but keeping it explicit here means
// that invariant staying true isn't a silent precondition of this function.
//
// isAlive() is not checked explicitly: the caller (GetEntitiesSnapshot)
// already skips removed entities, which is the closest client-observable
// proxy. The "another happy ghast/rider already aboard" branch of
// isCollidable is not implemented — this only covers the case that matters
// for a walking player deciding whether it can stand on one.
//
// Unknown tracked fields (HasIsBaby/HasHappyGhastStayingStill both false,
// e.g. no metadata has arrived yet) resolve to "not baby" / "not staying
// still" — the same "absence of a positive signal is the answer" reasoning
// already used for HorseFlags and the staying-still gate itself, since
// vanilla never transmits a tracked value still at its default.
func isStandableSurface(entityType models.EntityType, e *trackedEntity) bool {
	if entityType != models.EntityTypeHappyGhast {
		return false
	}
	if !e.HappyGhastStayingStill {
		return false
	}
	if e.IsBaby {
		return false
	}
	return true
}
