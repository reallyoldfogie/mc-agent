package models

// PortalFrame describes a detected, unlit nether portal frame: a
// rectangular obsidian ring with an all-passable interior, lying in a
// single-block-thick vertical plane along either the X or Z axis. See
// agent.FindPortalFrame's doc comment for the detection algorithm.
type PortalFrame struct {
	// Axis is "x" or "z" - which horizontal axis the frame's WIDTH runs
	// along. Matches minecraft:nether_portal's own "axis" block-state
	// property.
	Axis string
	// InteriorMin/InteriorMax bound the frame's interior (inclusive) - the
	// region that becomes nether_portal blocks once lit.
	InteriorMin, InteriorMax V3
	// IgnitionBlock is the obsidian block to right-click, and IgnitionFace
	// the face of it (pointing into the interior) to click, to ignite this
	// frame.
	IgnitionBlock V3
	IgnitionFace  BlockFace
}
