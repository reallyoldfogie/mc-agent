package following

import "time"

// FollowConfig holds configuration for following behavior
type FollowConfig struct {
	TargetDistance       float64       // Desired distance to maintain from target (blocks)
	StopDistance         float64       // Stop following when within this distance
	TargetStillDistance  float64       // Treat target as stationary if it moved less than this distance since last update
	MinRecalcInterval    time.Duration // Minimum time between path recalculations (adaptive)
	RecalcInterval       time.Duration // Maximum time between forced recalculations
	RecalcDistThreshold  float64       // Recalc if target moves more than this
	MaxPathSteps         int           // Maximum pathfinding search depth
	PathfindingTimeout   time.Duration // Maximum time to wait for pathfinding to complete
	StuckThreshold       time.Duration // Consider stuck after this time without movement
	MaxStuckAttempts     int           // Maximum recovery attempts before giving up
	JumpRecoveryHeight   float64       // How high to jump for recovery (blocks)
	SprintDistance       float64       // Start sprinting when farther than this (blocks)
	SneakDistance        float64       // Start sneaking when closer than this (blocks)
	TargetVelocityFactor float64       // Prediction strength for predictive targeting (0-1)
}

// DefaultFollowConfig returns default configuration
func DefaultFollowConfig() FollowConfig {
	return FollowConfig{
		TargetDistance:       3.0,               // Stay 3 blocks away
		StopDistance:         0.05,              // Stop when within 5cm
		TargetStillDistance:  0.05,              // Consider target stationary if it moved less than 5cm
		MinRecalcInterval:    500 * time.Millisecond, // Minimum time between recalcs (adaptive)
		RecalcInterval:       1 * time.Second,   // Maximum time between forced recalcs
		RecalcDistThreshold:  1.0,               // Recalc if target moves 1+ blocks
		MaxPathSteps:         20000,             // Search up to 20000 steps (allows ~40 block paths with 500x multiplier)
		PathfindingTimeout:   2 * time.Second,   // Abort pathfinding after 2 seconds
		StuckThreshold:       3 * time.Second,   // Stuck after 3 seconds
		MaxStuckAttempts:     3,                 // Try 3 recovery attempts
		JumpRecoveryHeight:   0.5,               // Jump 0.5 blocks for recovery
		SprintDistance:       8.0,               // Sprint when >8 blocks away
		SneakDistance:        2.5,               // Sneak when <2.5 blocks away
		TargetVelocityFactor: 0.5,               // Predict 50% ahead for moving targets
	}
}
