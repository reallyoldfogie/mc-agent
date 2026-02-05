package items

import (
	"errors"
	"fmt"
	"time"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	mcscreen "github.com/reallyoldfogie/mc-bot-go/bot/screen"

	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/basetypes"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// ContainerType represents different container types
type ContainerType int

const (
	ContainerTypeUnknown ContainerType = -1
	ContainerTypeChest   ContainerType = 0 // Generic chest (1-6 rows)
	// Add more container types as needed
)

// ContainerHelper provides high-level functions for interacting with containers
type ContainerHelper struct {
	itemUsage        *ItemUsage
	invMgr           models.InventoryManager
	screenMgr        mcscreen.Manager
	client           bot.Client
	packetMgr        protocol_models.PacketMgr
	movementHandler  common.MovementHandler // Version-specific movement handler
	currentWindowID  byte
	entityIDProvider EntityIDProvider // Optional: for entity containers that need player's entity ID
}

// NewContainerHelper creates a new ContainerHelper
func NewContainerHelper(itemUsage *ItemUsage, invMgr models.InventoryManager, screenMgr mcscreen.Manager, client bot.Client, packetMgr protocol_models.PacketMgr) *ContainerHelper {
	return &ContainerHelper{
		itemUsage: itemUsage,
		invMgr:    invMgr,
		screenMgr: screenMgr,
		client:    client,
		packetMgr: packetMgr,
	}
}

// SetMovementHandler sets the version-specific movement handler.
// This must be called before using OpenEntityContainer for proper version-specific packet handling.
func (ch *ContainerHelper) SetMovementHandler(handler common.MovementHandler) {
	ch.movementHandler = handler
}

// SetEntityIDProvider sets the entity ID provider (needed for entity container interactions)
func (ch *ContainerHelper) SetEntityIDProvider(provider EntityIDProvider) {
	ch.entityIDProvider = provider
}

// OpenContainer opens a container at the specified position and waits for the server to respond.
// Returns the window ID assigned by the server, or error if timeout/failure.
// cursorX, cursorY, cursorZ are the click position on the block face (0.0-1.0).
func (ch *ContainerHelper) OpenContainer(pos models.V3, face BlockFace, timeout time.Duration, cursorX, cursorY, cursorZ float32) (byte, error) {
	screensBefore := len(ch.screenMgr.Screens())
	screenIDsBefore := copyScreenIDs(ch.screenMgr.Screens())
	faces := buildFaceFallbacks(face)
	perAttempt := timeout / time.Duration(len(faces))
	if perAttempt < 200*time.Millisecond {
		perAttempt = 200 * time.Millisecond
	}

	for _, attemptFace := range faces {
		fmt.Printf("[OpenContainer] → Attempting open at pos=%v face=%d\n", pos, attemptFace)
		if err := ch.itemUsage.UseItemOnBlockWithCursor(pos, attemptFace, models.MainHand, cursorX, cursorY, cursorZ); err != nil {
			return 0, err
		}

		windowID, ok := waitForOpenScreen(ch.screenMgr, screensBefore, screenIDsBefore, perAttempt)
		if ok {
			ch.currentWindowID = windowID
			return windowID, nil
		}
	}

	// Log timeout details for debugging
	screenIDs := make([]int, 0, len(ch.screenMgr.Screens()))
	for id := range ch.screenMgr.Screens() {
		screenIDs = append(screenIDs, id)
	}
	fmt.Printf("[OpenContainer] TIMEOUT at pos=%v: had %d screens, still have %d screens %v\n",
		pos, screensBefore, len(ch.screenMgr.Screens()), screenIDs)

	return 0, fmt.Errorf("timeout waiting for container to open (%s)", timeout.String())
}

func waitForOpenScreen(screenMgr mcscreen.Manager, screensBefore int, screenIDsBefore map[int]struct{}, timeout time.Duration) (byte, bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screensNow := len(screenMgr.Screens())
		if screensNow >= screensBefore {
			if windowID, ok := findNewScreenID(screenMgr.Screens(), screenIDsBefore); ok {
				fmt.Printf("[OpenContainer] ✓ Received window ID %d from server (screens: %d -> %d)\n",
					windowID, screensBefore, screensNow)
				return windowID, true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, false
}

func copyScreenIDs(screens map[int]mcscreen.Container) map[int]struct{} {
	ids := make(map[int]struct{}, len(screens))
	for id := range screens {
		ids[id] = struct{}{}
	}
	return ids
}

func findNewScreenID(screens map[int]mcscreen.Container, before map[int]struct{}) (byte, bool) {
	maxID := 0
	for windowID := range screens {
		if windowID == 0 {
			continue
		}
		if _, ok := before[windowID]; !ok {
			if windowID > maxID {
				maxID = windowID
			}
		}
	}
	if maxID > 0 {
		return byte(maxID), true
	}
	return 0, false
}

func buildFaceFallbacks(primary BlockFace) []BlockFace {
	faces := []BlockFace{primary, FaceUp, FaceDown, FaceNorth, FaceSouth, FaceEast, FaceWest}
	seen := make(map[BlockFace]struct{}, len(faces))
	unique := make([]BlockFace, 0, len(faces))
	for _, face := range faces {
		if _, ok := seen[face]; ok {
			continue
		}
		seen[face] = struct{}{}
		unique = append(unique, face)
	}
	return unique
}

// OpenEntityContainer opens an entity container (horse, donkey, llama, chest boat, chest minecart).
// Returns the window ID assigned by the server, or error if timeout/failure.
//
// For rideable entities (horse/donkey/llama), this:
// 1. Interacts with the entity to mount it (3-packet sequence)
// 2. Waits for mount confirmation
// 3. Sends ServerboundPlayerCommand with OPEN_INVENTORY action
//
// For storage entities (chest boat/minecart), this uses entity interaction.
func (ch *ContainerHelper) OpenEntityContainer(entityID int32, timeout time.Duration) (byte, error) {
	// Record the current number of screens before interaction
	screensBefore := len(ch.screenMgr.Screens())

	fmt.Printf("[OpenEntityContainer] → Opening container for entity ID %d\n", entityID)

	// Get the player's own entity ID (required for ServerboundPlayerCommand)
	if ch.entityIDProvider == nil {
		return 0, errors.New("entity ID provider not set - call SetEntityIDProvider first")
	}
	playerEntityID := ch.entityIDProvider.GetEntityID()
	if playerEntityID == 0 {
		return 0, errors.New("player entity ID not yet initialized")
	}

	// STEP 1: Interact with the entity to mount it (for horses/rideable entities)
	// This sends the 3-packet sequence: InteractWith + InteractAt + Swing
	fmt.Printf("[OpenEntityContainer] → Step 1: Mounting entity %d\n", entityID)
	if err := ch.itemUsage.UseItemOnEntity(entityID, models.MainHand, false); err != nil {
		return 0, fmt.Errorf("mount entity: %w", err)
	}

	// STEP 2: Wait for mount confirmation (teleportation packet indicates mounting)
	// Protocol analysis showed ~2 second delay in real client, but we'll use shorter delay
	fmt.Printf("[OpenEntityContainer] → Step 2: Waiting for mount confirmation\n")
	time.Sleep(500 * time.Millisecond)

	// STEP 3: Send ServerboundPlayerCommand with OPEN_INVENTORY action (type 7)
	// This is what the real Minecraft client does for horse/llama inventories
	// NOTE: The packet uses the PLAYER's entity ID, not the horse's!
	fmt.Printf("[OpenEntityContainer] → Step 3: Sending OPEN_INVENTORY command\n")
	const OpenInventoryAction = 7 // OPEN_INVENTORY command type

	if ch.movementHandler == nil {
		return 0, common.ErrHandlerNotSet{HandlerName: "MovementHandler"}
	}

	fmt.Printf("[OpenEntityContainer] → Sending ServerboundPlayerCommand (action=OPEN_INVENTORY) player_id=%d (horse_id=%d)\n",
		playerEntityID, entityID)
	if err := ch.movementHandler.SendPlayerCommand(ch.client.Conn(), playerEntityID, OpenInventoryAction); err != nil {
		return 0, fmt.Errorf("send player command: %w", err)
	}

	// Wait for the screen manager to receive ClientboundOpenHorseScreen or ClientboundOpenScreen packet
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		screensNow := len(ch.screenMgr.Screens())

		// Check if a new screen was added
		if screensNow > screensBefore {
			// Find the new window ID (highest ID that's not 0)
			maxID := 0
			for windowID := range ch.screenMgr.Screens() {
				if windowID > maxID {
					maxID = windowID
				}
			}
			if maxID > 0 {
				ch.currentWindowID = byte(maxID)
				fmt.Printf("[OpenEntityContainer] ✓ Received window ID %d from server (screens: %d -> %d)\n",
					maxID, screensBefore, screensNow)
				return byte(maxID), nil
			}
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Log timeout details for debugging
	screenIDs := make([]int, 0, len(ch.screenMgr.Screens()))
	for id := range ch.screenMgr.Screens() {
		screenIDs = append(screenIDs, id)
	}
	fmt.Printf("[OpenEntityContainer] TIMEOUT for entity %d: had %d screens, still have %d screens %v\n",
		entityID, screensBefore, len(ch.screenMgr.Screens()), screenIDs)

	return 0, fmt.Errorf("timeout waiting for entity container to open (%s)", timeout.String())
}

// CloseContainer closes the currently open container window.
// Sends a close packet to the server and cleans up client-side state.
func (ch *ContainerHelper) CloseContainer() error {
	if ch.currentWindowID == 0 {
		return nil // Already on player inventory
	}

	windowToClose := int(ch.currentWindowID)

	fmt.Printf("[CloseContainer] → Sending close for window ID %d\n", windowToClose)

	// Send close window packet to server
	packet := serverbound.NewCloseWindow()
	packet.SetPacketID(int32(ch.packetMgr.GetServerboundPacketID("ServerboundContainerClose")))
	packet.WindowId = basetypes.ContainerID(ch.currentWindowID)

	if err := ch.client.Conn().WritePacket(packet.Marshal()); err != nil {
		return err
	}

	fmt.Printf("[CloseContainer] ✓ Close packet sent for window ID %d\n", windowToClose)

	// CRITICAL: Wait for server to process the close packet before cleaning up client-side
	// If we clean up too quickly and start the next container open, the server may:
	// 1. Receive the new open request while still processing the previous close
	// 2. Assign a new window ID
	// 3. Then process the late close packet and close the newly opened window!
	time.Sleep(150 * time.Millisecond)

	// Trigger screen manager's cleanup (server won't send ClientboundContainerClose for player-initiated closes)
	if err := ch.screenMgr.ForceCloseScreen(windowToClose); err != nil {
		return err
	}

	// Set window back to player inventory
	ch.invMgr.SetWindow(0)
	ch.currentWindowID = 0

	fmt.Printf("[CloseContainer] ✓ Cleanup complete, back to player inventory\n")

	return nil
}

// GetContainerSlotCount returns the number of slots in a container window.
// Returns -1 if window is not found or not a recognized container type.
func (ch *ContainerHelper) GetContainerSlotCount(windowID byte) int {
	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return -1
	}

	switch c := screen.(type) {
	case *mcscreen.Chest:
		return len(c.Slots)
	case *mcscreen.Inventory:
		return len(c.Slots)
	default:
		return -1
	}
}

// GetContainerType returns the type of container for a given window ID.
func (ch *ContainerHelper) GetContainerType(windowID byte) ContainerType {
	if windowID == 0 {
		return ContainerTypeUnknown // Player inventory, not a container
	}

	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return ContainerTypeUnknown
	}

	switch c := screen.(type) {
	case *mcscreen.Chest:
		// Chest types 0-5 represent 1-6 rows
		if c.Type >= 0 && c.Type < 6 {
			return ContainerTypeChest
		}
		return ContainerTypeUnknown
	default:
		return ContainerTypeUnknown
	}
}

// TakeItemFromChest takes an item from a chest slot and places it in the player's inventory.
// chestSlot is the slot index within the chest (0-26 for single chest, 0-53 for double chest).
// Returns error if operation fails.
func (ch *ContainerHelper) TakeItemFromChest(windowID byte, chestSlot int16) error {
	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return errors.New("container window not found")
	}

	chest, ok := screen.(*mcscreen.Chest)
	if !ok {
		return errors.New("window is not a chest")
	}

	// Validate chest slot index
	containerSlots := chest.Rows * 9
	if chestSlot < 0 || int(chestSlot) >= containerSlots {
		return errors.New("chest slot index out of range")
	}

	// Get the item in the chest slot
	if int(chestSlot) >= len(chest.Slots) {
		return errors.New("slot index exceeds chest slots")
	}
	chestSlotData := chest.Slots[chestSlot]
	if chestSlotData.Count == 0 {
		return nil // Empty slot, nothing to take
	}

	// Set the window to the chest
	ch.invMgr.SetWindow(windowID)

	// Convert screen.Slot to ItemStack
	fromItem := models.ItemStack{
		ItemID: int32(chestSlotData.ID),
		Count:  int8(chestSlotData.Count),
	}
	for _, comp := range chestSlotData.Components {
		fromItem.Components = append(fromItem.Components, models.ItemComponent{
			Type: int32(comp.Type),
			Data: comp.Data,
		})
	}
	for _, removeComp := range chestSlotData.RemoveComponents {
		fromItem.RemoveComponents = append(fromItem.RemoveComponents, int32(removeComp))
	}

	// Find an empty slot in the player's main inventory or hotbar
	// In a chest window, player inventory slots come after chest slots
	// Player inventory layout in chest window:
	//   - Chest slots: 0 to (Rows*9 - 1)
	//   - Player main: Rows*9 to Rows*9 + 26
	//   - Player hotbar: Rows*9 + 27 to Rows*9 + 35
	playerMainStart := int16(chest.Rows * 9)
	playerHotbarEnd := playerMainStart + 36

	var emptySlot int16 = -1
	for slot := playerMainStart; slot < playerHotbarEnd; slot++ {
		if int(slot) < len(chest.Slots) {
			if chest.Slots[slot].Count == 0 {
				emptySlot = slot
				break
			}
		}
	}

	if emptySlot == -1 {
		return errors.New("no empty slot in player inventory")
	}

	// Move item from chest to player inventory
	toItem := models.ItemStack{} // Empty slot
	return ch.invMgr.MoveItem(chestSlot, emptySlot, fromItem, toItem)
}

// PutItemInChest puts an item from the player's inventory into a chest slot.
// playerSlot is the slot index within the player's inventory section of the chest window.
// chestSlot is the slot index within the chest (0-26 for single chest).
func (ch *ContainerHelper) PutItemInChest(windowID byte, playerInventorySlot int16, chestSlot int16) error {
	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return errors.New("container window not found")
	}

	chest, ok := screen.(*mcscreen.Chest)
	if !ok {
		return errors.New("window is not a chest")
	}

	// Set the window to the chest
	ch.invMgr.SetWindow(windowID)

	// Get items from both slots
	if int(playerInventorySlot) >= len(chest.Slots) || int(chestSlot) >= len(chest.Slots) {
		return errors.New("slot index out of range")
	}

	playerSlotData := chest.Slots[playerInventorySlot]
	chestSlotData := chest.Slots[chestSlot]

	fromItem := models.ItemStack{
		ItemID: int32(playerSlotData.ID),
		Count:  int8(playerSlotData.Count),
	}
	for _, comp := range playerSlotData.Components {
		fromItem.Components = append(fromItem.Components, models.ItemComponent{
			Type: int32(comp.Type),
			Data: comp.Data,
		})
	}

	toItem := models.ItemStack{
		ItemID: int32(chestSlotData.ID),
		Count:  int8(chestSlotData.Count),
	}
	for _, comp := range chestSlotData.Components {
		toItem.Components = append(toItem.Components, models.ItemComponent{
			Type: int32(comp.Type),
			Data: comp.Data,
		})
	}

	return ch.invMgr.MoveItem(playerInventorySlot, chestSlot, fromItem, toItem)
}

// FindItemInPlayerInventory finds an item in the player's inventory when a chest is open.
// Returns the slot index within the chest window's player inventory section, or -1 if not found.
// The returned slot index can be used directly with PutItemInChest.
func (ch *ContainerHelper) FindItemInPlayerInventory(windowID byte, itemID int32) int16 {
	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return -1
	}

	chest, ok := screen.(*mcscreen.Chest)
	if !ok {
		return -1
	}

	// Search player inventory slots in the chest window
	// Player inventory starts after chest slots
	playerMainStart := chest.Rows * 9
	playerHotbarEnd := playerMainStart + 36

	for slot := playerMainStart; slot < playerHotbarEnd; slot++ {
		if slot < len(chest.Slots) {
			if int32(chest.Slots[slot].ID) == itemID && chest.Slots[slot].Count > 0 {
				return int16(slot)
			}
		}
	}

	return -1
}

// FindEmptyChestSlot finds an empty slot in a chest.
// Returns the slot index, or -1 if no empty slots.
func (ch *ContainerHelper) FindEmptyChestSlot(windowID byte) int16 {
	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return -1
	}

	chest, ok := screen.(*mcscreen.Chest)
	if !ok {
		return -1
	}

	containerSlots := chest.Rows * 9
	for slot := 0; slot < containerSlots; slot++ {
		if chest.Slots[slot].Count == 0 {
			return int16(slot)
		}
	}

	return -1
}

// GetChestRows returns the number of rows in a chest window.
// Returns -1 if the window is not a chest.
func (ch *ContainerHelper) GetChestRows(windowID byte) int {
	screen, ok := ch.screenMgr.Screens()[int(windowID)]
	if !ok || screen == nil {
		return -1
	}

	chest, ok := screen.(*mcscreen.Chest)
	if !ok {
		return -1
	}

	return chest.Rows
}
