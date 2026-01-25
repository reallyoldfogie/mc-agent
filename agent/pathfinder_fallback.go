package agent

import (
	"fmt"

	"github.com/reallyoldfogie/mc-agent/models"
)

// fallbackPathFinder tries the primary pathfinder first, then falls back to a secondary one.
type fallbackPathFinder struct {
	primary  models.PathFinder
	fallback models.PathFinder
}

func (pf fallbackPathFinder) FindPath(start, goal models.V3, maxSteps int) (*models.Path, error) {
	if pf.primary != nil {
		path, err := pf.primary.FindPath(start, goal, maxSteps)
		if err == nil && path != nil && path.Found {
			return path, nil
		}
		if pf.fallback == nil {
			if err != nil {
				return path, err
			}
			return path, fmt.Errorf("no path found")
		}
		fallbackPath, fallbackErr := pf.fallback.FindPath(start, goal, maxSteps)
		if fallbackErr == nil && fallbackPath != nil && fallbackPath.Found {
			return fallbackPath, nil
		}
		if err != nil && fallbackErr != nil {
			return fallbackPath, fmt.Errorf("primary pathfinder failed: %v; fallback failed: %w", err, fallbackErr)
		}
		if fallbackErr != nil {
			return fallbackPath, fallbackErr
		}
		return fallbackPath, fmt.Errorf("no path found")
	}

	if pf.fallback == nil {
		return nil, fmt.Errorf("no pathfinder available")
	}
	return pf.fallback.FindPath(start, goal, maxSteps)
}

func (pf fallbackPathFinder) FindGroundBelow(x, z float64, startY float64, maxSearchDepth float64) float64 {
	if pf.primary != nil {
		return pf.primary.FindGroundBelow(x, z, startY, maxSearchDepth)
	}
	if pf.fallback != nil {
		return pf.fallback.FindGroundBelow(x, z, startY, maxSearchDepth)
	}
	return 0
}
