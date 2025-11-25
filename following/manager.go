package following

import (
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/movement"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
)

// FollowConfig holds configuration for following behavior
type FollowConfig struct {
	TargetDistance      float64       // Desired distance to maintain from target (blocks)
	StopDistance        float64       // Stop following when within this distance
	RecalcInterval      time.Duration // How often to recalculate path
	RecalcDistThreshold float64       // Recalc if target moves more than this
	MaxPathSteps        int           // Maximum pathfinding search depth
	StuckThreshold      time.Duration // Consider stuck after this time without movement
}

// DefaultFollowConfig returns default configuration
func DefaultFollowConfig() FollowConfig {
	return FollowConfig{
		TargetDistance:      3.0,             // Stay 3 blocks away
		StopDistance:        2.0,             // Stop when within 2 blocks
		RecalcInterval:      2 * time.Second, // Recalc every 2 seconds
		RecalcDistThreshold: 3.0,             // Recalc if target moves 3+ blocks
		MaxPathSteps:        200,             // Search up to 200 steps
		StuckThreshold:      5 * time.Second, // Stuck after 5 seconds
	}
}

// FollowManager manages the follow behavior
type FollowManager interface {
	Start(targetPlayerName string) error
	Stop() error
	IsActive() bool
	GetState() FollowState
	GetStatus() string
}

// followManager implements FollowManager
type followManager struct {
	mu     sync.RWMutex
	active bool
	state  FollowState

	// Target info
	targetName     string
	targetEntityID int32
	lastTargetPos  struct {
		x, y, z float64
	}

	// Path info
	currentPath      *pathfinding.Path
	pathIndex        int
	lastPathTime     time.Time
	lastMovementTime time.Time

	// Control
	stopChan chan struct{}
	config   FollowConfig

	// Dependencies
	targetSelector   *TargetSelector
	pathFinder       pathfinding.PathFinder
	movementExecutor movement.MovementExecutor
	getBotPosition   func() (x, y, z float64, yaw, pitch float32, initialized bool)
	sendChatMessage  func(string) error
}

// NewFollowManager creates a new follow manager
func NewFollowManager(
	targetSelector *TargetSelector,
	pathFinder pathfinding.PathFinder,
	movementExecutor movement.MovementExecutor,
	getBotPos func() (float64, float64, float64, float32, float32, bool),
	sendChat func(string) error,
	config FollowConfig,
) FollowManager {
	return &followManager{
		targetSelector:   targetSelector,
		pathFinder:       pathFinder,
		movementExecutor: movementExecutor,
		getBotPosition:   getBotPos,
		sendChatMessage:  sendChat,
		config:           config,
		state:            StateIdle,
	}
}

// Start begins following a player
func (fm *followManager) Start(targetPlayerName string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.active {
		return fmt.Errorf("already following %s", fm.targetName)
	}

	// Find target player
	fm.state = StateTargeting
	target, err := fm.targetSelector.FindPlayerByName(targetPlayerName)
	if err != nil {
		fm.state = StateIdle
		return fmt.Errorf("failed to find target: %w", err)
	}

	// Initialize follow state
	fm.active = true
	fm.targetName = targetPlayerName
	fm.targetEntityID = target.EntityID
	fm.lastTargetPos.x = target.X
	fm.lastTargetPos.y = target.Y
	fm.lastTargetPos.z = target.Z
	fm.stopChan = make(chan struct{})
	fm.pathIndex = 0
	fm.lastMovementTime = time.Now()

	// Start follow loop
	go fm.followLoop()

	return nil
}

// Stop stops following
func (fm *followManager) Stop() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if !fm.active {
		return fmt.Errorf("not currently following")
	}

	fm.active = false
	fm.state = StateIdle
	close(fm.stopChan)

	return nil
}

// IsActive returns whether following is active
func (fm *followManager) IsActive() bool {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.active
}

// GetState returns the current state
func (fm *followManager) GetState() FollowState {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
	return fm.state
}

// GetStatus returns a human-readable status string
func (fm *followManager) GetStatus() string {
	fm.mu.RLock()
	defer fm.mu.RUnlock()

	if !fm.active {
		return "Not following"
	}

	distance, _ := fm.targetSelector.CalculateDistance(fm.targetEntityID)
	return fmt.Sprintf("Following %s | Distance: %.1f blocks | State: %s",
		fm.targetName, distance, fm.state)
}

// followLoop is the main follow loop (runs in goroutine)
func (fm *followManager) followLoop() {
	ticker := time.NewTicker(50 * time.Millisecond) // Update 20 times per second to match minecraft tick rate
	defer ticker.Stop()

	for {
		select {
		case <-fm.stopChan:
			return
		case <-ticker.C:
			if err := fm.update(); err != nil {
				log.Printf("Follow loop error: %v", err)
				// Don't stop on error, just log it
			}
		}
	}
}

// update performs one iteration of the follow loop
func (fm *followManager) update() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if !fm.active {
		return nil
	}

	// Check if target still exists
	targetX, targetY, targetZ, exists := fm.targetSelector.GetTargetPosition(fm.targetEntityID)
	if !exists {
		fm.state = StateLost
		fm.sendChatMessage(fmt.Sprintf("Lost target %s", fm.targetName))
		fm.active = false
		return fmt.Errorf("target lost")
	}

	// Check distance to target
	botX, botY, botZ, _, _, initialized := fm.getBotPosition()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	distance := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)

	// Always look at the target player (continuous head tracking)
	// This ensures the bot tracks the player even when stationary or arrived
	if err := fm.movementExecutor.LookAt(targetX, targetY+1.62, targetZ, true); err != nil {
		log.Printf("LookAt target player error: %v", err)
	}

	// Check if we're close enough
	if distance <= fm.config.StopDistance {
		if fm.state != StateArrived {
			fm.state = StateArrived
			fm.sendChatMessage(fmt.Sprintf("Arrived at %s (%.1f blocks)", fm.targetName, distance))
		}
		return nil
	}

	// Check if we need to recalculate path
	needsRecalc := fm.shouldRecalculatePath(targetX, targetY, targetZ)

	if needsRecalc || fm.currentPath == nil {
		return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
	}

	// Follow current path
	return fm.followCurrentPath(botX, botY, botZ)
}

// shouldRecalculatePath checks if path needs recalculation
func (fm *followManager) shouldRecalculatePath(targetX, targetY, targetZ float64) bool {
	// Recalc if enough time has passed
	if time.Since(fm.lastPathTime) > fm.config.RecalcInterval {
		return true
	}

	// Recalc if target moved significantly
	dx := targetX - fm.lastTargetPos.x
	dy := targetY - fm.lastTargetPos.y
	dz := targetZ - fm.lastTargetPos.z
	distMoved := math.Sqrt(dx*dx + dy*dy + dz*dz)

	if distMoved > fm.config.RecalcDistThreshold {
		return true
	}

	// Recalc if path is exhausted
	if fm.currentPath != nil && fm.pathIndex >= len(fm.currentPath.Steps) {
		return true
	}

	return false
}

// calculateNewPath calculates a new path to the target
func (fm *followManager) calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	fm.state = StateCalculatingPath

	// Calculate goal position that maintains target distance from player
	// Phase 3: Use horizontal-only distance since we disabled vertical movement
	// TODO Phase 4: Switch back to 3D distance when vertical movement is re-enabled
	dx := botX - targetX
	dz := botZ - targetZ

	// Calculate horizontal distance
	horizontalDist := math.Sqrt(dx*dx + dz*dz)

	// Calculate goal position at TargetDistance from target (horizontal only)
	var goalX, goalZ float64
	if horizontalDist > 0.1 { // If bot is not at exactly the same position horizontally
		// Normalize direction vector and scale to TargetDistance
		goalX = targetX + (dx/horizontalDist)*fm.config.TargetDistance
		goalZ = targetZ + (dz/horizontalDist)*fm.config.TargetDistance
	} else {
		// If at exactly the same position horizontally, pick a direction (north)
		goalX = targetX
		goalZ = targetZ - fm.config.TargetDistance
	}

	// Keep goal at bot's current Y level (horizontal movement only for Phase 3/4 without world integration)
	goalY := botY

	// Convert to block coordinates
	// For start, use floor to get the block the bot is standing on
	start := pathfinding.V3{
		X: int(math.Floor(botX)),
		Y: int(math.Floor(botY)), // This is the block containing bot's feet
		Z: int(math.Floor(botZ)),
	}
	goal := pathfinding.V3{
		X: int(math.Floor(goalX)),
		Y: int(math.Floor(goalY)), // Keep same Y as bot
		Z: int(math.Floor(goalZ)),
	}

	log.Printf("Pathfinding from (%d, %d, %d) to (%d, %d, %d) [target distance: %.1f blocks from player]",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z, fm.config.TargetDistance)

	// Find path
	path, err := fm.pathFinder.FindPath(start, goal, fm.config.MaxPathSteps)
	if err != nil {
		fm.state = StateStuck
		log.Printf("Pathfinding failed: %v", err)
		return err
	}

	if !path.Found {
		fm.state = StateStuck
		return fmt.Errorf("no path found")
	}

	// Update path state
	fm.currentPath = path
	fm.pathIndex = 0
	fm.lastPathTime = time.Now()
	fm.lastTargetPos.x = targetX
	fm.lastTargetPos.y = targetY
	fm.lastTargetPos.z = targetZ
	fm.state = StateFollowingPath

	log.Printf("New path calculated: %d steps, cost %.2f", len(path.Steps), path.TotalCost)
	return nil
}

// followCurrentPath executes the current path
func (fm *followManager) followCurrentPath(botX, botY, botZ float64) error {
	if fm.currentPath == nil || fm.pathIndex >= len(fm.currentPath.Steps) {
		return nil
	}

	// Get next step
	step := fm.currentPath.Steps[fm.pathIndex]

	// Calculate target position (center of block)
	stepX, stepZ := float64(step.Position.X), float64(step.Position.Z)
	targetX := stepX + 0.5
	// IMPORTANT: Keep bot at current Y level to avoid walking on air
	// Until we have world integration, we maintain the bot's Y position
	// The step.Position.Y is used as the "target block", but we move to the bot's actual Y
	targetY := botY  // Use bot's current Y instead of step Y
	targetZ := stepZ + 0.5

	// Check if we've reached this step (compare to block center, not corner)
	distance := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)

	log.Printf("Following path step %d/%d: bot at (%.1f, %.1f, %.1f), target step at (%d, %d, %d), distance: %.2f",
		fm.pathIndex+1, len(fm.currentPath.Steps), botX, botY, botZ, step.Position.X, step.Position.Y, step.Position.Z, distance)

	if distance < 0.5 { // Within 0.5 blocks of step center
		fm.pathIndex++
		fm.lastMovementTime = time.Now()
		log.Printf("Reached step %d/%d: %s to (%d, %d, %d)",
			fm.pathIndex, len(fm.currentPath.Steps), step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
		return nil
	}

	// Check for stuck
	if time.Since(fm.lastMovementTime) > fm.config.StuckThreshold {
		fm.state = StateStuck
		fm.sendChatMessage(fmt.Sprintf("Stuck while following %s - attempting recovery", fm.targetName))
		log.Printf("Stuck detected, performing recovery")

		// Aggressive recovery: clear path and stop briefly
		fm.currentPath = nil
		fm.pathIndex = 0

		// Reset movement timer to give recovery a chance
		fm.lastMovementTime = time.Now()

		return nil
	}

	// Move towards step incrementally (0.2 blocks per update)
	// (Head tracking is done in main update loop, not here)
	// MoveTowards will update position but we already set rotation above
	// This respects server-side movement validation
	log.Printf("Moving towards (%.1f, %.1f, %.1f) by 0.2 blocks", targetX, targetY, targetZ)
	newX, newY, newZ, err := fm.movementExecutor.MoveTowards(targetX, targetY, targetZ, 0.2, true)
	if err != nil {
		log.Printf("Movement error: %v", err)
		return err
	}

	// Check if we actually moved
	moveDistance := fm.calculateDistance(botX, botY, botZ, newX, newY, newZ)
	log.Printf("After MoveTowards: new pos (%.1f, %.1f, %.1f), moved %.3f blocks", newX, newY, newZ, moveDistance)
	if moveDistance > 0.01 {
		fm.lastMovementTime = time.Now()
	}

	return nil
}

// calculateDistance is a helper to calculate 3D distance
func (fm *followManager) calculateDistance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	dz := z2 - z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}
