package movement

import "github.com/reallyoldfogie/mc-agent/physics"

type blockStateWorld interface {
	GetBlockAt(x, y, z float64) (uint32, bool)
}

type physicsWorldAdapter struct {
	world blockStateWorld
}

// NewPhysicsWorldAdapter adapts a GetBlockAt world to the physics.World interface.
func NewPhysicsWorldAdapter(world blockStateWorld) physics.World {
	return physicsWorldAdapter{world: world}
}

func (pw physicsWorldAdapter) GetBlockStatus(x, y, z int) (uint32, bool) {
	if pw.world == nil {
		return 0, false
	}
	stateID, loaded := pw.world.GetBlockAt(float64(x), float64(y), float64(z))
	if !loaded {
		return 0, false // Indicate chunk not loaded to physics engine
	}
	return stateID, true
}
