package models

import "context"

// PathFinder finds paths using A* algorithm.
type PathFinder interface {
	FindPath(ctx context.Context, start, goal V3, maxSteps int) (*Path, error)
	FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64
}
