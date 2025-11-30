package following

import (
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/movement"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/utils"
)

// FollowConfig holds configuration for following behavior
type FollowConfig struct {
	TargetDistance      float64       // Desired distance to maintain from target (blocks)
	StopDistance        float64       // Stop following when within this distance
	RecalcInterval      time.Duration // How often to recalculate path
	RecalcDistThreshold float64       // Recalc if target moves more than this
	MaxPathSteps        int           // Maximum pathfinding search depth
	StuckThreshold      time.Duration // Consider stuck after this time without movement
	MaxStuckAttempts    int           // Maximum recovery attempts before giving up
	JumpRecoveryHeight  float64       // How high to jump for recovery (blocks)
	SprintDistance      float64       // Start sprinting when farther than this (blocks)
	SneakDistance       float64       // Start sneaking when closer than this (blocks)
}

// DefaultFollowConfig returns default configuration
func DefaultFollowConfig() FollowConfig {
	return FollowConfig{
		TargetDistance:      3.0,             // Stay 3 blocks away
		StopDistance:        0.2,             // Stop when within .2 blocks
		RecalcInterval:      2 * time.Second, // Recalc every 2 seconds
		RecalcDistThreshold: 3.0,             // Recalc if target moves 3+ blocks
		MaxPathSteps:        200,             // Search up to 200 steps
		StuckThreshold:      5 * time.Second, // Stuck after 5 seconds
		MaxStuckAttempts:    3,               // Try 3 recovery attempts
		JumpRecoveryHeight:  0.5,             // Jump 0.5 blocks for recovery
		SprintDistance:      8.0,             // Sprint when >8 blocks away
		SneakDistance:       2.5,             // Sneak when <2.5 blocks away
	}
}

// FollowManager manages the follow behavior
type FollowManager interface {
	Start(targetPlayerName string) error
	Stop() error
	IsActive() bool
	GetState() FollowState
	GetStatus() string
	GetPath() *pathfinding.Path
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

	// Stuck detection and recovery (Phase 5)
	stuckAttempts  int       // Number of recovery attempts made
	lastStuckCheck time.Time // Last time we checked for stuck

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

func (fm *followManager) GetPath() *pathfinding.Path {
	return fm.currentPath
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

	// Enhanced target validation
	targetX, targetY, targetZ, exists := fm.targetSelector.GetTargetPosition(fm.targetEntityID)
	if !exists {
		return fm.handleTargetLost("target no longer exists (disconnected or despawned) [" + strconv.FormatInt(int64(fm.targetEntityID), 10) + "]")
	} else {
		log.Printf("current following state: %s", fm.state)
		if fm.state == StateLost {
			fm.state = StateFollowingPath
			fm.sendChatMessage("target re-located")
		}
	}

	// Check for teleportation (large distance change)
	if fm.lastTargetPos.x != 0 || fm.lastTargetPos.y != 0 || fm.lastTargetPos.z != 0 {
		lastDist := fm.calculateDistance(
			fm.lastTargetPos.x, fm.lastTargetPos.y, fm.lastTargetPos.z,
			targetX, targetY, targetZ)

		// If target moved more than 20 blocks in one update, likely teleported
		if lastDist > 20.0 {
			log.Printf("Target teleported %.1f blocks", lastDist)
			fm.sendChatMessage(fmt.Sprintf("%s teleported - recalculating path", fm.targetName))
			// Force path recalculation but don't stop following
			fm.currentPath = nil
			fm.pathIndex = 0
			fm.stuckAttempts = 0 // Reset stuck counter after teleport
		}
	}

	// Check distance to target
	botX, botY, botZ, _, _, initialized := fm.getBotPosition()
	if !initialized {
		return fmt.Errorf("bot position not initialized")
	}

	distance := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)

	// Check if we're close enough
	if distance <= fm.config.StopDistance {
		if fm.state != StateArrived {
			fm.state = StateArrived
			fm.sendChatMessage(fmt.Sprintf("Arrived at %s (%.1f blocks)", fm.targetName, distance))
		}

		return nil
	}

	// CRITICAL: When stationary, we must still send position+rotation packets regularly
	// to keep the server updated and prevent it from unloading entities.
	// LookAt() only sends rotation, so we need to send position+rotation explicitly.

	// Calculate look angles to target
	fromY := botY + 1.62  // Bot eye level
	toY := targetY + 1.62 // Target eye level

	dx := targetX - botX
	dy := toY - fromY
	dz := targetZ - botZ

	horizontalDist := math.Sqrt(dx*dx + dz*dz)
	yaw := float32(math.Atan2(-dx, dz) * 180 / math.Pi)
	pitch := float32(-math.Atan2(dy, horizontalDist) * 180 / math.Pi)

	// Send position AND rotation (not just rotation) to keep server happy
	if err := fm.movementExecutor.SendPositionAndRotation(botX, botY, botZ, yaw, pitch, true); err != nil {
		log.Printf("SendPositionAndRotation error while stationary: %v", err)
	}

	// Always look at the target player (continuous head tracking)
	// This ensures the bot tracks the player even when moving
	if err := fm.movementExecutor.LookAt(targetX, targetY+1.62, targetZ, true); err != nil {
		log.Printf("LookAt target player error: %v", err)
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
// Performance optimized with rate limiting
func (fm *followManager) shouldRecalculatePath(targetX, targetY, targetZ float64) bool {
	// Phase 5: Don't recalculate too frequently (performance optimization)
	timeSinceLastCalc := time.Since(fm.lastPathTime)
	if timeSinceLastCalc < 1*time.Second {
		// Minimum 1 second between recalculations to prevent CPU spikes
		return false
	}

	// Recalc if enough time has passed
	if timeSinceLastCalc > fm.config.RecalcInterval {
		log.Printf("Path recalc: time interval exceeded (%.1fs)", timeSinceLastCalc.Seconds())
		return true
	}

	// Recalc if target moved significantly
	dx := targetX - fm.lastTargetPos.x
	dy := targetY - fm.lastTargetPos.y
	dz := targetZ - fm.lastTargetPos.z
	distMoved := math.Sqrt(dx*dx + dy*dy + dz*dz)

	if distMoved > fm.config.RecalcDistThreshold {
		log.Printf("Path recalc: target moved %.1f blocks", distMoved)
		return true
	}

	// Recalc if path is exhausted
	if fm.currentPath != nil && fm.pathIndex >= len(fm.currentPath.Steps) {
		log.Printf("Path recalc: path exhausted")
		return true
	}

	return false
}

// calculateNewPath calculates a new path to the target
func (fm *followManager) calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	fm.state = StateCalculatingPath

	// Calculate goal position that maintains target distance from player
	// Calculate 3D distance with world integration
	dx := botX - targetX
	dy := botY - targetY
	dz := botZ - targetZ

	// Calculate 3D distance
	distance3D := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Calculate goal position at TargetDistance from target (3D)
	var goalX, goalY, goalZ float64
	if distance3D > 0.1 { // If bot is not at exactly the same position
		// Normalize direction vector and scale to TargetDistance
		goalX = targetX + (dx/distance3D)*fm.config.TargetDistance
		goalY = targetY + (dy/distance3D)*fm.config.TargetDistance
		goalZ = targetZ + (dz/distance3D)*fm.config.TargetDistance
	} else {
		// If at exactly the same position, pick a direction (north)
		goalX = targetX
		goalY = targetY
		goalZ = targetZ - fm.config.TargetDistance
	}

	// Convert to block coordinates
	// IMPORTANT: Trust the server's reported positions rather than trying to "correct" based on world data
	// The world chunk data may be stale/incorrect, but the server position is authoritative
	start := pathfinding.V3{
		X: math.Floor(botX),
		Y: math.Floor(botY), // This is the block containing bot's feet
		Z: math.Floor(botZ),
	}
	goal := pathfinding.V3{
		X: math.Floor(goalX),
		Y: math.Floor(goalY),
		Z: math.Floor(goalZ),
	}

	log.Printf("Pathfinding from (%f, %f, %f) to (%f, %f, %f) [target distance: %.1f blocks from player]",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z, fm.config.TargetDistance)

	line := utils.Line(start, goal)
	lineLength := len(line)

	log.Printf("Pathfinding straight line distance from (%f, %f, %f) to (%f, %f, %f) = %d",
		start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z, lineLength)

	// Allow up to 50x the straight-line distance for pathfinding
	// This gives A* enough steps to explore around obstacles
	// For a 9-block distance, this allows ~450 steps
	maxPathSteps := min(fm.config.MaxPathSteps, 50*lineLength)

	// Find path
	path, err := fm.pathFinder.FindPath(start, goal, maxPathSteps)
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

	// Get target position for distance-based sprint/sneak
	targetX, targetY, targetZ, exists := fm.targetSelector.GetTargetPosition(fm.targetEntityID)
	if exists {
		distance := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)

		// Sprint/Sneak based on distance (only when we have a valid path)
		if distance > fm.config.SprintDistance {
			// Far from target - sprint
			if err := fm.movementExecutor.StartSprinting(); err != nil {
				log.Printf("StartSprinting error: %v", err)
			}
			// Make sure not sneaking while sprinting
			if err := fm.movementExecutor.StopSneaking(); err != nil {
				log.Printf("StopSneaking error: %v", err)
			}
		} else if distance < fm.config.SneakDistance {
			// Very close to target - sneak
			if err := fm.movementExecutor.StopSprinting(); err != nil {
				log.Printf("StopSprinting error: %v", err)
			}
			if err := fm.movementExecutor.StartSneaking(); err != nil {
				log.Printf("StartSneaking error: %v", err)
			}
		} else {
			// Normal distance - walk normally
			if err := fm.movementExecutor.StopSprinting(); err != nil {
				log.Printf("StopSprinting error: %v", err)
			}
			if err := fm.movementExecutor.StopSneaking(); err != nil {
				log.Printf("StopSneaking error: %v", err)
			}
		}
	}

	// Get next step
	step := fm.currentPath.Steps[fm.pathIndex]

	stepIncrement := 0.5
	if fm.movementExecutor.IsSprinting() {
		stepIncrement = 0.8
	} else if fm.movementExecutor.IsSneaking() {
		stepIncrement = 0.3
	}

	// Calculate target position (center of block)
	stepX, stepY, stepZ := float64(step.Position.X), float64(step.Position.Y), float64(step.Position.Z)
	stepTargetX := stepX + stepIncrement
	stepTargetY := stepY + stepIncrement // Use Y from pathfinding step (world integration complete)
	stepTargetZ := stepZ + stepIncrement

	// Check if we've reached this step (compare to block center, not corner)
	distance := fm.calculateDistance(botX, botY, botZ, stepTargetX, stepTargetY, stepTargetZ)

	log.Printf("Following path step %d/%d: bot at (%.1f, %.1f, %.1f), target step at (%f, %f, %f), distance: %.2f",
		fm.pathIndex+1, len(fm.currentPath.Steps), botX, botY, botZ, step.Position.X, step.Position.Y, step.Position.Z, distance)

	if distance < 0.5 { // Within 0.5 blocks of step center
		fm.pathIndex++
		fm.lastMovementTime = time.Now()
		log.Printf("Reached step %d/%d: %s to (%f, %f, %f)",
			fm.pathIndex, len(fm.currentPath.Steps), step.Movement, step.Position.X, step.Position.Y, step.Position.Z)
		return nil
	}

	// Check for stuck (Phase 5: Enhanced with recovery strategies)
	if time.Since(fm.lastMovementTime) > fm.config.StuckThreshold {
		return fm.handleStuck(botX, botY, botZ, stepTargetX, stepTargetY, stepTargetZ)
	}

	// Move towards step incrementally (0.2 blocks per update)
	// (Head tracking is done in main update loop, not here)
	// MoveTowards will update position but we already set rotation above
	// This respects server-side movement validation
	log.Printf("Moving towards (%.1f, %.1f, %.1f) by 0.2 blocks", stepTargetX, stepTargetY, stepTargetZ)
	newX, newY, newZ, err := fm.movementExecutor.MoveTowards(stepTargetX, stepTargetY, stepTargetZ, 0.2, true)
	if err != nil {
		log.Printf("Movement error: %v", err)
		return err
	}

	// Check if we actually moved
	moveDistance := fm.calculateDistance(botX, botY, botZ, newX, newY, newZ)
	log.Printf("After MoveTowards: new pos (%.1f, %.1f, %.1f), moved %.3f blocks", newX, newY, newZ, moveDistance)
	if moveDistance > 0.01 {
		fm.lastMovementTime = time.Now()
		// Reset stuck counter when bot successfully moves
		fm.resetStuckCounter()
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

// ============================================================================
// Enhanced Stuck Detection and Recovery
// ============================================================================

// handleStuck implements recovery strategies when bot is stuck
func (fm *followManager) handleStuck(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	fm.state = StateStuck
	fm.stuckAttempts++

	log.Printf("Stuck detected (attempt %d/%d) at position (%.1f, %.1f, %.1f)",
		fm.stuckAttempts, fm.config.MaxStuckAttempts, botX, botY, botZ)

	// Check if we've exhausted recovery attempts
	if fm.stuckAttempts >= fm.config.MaxStuckAttempts {
		fm.sendChatMessage(fmt.Sprintf("Unable to reach %s after %d attempts - giving up",
			fm.targetName, fm.stuckAttempts))
		log.Printf("Exhausted recovery attempts, stopping follow")
		fm.active = false
		fm.state = StateIdle
		return fmt.Errorf("stuck after %d recovery attempts", fm.stuckAttempts)
	}

	// Try recovery strategies in order
	var recoveryErr error

	switch fm.stuckAttempts {
	case 1:
		// First attempt: Jump in place to potentially get unstuck
		fm.sendChatMessage(fmt.Sprintf("Stuck - attempting jump recovery (attempt %d/%d)",
			fm.stuckAttempts, fm.config.MaxStuckAttempts))
		recoveryErr = fm.tryJumpRecovery(botX, botY, botZ)

	case 2:
		// Second attempt: Try alternate nearby destination
		fm.sendChatMessage(fmt.Sprintf("Stuck - trying alternate path (attempt %d/%d)",
			fm.stuckAttempts, fm.config.MaxStuckAttempts))
		recoveryErr = fm.tryAlternateDestination(botX, botY, botZ, targetX, targetY, targetZ)

	case 3:
		// Third attempt: Move backwards and retry
		fm.sendChatMessage(fmt.Sprintf("Stuck - backing up (attempt %d/%d)",
			fm.stuckAttempts, fm.config.MaxStuckAttempts))
		recoveryErr = fm.tryBackupRecovery(botX, botY, botZ, targetX, targetY, targetZ)

	default:
		// Fallback: Clear path and hope for the best
		fm.sendChatMessage(fmt.Sprintf("Stuck - clearing path (attempt %d/%d)",
			fm.stuckAttempts, fm.config.MaxStuckAttempts))
		recoveryErr = fm.tryClearPathRecovery()
	}

	if recoveryErr != nil {
		log.Printf("Recovery attempt %d failed: %v", fm.stuckAttempts, recoveryErr)
	}

	// Reset movement timer to give recovery a chance
	fm.lastMovementTime = time.Now()
	fm.lastStuckCheck = time.Now()

	return nil
}

// tryJumpRecovery attempts to get unstuck by jumping in place
func (fm *followManager) tryJumpRecovery(botX, botY, botZ float64) error {
	log.Printf("Attempting jump recovery at (%.1f, %.1f, %.1f)", botX, botY, botZ)

	// Jump up by configured height
	jumpY := botY + fm.config.JumpRecoveryHeight

	// Send position update to jump
	err := fm.movementExecutor.SendPosition(botX, jumpY, botZ, false) // OnGround = false while jumping
	if err != nil {
		return fmt.Errorf("jump recovery failed: %w", err)
	}

	// Wait a moment for jump to complete
	time.Sleep(100 * time.Millisecond)

	// Land back down
	err = fm.movementExecutor.SendPosition(botX, botY, botZ, true) // OnGround = true after landing
	if err != nil {
		return fmt.Errorf("jump landing failed: %w", err)
	}

	// Clear current path to force recalculation
	fm.currentPath = nil
	fm.pathIndex = 0

	log.Printf("Jump recovery completed")
	return nil
}

// tryAlternateDestination tries to path to a nearby alternate location
func (fm *followManager) tryAlternateDestination(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	log.Printf("Attempting alternate destination recovery")

	// Try destinations at different angles around the target
	alternateOffsets := []struct{ dx, dz float64 }{
		{2, 0},   // East
		{-2, 0},  // West
		{0, 2},   // South
		{0, -2},  // North
		{1, 1},   // SE
		{-1, 1},  // SW
		{1, -1},  // NE
		{-1, -1}, // NW
	}

	for i, offset := range alternateOffsets {
		altX := targetX + offset.dx
		altZ := targetZ + offset.dz

		// Convert to block coordinates
		start := pathfinding.V3{
			X: math.Floor(botX),
			Y: math.Floor(botY),
			Z: math.Floor(botZ),
		}
		goal := pathfinding.V3{
			X: math.Floor(altX),
			Y: math.Floor(botY), // Keep same Y level
			Z: math.Floor(altZ),
		}

		log.Printf("Trying alternate destination %d: (%f, %f, %f)", i+1, goal.X, goal.Y, goal.Z)

		// Try to find path to alternate destination
		path, err := fm.pathFinder.FindPath(start, goal, fm.config.MaxPathSteps)
		if err == nil && path.Found {
			log.Printf("Found alternate path with %d steps", len(path.Steps))
			fm.currentPath = path
			fm.pathIndex = 0
			fm.lastPathTime = time.Now()
			return nil
		}
	}

	return fmt.Errorf("no alternate destination found")
}

// tryBackupRecovery moves the bot backwards and clears path
func (fm *followManager) tryBackupRecovery(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	log.Printf("Attempting backup recovery")

	// Calculate direction away from target
	dx := botX - targetX
	dz := botZ - targetZ
	dist := math.Sqrt(dx*dx + dz*dz)

	if dist < 0.01 {
		// If too close, just move in arbitrary direction
		dx = 1.0
		dz = 0.0
		dist = 1.0
	}

	// Normalize and move back 1 block
	dx /= dist
	dz /= dist

	backupX := botX + dx*1.0
	backupZ := botZ + dz*1.0

	log.Printf("Moving backwards from (%.1f, %.1f, %.1f) to (%.1f, %.1f, %.1f)",
		botX, botY, botZ, backupX, botY, backupZ)

	// Send position update to move backwards
	err := fm.movementExecutor.SendPosition(backupX, botY, backupZ, true)
	if err != nil {
		return fmt.Errorf("backup movement failed: %w", err)
	}

	// Clear path to force recalculation from new position
	fm.currentPath = nil
	fm.pathIndex = 0

	log.Printf("Backup recovery completed")
	return nil
}

// tryClearPathRecovery simply clears the current path to force recalculation
func (fm *followManager) tryClearPathRecovery() error {
	log.Printf("Clearing path for fresh recalculation")

	fm.currentPath = nil
	fm.pathIndex = 0
	fm.lastPathTime = time.Time{} // Force immediate recalc

	return nil
}

// resetStuckCounter resets stuck detection when bot successfully moves
func (fm *followManager) resetStuckCounter() {
	if fm.stuckAttempts > 0 {
		log.Printf("Bot recovered! Resetting stuck counter (was %d attempts)", fm.stuckAttempts)
		fm.stuckAttempts = 0
	}
}

// ============================================================================
// Phase 5: Enhanced Target Validation
// ============================================================================

// handleTargetLost handles the case when target is lost (disconnect, despawn, etc.)
func (fm *followManager) handleTargetLost(reason string) error {
	log.Printf("[TARGET_LOST] transitioning following state from %s to %s", fm.state.String(), StateLost.String())
	fm.state = StateLost
	fm.sendChatMessage(fmt.Sprintf("Lost target %s: %s", fm.targetName, reason))
	log.Printf("Target lost: %s", reason)
	// fm.active = false
	return fmt.Errorf("target lost: %s", reason)
}
