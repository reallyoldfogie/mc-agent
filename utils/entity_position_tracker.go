package utils

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/reallyoldfogie/mc-agent/models"
)

const positionWindowSize = 1000

// EntityPositionTracker tracks entity position updates via callbacks.
// It records the peak (maximum) and minimum coordinates reached by an entity over time.
// Useful for movement analysis, testing, and monitoring entity behavior.
type EntityPositionTracker struct {
	entityID         int32
	initialPos       models.V3 // Initial position of the entity when the tracker is created
	minPos           models.V3 // Minimum coordinates reached
	maxPos           models.V3 // Maximum coordinates reached
	trackedPositions *RingBuffer[models.V3]
	mu               sync.RWMutex
	logger           *slog.Logger
}

func (t *EntityPositionTracker) String() string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return fmt.Sprintf("EntityPositionTracker for EntityID %d:\n  Initial Position: %s\n  Max Position: %s\n  Min Position: %s\n  Distance from Initial to Min: %.02f\n  Distance from Initial to Max: %.02f\n  Distance between Min and Max: %.02f\n  Tracked Positions: %s",
		t.entityID, t.initialPos, t.maxPos, t.minPos,
		t.GetMinHorizontalDistanceFromInitial(),
		t.GetMaxHorizontalDistanceFromInitial(),
		t.GetMinMaxHorizontalDistance(),
		t.trackedPositions)
}

// NewEntityPositionTracker creates a tracker that uses callbacks to track entity positions.
// The entity parameter should provide a RegisterEntityPositionCallback method that accepts
// a callback function with signature: func(entityID int32, x, y, z float64)
func NewEntityPositionTracker(entityID int32, initialPos models.V3, logger *slog.Logger) *EntityPositionTracker {
	logger = SafeLogger(logger)
	logger.Debug("creating EntityPositionTracker", "entityID", entityID, "x", initialPos.X, "y", initialPos.Y, "z", initialPos.Z)

	return &EntityPositionTracker{
		entityID:         entityID,
		initialPos:       initialPos,
		minPos:           initialPos,
		maxPos:           initialPos,
		trackedPositions: NewRingBuffer[models.V3](positionWindowSize),
		logger:           logger,
	}
}

// RegisterCallback registers this tracker with an entity position callback system.
// Pass in the agent or any object that has RegisterEntityPositionCallback method.
func (t *EntityPositionTracker) RegisterCallback(callbackRegistry models.EntityCallbackRegistry) {
	SafeLogger(t.logger).Debug("registering EntityPositionTracker callback", "entityID", t.entityID)

	callbackRegistry.RegisterEntityPositionCallback(t.positionCallback)
}
func (t *EntityPositionTracker) positionCallback(id int32, x, y, z float64) {
	// Only track the entity we're interested in
	if id != t.entityID {
		return
	}

	SafeLogger(t.logger).Debug("EntityPositionTracker callback received position update", "entityID", id, "x", x, "y", y, "z", z)

	t.mu.Lock()
	defer t.mu.Unlock()

	t.trackedPositions.Add(models.V3{X: x, Y: y, Z: z})

	// Track peak (max) coordinates
	if x > t.maxPos.X {
		t.maxPos.X = x
	}
	if y > t.maxPos.Y {
		t.maxPos.Y = y
	}
	if z > t.maxPos.Z {
		t.maxPos.Z = z
	}

	// Track minimum coordinates
	if x < t.minPos.X {
		t.minPos.X = x
	}
	if y < t.minPos.Y {
		t.minPos.Y = y
	}
	if z < t.minPos.Z {
		t.minPos.Z = z
	}
}

// GetMaxPos returns the maximum coordinates observed
func (t *EntityPositionTracker) GetMaxPos() models.V3 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.maxPos
}

// GetMinPos returns the minimum coordinates observed
func (t *EntityPositionTracker) GetMinPos() models.V3 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.minPos
}

// GetRange returns the range (peak - min) for each coordinate
func (t *EntityPositionTracker) GetRange() models.V3 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return models.V3{
		X: t.maxPos.X - t.minPos.X,
		Y: t.maxPos.Y - t.minPos.Y,
		Z: t.maxPos.Z - t.minPos.Z,
	}
}

// GetMinMaxAltitude returns the vertical distance traveled (max.Y - min.Y)
func (t *EntityPositionTracker) GetMinMaxAltitude() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.maxPos.Y - t.minPos.Y
}

// GetMinMaxAltitude returns the vertical distance traveled (initial.Y - min.Y)
func (t *EntityPositionTracker) GetMinAltitudeFromInitial() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.initialPos.Y - t.minPos.Y
}

// GetMinMaxAltitude returns the vertical distance traveled (initial.Y - max.Y)
func (t *EntityPositionTracker) GetMaxAltitudeFromInitial() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.initialPos.Y - t.maxPos.Y
}

// GetMinMaxHorizontalDistance returns the horizontal distance traveled (max of X and Z ranges)
func (t *EntityPositionTracker) GetMinMaxHorizontalDistance() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	rangeX := t.maxPos.X - t.minPos.X
	rangeZ := t.maxPos.Z - t.minPos.Z
	if rangeX > rangeZ {
		return rangeX
	}
	return rangeZ
}

// GetInitialPos returns the initial position of the entity when the tracker was created
func (t *EntityPositionTracker) GetInitialPos() models.V3 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.initialPos
}

func (t *EntityPositionTracker) GetMaxHorizontalDistanceFromInitial() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Calculate horizontal distance from initial position to current max position
	maxDist := t.initialPos.DistanceTo(t.maxPos)
	minDist := t.initialPos.DistanceTo(t.minPos)

	if maxDist > minDist {
		return maxDist
	}
	return minDist
}

func (t *EntityPositionTracker) GetMinHorizontalDistanceFromInitial() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Calculate horizontal distance from initial position to current max position
	maxDist := t.initialPos.DistanceTo(t.maxPos)
	minDist := t.initialPos.DistanceTo(t.minPos)

	if maxDist < minDist {
		return maxDist
	}
	return minDist
}

// GetTrackedPositions returns a copy of the list of all tracked positions
func (t *EntityPositionTracker) GetTrackedPositions() []models.V3 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.trackedPositions.Items()
}
