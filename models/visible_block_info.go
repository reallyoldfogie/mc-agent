package models

import "fmt"

// VisibleBlockInfo represents a block that is visible (has line of sight) from the agent's position.
type VisibleBlockInfo struct {
	X         int
	Y         int
	Z         int
	StateID   uint32
	BlockName string
	Distance  float64
}

func (v VisibleBlockInfo) String() string {
	return fmt.Sprintf("VisibleBlockInfo{X: %d, Y: %d, Z: %d, StateID: %d, BlockName: '%s', Distance: %.2f}", v.X, v.Y, v.Z, v.StateID, v.BlockName, v.Distance)
}
