package models

import "fmt"

// VisibleEntityInfo represents an entity that is visible (has line of sight)
// from the agent's position — the entity analogue of VisibleBlockInfo.
type VisibleEntityInfo struct {
	EntityID   int32
	EntityType int32 // raw entity_type registry ID
	// TypeName is the resolved entity type name, stripped of its
	// "minecraft:" namespace prefix (matching models.EntityType's own
	// constants, e.g. EntityTypeThrowableItem = "item") — e.g. "item",
	// "cow", not "minecraft:item"/"minecraft:cow". "unknown" if the type
	// couldn't be resolved (entity registry not ready, or a genuinely
	// unrecognized type).
	TypeName string
	X, Y, Z  float64
	Distance float64
}

func (v VisibleEntityInfo) String() string {
	return fmt.Sprintf("VisibleEntityInfo{EntityID: %d, Type: %s, X: %.2f, Y: %.2f, Z: %.2f, Distance: %.2f}",
		v.EntityID, v.TypeName, v.X, v.Y, v.Z, v.Distance)
}
