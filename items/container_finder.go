package items

import (
	"strings"

	"github.com/reallyoldfogie/mc-agent/models"
	mc_versions "github.com/reallyoldfogie/mc-protocol-go/data/versions"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// ContainerInfo represents a found container in the world
type ContainerInfo struct {
	Position models.V3
	Name     string // e.g. "minecraft:chest", "minecraft:barrel"
	BlockID  protocol_models.BlockID
}

// ContainerFinder helps locate containers in the world
type ContainerFinder struct {
	world    WorldAccess
	blockMgr mc_versions.BlockMgr
}

// NewContainerFinder creates a new ContainerFinder
func NewContainerFinder(world WorldAccess, blockMgr mc_versions.BlockMgr) *ContainerFinder {
	return &ContainerFinder{
		world:    world,
		blockMgr: blockMgr,
	}
}

// FindContainersNearby searches for containers within a given radius of a position.
// Returns a list of found containers sorted by distance (nearest first).
// Radius is measured in blocks (e.g., radius 5 = 5 blocks in each direction).
//
// IMPORTANT: This function can only find containers in chunks that have been loaded
// by the server and sent to the client. Containers in unloaded chunks will not be found.
// The agent's view distance determines which chunks are loaded.
func (cf *ContainerFinder) FindContainersNearby(center models.V3, radius int) []ContainerInfo {
	containers := []ContainerInfo{}

	// Scan in a cubic volume around the center
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				x := int(center.X) + dx
				y := int(center.Y) + dy
				z := int(center.Z) + dz

				// Get block at this position
				blockStateID, loaded := cf.world.GetBlockAt(float64(x), float64(y), float64(z))
				if !loaded {
					continue
				}
				if blockStateID == 0 {
					continue // Air block or unloaded chunk
				}

				// Convert state ID to block ID
				if blockID, ok := cf.blockMgr.BlockIDByStateID(blockStateID); ok {
					if block, ok := cf.blockMgr.GetByID(blockID); ok {

						// Check if this is a container block
						if cf.isContainer(block.Name) {
							containers = append(containers, ContainerInfo{
								Position: models.V3{X: float64(x), Y: float64(y), Z: float64(z)},
								Name:     block.Name,
								BlockID:  blockID,
							})
						}
					}
				}
			}
		}
	}

	// Sort by distance from center
	cf.sortByDistance(containers, center)

	return containers
}

// FindNearestContainer finds the nearest container to a position within the given radius.
// Returns the container info and true if found, or empty ContainerInfo and false if not found.
func (cf *ContainerFinder) FindNearestContainer(center models.V3, radius int) (ContainerInfo, bool) {
	containers := cf.FindContainersNearby(center, radius)
	if len(containers) == 0 {
		return ContainerInfo{}, false
	}
	return containers[0], true
}

// FindReachableContainers returns only containers that are within interaction reach distance.
// Interaction reach is ~4.5 blocks for containers in Minecraft.
// This filters the results of FindContainersNearby to only include containers the agent can interact with.
func (cf *ContainerFinder) FindReachableContainers(center models.V3, searchRadius int) []ContainerInfo {
	const maxInteractionDistance = 4.5

	allContainers := cf.FindContainersNearby(center, searchRadius)
	reachable := []ContainerInfo{}

	for _, container := range allContainers {
		distance := center.DistanceTo(container.Position)
		if distance <= maxInteractionDistance {
			reachable = append(reachable, container)
		}
	}

	return reachable
}

// FindNearestReachableContainer finds the nearest container that is within interaction reach.
// Returns the container info and true if found, or empty ContainerInfo and false if not found.
func (cf *ContainerFinder) FindNearestReachableContainer(center models.V3, searchRadius int) (ContainerInfo, bool) {
	reachable := cf.FindReachableContainers(center, searchRadius)
	if len(reachable) == 0 {
		return ContainerInfo{}, false
	}
	return reachable[0], true
}

// FindContainersByType searches for containers of a specific type (e.g., "chest", "barrel").
// The type parameter can be a partial match (e.g., "chest" matches "chest", "ender_chest", "trapped_chest").
func (cf *ContainerFinder) FindContainersByType(center models.V3, radius int, containerType string) []ContainerInfo {
	allContainers := cf.FindContainersNearby(center, radius)
	filtered := []ContainerInfo{}

	for _, container := range allContainers {
		// Extract the block name without "minecraft:" prefix
		blockName := strings.TrimPrefix(container.Name, "minecraft:")
		if strings.Contains(blockName, containerType) {
			filtered = append(filtered, container)
		}
	}

	return filtered
}

// isContainer checks if a block name represents a container
func (cf *ContainerFinder) isContainer(blockName string) bool {
	// Remove "minecraft:" prefix if present
	name := strings.TrimPrefix(blockName, "minecraft:")

	// List of container types
	containerTypes := []string{
		"chest",
		"trapped_chest",
		"ender_chest",
		"barrel",
		"shulker_box",
		"furnace",
		"blast_furnace",
		"smoker",
		"hopper",
		"dropper",
		"dispenser",
	}

	// Check if the block name contains any container type
	for _, containerType := range containerTypes {
		if strings.Contains(name, containerType) {
			return true
		}
	}

	return false
}

// sortByDistance sorts containers by distance from a center point (nearest first)
func (cf *ContainerFinder) sortByDistance(containers []ContainerInfo, center models.V3) {
	// Simple bubble sort (good enough for small lists)
	n := len(containers)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			dist1 := center.DistanceTo(containers[j].Position)
			dist2 := center.DistanceTo(containers[j+1].Position)
			if dist1 > dist2 {
				containers[j], containers[j+1] = containers[j+1], containers[j]
			}
		}
	}
}

// GetContainerType returns a human-readable container type from a container name.
// E.g., "minecraft:chest" -> "chest", "minecraft:ender_chest" -> "ender_chest"
func GetContainerType(containerName string) string {
	// Remove "minecraft:" prefix
	name := strings.TrimPrefix(containerName, "minecraft:")

	// Map shulker box variants to generic "shulker_box"
	if strings.HasSuffix(name, "_shulker_box") {
		return "shulker_box"
	}

	return name
}

// IsChest checks if a container name represents a chest (any variant)
func IsChest(containerName string) bool {
	name := strings.TrimPrefix(containerName, "minecraft:")
	return strings.Contains(name, "chest") && !strings.Contains(name, "shulker")
}

// IsShulkerBox checks if a container name represents a shulker box
func IsShulkerBox(containerName string) bool {
	name := strings.TrimPrefix(containerName, "minecraft:")
	return strings.Contains(name, "shulker_box")
}

// IsFurnace checks if a container name represents a furnace (any variant)
func IsFurnace(containerName string) bool {
	name := strings.TrimPrefix(containerName, "minecraft:")
	return strings.Contains(name, "furnace") || strings.Contains(name, "smoker")
}
