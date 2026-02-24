package agent

import "github.com/reallyoldfogie/mc-agent/models"

// GetBlockAt returns the block state ID at the given coordinates.
// Implements TrajectoryValidator interface.
func (a *agent) GetBlockAt(x, y, z float64) (blockStateID uint32, loaded bool) {
	if a.worldMgr == nil {
		return 0, false
	}
	return a.worldMgr.GetBlockAt(x, y, z)
}

// GetCollisionBoxes returns the collision shapes for a block at the given position.
// Implements TrajectoryValidator interface.
func (a *agent) GetCollisionBoxes(blockStateID uint32, x, y, z int) []models.AABB {
	if a.shapeMgr == nil {
		return nil
	}
	return a.shapeMgr.GetCollisionBoxes(blockStateID, x, y, z)
}

// IsSolid returns true if the given block state is solid (has collision).
// Implements TrajectoryValidator interface.
func (a *agent) IsSolid(stateID uint32) bool {
	if a.shapeMgr == nil {
		return false
	}
	return a.shapeMgr.IsSolid(stateID)
}

func (a *agent) BlockName(stateID uint32) string {
	if a.shapeMgr == nil {
		return ""
	}
	return a.shapeMgr.BlockName(stateID)
}

func (a *agent) FullBlockName(stateID uint32) string {
	if a.shapeMgr == nil {
		return ""
	}
	return a.shapeMgr.FullBlockName(stateID)
}
