# Container Interaction Guide

**Date**: 2025-12-25
**Status**: ✅ **IMPLEMENTED** - Full chest interaction support

---

## Summary

Implemented comprehensive container interaction functionality that allows the agent to:
1. Open containers (chests, barrels, etc.)
2. Transfer items between player inventory and containers
3. Track which containers are open
4. Handle container-specific slot mappings

All functionality is accessible to the agent via the `items` package, not encapsulated in tests.

---

## New Files Created

### 1. `items/container_helper.go`
High-level API for container interactions:

**Key Functions**:
- `OpenContainer(pos, face, timeout)` - Opens a container and returns its window ID
- `TakeItemFromChest(windowID, chestSlot)` - Takes an item from chest to player inventory
- `PutItemInChest(windowID, playerSlot, chestSlot)` - Puts item from player inventory into chest
- `FindItemInPlayerInventory(windowID, itemID)` - Finds item in player inventory when chest is open
- `FindEmptyChestSlot(windowID)` - Finds an empty slot in the chest
- `GetChestRows(windowID)` - Returns the number of rows in a chest
- `CloseContainer()` - Closes the currently open container

**Container Type Support**:
- Chests (single and double)
- Extensible design for future container types (furnaces, hoppers, etc.)

### 2. `testing/container_integration_test.go`
Comprehensive integration tests:

**TestChestInteraction**:
1. Clears agent inventory via RCON
2. Places a chest with a stone block via RCON
3. Agent opens the chest
4. Agent takes the item from the chest
5. Agent closes the chest
6. Agent places the block in the world
7. All steps verified with RCON

**TestMultipleChestOperations**:
1. Places chest with 3 different item types
2. Agent takes all items from chest (multiple operations)
3. Verifies all items transferred correctly

### 3. `testing/rcon_helpers.go`
Shared RCON helper functions for testing:
- `GetChestContents(ctx, rcon, pos)` - Gets chest inventory via RCON
- `GetPlayerPosition(ctx, rcon, playerName)` - Gets player position
- `GetBlockAt(ctx, rcon, pos)` - Gets block type at position
- `GetInventoryItems(ctx, rcon, playerName)` - Gets player inventory

### 4. `testing/packet_mgr_adapter.go`
Adapter to bridge PacketMgr interface differences between packages

---

## How to Use (Agent Code)

### Opening a Chest and Taking an Item

```go
// Create helpers
packetMgrAdapt := packetMgrAdapter{pmgr: config.PacketMgr}
itemUsage := items.NewItemUsage(client.Conn, packetMgrAdapt)
invMgr := items.NewInventoryManager(screenMgr)
containerHelper := items.NewContainerHelper(itemUsage, invMgr, screenMgr)

// Open chest at position
chestPos := models.V3{X: 10, Y: 64, Z: 20}
windowID, err := containerHelper.OpenContainer(chestPos, items.FaceNorth, 5*time.Second)
if err != nil {
    return fmt.Errorf("failed to open chest: %w", err)
}

// Take item from chest slot 0
err = containerHelper.TakeItemFromChest(windowID, 0)
if err != nil {
    return fmt.Errorf("failed to take item: %w", err)
}

// Close the chest
containerHelper.CloseContainer()
```

### Finding and Transferring Items

```go
// Open chest
windowID, err := containerHelper.OpenContainer(chestPos, items.FaceNorth, 5*time.Second)
// ...

// Find stone in player inventory (returns slot index in chest window)
stoneSlot := containerHelper.FindItemInPlayerInventory(windowID, 1) // 1 = stone ID
if stoneSlot == -1 {
    return errors.New("no stone found")
}

// Find empty chest slot
emptySlot := containerHelper.FindEmptyChestSlot(windowID)
if emptySlot == -1 {
    return errors.New("chest is full")
}

// Put stone in chest
err = containerHelper.PutItemInChest(windowID, stoneSlot, emptySlot)
// ...
```

---

## Container Slot Mapping

### Chest Window Layout

When a chest is open, the window contains both chest slots and player inventory slots:

**Single Chest (3 rows)**:
- Slots 0-26: Chest slots (3 rows × 9 columns)
- Slots 27-53: Player main inventory (3 rows × 9 columns)
- Slots 54-62: Player hotbar (9 slots)

**Double Chest (6 rows)**:
- Slots 0-53: Chest slots (6 rows × 9 columns)
- Slots 54-80: Player main inventory
- Slots 81-89: Player hotbar

### Player Inventory (NBT vs Window)

When verifying with RCON, slot indices differ from window indices:

**Window Slots** (when chest is closed):
- 0-8: Hotbar
- 9-35: Main inventory
- 36-39: Armor
- 40: Offhand

**NBT Slots** (RCON data):
- 0-8: Hotbar
- 9-35: Main inventory
- 100-103: Armor
- -106: Offhand

---

## Integration with Existing Inventory System

The container system builds on top of the existing inventory management:

1. **State ID Management**: Uses the fixed `ContainerClick()` from mc-bot-go
2. **Cursor Tracking**: Uses `InventoryManager` for proper cursor state
3. **Slot Mapping**: Handles container-specific slot layouts automatically
4. **Window Tracking**: Tracks which window is currently open

---

## Running the Tests

```bash
# Run all container tests
testing/run_tests.sh -t TestChest -p 1 -T 5m

# Run specific test
testing/run_tests.sh -t TestChestInteraction -p 1 -T 5m
```

**Expected Results**:
- ✅ TestChestInteraction - Complete chest interaction flow
- ✅ TestMultipleChestOperations - Multiple item transfers

---

## Technical Details

### Container Opening Flow

1. Client sends `ServerboundUseItemOn` packet to interact with chest block
2. Server responds with `ClientboundOpenScreen` packet containing window ID
3. Screen manager creates `Chest` object in `Screens[windowID]` map
4. `ContainerHelper.OpenContainer()` waits for screen to appear in map
5. Returns window ID for use in subsequent operations

### Item Transfer Flow

1. Set inventory manager's window ID to chest window
2. Use `InventoryManager.MoveItem()` with container slot indices
3. `ContainerClick()` sends packets with correct state IDs
4. Server processes clicks and updates both chest and player inventory
5. Screen manager receives `SetSlot` packets with updated inventory state

### Slot Index Translation

`TakeItemFromChest()` handles the translation:
- Input: Chest slot (0-26 for single chest)
- Finds empty slot in player inventory section of chest window
- Player inventory starts at: `chest.Rows * 9`
- Calls `MoveItem()` with chest slot → player slot indices

---

## Future Enhancements

### Additional Container Types (Not Yet Implemented)

1. **Furnaces**:
   - Slot 0: Input
   - Slot 1: Fuel
   - Slot 2: Output
   - Methods: `PutItemInFurnace()`, `TakeFurnaceOutput()`

2. **Crafting Tables**:
   - Slots 0-8: Crafting grid
   - Slot 9: Output
   - Methods: `PlaceCraftingPattern()`, `TakeCraftedItem()`

3. **Hoppers/Droppers/Dispensers**:
   - 5 slots
   - Methods: `FillHopper()`, `EmptyHopper()`

### Advanced Operations

1. **Shift-Click Support**:
   - Quick transfer between container and player inventory
   - `containerHelper.ShiftClickSlot(windowID, slot)`

2. **Sorting/Organization**:
   - `containerHelper.SortChest(windowID)` - Sort items by type
   - `containerHelper.StackItems(windowID)` - Combine partial stacks

3. **Batch Operations**:
   - `containerHelper.TransferAllItems(fromWindow, toWindow)`
   - `containerHelper.DepositAllItems(windowID, itemType)`

---

## Troubleshooting

### Common Issues

**Container doesn't open**:
- Check that position is correct and contains a chest block
- Verify agent is within reach distance (< 4.5 blocks)
- Ensure server has processed the block placement

**Items don't transfer**:
- Verify window ID is correct
- Check that slots contain the expected items
- Ensure `SetWaitForUpdates(false)` is set (temporary workaround)

**Slot index errors**:
- Remember that chest window contains both chest AND player inventory slots
- Use `GetChestRows()` to calculate where player inventory starts
- Use RCON helpers to verify actual slot positions

### Debug Tips

1. **Enable debug logging**:
   ```go
   os.Setenv("MC_AGENT_CLICK_DEBUG_PATH", "logs/click_debug.log")
   ```

2. **Verify with RCON**:
   ```bash
   # Check chest contents
   data get block <x> <y> <z> Items

   # Check player inventory
   data get entity <player> Inventory
   ```

3. **Check window state**:
   ```go
   rows := containerHelper.GetChestRows(windowID)
   slotCount := containerHelper.GetContainerSlotCount(windowID)
   ```

---

## Related Files

- `items/inventory.go` - Base inventory management with state ID fix
- `items/usage.go` - Item usage and block interaction
- `mc-bot-go/bot/screen/screen.go` - Screen manager with fixed state ID increment
- `mc-bot-go/bot/screen/chest.go` - Chest container implementation
- `docs/INVENTORY_TIMEOUT_INVESTIGATION.md` - Investigation and fixes for inventory system

---

## Conclusion

The container interaction system is fully functional and ready for use. All logic is in the agent code (not test-only), properly handles slot mapping, tracks window IDs, and integrates seamlessly with the existing inventory system.

**Key Achievements**:
- ✅ Full chest interaction support
- ✅ Proper slot index mapping
- ✅ Window ID tracking
- ✅ Integration with fixed inventory system
- ✅ Comprehensive integration tests
- ✅ RCON verification for all operations

The system is extensible and ready for additional container types (furnaces, hoppers, etc.) when needed.
