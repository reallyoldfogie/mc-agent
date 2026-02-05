package agent

import (
	"fmt"
	"math"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
)

// trackedEntity mirrors the runtime fields we need to follow players/entities.
type trackedEntity struct {
	EntityID   int32
	EntityType int32
	UUID       [16]byte
	X, Y, Z    float64
	Yaw        int8
	Pitch      int8
	Health     float32 // Current health (0 = dead)
	MaxHealth  float32 // Maximum health (typically 20.0 for mobs)
	Removed    bool
	RemovedAt  time.Time
}

// TrackedEntityInfo exposes entity tracking data for external use (tests, following, etc.)
type TrackedEntityInfo = models.TrackedEntityInfo

// GetPosition returns the current bot position and rotation.
func (a *agent) GetPosition() (x, y, z float64, yaw, pitch float32, initialized bool) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	return a.posX, a.posY, a.posZ, a.posYaw, a.posPitch, a.posInitialized
}

// setPosition updates the bot position and rotation.
func (a *agent) setPosition(x, y, z float64, yaw, pitch float32) {
	a.posMu.Lock()
	a.posX, a.posY, a.posZ = x, y, z
	a.posYaw, a.posPitch = yaw, pitch
	a.posInitialized = true
	a.posMu.Unlock()
}

// GetPositionSimple returns bot position without rotation.
func (a *agent) GetPositionSimple() (x, y, z float64, initialized bool) {
	a.posMu.RLock()
	defer a.posMu.RUnlock()
	return a.posX, a.posY, a.posZ, a.posInitialized
}

// GetEntityID returns the bot's entity ID.
func (a *agent) GetEntityID() int32 {
	a.entIDMu.RLock()
	defer a.entIDMu.RUnlock()
	return a.entID
}

// setEntityID sets the bot's entity ID.
func (a *agent) setEntityID(id int32) {
	a.entIDMu.Lock()
	a.entID = id
	a.entIDMu.Unlock()
}

// snapshotEntities returns a shallow copy of tracked entities for external consumption/tests.
func (a *agent) snapshotEntities() map[int32]trackedEntity {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]trackedEntity, len(a.entities))
	for id, e := range a.entities {
		out[id] = *e
	}
	return out
}

// startEntityCleanup runs a periodic task to purge soft-deleted entities.
func (a *agent) startEntityCleanup(ctxDone <-chan struct{}) {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		ticker := time.NewTicker(EntityCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctxDone:
				return
			case <-ticker.C:
				a.cleanupRemovedEntities()
			}
		}
	}()
}

func (a *agent) cleanupRemovedEntities() {
	now := time.Now()
	a.entitiesMu.Lock()
	for id, e := range a.entities {
		if e.Removed && now.Sub(e.RemovedAt) > EntityRemovalGracePeriod {
			delete(a.entities, id)
		}
	}
	a.entitiesMu.Unlock()
}

// getTrackedEntitiesForFollowing converts internal entities to following.TrackedEntity map.
func (a *agent) GetTrackedEntitiesForFollowing() map[int32]*models.TrackedEntity {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]*models.TrackedEntity, len(a.entities))
	for id, e := range a.entities {
		out[id] = &models.TrackedEntity{UUID: e.UUID, X: e.X, Y: e.Y, Z: e.Z, Yaw: e.Yaw, Pitch: e.Pitch}
	}
	return out
}

// GetTrackedEntities returns a snapshot of all tracked entities for external consumption (tests, debugging, etc.)
func (a *agent) GetTrackedEntities() map[int32]TrackedEntityInfo {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]TrackedEntityInfo, len(a.entities))
	for id, e := range a.entities {
		out[id] = TrackedEntityInfo{
			EntityID:   e.EntityID,
			EntityType: e.EntityType,
			UUID:       e.UUID,
			X:          e.X,
			Y:          e.Y,
			Z:          e.Z,
			Yaw:        e.Yaw,
			Pitch:      e.Pitch,
			Health:     e.Health,
			MaxHealth:  e.MaxHealth,
			Removed:    e.Removed,
		}
	}
	return out
}

// FindNearestEntityByType finds the nearest entity of a specific type to a position.
// Returns entityID, distance, and whether found.
// Excludes entities marked as Removed.
func (a *agent) FindNearestEntityByType(entityType int32, x, y, z float64) (int32, float64, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	var nearestID int32
	nearestDist := math.MaxFloat64
	found := false

	for id, e := range a.entities {
		if e.Removed || e.EntityType != entityType {
			continue
		}

		dx := e.X - x
		dy := e.Y - y
		dz := e.Z - z
		dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if dist < nearestDist {
			nearestDist = dist
			nearestID = id
			found = true
		}
	}

	return nearestID, nearestDist, found
}

// FaceEntity makes the bot look at a target entity by calculating and sending rotation.
func (a *agent) FaceEntity(entityID int32) error {
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	// Get current bot position
	botX, botY, botZ, initialized := a.GetPositionSimple()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	// Get target entity position
	a.entitiesMu.RLock()
	targetEnt, exists := a.entities[entityID]
	a.entitiesMu.RUnlock()

	if !exists {
		return fmt.Errorf("target entity %d not found", entityID)
	}

	// Calculate eye-level position (add player eye height)
	botEyeY := botY + 1.62 // Minecraft player eye height

	// Calculate deltas
	dx := targetEnt.X - botX
	dy := targetEnt.Y - botEyeY
	dz := targetEnt.Z - botZ

	// Calculate yaw (horizontal rotation)
	yaw := math.Atan2(-dx, -dz) * 180 / math.Pi

	// Calculate pitch (vertical rotation)
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch := math.Atan2(-dy, horizontalDist) * 180 / math.Pi

	// Send rotation packet
	return a.versionHandler.Play().Movement().SendRotation(a.client.Conn(), float32(yaw), float32(pitch), true)
}

// FacePosition makes the bot look at a specific position.
func (a *agent) FacePosition(targetX, targetY, targetZ float64) error {
	if a.versionHandler == nil || a.client == nil {
		return fmt.Errorf("version handler or client not initialized")
	}

	// Get current bot position
	botX, botY, botZ, initialized := a.GetPositionSimple()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	// Calculate eye-level position (add player eye height)
	botEyeY := botY + 1.62 // Minecraft player eye height

	// Calculate deltas
	dx := targetX - botX
	dy := targetY - botEyeY
	dz := targetZ - botZ

	// Calculate yaw (horizontal rotation)
	yaw := math.Atan2(-dx, -dz) * 180 / math.Pi

	// Calculate pitch (vertical rotation)
	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	pitch := math.Atan2(-dy, horizontalDist) * 180 / math.Pi

	// Send rotation packet
	return a.versionHandler.Play().Movement().SendRotation(a.client.Conn(), float32(yaw), float32(pitch), true)
}
