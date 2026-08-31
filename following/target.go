package following

import (
	"crypto/md5"
	"fmt"
	"math"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

type targetSelector struct {
	getTrackedEntities func() map[int32]*models.TrackedEntity
	getPlayerUUID      func(playerName string) ([16]byte, error)
	getBotPosition     func() (models.V3, bool)
}

// NewTargetSelector creates a new target selector
func NewTargetSelector(
	getEntities func() map[int32]*models.TrackedEntity,
	getPlayerUUID func(string) ([16]byte, error),
	getBotPos func() (models.V3, bool),
) models.TargetSelector {
	return &targetSelector{
		getTrackedEntities: getEntities,
		getPlayerUUID:      getPlayerUUID,
		getBotPosition:     getBotPos,
	}
}

// FindPlayerByName finds a player entity by name
func (ts *targetSelector) FindPlayerByName(name string) (*models.TargetInfo, error) {
	// Get player UUID from player list
	uuid, err := ts.getPlayerUUID(name)
	uuidSource := "player list"
	if err != nil {
		uuid = deriveOfflineUUID(name)
		uuidSource = "offline UUID"
	}

	// Get tracked entities
	entities := ts.getTrackedEntities()
	if entities == nil {
		return nil, fmt.Errorf("no entities tracked")
	}

	// Find entity with matching UUID
	for entityID, entity := range entities {
		if entity.UUID == uuid {
			// Calculate distance to bot
			distance := 0.0
			botPos, initialized := ts.getBotPosition()
			if initialized {
				dx := entity.X - botPos.X
				dy := entity.Y - botPos.Y
				dz := entity.Z - botPos.Z
				distance = math.Sqrt(dx*dx + dy*dy + dz*dz)
			}

			return &models.TargetInfo{
				EntityID: entityID,
				UUID:     uuid,
				Name:     name,
				X:        entity.X,
				Y:        entity.Y,
				Z:        entity.Z,
				Distance: distance,
			}, nil
		}
	}

	return nil, fmt.Errorf("player %s not found in tracked entities (UUID source: %s, may be too far away)", name, uuidSource)
}

// deriveOfflineUUID mirrors vanilla's own offline-mode UUID assignment
// (Java's UUID.nameUUIDFromBytes("OfflinePlayer:"+name)): a raw MD5 digest
// of the name bytes with version (3) and IETF variant bits set on the
// digest directly - NOT an RFC4122 "UUIDv3 with namespace" derivation,
// which additionally prepends the namespace UUID's own bytes before
// hashing and produces a different (wrong) result. Confirmed live: the
// previous implementation (uuid.NewMD5(uuid.NameSpaceOID, ...), the
// namespaced form) never matched a real offline-mode server's actual
// assigned UUID, silently breaking this whole fallback path (FindPlayerByName
// would report "not found" for any target whose UUID wasn't independently
// resolvable via the player list first).
func deriveOfflineUUID(name string) [16]byte {
	sum := md5.Sum([]byte("OfflinePlayer:" + name))
	sum[6] = (sum[6] & 0x0f) | 0x30 // version 3
	sum[8] = (sum[8] & 0x3f) | 0x80 // IETF variant
	return sum
}

// FindNearestPlayer finds the nearest player entity
func (ts *targetSelector) FindNearestPlayer() (*models.TargetInfo, error) {
	entities := ts.getTrackedEntities()
	if len(entities) == 0 {
		return nil, fmt.Errorf("no entities tracked")
	}

	botPos, initialized := ts.getBotPosition()
	if !initialized {
		return nil, fmt.Errorf("bot position not initialized")
	}

	var nearest *models.TargetInfo
	minDistance := math.MaxFloat64

	for entityID, entity := range entities {
		dx := entity.X - botPos.X
		dy := entity.Y - botPos.Y
		dz := entity.Z - botPos.Z
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if distance < minDistance {
			minDistance = distance
			nearest = &models.TargetInfo{
				EntityID: entityID,
				UUID:     entity.UUID,
				Name:     "", // We don't know the name without reverse lookup
				X:        entity.X,
				Y:        entity.Y,
				Z:        entity.Z,
				Distance: distance,
			}
		}
	}

	if nearest == nil {
		return nil, fmt.Errorf("no players found")
	}

	return nearest, nil
}

// GetTargetPosition gets the current position of a target entity
func (ts *targetSelector) GetTargetPosition(entityID int32) (models.V3, bool) {
	entities := ts.getTrackedEntities()
	if entities == nil {
		return models.V3{}, false
	}

	entity, ok := entities[entityID]
	if !ok {
		return models.V3{}, false
	}

	return models.V3{X: entity.X, Y: entity.Y, Z: entity.Z}, true
}

// CalculateDistance calculates distance to target
func (ts *targetSelector) CalculateDistance(entityID int32) (float64, error) {
	targetPos, exists := ts.GetTargetPosition(entityID)
	if !exists {
		return 0, fmt.Errorf("target entity not found")
	}

	botPos, initialized := ts.getBotPosition()
	if !initialized {
		return 0, fmt.Errorf("bot position not initialized")
	}

	dx := targetPos.X - botPos.X
	dy := targetPos.Y - botPos.Y
	dz := targetPos.Z - botPos.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz), nil
}

// SafeTrackedEntities provides thread-safe access to tracked entities
type SafeTrackedEntities struct {
	mu       sync.RWMutex
	entities map[int32]*models.TrackedEntity
}

// NewSafeTrackedEntities creates a new thread-safe entity tracker
func NewSafeTrackedEntities() *SafeTrackedEntities {
	return &SafeTrackedEntities{
		entities: make(map[int32]*models.TrackedEntity),
	}
}

// Get returns a copy of the entities map
func (ste *SafeTrackedEntities) Get() map[int32]*models.TrackedEntity {
	ste.mu.RLock()
	defer ste.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	copy := make(map[int32]*models.TrackedEntity, len(ste.entities))
	for k, v := range ste.entities {
		entityCopy := *v
		copy[k] = &entityCopy
	}
	return copy
}

// Set updates or adds an entity
func (ste *SafeTrackedEntities) Set(entityID int32, entity *models.TrackedEntity) {
	ste.mu.Lock()
	defer ste.mu.Unlock()
	ste.entities[entityID] = entity
}

// Delete removes an entity
func (ste *SafeTrackedEntities) Delete(entityID int32) {
	ste.mu.Lock()
	defer ste.mu.Unlock()
	delete(ste.entities, entityID)
}
