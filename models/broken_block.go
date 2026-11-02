package models

import "fmt"

// BrokenBlock identifies one block the agent has confirmed broken, by its
// exact position and the block's own name immediately before breaking -
// see agent.DrainBrokenBlocks and rlenv.BrokenBlocksReporter's own doc
// comments for why exact identity (not an existence or count check over
// an area) is what a caller needs to credit a specific block's
// destruction precisely.
type BrokenBlock struct {
	X, Y, Z   int
	BlockName string
}

func (b BrokenBlock) String() string {
	return fmt.Sprintf("BrokenBlock{X: %d, Y: %d, Z: %d, BlockName: '%s'}", b.X, b.Y, b.Z, b.BlockName)
}
