package pathfinding

// PathfinderConfig controls shared pathfinder behavior.
type PathfinderConfig struct {
	GoalRadius float64
}

const defaultGoalRadius = 0.5

func normalizeGoalRadius(goalRadius float64) float64 {
	if goalRadius <= 0 {
		return defaultGoalRadius
	}
	return goalRadius
}
