package models

// DependencySource says how one DependencyNode is expected to be
// satisfied — see docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md Part 2.
type DependencySource int

const (
	// SourceInventory means the item is already held in sufficient
	// quantity.
	SourceInventory DependencySource = iota
	// SourceMemory means a remembered, currently-unreserved sighting
	// covers the item — LocationPos/InContainer say where.
	SourceMemory
	// SourceCraft means the item has a known recipe, resolved into
	// Children: one node per distinct ingredient requirement.
	SourceCraft
	// SourceUnknown means none of the above — nothing to do but go find
	// more of it.
	SourceUnknown
)

func (s DependencySource) String() string {
	switch s {
	case SourceInventory:
		return "inventory"
	case SourceMemory:
		return "memory"
	case SourceCraft:
		return "craft"
	case SourceUnknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// DependencyNode is one item's resolution within a crafting-dependency
// tree, per docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md Part 2: "where is
// everything I need for A" — satisfied-from-inventory /
// satisfied-from-memory-at-location / needs-crafting (recurse) /
// unknown-source (raw gather).
type DependencyNode struct {
	Item   string
	Needed int
	Source DependencySource

	// LocationPos/InContainer are set iff Source == SourceMemory.
	// InContainer reports whether LocationPos names a container to open
	// (true) or a loose block/entity to walk up to (false).
	LocationPos V3
	InContainer bool

	// Children is set iff Source == SourceCraft: one node per distinct
	// ingredient requirement in Item's own recipe.
	Children []DependencyNode
}
