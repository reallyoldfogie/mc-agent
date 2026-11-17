package models

// ContainerSighting is one non-empty slot items.ContainerHelper observed
// while a container happened to be open for an unrelated reason
// (TakeItemFromChest/PutItemInChest) — see
// docs/plans/PERSISTENT_WORLD_MEMORY_PLAN.md Part 3's "opportunistic,
// never guessed": the only legitimate source of container contents is
// actually opening the container, never inferred any other way.
//
// ItemID is the raw protocol item ID rather than a resolved name — the
// implementation of ContainerSightingRecorder is expected to already have
// an ItemManager (see item_manager.go) for that lookup, so ContainerHelper
// itself doesn't need yet another optional dependency on top of its
// existing providers just to report what it already read.
type ContainerSighting struct {
	ContainerPos V3
	SlotIndex    int
	ItemID       int32
}

// ContainerSightingRecorder lets items.ContainerHelper report a
// ContainerSighting whenever it has one in hand. Implementations should
// treat this as fire-and-forget: a failure to persist a sighting must
// never surface as an error from the chest operation that happened to
// trigger it.
type ContainerSightingRecorder interface {
	RecordContainerSighting(s ContainerSighting)
}
