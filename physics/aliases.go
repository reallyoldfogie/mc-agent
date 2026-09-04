package physics

import "github.com/reallyoldfogie/mc-agent/models"

// Interface aliases to models.
type (
	World              = models.World
	BlockShapeProvider = models.BlockShapeManager
	AABB               = models.AABB
	MinMax             = models.MinMax
)

// Re-export constructors for convenience.
var (
	NewAABB       = models.NewAABB
	NewPlayerAABB = models.NewPlayerAABB
)
