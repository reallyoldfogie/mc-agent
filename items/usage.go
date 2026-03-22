package items

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"

	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

const (
	FaceDown  BlockFace = 0 // -Y
	FaceUp    BlockFace = 1 // +Y
	FaceNorth BlockFace = 2 // -Z
	FaceSouth BlockFace = 3 // +Z
	FaceWest  BlockFace = 4 // -X
	FaceEast  BlockFace = 5 // +X
)

// InteractionType for entity interactions
type InteractionType int

const (
	InteractAt   InteractionType = 0 // Right-click on entity
	Attack       InteractionType = 1 // Left-click (attack)
	InteractWith InteractionType = 2 // Right-click with specific position
)

// ItemUsage provides methods for using items and placing blocks
type ItemUsage struct {
	client           models.PacketSender
	packetMgr        protocol_models.PacketMgr
	containerHandler models.ContainerHandler // Version-specific container handler
	actionHandler    models.ActionHandler    // Version-specific action handler
	entityHandler    models.EntityHandler    // Version-specific entity handler
	sequence         int32                   // Anti-cheat sequence number
}

// NewItemUsage creates a new ItemUsage instance
func NewItemUsage(client models.PacketSender, packetMgr protocol_models.PacketMgr) *ItemUsage {
	return &ItemUsage{
		client:    client,
		packetMgr: packetMgr,
		sequence:  0,
	}
}

// SetContainerHandler sets the version-specific container handler.
// This must be called before using PlaceBlock/UseItemOnBlock for proper version-specific packet handling.
func (iu *ItemUsage) SetContainerHandler(handler models.ContainerHandler) {
	iu.containerHandler = handler
}

// SetActionHandler sets the version-specific action handler.
// This must be called before using UseItemOnBlockWithCursor/UseItemOnEntity for proper version-specific packet handling.
func (iu *ItemUsage) SetActionHandler(handler models.ActionHandler) {
	iu.actionHandler = handler
}

// SetEntityHandler sets the version-specific entity handler.
// This must be called before using UseItemOnEntity/AttackEntity for proper version-specific packet handling.
func (iu *ItemUsage) SetEntityHandler(handler models.EntityHandler) {
	iu.entityHandler = handler
}

// PlaceBlock places a block or uses an item on a block at the specified position.
// This sends a ServerboundUseItemOn packet.
// cursorX, cursorY, cursorZ are the click position on the block face (0.0-1.0)
func (iu *ItemUsage) PlaceBlock(pos models.V3, face BlockFace, hand models.Hand, cursorX, cursorY, cursorZ float32) error {
	if iu.containerHandler == nil {
		return common.ErrHandlerNotSet{HandlerName: "ContainerHandler"}
	}

	// Convert position to block coordinates
	x := int(math.Floor(pos.X))
	y := int(math.Floor(pos.Y))
	z := int(math.Floor(pos.Z))

	// Increment sequence for anti-cheat
	iu.sequence++

	return iu.containerHandler.SendUseItemOn(
		iu.client,
		hand,
		x, y, z,
		int32(face),
		cursorX, cursorY, cursorZ,
		false, // insideBlock
		iu.sequence,
	)
}

// GetRealisticCursorPosition returns a randomized cursor position that looks natural.
// Returns values in range [0.2, 0.9] to avoid perfectly centered clicks.
// This helps evade anti-bot detection in Minecraft 1.21.5+.
func GetRealisticCursorPosition() (x, y, z float32) {
	// Use current time as seed for randomization
	rand.Seed(time.Now().UnixNano())

	// Generate values in range 0.2 to 0.9 (avoid edges and perfect center)
	x = 0.2 + rand.Float32()*0.7 // Range: [0.2, 0.9]
	y = 0.2 + rand.Float32()*0.7
	z = 0.2 + rand.Float32()*0.7

	return x, y, z
}

// UseItemOnBlockWithCursor uses the held item on a block with an explicit cursor position.
func (iu *ItemUsage) UseItemOnBlockWithCursor(pos models.V3, face BlockFace, hand models.Hand, cursorX, cursorY, cursorZ float32) error {
	fmt.Printf("[UseItemOnBlock] → Interacting with block at pos=(%v) face=%d hand=%d\n", pos, face, hand)
	fmt.Printf("[UseItemOnBlock] → Using cursor position (%.3f, %.3f, %.3f)\n", cursorX, cursorY, cursorZ)

	// Send use_item_on packet
	err := iu.PlaceBlock(pos, face, hand, cursorX, cursorY, cursorZ)
	if err != nil {
		fmt.Printf("[UseItemOnBlock] ✗ Error sending use_item_on: %v\n", err)
		return err
	}
	fmt.Printf("[UseItemOnBlock] ✓ use_item_on packet sent\n")

	// Send swing packet (required for some blocks like loom/beacon in 1.21.5+)
	if iu.actionHandler == nil {
		return common.ErrHandlerNotSet{HandlerName: "ActionHandler"}
	}

	if err := iu.actionHandler.SendSwing(iu.client, hand); err != nil {
		fmt.Printf("[UseItemOnBlock] ✗ Error sending swing: %v\n", err)
		return err
	}

	fmt.Printf("[UseItemOnBlock] ✓ swing packet sent - interaction complete\n")
	return nil
}

// UseItemOnBlock uses the held item on a block (e.g., empty bucket on water source).
// Uses randomized cursor position to avoid anti-bot detection (Minecraft 1.21.5+).
// Sends use_item_on packet followed by swing packet to match vanilla client behavior.
func (iu *ItemUsage) UseItemOnBlock(pos models.V3, face BlockFace, hand models.Hand) error {
	cursorX, cursorY, cursorZ := GetRealisticCursorPosition()
	return iu.UseItemOnBlockWithCursor(pos, face, hand, cursorX, cursorY, cursorZ)
}

// UseItemOnEntity uses an item on an entity (e.g., bucket on fish to catch it, open horse inventory).
// This sends a ServerboundInteract (type 0) packet.
func (iu *ItemUsage) UseItemOnEntity(entityID int32, hand models.Hand, sneaking bool) error {
	log.Printf("[UseItemOnEntity] → Interacting with entity %d\n", entityID)

	// Send type 0 (INTERACT) packet
	if iu.entityHandler == nil {
		log.Printf("[UseItemOnEntity] ✗ entity handler not set")
		return common.ErrHandlerNotSet{HandlerName: "EntityHandler"}
	}
	if err := iu.entityHandler.SendInteract(iu.client, entityID, hand, sneaking); err != nil {
		log.Printf("[UseItemOnEntity] ✗ Error: %v\n", err)
		return err
	}

	// Optional: swing packet
	if iu.actionHandler == nil {
		log.Printf("[UseItemOnEntity] ✗ action handler not set")
		return common.ErrHandlerNotSet{HandlerName: "ActionHandler"}
	}
	if err := iu.actionHandler.SendSwing(iu.client, hand); err != nil {
		log.Printf("[UseItemOnEntity] ✗ Error: %v\n", err)
		return err
	}

	log.Printf("[UseItemOnEntity] ✓ Complete\n")
	return nil
}

// UseItemOnEntityAt uses an item on an entity at a specific position.
// Used for more precise interactions.
func (iu *ItemUsage) UseItemOnEntityAt(entityID int32, targetX, targetY, targetZ float32, hand models.Hand, sneaking bool) error {
	if iu.entityHandler == nil {
		return common.ErrHandlerNotSet{HandlerName: "EntityHandler"}
	}
	return iu.entityHandler.SendInteractAt(iu.client, entityID, targetX, targetY, targetZ, hand, sneaking)
}

// AttackEntity attacks an entity (left-click).
func (iu *ItemUsage) AttackEntity(entityID int32, sneaking bool) error {
	if iu.entityHandler == nil {
		return common.ErrHandlerNotSet{HandlerName: "EntityHandler"}
	}
	return iu.entityHandler.SendAttack(iu.client, entityID, sneaking)
}

// SwitchToSlot switches the active hotbar slot (0-8).
// This sends a ServerboundSetCarriedItem packet.
func (iu *ItemUsage) SwitchToSlot(slotIndex int) error {
	if slotIndex < 0 || slotIndex > 8 {
		return nil // Invalid slot, ignore
	}

	if iu.containerHandler == nil {
		return common.ErrHandlerNotSet{HandlerName: "ContainerHandler"}
	}

	return iu.containerHandler.SendSetCarriedItem(iu.client, int16(slotIndex))
}

// GetSequence returns the current sequence number (for debugging/testing)
func (iu *ItemUsage) GetSequence() int32 {
	return iu.sequence
}

// Helper functions for common use cases

// PlaceWaterBucket places water at a position (clutch mechanic)
func (iu *ItemUsage) PlaceWaterBucket(pos models.V3, face BlockFace) error {
	return iu.UseItemOnBlock(pos, face, models.MainHand)
}

// CollectPowderSnow collects powder snow with an empty bucket
func (iu *ItemUsage) CollectPowderSnow(pos models.V3, face BlockFace) error {
	return iu.UseItemOnBlock(pos, face, models.MainHand)
}

// CatchFish catches a fish with a water bucket
func (iu *ItemUsage) CatchFish(entityID int32) error {
	return iu.UseItemOnEntity(entityID, models.MainHand, false)
}

// CatchAxolotl catches an axolotl with a water bucket
func (iu *ItemUsage) CatchAxolotl(entityID int32) error {
	return iu.UseItemOnEntity(entityID, models.MainHand, false)
}

// FindItemInInventory searches for an item by ID in any inventory.
// Returns the slot index where the item was found, or -1 if not found.
// Works with all inventory types:
//   - Player inventory (45 slots): 0-8 hotbar, 9-35 main, 36-39 armor, 40 offhand, 41-44 crafting
//   - Chest (27 slots): 0-26
//   - Double chest (54 slots): 0-53
//   - Ender chest (27 slots): 0-26
//   - Barrel (27 slots): 0-26
//   - Shulker box (27 slots): 0-26
//   - Horse/Donkey (2-17 slots): 0-1 saddle/armor, 2-16 chest slots (if equipped)
//   - Hopper (5 slots): 0-4
//   - Furnace (3 slots): 0 input, 1 fuel, 2 output
func (iu *ItemUsage) FindItemInInventory(itemID int32, inventory InventoryProvider) int {
	// Search all slots in this inventory
	slotCount := inventory.GetSlotCount()
	for slot := 0; slot < slotCount; slot++ {
		slotItemID, _, ok := inventory.GetSlot(slot)
		if ok && slotItemID == itemID {
			return slot
		}
	}
	return -1 // Not found
}

// Reach distance constants for validation
const (
	MaxBlockReachDistance  = 4.5 // Max distance to interact with blocks
	MaxEntityReachDistance = 3.0 // Max distance to interact with entities
)

// ValidateBlockReachDistance checks if a target block position is within reach distance.
// playerPos is the player's current position (feet level).
// targetPos is the block position being interacted with.
// Returns true if the block is within reach (~4.5 blocks), false otherwise.
func ValidateBlockReachDistance(playerPos, targetPos models.V3) bool {
	distance := playerPos.DistanceTo(targetPos)
	return distance <= MaxBlockReachDistance
}

// ValidateEntityReachDistance checks if a target entity is within interaction range.
// playerPos is the player's current position (feet level).
// entityPos is the entity's current position.
// Returns true if the entity is within reach (~3.0 blocks), false otherwise.
func ValidateEntityReachDistance(playerPos, entityPos models.V3) bool {
	distance := playerPos.DistanceTo(entityPos)
	return distance <= MaxEntityReachDistance
}
