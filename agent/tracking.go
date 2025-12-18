package agent

import (
	"time"

	"github.com/reallyoldfogie/mc-agent/following"
)

// trackedEntity mirrors the runtime fields we need to follow players/entities.
type trackedEntity struct {
	EntityID   int32
	EntityType int32
	UUID       [16]byte
	X, Y, Z    float64
	Yaw        int8
	Pitch      int8
	Removed    bool
	RemovedAt  time.Time
}

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
func (a *agent) GetTrackedEntitiesForFollowing() map[int32]*following.TrackedEntity {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	out := make(map[int32]*following.TrackedEntity, len(a.entities))
	for id, e := range a.entities {
		out[id] = &following.TrackedEntity{UUID: e.UUID, X: e.X, Y: e.Y, Z: e.Z, Yaw: e.Yaw, Pitch: e.Pitch}
	}
	return out
}
