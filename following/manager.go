package following

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/movement"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/utils"
)

type pathExecutor interface {
	SetPath(*pathfinding.Path) error
	ClearPath()
}

// followManager implements FollowManager
type followManager struct {
	mu     sync.RWMutex
	active bool
	state  models.FollowState

	getFollowerName func() string

	// Target info
	targetName     string
	targetEntityID int32
	lastTargetPos  struct {
		x, y, z float64
	}
	lastTargetPosTime time.Time
	// Target position history for velocity calculation
	targetPosHistory []struct {
		x, y, z   float64
		timestamp time.Time
	}
	historyMaxSize      int
	targetWasStationary bool // Track if target was recently stationary

	// Path info
	currentPath       *models.Path
	lastPathTime      time.Time
	lastPathTargetPos models.V3 // Target position when last path was calculated

	// Stuck detection and recovery
	stuckAttempts  int       // Number of recovery attempts made
	lastStuckCheck time.Time // Last time we checked for stuck
	lastBotPos     struct {
		x, y, z float64
	}
	lastBotPosTime    time.Time
	lastProgressCheck time.Time

	// Logging
	lastDetailedLog       time.Time
	lastRecalcDecisionLog time.Time
	lastRecalcStatsLog    time.Time

	// Stats (reset every ~1s)
	statTickTotal          int
	statShouldRecalcCalls  int
	statSkipInactive       int
	statSkipTargetLost     int
	statSkipArrived        int
	statSkipTransition     int
	statSkipUninit         int
	statSkipPathNil        int
	statSkipProgressRecalc int

	// Timers
	lastTransitionTime time.Time

	// Control
	stopChan chan struct{}
	config   FollowConfig

	// Dependencies
	targetSelector   models.TargetSelector
	pathFinder       models.PathFinder
	movementExecutor movement.MovementExecutor
	pathExecutor     pathExecutor
	getBotPosition   func() (x, y, z float64, yaw, pitch float64, initialized bool)
	sendChatMessage  func(string) error
}

// NewFollowManager creates a new follow manager
func NewFollowManager(
	targetSelector models.TargetSelector,
	pathFinder models.PathFinder,
	movementExecutor movement.MovementExecutor,
	getBotPos func() (float64, float64, float64, float64, float64, bool),
	sendChat func(string) error,
	config FollowConfig,
	getFollowerName func() string,
) models.FollowManager {
	var pe pathExecutor
	if exec, ok := movementExecutor.(pathExecutor); ok {
		pe = exec
	}
	return &followManager{
		getFollowerName:     getFollowerName,
		targetSelector:      targetSelector,
		pathFinder:          pathFinder,
		movementExecutor:    movementExecutor,
		pathExecutor:        pe,
		getBotPosition:      getBotPos,
		sendChatMessage:     sendChat,
		config:              config,
		state:               models.StateIdle,
		historyMaxSize:      5,    // Track last 5 positions for velocity calculation
		targetWasStationary: true, // Assume target starts stationary
	}
}

// Start begins following a player
func (fm *followManager) Start(targetPlayerName string) error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	if fm.active {
		return fmt.Errorf("already following %s", fm.targetName)
	}

	log.Printf("[FollowManager %s] Start called, target='%s'", fm.getFollowerName(), targetPlayerName)

	// Initialize follow state
	fm.state = models.StateTargeting
	fm.active = true
	fm.targetName = targetPlayerName
	fm.targetEntityID = 0
	fm.lastTargetPos = struct{ x, y, z float64 }{}
	fm.lastTargetPosTime = time.Time{}
	fm.lastPathTargetPos = models.V3{}
	fm.stopChan = make(chan struct{})
	fm.lastPathTime = time.Time{}
	fm.currentPath = nil
	fm.targetPosHistory = nil
	fm.targetWasStationary = true
	fm.stuckAttempts = 0
	fm.lastStuckCheck = time.Time{}
	fm.lastBotPosTime = time.Time{}
	fm.lastProgressCheck = time.Now()
	fm.lastTransitionTime = time.Time{}
	fm.lastDetailedLog = time.Time{}
	fm.lastRecalcDecisionLog = time.Time{}
	fm.lastRecalcStatsLog = time.Time{}
	fm.statTickTotal = 0
	fm.statShouldRecalcCalls = 0
	fm.statSkipInactive = 0
	fm.statSkipTargetLost = 0
	fm.statSkipArrived = 0
	fm.statSkipTransition = 0
	fm.statSkipUninit = 0
	fm.statSkipPathNil = 0
	fm.statSkipProgressRecalc = 0

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

	fm.clearPath()
	fm.active = false
	fm.state = models.StateIdle
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
func (fm *followManager) GetState() models.FollowState {
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

func (fm *followManager) GetPath() *models.Path {
	fm.mu.RLock()
	defer fm.mu.RUnlock()
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
				log.Printf("[FollowManager %s] Follow loop error: %v", fm.getFollowerName(), err)
				// Don't stop on error, just log it
			}
		}
	}
}

// update performs one iteration of the follow loop
func (fm *followManager) update() error {
	fm.mu.Lock()
	defer fm.mu.Unlock()

	// Tick + periodic stats log (once per second per follower)
	fm.statTickTotal++
	if fm.lastRecalcStatsLog.IsZero() || time.Since(fm.lastRecalcStatsLog) >= 1*time.Second {
		log.Printf("[FollowManager %s] RecalcStats ticks=%d calls=%d skips={inactive:%d target_lost:%d arrived:%d transition:%d uninit:%d path_nil:%d progress:%d}",
			fm.getFollowerName(),
			fm.statTickTotal,
			fm.statShouldRecalcCalls,
			fm.statSkipInactive,
			fm.statSkipTargetLost,
			fm.statSkipArrived,
			fm.statSkipTransition,
			fm.statSkipUninit,
			fm.statSkipPathNil,
			fm.statSkipProgressRecalc,
		)
		fm.statTickTotal = 0
		fm.statShouldRecalcCalls = 0
		fm.statSkipInactive = 0
		fm.statSkipTargetLost = 0
		fm.statSkipArrived = 0
		fm.statSkipTransition = 0
		fm.statSkipUninit = 0
		fm.statSkipPathNil = 0
		fm.statSkipProgressRecalc = 0
		fm.lastRecalcStatsLog = time.Now()
	}

	if !fm.active {
		fm.statSkipInactive++
		return nil
	}

	// Enhanced target validation
	var (
		targetX, targetY, targetZ float64
		exists                    bool
	)
	if fm.targetEntityID == 0 {
		target, err := fm.targetSelector.FindPlayerByName(fm.targetName)
		if err != nil {
			// Target not visible yet; keep waiting in targeting mode.
			return nil
		}
		fm.targetEntityID = target.EntityID
		fm.lastTargetPos.x = target.X
		fm.lastTargetPos.y = target.Y
		fm.lastTargetPos.z = target.Z
		fm.lastTargetPosTime = time.Now()
		fm.lastPathTargetPos = models.V3{X: target.X, Y: target.Y, Z: target.Z}
		fm.currentPath = nil
		fm.stuckAttempts = 0
		fm.lastPathTime = time.Now()
		fm.targetPosHistory = nil
		fm.targetWasStationary = true
		fm.state = models.StateFollowingPath
		targetX, targetY, targetZ = target.X, target.Y, target.Z
		exists = true
	} else {
		targetX, targetY, targetZ, exists = fm.targetSelector.GetTargetPosition(fm.targetEntityID)
	}
	if !exists {
		fm.statSkipTargetLost++
		return fm.handleTargetLost("target no longer exists (disconnected or despawned) [" + strconv.FormatInt(int64(fm.targetEntityID), 10) + "]")
	}
	log.Printf("[FollowManager %s] current following state: %s", fm.getFollowerName(), fm.state)
	if fm.state == models.StateLost {
		fm.state = models.StateFollowingPath
		fm.sendChatMessage("target re-located")
	}

	var targetMovedDistance float64

	// Check for teleportation (large distance change relative to elapsed time)
	// Only check if we have a previous position recorded
	if !fm.lastTargetPosTime.IsZero() {
		lastDist := fm.calculateDistance(
			fm.lastTargetPos.x, fm.lastTargetPos.y, fm.lastTargetPos.z,
			targetX, targetY, targetZ)
		targetMovedDistance = lastDist

		// Calculate time elapsed since last position check
		elapsedTime := time.Since(fm.lastTargetPosTime)

		// Calculate maximum expected movement based on elapsed time
		// Players can sprint at ~5.6 m/s in Minecraft
		// Add 50% safety margin to account for speed effects, elytra, etc.
		maxExpectedMovement := (5.6 * elapsedTime.Seconds()) * 1.5

		// If target moved significantly more than expected, likely teleported
		if lastDist > maxExpectedMovement && lastDist > 2.0 {
			log.Printf("[FollowManager %s] Target teleported %.1f blocks from (%.2f %.2f %.2f) to (%.2f %.2f %.2f) in %.2fs (expected max: %.1f)",
				fm.getFollowerName(),
				lastDist,
				fm.lastTargetPos.x, fm.lastTargetPos.y, fm.lastTargetPos.z,
				targetX, targetY, targetZ,
				elapsedTime.Seconds(),
				maxExpectedMovement)

			fm.sendChatMessage(fmt.Sprintf("[FollowManager %s] %s may have teleported - recalculating path", fm.getFollowerName(), fm.targetName))
			// Force path recalculation but don't stop following
			fm.currentPath = nil
			fm.stuckAttempts = 0 // Reset stuck counter after teleport
		}
	}

	// Add current position to history BEFORE transition detection so velocity can be calculated
	fm.addPositionToHistory(targetX, targetY, targetZ, time.Now())

	// Update lastTargetPos after teleportation check to track position changes between updates
	fm.lastTargetPos.x = targetX
	fm.lastTargetPos.y = targetY
	fm.lastTargetPos.z = targetZ
	fm.lastTargetPosTime = time.Now()

	// Detect target stationary-to-moving transition
	// Calculate velocity from accumulated history (need at least 2 samples)
	transitionedToMoving := false
	if len(fm.targetPosHistory) >= 2 {
		vx, vy, vz := fm.calculateTargetVelocity()
		velocityMagnitude := math.Sqrt(vx*vx + vy*vy + vz*vz)

		if fm.targetWasStationary && velocityMagnitude > 0.5 {
			// Target transitioned from stationary to moving based on velocity
			log.Printf("[FollowManager %s] Target started moving (velocity %.2f b/s) - clearing stale position history and forcing immediate recalc",
				fm.getFollowerName(), velocityMagnitude)
			fm.targetPosHistory = nil
			// Add current position as first entry in fresh history
			fm.addPositionToHistory(targetX, targetY, targetZ, time.Now())
			fm.targetWasStationary = false
			transitionedToMoving = true
			fm.statSkipTransition++
			fm.lastTransitionTime = time.Now()
		} else if velocityMagnitude < 0.1 {
			// Target is stationary (velocity < 0.1 b/s)
			fm.targetWasStationary = true
		} else {
			// Target is moving continuously
			fm.targetWasStationary = false
		}
	}

	// Check distance to target
	botX, botY, botZ, _, _, initialized := fm.getBotPosition()
	if !initialized {
		fm.statSkipUninit++
		return fmt.Errorf("bot position not initialized")
	}

	distance := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)

	// Periodic detailed logging (every 5 seconds)
	if fm.lastDetailedLog.IsZero() || time.Since(fm.lastDetailedLog) > 5*time.Second {
		vx, vy, vz := fm.calculateTargetVelocity()
		velocityMagnitude := math.Sqrt(vx*vx + vy*vy + vz*vz)
		timeSinceLastPath := time.Since(fm.lastPathTime)
		predictiveStr := "no"
		if velocityMagnitude > 1.0 {
			predictiveStr = "yes"
		}
		log.Printf("[FollowManager %s] === Follow State === Distance: %.2f | Velocity: %.2f b/s | State: %s | Last path: %.1fs ago | Predictive: %s",
			fm.getFollowerName(), distance, velocityMagnitude, fm.state, timeSinceLastPath.Seconds(), predictiveStr)
		fm.lastDetailedLog = time.Now()
	}

	// Pause only when we're close and the target is not moving.
	arrivedGate := distance <= fm.config.StopDistance && targetMovedDistance <= fm.config.TargetStillDistance

	// Minimum chase window after movement transition
	chaseWindow := 500 * time.Millisecond
	if !fm.lastTransitionTime.IsZero() && time.Since(fm.lastTransitionTime) < chaseWindow {
		// Suppress arrived gate during chase window
		if arrivedGate {
			log.Printf("[FollowManager %s] Arrived gate SUPPRESSED during chase window (%.0fms) dist=%.2f, moved=%.2f", fm.getFollowerName(), chaseWindow.Seconds()*1000, distance, targetMovedDistance)
		}
		arrivedGate = false
	}

	// If target just started moving, override Arrived gate immediately.
	if arrivedGate && transitionedToMoving {
		if fm.state == models.StateArrived {
			log.Printf("[FollowManager %s] Arrived gate OFF (transition detected) dist=%.2f, moved=%.2f>%.2f", fm.getFollowerName(), distance, targetMovedDistance, fm.config.TargetStillDistance)
			fm.state = models.StateFollowingPath
		}
		// Do not early-return; allow immediate recalc below.
	} else if arrivedGate {
		// Normal Arrived handling (no movement)
		if fm.state != models.StateArrived {
			log.Printf("[FollowManager %s] Arrived gate ON dist=%.2f<=%.2f, still=%.2f<=%.2f", fm.getFollowerName(), distance, fm.config.StopDistance, targetMovedDistance, fm.config.TargetStillDistance)
			fm.state = models.StateArrived
			fm.sendChatMessage(fmt.Sprintf("Arrived at %s (%.1f blocks)", fm.targetName, distance))
			fm.clearPath()
		}
		fm.statSkipArrived++
		return nil
	} else if fm.state == models.StateArrived {
		log.Printf("[FollowManager %s] Arrived gate OFF dist=%.2f>%.2f or moved=%.2f>%.2f", fm.getFollowerName(), distance, fm.config.StopDistance, targetMovedDistance, fm.config.TargetStillDistance)
		fm.state = models.StateFollowingPath
	}

	// For executors without path execution capability, apply head tracking and MoveTowards fallback
	if fm.pathExecutor == nil {
		return fm.followWithMoveTowards(botX, botY, botZ, targetX, targetY, targetZ)
	}

	// Force immediate recalculation if target just started moving
	if transitionedToMoving {
		return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
	}

	// For executors with path capability, only handle path recalculation and stuck recovery
	return fm.monitorPathExecution(botX, botY, botZ, targetX, targetY, targetZ)
}

// monitorPathExecution monitors path execution by the executor and handles recalculation/recovery
func (fm *followManager) monitorPathExecution(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	// Progress-based stuck detection: check if bot and target have diverging movement
	now := time.Now()
	if fm.lastProgressCheck.IsZero() {
		fm.lastProgressCheck = now
	}
	if !fm.lastProgressCheck.IsZero() && now.Sub(fm.lastProgressCheck) > 2*time.Second {
		// Calculate bot movement since last progress check
		botDistMoved := 0.0
		if !fm.lastBotPosTime.IsZero() {
			botDistMoved = fm.calculateDistance(
				fm.lastBotPos.x, fm.lastBotPos.y, fm.lastBotPos.z,
				botX, botY, botZ)
		}
		if botDistMoved > 0.3 {
			// Treat forward progress as non-stuck.
			fm.lastPathTime = now
		}

		// Calculate target movement since last progress check
		targetDistMoved := 0.0
		if len(fm.targetPosHistory) >= 2 {
			// Compare current target position with position from 2 seconds ago
			for i := len(fm.targetPosHistory) - 1; i >= 0; i-- {
				if now.Sub(fm.targetPosHistory[i].timestamp) >= 2*time.Second {
					targetDistMoved = fm.calculateDistance(
						fm.targetPosHistory[i].x, fm.targetPosHistory[i].y, fm.targetPosHistory[i].z,
						targetX, targetY, targetZ)
					break
				}
			}
		}

		// If bot hasn't moved but target has moved significantly, force immediate recalc
		if botDistMoved < 0.5 && targetDistMoved > 2.0 {
			fm.statSkipProgressRecalc++
			log.Printf("[FollowManager %s] Stuck following outdated path: bot moved %.2f, target moved %.2f - forcing recalc",
				fm.getFollowerName(), botDistMoved, targetDistMoved)
			return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
		}

		fm.lastProgressCheck = now
	}

	// Update last bot position tracking
	if fm.lastBotPosTime.IsZero() || now.Sub(fm.lastBotPosTime) > 500*time.Millisecond {
		fm.lastBotPos.x = botX
		fm.lastBotPos.y = botY
		fm.lastBotPos.z = botZ
		fm.lastBotPosTime = now
	}

	// Check if we need to recalculate path
	needsRecalc := fm.shouldRecalculatePath(targetX, targetY, targetZ)

	if fm.currentPath == nil {
		fm.statSkipPathNil++
		log.Printf("[FollowManager %s] monitor: currentPath is nil, forcing path calculation", fm.getFollowerName())
		return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
	}

	if needsRecalc {
		log.Printf("[FollowManager %s] monitor: needsRecalc=true (dist=%.2f, sinceLastPath=%.2fs)", fm.getFollowerName(), fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ), time.Since(fm.lastPathTime).Seconds())
		return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
	}

	// Check for stuck condition by monitoring if target hasn't changed
	// and executor hasn't made progress (executor tracks movement internally)
	if time.Since(fm.lastPathTime) > fm.config.StuckThreshold {
		// Also check if distance to target has increased (moving away from target)
		currentDist := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)
		if fm.currentPath != nil && len(fm.currentPath.Steps) > 0 {
			// Compare with distance when path was calculated
			startPos := fm.currentPath.Steps[0].Position
			initialDist := fm.calculateDistance(startPos.X, startPos.Y, startPos.Z, targetX, targetY, targetZ)
			if currentDist > initialDist {
				log.Printf("[FollowManager %s] Distance to target increased (%.2f -> %.2f) - forcing recalc",
					fm.getFollowerName(), initialDist, currentDist)
				return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
			}
		}

		// Stuck - executor hasn't completed path in reasonable time
		return fm.handleStuck(botX, botY, botZ, targetX, targetY, targetZ)
	}

	return nil
}

// followWithMoveTowards implements simple path following for executors without path capability
func (fm *followManager) followWithMoveTowards(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	// Check if we need to recalculate path
	needsRecalc := fm.shouldRecalculatePath(targetX, targetY, targetZ)

	if needsRecalc || fm.currentPath == nil {
		return fm.calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ)
	}

	if fm.currentPath == nil {
		return nil
	}

	// Get target position for distance-based sprint/sneak
	distance := fm.calculateDistance(botX, botY, botZ, targetX, targetY, targetZ)

	// Sprint/Sneak based on distance
	if distance > fm.config.SprintDistance {
		// Far from target - sprint
		if err := fm.movementExecutor.StartSprinting(); err != nil {
			log.Printf("[FollowManager %s] StartSprinting error: %v", fm.getFollowerName(), err)
		}
		// Make sure not sneaking while sprinting
		if err := fm.movementExecutor.StopSneaking(); err != nil {
			log.Printf("[FollowManager %s] StopSneaking error: %v", fm.getFollowerName(), err)
		}
	} else if distance < fm.config.SneakDistance {
		// Very close to target - sneak
		if err := fm.movementExecutor.StopSprinting(); err != nil {
			log.Printf("[FollowManager %s] StopSprinting error: %v", fm.getFollowerName(), err)
		}
		if err := fm.movementExecutor.StartSneaking(); err != nil {
			log.Printf("[FollowManager %s] StartSneaking error: %v", fm.getFollowerName(), err)
		}
	} else {
		// Normal distance - walk normally
		if err := fm.movementExecutor.StopSprinting(); err != nil {
			log.Printf("[FollowManager %s] StopSprinting error: %v", fm.getFollowerName(), err)
		}
		if err := fm.movementExecutor.StopSneaking(); err != nil {
			log.Printf("[FollowManager %s] StopSneaking error: %v", fm.getFollowerName(), err)
		}
	}

	// Look at the target player
	if err := fm.movementExecutor.LookAt(targetX, targetY, targetZ, true); err != nil {
		log.Printf("[FollowManager %s] LookAt target player error: %v", fm.getFollowerName(), err)
	}

	// For fallback MoveTowards path following, move towards target
	// This is a simple path following without explicit path stepping
	_, _, _, err := fm.movementExecutor.MoveTowards(targetX, targetY, targetZ, 0.2, true)
	if err != nil {
		log.Printf("[FollowManager %s] Movement error: %v", fm.getFollowerName(), err)
		return err
	}

	return nil
}

// shouldRecalculatePath checks if path needs recalculation
// Simply checks if target has moved beyond threshold
func (fm *followManager) shouldRecalculatePath(targetX, targetY, targetZ float64) bool {
	fm.statShouldRecalcCalls++
	if !fm.lastPathTime.IsZero() {
		if time.Since(fm.lastPathTime) < fm.config.MinRecalcInterval {
			return false
		}
		if time.Since(fm.lastPathTime) > fm.config.RecalcInterval {
			return true
		}
	}
	// Check if target moved significantly since last path calculation
	dx := targetX - fm.lastPathTargetPos.X
	dy := targetY - fm.lastPathTargetPos.Y
	dz := targetZ - fm.lastPathTargetPos.Z
	distMoved := math.Sqrt(dx*dx + dy*dy + dz*dz)
	threshold := fm.config.RecalcDistThreshold

	if distMoved > threshold {
		log.Printf("[FollowManager %s] Path recalc needed: target moved %.2f blocks (from %.2f,%.2f,%.2f to %.2f,%.2f,%.2f)",
			fm.getFollowerName(), distMoved, fm.lastPathTargetPos.X, fm.lastPathTargetPos.Y, fm.lastPathTargetPos.Z, targetX, targetY, targetZ)
		return true
	}

	// Throttled debug log when no recalc is needed (at most once per second)
	if fm.lastRecalcDecisionLog.IsZero() || time.Since(fm.lastRecalcDecisionLog) >= 1*time.Second {
		log.Printf("[FollowManager %s] No recalc: target moved %.2f ≤ %.2f since last path (from %.2f,%.2f,%.2f to %.2f,%.2f,%.2f)",
			fm.getFollowerName(), distMoved, threshold, fm.lastPathTargetPos.X, fm.lastPathTargetPos.Y, fm.lastPathTargetPos.Z, targetX, targetY, targetZ)
		fm.lastRecalcDecisionLog = time.Now()
	}

	return false
}

// calculateNewPath calculates a new path to the target
func (fm *followManager) calculateNewPath(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	fm.state = models.StateCalculatingPath

	log.Printf("[FollowManager %s] Calculating new path from (%.2f %.2f %.2f) to (%.2f %.2f %.2f)", fm.getFollowerName(), botX, botY, botZ, targetX, targetY, targetZ)

	// Calculate target velocity for predictive targeting
	vx, vy, vz := fm.calculateTargetVelocity()
	velocityMagnitude := math.Sqrt(vx*vx + vy*vy + vz*vz)

	// Apply predictive targeting if target is moving
	predictedTargetX, predictedTargetY, predictedTargetZ := targetX, targetY, targetZ
	if velocityMagnitude > 1.0 { // Target is moving (>1 block/sec)
		// Predict target position based on velocity and config factor
		predictionTime := fm.config.TargetVelocityFactor * 2.0 // 0.5 factor = 1 second ahead
		predictedTargetX = targetX + vx*predictionTime
		predictedTargetY = targetY + vy*predictionTime
		predictedTargetZ = targetZ + vz*predictionTime
		log.Printf("[FollowManager %s] Predictive targeting: velocity=%.2f b/s, predicting %.2fs ahead to (%.2f, %.2f, %.2f)",
			fm.getFollowerName(), velocityMagnitude, predictionTime, predictedTargetX, predictedTargetY, predictedTargetZ)
	}

	// Calculate goal position: follow behind the leader's direction of movement
	// If leader is moving, position goal behind leader along movement direction
	// If leader is stationary, position goal at current position
	var goalX, goalY, goalZ float64
	if velocityMagnitude > 0.5 { // Leader is moving
		// Normalize velocity vector
		vMag := velocityMagnitude
		vxNorm := vx / vMag
		vyNorm := vy / vMag
		vzNorm := vz / vMag
		// Position goal TargetDistance blocks BEHIND predicted target position
		// (opposite to movement direction)
		goalX = predictedTargetX - vxNorm*fm.config.TargetDistance
		goalY = predictedTargetY - vyNorm*fm.config.TargetDistance
		goalZ = predictedTargetZ - vzNorm*fm.config.TargetDistance
	} else {
		// Leader is stationary, just path toward leader position
		// Goal is the predicted target position itself (or close to it)
		goalX = predictedTargetX
		goalY = predictedTargetY
		goalZ = predictedTargetZ
	}

	// Convert to block coordinates for pathfinding
	// IMPORTANT: Both start AND goal must be floored to block coordinates because
	// A* operates on integer block positions. Using fractional goal coordinates
	// causes A* to never reach the goal (nearest integer positions are outside goalRadius).
	start := models.V3{
		X: math.Floor(botX),
		Y: math.Floor(botY),
		Z: math.Floor(botZ),
	}

	goal := models.V3{
		X: math.Floor(goalX),
		Y: math.Floor(goalY),
		Z: math.Floor(goalZ),
	}

	log.Printf("[FollowManager %s] Pathfinding from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) [target distance: %.1f blocks from player]",
		fm.getFollowerName(), start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z, fm.config.TargetDistance)

	line := utils.Line(start, goal)
	lineLength := len(line)

	log.Printf("[FollowManager %s] Pathfinding straight line distance from (%.2f, %.2f, %.2f) to (%.2f, %.2f, %.2f) = %d",
		fm.getFollowerName(), start.X, start.Y, start.Z, goal.X, goal.Y, goal.Z, lineLength)

	// Allow up to 50x the straight-line distance for pathfinding
	// This gives A* enough steps to explore around obstacles
	// For a 9-block distance, this allows ~450 steps
	// Use max() to ensure we use at least the configured MaxPathSteps
	maxPathSteps := max(fm.config.MaxPathSteps, 50*lineLength)

	// Find path with timeout to prevent blocking forever
	type pathResult struct {
		path *models.Path
		err  error
	}
	resultCh := make(chan pathResult, 1)

	go func() {
		path, err := fm.pathFinder.FindPath(context.Background(), start, goal, maxPathSteps)
		resultCh <- pathResult{path: path, err: err}
	}()

	var path *models.Path
	var err error

	select {
	case result := <-resultCh:
		path = result.path
		err = result.err
	case <-time.After(fm.config.PathfindingTimeout):
		fm.state = models.StateStuck
		log.Printf("[FollowManager %s] Pathfinding timed out after %v", fm.getFollowerName(), fm.config.PathfindingTimeout)
		return fmt.Errorf("pathfinding timed out after %v", fm.config.PathfindingTimeout)
	}

	if err != nil {
		fm.state = models.StateStuck
		log.Printf("[FollowManager %s] Pathfinding failed: %v", fm.getFollowerName(), err)
		return err
	}

	if path == nil || !path.Found {
		fm.state = models.StateStuck
		log.Printf("[FollowManager %s] no path found", fm.getFollowerName())
		return fmt.Errorf("no path found")
	}

	// Update path state
	fm.currentPath = path
	fm.lastPathTime = time.Now()
	fm.lastPathTargetPos = models.V3{X: targetX, Y: targetY, Z: targetZ}
	log.Printf("[FollowManager %s] lastPathTargetPos updated to (%.2f, %.2f, %.2f)", fm.getFollowerName(), fm.lastPathTargetPos.X, fm.lastPathTargetPos.Y, fm.lastPathTargetPos.Z)
	// Note: lastTargetPos is now updated in the main update loop to track per-tick changes
	fm.state = models.StateFollowingPath
	fm.stuckAttempts = 0

	// Delegate path execution to executor if it supports it
	if fm.pathExecutor != nil {
		if err := fm.pathExecutor.SetPath(path); err != nil {
			log.Printf("[FollowManager %s] Failed to set path on executor: %v", fm.getFollowerName(), err)
			return err
		}
	}

	// Log detailed path information
	log.Printf("[FollowManager %s]  %s", fm.getFollowerName(), path.LogSummary())
	log.Printf("[FollowManager %s]  Path details:\n%s", fm.getFollowerName(), path.LogDetails(true))

	return nil
}

// calculateDistance is a helper to calculate 3D distance
func (fm *followManager) calculateDistance(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	dz := z2 - z1
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// addPositionToHistory adds a position sample to the history for velocity calculation
func (fm *followManager) addPositionToHistory(x, y, z float64, timestamp time.Time) {
	// Add new position to history
	fm.targetPosHistory = append(fm.targetPosHistory, struct {
		x, y, z   float64
		timestamp time.Time
	}{
		x:         x,
		y:         y,
		z:         z,
		timestamp: timestamp,
	})

	// Keep only the most recent samples
	if len(fm.targetPosHistory) > fm.historyMaxSize {
		fm.targetPosHistory = fm.targetPosHistory[1:]
	}
}

// calculateTargetVelocity calculates target velocity based on position history
// Returns velocity in blocks/second for each axis
func (fm *followManager) calculateTargetVelocity() (vx, vy, vz float64) {
	if len(fm.targetPosHistory) < 2 {
		return 0, 0, 0
	}

	// Use oldest and newest positions for velocity calculation
	oldest := fm.targetPosHistory[0]
	newest := fm.targetPosHistory[len(fm.targetPosHistory)-1]

	// Calculate time difference
	deltaTime := newest.timestamp.Sub(oldest.timestamp).Seconds()
	if deltaTime < 0.001 { // Avoid division by zero
		return 0, 0, 0
	}

	// Calculate velocity (blocks per second)
	vx = (newest.x - oldest.x) / deltaTime
	vy = (newest.y - oldest.y) / deltaTime
	vz = (newest.z - oldest.z) / deltaTime

	return vx, vy, vz
}

// ============================================================================
// Stuck Detection and Recovery
// ============================================================================

// handleStuck implements recovery strategies when bot is stuck
func (fm *followManager) handleStuck(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	fm.state = models.StateStuck
	fm.stuckAttempts++

	log.Printf("[FollowManager %s] Stuck detected (attempt %d/%d) at position (%.1f, %.1f, %.1f)",
		fm.getFollowerName(), fm.stuckAttempts, fm.config.MaxStuckAttempts, botX, botY, botZ)

	// Check if we've exhausted recovery attempts
	if fm.stuckAttempts >= fm.config.MaxStuckAttempts {
		fm.sendChatMessage(fmt.Sprintf("Unable to reach %s after %d attempts - giving up",
			fm.targetName, fm.stuckAttempts))
		log.Printf("[FollowManager %s] Exhausted recovery attempts, stopping follow", fm.getFollowerName())
		fm.active = false
		fm.state = models.StateIdle
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
		// First attempt: try to move sideways to potentially get unstuck
		fm.sendChatMessage(fmt.Sprintf("Stuck - attempting sideways recovery (attempt %d/%d)",
			fm.stuckAttempts, fm.config.MaxStuckAttempts))
		recoveryErr = fm.trySidewaysRecovery(botX, botY, botZ, targetX, targetY, targetZ)

	case 3:
		// Second attempt: Try alternate nearby destination
		fm.sendChatMessage(fmt.Sprintf("Stuck - trying alternate path (attempt %d/%d)",
			fm.stuckAttempts, fm.config.MaxStuckAttempts))
		recoveryErr = fm.tryAlternateDestination(botX, botY, botZ, targetX, targetY, targetZ)

	case 4:
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
		log.Printf("[FollowManager %s] Recovery attempt %d failed: %v", fm.getFollowerName(), fm.stuckAttempts, recoveryErr)
	}

	// Reset path timer to give recovery a chance
	fm.lastPathTime = time.Now()
	fm.lastStuckCheck = time.Now()

	return nil
}

// tryJumpRecovery attempts to get unstuck by jumping in place
func (fm *followManager) tryJumpRecovery(botX, botY, botZ float64) error {
	log.Printf("[FollowManager %s] Attempting jump recovery at (%.1f, %.1f, %.1f)", fm.getFollowerName(), botX, botY, botZ)

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
	fm.clearPath()

	log.Printf("[FollowManager %s] Jump recovery completed", fm.getFollowerName())
	return nil
}

// trySidewaysRecovery attempts to get unstuck by moving sideways
func (fm *followManager) trySidewaysRecovery(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	log.Printf("[FollowManager %s] Attempting sideways recovery at (%.1f, %.1f, %.1f)", fm.getFollowerName(), botX, botY, botZ)

	var err error
	sidewaysX := botX
	sidewaysZ := botZ
	for attempt := range 4 {
		moveX := 0.0
		moveZ := 0.0
		switch attempt {
		case 0:
			moveZ = -1.0 // North
		case 1:
			moveZ = 1.0 // South
		case 2:
			moveX = 1.0 // East
		case 3:
			moveX = -1.0 // West
		}

		// Move sideways by configured distance (to the right)
		sidewaysX = botX + moveX
		sidewaysZ = botZ + moveZ
		err = fm.tryAlternateDestination(botX, botY, botZ, sidewaysX, botY, sidewaysZ)
		if err == nil {
			log.Printf("[FollowManager %s] Sideways movement succeeded to (%.1f, %.1f, %.1f)", fm.getFollowerName(), sidewaysX, botY, sidewaysZ)
			break
		}
		log.Printf("[FollowManager %s] Sideways movement attempt %d failed: %v", fm.getFollowerName(), attempt+1, err)
	}

	if err != nil {
		return fmt.Errorf("sideways movement failed: %w", err)
	}

	err = fm.tryAlternateDestination(sidewaysX, botY, sidewaysZ, targetX, targetY, targetZ)
	if err != nil {
		return fmt.Errorf("sideways recovery failed: %w", err)
	}

	log.Printf("[FollowManager %s] Sideways recovery completed", fm.getFollowerName())
	return nil
}

// tryAlternateDestination tries to path to a nearby alternate location
func (fm *followManager) tryAlternateDestination(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	log.Printf("[FollowManager %s] Attempting alternate destination recovery", fm.getFollowerName())

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
		start := models.V3{
			X: math.Floor(botX),
			Y: math.Floor(botY),
			Z: math.Floor(botZ),
		}
		goal := models.V3{
			X: math.Floor(altX),
			Y: math.Floor(botY), // Keep same Y level
			Z: math.Floor(altZ),
		}

		log.Printf("[FollowManager %s] Trying alternate destination %d: (%f, %f, %f)", fm.getFollowerName(), i+1, goal.X, goal.Y, goal.Z)

		// Try to find path to alternate destination
		path, err := fm.pathFinder.FindPath(context.Background(), start, goal, fm.config.MaxPathSteps)
		if err == nil && path.Found {
			log.Printf("[FollowManager %s] Found alternate path with %d steps", fm.getFollowerName(), len(path.Steps))
			fm.currentPath = path
			fm.lastPathTime = time.Now()
			if fm.pathExecutor != nil {
				if err := fm.pathExecutor.SetPath(path); err != nil {
					return err
				}
			}
			return nil
		}
	}

	return fmt.Errorf("no alternate destination found")
}

// tryBackupRecovery moves the bot backwards and clears path
func (fm *followManager) tryBackupRecovery(botX, botY, botZ, targetX, targetY, targetZ float64) error {
	log.Printf("[FollowManager %s] Attempting backup recovery", fm.getFollowerName())

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

	log.Printf("[FollowManager %s] Moving backwards from (%.1f, %.1f, %.1f) to (%.1f, %.1f, %.1f)",
		fm.getFollowerName(), botX, botY, botZ, backupX, botY, backupZ)

	// Send position update to move backwards
	err := fm.movementExecutor.SendPosition(backupX, botY, backupZ, true)
	if err != nil {
		return fmt.Errorf("backup movement failed: %w", err)
	}

	// Clear path to force recalculation from new position
	fm.clearPath()

	log.Printf("[FollowManager %s] Backup recovery completed", fm.getFollowerName())
	return nil
}

// tryClearPathRecovery simply clears the current path to force recalculation
func (fm *followManager) tryClearPathRecovery() error {
	log.Printf("Clearing path for fresh recalculation")

	fm.clearPath()
	fm.lastPathTime = time.Time{} // Force immediate recalc

	return nil
}

// ============================================================================
// Target Validation
// ============================================================================

// handleTargetLost handles the case when target is lost (disconnect, despawn, etc.)
func (fm *followManager) handleTargetLost(reason string) error {
	log.Printf("[FollowManager %s] [TARGET_LOST] transitioning following state from %s to %s", fm.getFollowerName(), fm.state.String(), models.StateLost.String())
	fm.state = models.StateLost
	fm.sendChatMessage(fmt.Sprintf("Lost target %s: %s", fm.targetName, reason))
	log.Printf("Target lost: %s", reason)
	fm.targetEntityID = 0
	fm.lastTargetPosTime = time.Time{}
	fm.clearPath()
	// fm.active = false
	return fmt.Errorf("target lost: %s", reason)
}

func (fm *followManager) clearPath() {
	fm.currentPath = nil
	if fm.pathExecutor != nil {
		fm.pathExecutor.ClearPath()
	}
}
