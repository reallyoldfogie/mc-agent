package pathfinding

import "time"

// PathfinderConfig controls shared pathfinder behavior.
type PathfinderConfig struct {
	// GoalRadius is the distance at which a position is considered "at the goal"
	GoalRadius float64

	// MaxSearchTime is the maximum duration for a single FindPath call.
	// If zero, no time limit is enforced (context deadline still applies).
	MaxSearchTime time.Duration

	// ContextCheckFreq controls how often to check for context cancellation.
	// The check occurs every N iterations of the search loop.
	// If zero or negative, defaults to 100.
	ContextCheckFreq int

	// MaxAbstractNodes limits the number of nodes expanded during abstract graph search.
	// If zero, no limit is enforced.
	MaxAbstractNodes int
}

const (
	defaultGoalRadius       = 0.5
	defaultContextCheckFreq = 100
)

func normalizeGoalRadius(goalRadius float64) float64 {
	if goalRadius <= 0 {
		return defaultGoalRadius
	}
	return goalRadius
}

func normalizeContextCheckFreq(freq int) int {
	if freq <= 0 {
		return defaultContextCheckFreq
	}
	return freq
}
