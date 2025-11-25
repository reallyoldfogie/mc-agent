package following

import (
	"fmt"
	"math"
	"sync"
)

// TrackedEntity represents a tracked entity (from main.go)
type TrackedEntity struct {
	UUID    [16]byte
	X, Y, Z float64
	Yaw     int8
	Pitch   int8
}

// TargetSelector finds and validates target players
type TargetSelector struct {
	getTrackedEntities func() map[int32]*TrackedEntity
	getPlayerUUID      func(playerName string) ([16]byte, error)
	getBotPosition     func() (x, y, z float64, initialized bool)
}

// NewTargetSelector creates a new target selector
func NewTargetSelector(
	getEntities func() map[int32]*TrackedEntity,
	getPlayerUUID func(string) ([16]byte, error),
	getBotPos func() (float64, float64, float64, bool),
) *TargetSelector {
	return &TargetSelector{
		getTrackedEntities: getEntities,
		getPlayerUUID:      getPlayerUUID,
		getBotPosition:     getBotPos,
	}
}

// TargetInfo contains information about a selected target
type TargetInfo struct {
	EntityID int32
	UUID     [16]byte
	Name     string
	X, Y, Z  float64
	Distance float64
}

// FindPlayerByName finds a player entity by name
func (ts *TargetSelector) FindPlayerByName(name string) (*TargetInfo, error) {
	// Get player UUID from player list
	uuid, err := ts.getPlayerUUID(name)
	if err != nil {
		return nil, fmt.Errorf("player %s not found in player list: %w", name, err)
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
			botX, botY, botZ, initialized := ts.getBotPosition()
			if initialized {
				dx := entity.X - botX
				dy := entity.Y - botY
				dz := entity.Z - botZ
				distance = math.Sqrt(dx*dx + dy*dy + dz*dz)
			}

			return &TargetInfo{
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

	return nil, fmt.Errorf("player %s not found in tracked entities (may be too far away)", name)
}

// FindNearestPlayer finds the nearest player entity
func (ts *TargetSelector) FindNearestPlayer() (*TargetInfo, error) {
	entities := ts.getTrackedEntities()
	if entities == nil || len(entities) == 0 {
		return nil, fmt.Errorf("no entities tracked")
	}

	botX, botY, botZ, initialized := ts.getBotPosition()
	if !initialized {
		return nil, fmt.Errorf("bot position not initialized")
	}

	var nearest *TargetInfo
	minDistance := math.MaxFloat64

	for entityID, entity := range entities {
		dx := entity.X - botX
		dy := entity.Y - botY
		dz := entity.Z - botZ
		distance := math.Sqrt(dx*dx + dy*dy + dz*dz)

		if distance < minDistance {
			minDistance = distance
			nearest = &TargetInfo{
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
func (ts *TargetSelector) GetTargetPosition(entityID int32) (x, y, z float64, exists bool) {
	entities := ts.getTrackedEntities()
	if entities == nil {
		return 0, 0, 0, false
	}

	entity, ok := entities[entityID]
	if !ok {
		return 0, 0, 0, false
	}

	return entity.X, entity.Y, entity.Z, true
}

// CalculateDistance calculates distance to target
func (ts *TargetSelector) CalculateDistance(entityID int32) (float64, error) {
	x, y, z, exists := ts.GetTargetPosition(entityID)
	if !exists {
		return 0, fmt.Errorf("target entity not found")
	}

	botX, botY, botZ, initialized := ts.getBotPosition()
	if !initialized {
		return 0, fmt.Errorf("bot position not initialized")
	}

	dx := x - botX
	dy := y - botY
	dz := z - botZ
	return math.Sqrt(dx*dx + dy*dy + dz*dz), nil
}

// SafeTrackedEntities provides thread-safe access to tracked entities
type SafeTrackedEntities struct {
	mu       sync.RWMutex
	entities map[int32]*TrackedEntity
}

// NewSafeTrackedEntities creates a new thread-safe entity tracker
func NewSafeTrackedEntities() *SafeTrackedEntities {
	return &SafeTrackedEntities{
		entities: make(map[int32]*TrackedEntity),
	}
}

// Get returns a copy of the entities map
func (ste *SafeTrackedEntities) Get() map[int32]*TrackedEntity {
	ste.mu.RLock()
	defer ste.mu.RUnlock()

	// Return a copy to avoid concurrent access issues
	copy := make(map[int32]*TrackedEntity, len(ste.entities))
	for k, v := range ste.entities {
		entityCopy := *v
		copy[k] = &entityCopy
	}
	return copy
}

// Set updates or adds an entity
func (ste *SafeTrackedEntities) Set(entityID int32, entity *TrackedEntity) {
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
