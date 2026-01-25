package items

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockPacketSender captures packets for testing
type MockPacketSender struct {
	packets []pk.Packet
	err     error
}

func (m *MockPacketSender) WritePacket(packet pk.Packet) error {
	if m.err != nil {
		return m.err
	}
	m.packets = append(m.packets, packet)
	return nil
}

func (m *MockPacketSender) GetPacketCount() int {
	return len(m.packets)
}

func (m *MockPacketSender) Clear() {
	m.packets = nil
}

// MockPacketManager returns fake packet IDs for testing
type MockPacketManager struct {
	packets map[string]protocol_models.ServerboundPacketID
}

func (m *MockPacketManager) GetServerboundPacketID(name string) protocol_models.ServerboundPacketID {
	if id, ok := m.packets[name]; ok {
		return id
	}
	return 0
}

// Stub methods to satisfy PacketMgr interface
func (m *MockPacketManager) GetClientboundPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (m *MockPacketManager) GetClientboundConfigPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (m *MockPacketManager) GetServerboundConfigPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (m *MockPacketManager) GetClientboundPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetServerboundPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetClientboundConfigPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetServerboundConfigPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) ClientboundToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (m *MockPacketManager) ServerboundToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (m *MockPacketManager) ClientboundConfigToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (m *MockPacketManager) ServerboundConfigToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (m *MockPacketManager) GetClientboundLoginPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (m *MockPacketManager) GetServerboundLoginPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (m *MockPacketManager) GetClientboundLoginPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetServerboundLoginPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) ClientboundLoginToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (m *MockPacketManager) ServerboundLoginToString(id protocol_models.ServerboundPacketID) string {
	return ""
}
func (m *MockPacketManager) GetClientboundStatusPacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (m *MockPacketManager) GetServerboundStatusPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (m *MockPacketManager) GetClientboundStatusPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetServerboundStatusPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) ClientboundStatusToString(id protocol_models.ClientboundPacketID) string {
	return ""
}
func (m *MockPacketManager) ServerboundStatusToString(id int32) string { return "" }
func (m *MockPacketManager) GetClientboundHandshakePacketID(name string) protocol_models.ClientboundPacketID {
	return 0
}
func (m *MockPacketManager) GetServerboundHandshakingPacketID(name string) protocol_models.ServerboundPacketID {
	return 0
}
func (m *MockPacketManager) GetClientboundHandshakingPacketByID(id protocol_models.ClientboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetServerboundHandshakingPacketByID(id protocol_models.ServerboundPacketID) (protocol_models.PacketMarshaller, error) {
	return nil, nil
}
func (m *MockPacketManager) GetEntityTypeID(name string) int32           { return 0 }
func (m *MockPacketManager) Name() string                                { return "MockPacketManager" }
func (m *MockPacketManager) Version() string                             { return "1.0.0-mock" }
func (m *MockPacketManager) GetVersionMetaData() *models.VersionMetaData { return nil }
func (m *MockPacketManager) VersionProtocol() uint64                     { return 0 }

func newMockPacketManager() *MockPacketManager {
	return &MockPacketManager{
		packets: map[string]protocol_models.ServerboundPacketID{
			"ServerboundUseItemOn":            0x38,
			"ServerboundInteract":             0x16,
			"ServerboundSetCarriedItem":       0x2F,
			"ServerboundContainerButtonClick": 0x11,
		},
	}
}

func TestNewItemUsage(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()

	usage := NewItemUsage(client, packetMgr)

	require.NotNil(t, usage)
	assert.Equal(t, int32(0), usage.sequence, "Initial sequence should be 0")
}

func TestPlaceBlock(t *testing.T) {
	tests := []struct {
		name      string
		pos       models.V3
		face      BlockFace
		hand      Hand
		cursorX   float32
		cursorY   float32
		cursorZ   float32
		expectSeq int32
	}{
		{
			name:      "Place block on top face with main hand",
			pos:       models.V3{X: 10.5, Y: 64.0, Z: 20.3},
			face:      FaceUp,
			hand:      MainHand,
			cursorX:   0.5,
			cursorY:   0.5,
			cursorZ:   0.5,
			expectSeq: 1,
		},
		{
			name:      "Place block on north face with offhand",
			pos:       models.V3{X: 5.0, Y: 70.0, Z: 15.0},
			face:      FaceNorth,
			hand:      OffHand,
			cursorX:   0.3,
			cursorY:   0.7,
			cursorZ:   0.2,
			expectSeq: 2,
		},
		{
			name:      "Place block at edge cursor position",
			pos:       models.V3{X: 0.0, Y: 0.0, Z: 0.0},
			face:      FaceDown,
			hand:      MainHand,
			cursorX:   0.0,
			cursorY:   0.0,
			cursorZ:   1.0,
			expectSeq: 3,
		},
	}

	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client.Clear()

			err := usage.PlaceBlock(tt.pos, tt.face, tt.hand, tt.cursorX, tt.cursorY, tt.cursorZ)

			require.NoError(t, err)
			require.Len(t, client.packets, 1, "Should send exactly one packet")

			// Verify sequence incremented
			assert.Equal(t, tt.expectSeq, usage.GetSequence(), "Sequence should increment")
		})
	}
}

func TestPlaceBlock_SequenceIncrement(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	pos := models.V3{X: 0, Y: 64, Z: 0}

	// Place multiple blocks and verify sequence increments
	for i := 1; i <= 10; i++ {
		err := usage.PlaceBlock(pos, FaceUp, MainHand, 0.5, 0.5, 0.5)
		require.NoError(t, err)
		assert.Equal(t, int32(i), usage.GetSequence(), "Sequence should be %d", i)
	}
}

func TestUseItemOnBlock(t *testing.T) {
	tests := []struct {
		name string
		pos  models.V3
		face BlockFace
		hand Hand
	}{
		{
			name: "Use bucket on powder snow",
			pos:  models.V3{X: 10, Y: 64, Z: 20},
			face: FaceUp,
			hand: MainHand,
		},
		{
			name: "Use bucket on water source",
			pos:  models.V3{X: -5, Y: 60, Z: 100},
			face: FaceUp,
			hand: OffHand,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockPacketSender{}
			packetMgr := newMockPacketManager()
			usage := NewItemUsage(client, packetMgr)

			err := usage.UseItemOnBlock(tt.pos, tt.face, tt.hand)

			require.NoError(t, err)
			// UseItemOnBlock sends 2 packets: use_item_on + swing (matches vanilla client)
			require.Len(t, client.packets, 2, "Should send use_item_on + swing packets")

			// Verify sequence incremented
			assert.Equal(t, int32(1), usage.GetSequence())
		})
	}
}

func TestUseItemOnEntity(t *testing.T) {
	tests := []struct {
		name     string
		entityID int32
		hand     Hand
		sneaking bool
	}{
		{
			name:     "Catch fish with bucket",
			entityID: 123,
			hand:     MainHand,
			sneaking: false,
		},
		{
			name:     "Catch axolotl while sneaking",
			entityID: 456,
			hand:     MainHand,
			sneaking: true,
		},
		{
			name:     "Use offhand on entity",
			entityID: 789,
			hand:     OffHand,
			sneaking: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockPacketSender{}
			packetMgr := newMockPacketManager()
			usage := NewItemUsage(client, packetMgr)

			err := usage.UseItemOnEntity(tt.entityID, tt.hand, tt.sneaking)

			require.NoError(t, err)
			// UseItemOnEntity sends 3 packets: InteractWith + InteractAt + Swing (matches vanilla client)
			require.Len(t, client.packets, 3, "Should send InteractWith + InteractAt + Swing packets")
		})
	}
}

func TestUseItemOnEntityAt(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	err := usage.UseItemOnEntityAt(123, 1.5, 2.5, 3.5, MainHand, false)

	require.NoError(t, err)
	require.Len(t, client.packets, 1, "Should send exactly one packet")
}

func TestAttackEntity(t *testing.T) {
	tests := []struct {
		name     string
		entityID int32
		sneaking bool
	}{
		{
			name:     "Attack entity normally",
			entityID: 100,
			sneaking: false,
		},
		{
			name:     "Attack entity while sneaking",
			entityID: 200,
			sneaking: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockPacketSender{}
			packetMgr := newMockPacketManager()
			usage := NewItemUsage(client, packetMgr)

			err := usage.AttackEntity(tt.entityID, tt.sneaking)

			require.NoError(t, err)
			require.Len(t, client.packets, 1, "Should send exactly one packet")
		})
	}
}

func TestSwitchToSlot(t *testing.T) {
	tests := []struct {
		name        string
		slotIndex   int
		shouldSend  bool
		description string
	}{
		{
			name:        "Valid slot 0",
			slotIndex:   0,
			shouldSend:  true,
			description: "First hotbar slot",
		},
		{
			name:        "Valid slot 8",
			slotIndex:   8,
			shouldSend:  true,
			description: "Last hotbar slot",
		},
		{
			name:        "Valid slot 4",
			slotIndex:   4,
			shouldSend:  true,
			description: "Middle hotbar slot",
		},
		{
			name:        "Invalid slot -1",
			slotIndex:   -1,
			shouldSend:  false,
			description: "Below minimum",
		},
		{
			name:        "Invalid slot 9",
			slotIndex:   9,
			shouldSend:  false,
			description: "Above maximum",
		},
		{
			name:        "Invalid slot 100",
			slotIndex:   100,
			shouldSend:  false,
			description: "Far above maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &MockPacketSender{}
			packetMgr := newMockPacketManager()
			usage := NewItemUsage(client, packetMgr)

			err := usage.SwitchToSlot(tt.slotIndex)

			require.NoError(t, err, "SwitchToSlot should not return error")

			if tt.shouldSend {
				require.Len(t, client.packets, 1, "Should send packet for valid slot")
			} else {
				require.Len(t, client.packets, 0, "Should not send packet for invalid slot")
			}
		})
	}
}

func TestGetSequence(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	assert.Equal(t, int32(0), usage.GetSequence(), "Initial sequence should be 0")

	// Place a block
	_ = usage.PlaceBlock(models.V3{X: 0, Y: 64, Z: 0}, FaceUp, MainHand, 0.5, 0.5, 0.5)
	assert.Equal(t, int32(1), usage.GetSequence(), "Sequence should be 1 after one placement")

	// Use item on block
	_ = usage.UseItemOnBlock(models.V3{X: 0, Y: 64, Z: 0}, FaceUp, MainHand)
	assert.Equal(t, int32(2), usage.GetSequence(), "Sequence should be 2 after two operations")
}

func TestPlaceWaterBucket(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	pos := models.V3{X: 10, Y: 64, Z: 20}
	err := usage.PlaceWaterBucket(pos, FaceUp)

	require.NoError(t, err)
	// PlaceWaterBucket calls UseItemOnBlock which sends 2 packets
	require.Len(t, client.packets, 2, "Should send use_item_on + swing packets")
	assert.Equal(t, int32(1), usage.GetSequence())
}

func TestCollectPowderSnow(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	pos := models.V3{X: 5, Y: 70, Z: 15}
	err := usage.CollectPowderSnow(pos, FaceUp)

	require.NoError(t, err)
	// CollectPowderSnow calls UseItemOnBlock which sends 2 packets
	require.Len(t, client.packets, 2, "Should send use_item_on + swing packets")
	assert.Equal(t, int32(1), usage.GetSequence())
}

func TestCatchFish(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	err := usage.CatchFish(123)

	require.NoError(t, err)
	// CatchFish calls UseItemOnEntity which sends 3 packets
	require.Len(t, client.packets, 3, "Should send InteractWith + InteractAt + Swing packets")
}

func TestCatchAxolotl(t *testing.T) {
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	err := usage.CatchAxolotl(456)

	require.NoError(t, err)
	// CatchAxolotl calls UseItemOnEntity which sends 3 packets
	require.Len(t, client.packets, 3, "Should send InteractWith + InteractAt + Swing packets")
}

func TestHandEnumValues(t *testing.T) {
	assert.Equal(t, Hand(0), MainHand, "MainHand should be 0")
	assert.Equal(t, Hand(1), OffHand, "OffHand should be 1")
}

func TestBlockFaceEnumValues(t *testing.T) {
	assert.Equal(t, BlockFace(0), FaceDown, "FaceDown should be 0 (-Y)")
	assert.Equal(t, BlockFace(1), FaceUp, "FaceUp should be 1 (+Y)")
	assert.Equal(t, BlockFace(2), FaceNorth, "FaceNorth should be 2 (-Z)")
	assert.Equal(t, BlockFace(3), FaceSouth, "FaceSouth should be 3 (+Z)")
	assert.Equal(t, BlockFace(4), FaceWest, "FaceWest should be 4 (-X)")
	assert.Equal(t, BlockFace(5), FaceEast, "FaceEast should be 5 (+X)")
}

func TestInteractionTypeEnumValues(t *testing.T) {
	assert.Equal(t, InteractionType(0), InteractAt, "InteractAt should be 0")
	assert.Equal(t, InteractionType(1), Attack, "Attack should be 1")
	assert.Equal(t, InteractionType(2), InteractWith, "InteractWith should be 2")
}

func TestPacketSenderError(t *testing.T) {
	// Test that errors from PacketSender are propagated
	client := &MockPacketSender{err: assert.AnError}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	err := usage.PlaceBlock(models.V3{X: 0, Y: 64, Z: 0}, FaceUp, MainHand, 0.5, 0.5, 0.5)

	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestMultipleOperations(t *testing.T) {
	// Test that multiple operations work correctly and sequence tracks properly
	client := &MockPacketSender{}
	packetMgr := newMockPacketManager()
	usage := NewItemUsage(client, packetMgr)

	pos := models.V3{X: 0, Y: 64, Z: 0}

	// Place block (sends 1 packet)
	err := usage.PlaceBlock(pos, FaceUp, MainHand, 0.5, 0.5, 0.5)
	require.NoError(t, err)
	assert.Equal(t, 1, len(client.packets))
	assert.Equal(t, int32(1), usage.GetSequence())

	// Use item on block (sends 2 packets: use_item_on + swing)
	err = usage.UseItemOnBlock(pos, FaceNorth, MainHand)
	require.NoError(t, err)
	assert.Equal(t, 3, len(client.packets), "PlaceBlock(1) + UseItemOnBlock(2) = 3 packets")
	assert.Equal(t, int32(2), usage.GetSequence())

	// Use item on entity (sends 3 packets: interact + interact-at + swing)
	err = usage.UseItemOnEntity(123, MainHand, false)
	require.NoError(t, err)
	assert.Equal(t, 6, len(client.packets), "Previous(3) + UseItemOnEntity(3) = 6 packets")
	// Note: UseItemOnEntity doesn't increment sequence (uses different packet type)
	assert.Equal(t, int32(2), usage.GetSequence())

	// Switch slot (sends 1 packet)
	err = usage.SwitchToSlot(5)
	require.NoError(t, err)
	assert.Equal(t, 7, len(client.packets), "Previous(6) + SwitchToSlot(1) = 7 packets")
	// Note: SwitchToSlot doesn't increment sequence
	assert.Equal(t, int32(2), usage.GetSequence())
}

// MockInventory implements InventoryProvider for testing
type MockInventory struct {
	slots map[int]struct {
		itemID int32
		count  int
	}
	slotCount int // Total number of slots in this inventory
}

func newMockInventory() *MockInventory {
	return newMockInventoryWithSize(45) // Default: player inventory size
}

func newMockInventoryWithSize(slotCount int) *MockInventory {
	return &MockInventory{
		slots: make(map[int]struct {
			itemID int32
			count  int
		}),
		slotCount: slotCount,
	}
}

func (m *MockInventory) SetSlot(index int, itemID int32, count int) {
	m.slots[index] = struct {
		itemID int32
		count  int
	}{itemID, count}
}

func (m *MockInventory) GetSlot(index int) (int32, int, bool) {
	slot, ok := m.slots[index]
	if !ok {
		return 0, 0, false
	}
	return slot.itemID, slot.count, true
}

func (m *MockInventory) GetSlotCount() int {
	return m.slotCount
}

func TestFindItemInInventory(t *testing.T) {
	usage := NewItemUsage(&MockPacketSender{}, newMockPacketManager())
	inventory := newMockInventory()

	// Set up inventory with some items
	inventory.SetSlot(0, 276, 1)  // Diamond sword in hotbar slot 0
	inventory.SetSlot(5, 325, 16) // Water bucket in hotbar slot 5
	inventory.SetSlot(15, 1, 64)  // Stone in main inventory
	inventory.SetSlot(40, 276, 1) // Diamond sword in offhand

	tests := []struct {
		name          string
		itemID        int32
		expectedSlot  int
		expectedFound bool
	}{
		{
			name:          "Find diamond sword in hotbar",
			itemID:        276,
			expectedSlot:  0, // First occurrence
			expectedFound: true,
		},
		{
			name:          "Find water bucket",
			itemID:        325,
			expectedSlot:  5,
			expectedFound: true,
		},
		{
			name:          "Find stone in main inventory",
			itemID:        1,
			expectedSlot:  15,
			expectedFound: true,
		},
		{
			name:          "Item not in inventory",
			itemID:        999,
			expectedSlot:  -1,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slot := usage.FindItemInInventory(tt.itemID, inventory)
			if tt.expectedFound {
				assert.Equal(t, tt.expectedSlot, slot,
					"Should find item in correct slot")
			} else {
				assert.Equal(t, -1, slot,
					"Should return -1 when item not found")
			}
		})
	}
}

func TestFindItemInInventory_EmptyInventory(t *testing.T) {
	usage := NewItemUsage(&MockPacketSender{}, newMockPacketManager())
	inventory := newMockInventory()

	slot := usage.FindItemInInventory(276, inventory)
	assert.Equal(t, -1, slot, "Should return -1 for empty inventory")
}

func TestFindItemInInventory_FirstMatch(t *testing.T) {
	usage := NewItemUsage(&MockPacketSender{}, newMockPacketManager())
	inventory := newMockInventory()

	// Put same item in multiple slots
	inventory.SetSlot(3, 276, 1)
	inventory.SetSlot(10, 276, 1)
	inventory.SetSlot(20, 276, 1)

	slot := usage.FindItemInInventory(276, inventory)
	assert.Equal(t, 3, slot, "Should return first matching slot")
}

func TestFindItemInInventory_DifferentInventoryTypes(t *testing.T) {
	usage := NewItemUsage(&MockPacketSender{}, newMockPacketManager())

	tests := []struct {
		name         string
		slotCount    int
		itemSlot     int // Where to place the item
		itemID       int32
		expectedSlot int
		description  string
	}{
		{
			name:         "Chest (27 slots)",
			slotCount:    27,
			itemSlot:     15,
			itemID:       264, // Diamond
			expectedSlot: 15,
			description:  "Single chest has 27 slots (3 rows x 9 columns)",
		},
		{
			name:         "Double chest (54 slots)",
			slotCount:    54,
			itemSlot:     40,
			itemID:       54, // Chest block
			expectedSlot: 40,
			description:  "Double chest has 54 slots (6 rows x 9 columns)",
		},
		{
			name:         "Ender chest (27 slots)",
			slotCount:    27,
			itemSlot:     26,
			itemID:       368, // Ender pearl
			expectedSlot: 26,
			description:  "Ender chest has 27 slots (same as regular chest)",
		},
		{
			name:         "Barrel (27 slots)",
			slotCount:    27,
			itemSlot:     10,
			itemID:       326, // Water bucket
			expectedSlot: 10,
			description:  "Barrel has 27 slots (same as chest)",
		},
		{
			name:         "Shulker box (27 slots)",
			slotCount:    27,
			itemSlot:     20,
			itemID:       1, // Stone
			expectedSlot: 20,
			description:  "Shulker box has 27 slots (portable chest)",
		},
		{
			name:         "Hopper (5 slots)",
			slotCount:    5,
			itemSlot:     3,
			itemID:       388, // Emerald
			expectedSlot: 3,
			description:  "Hopper has 5 slots (single row)",
		},
		{
			name:         "Furnace (3 slots)",
			slotCount:    3,
			itemSlot:     2,
			itemID:       265, // Iron ingot
			expectedSlot: 2,
			description:  "Furnace has 3 slots (input, fuel, output)",
		},
		{
			name:         "Horse with chest (17 slots)",
			slotCount:    17,
			itemSlot:     10,
			itemID:       322, // Golden apple
			expectedSlot: 10,
			description:  "Horse with chest: 2 equipment + 15 storage slots",
		},
		{
			name:         "Horse without chest (2 slots)",
			slotCount:    2,
			itemSlot:     0,
			itemID:       329, // Saddle
			expectedSlot: 0,
			description:  "Horse without chest: saddle + armor slots only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inventory := newMockInventoryWithSize(tt.slotCount)

			// Verify slot count
			assert.Equal(t, tt.slotCount, inventory.GetSlotCount(),
				"Inventory should have correct slot count")

			// Place item in the specified slot
			inventory.SetSlot(tt.itemSlot, tt.itemID, 1)

			// Find the item
			slot := usage.FindItemInInventory(tt.itemID, inventory)
			assert.Equal(t, tt.expectedSlot, slot,
				"Should find item in %s at slot %d", tt.name, tt.expectedSlot)

			// Verify item not found when searching for different item
			notFoundSlot := usage.FindItemInInventory(999, inventory)
			assert.Equal(t, -1, notFoundSlot,
				"Should return -1 for item not in %s", tt.name)
		})
	}
}

func TestFindItemInInventory_BoundaryRespect(t *testing.T) {
	usage := NewItemUsage(&MockPacketSender{}, newMockPacketManager())

	// Test that search respects inventory boundaries
	// Create a small inventory (hopper with 5 slots)
	hopper := newMockInventoryWithSize(5)

	// Put item in slot 2 (valid)
	hopper.SetSlot(2, 264, 1) // Diamond

	// Put item in slot 10 (beyond hopper's boundary, but allowed by mock)
	hopper.SetSlot(10, 388, 1) // Emerald

	// Search should only find items within the inventory's slot count (0-4)
	diamondSlot := usage.FindItemInInventory(264, hopper)
	assert.Equal(t, 2, diamondSlot, "Should find diamond at slot 2")

	// Emerald at slot 10 should NOT be found (beyond slot count)
	emeraldSlot := usage.FindItemInInventory(388, hopper)
	assert.Equal(t, -1, emeraldSlot,
		"Should not find emerald at slot 10 (beyond hopper's 5 slots)")
}

func TestValidateBlockReachDistance(t *testing.T) {
	tests := []struct {
		name        string
		playerPos   models.V3
		targetPos   models.V3
		withinReach bool
	}{
		{
			name:        "Same position",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			targetPos:   models.V3{X: 0, Y: 64, Z: 0},
			withinReach: true,
		},
		{
			name:        "Just within reach (4.0 blocks)",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			targetPos:   models.V3{X: 4, Y: 64, Z: 0},
			withinReach: true,
		},
		{
			name:        "At max reach (4.5 blocks)",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			targetPos:   models.V3{X: 4.5, Y: 64, Z: 0},
			withinReach: true,
		},
		{
			name:        "Just beyond reach (5.0 blocks)",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			targetPos:   models.V3{X: 5, Y: 64, Z: 0},
			withinReach: false,
		},
		{
			name:        "Diagonal beyond reach",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			targetPos:   models.V3{X: 3, Y: 67, Z: 2},
			withinReach: false, // √(3²+3²+2²) = √22 ≈ 4.69, exceeds 4.5 limit
		},
		{
			name:        "Far away",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			targetPos:   models.V3{X: 10, Y: 64, Z: 10},
			withinReach: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateBlockReachDistance(tt.playerPos, tt.targetPos)
			assert.Equal(t, tt.withinReach, result)
		})
	}
}

func TestValidateEntityReachDistance(t *testing.T) {
	tests := []struct {
		name        string
		playerPos   models.V3
		entityPos   models.V3
		withinReach bool
	}{
		{
			name:        "Same position",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			entityPos:   models.V3{X: 0, Y: 64, Z: 0},
			withinReach: true,
		},
		{
			name:        "Just within reach (2.5 blocks)",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			entityPos:   models.V3{X: 2.5, Y: 64, Z: 0},
			withinReach: true,
		},
		{
			name:        "At max reach (3.0 blocks)",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			entityPos:   models.V3{X: 3, Y: 64, Z: 0},
			withinReach: true,
		},
		{
			name:        "Just beyond reach (3.5 blocks)",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			entityPos:   models.V3{X: 3.5, Y: 64, Z: 0},
			withinReach: false,
		},
		{
			name:        "Vertical within reach",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			entityPos:   models.V3{X: 0, Y: 67, Z: 0},
			withinReach: true, // 3 blocks vertical
		},
		{
			name:        "Diagonal just beyond reach",
			playerPos:   models.V3{X: 0, Y: 64, Z: 0},
			entityPos:   models.V3{X: 2, Y: 66, Z: 2},
			withinReach: false, // √(2²+2²+2²) = √12 ≈ 3.46 > 3.0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateEntityReachDistance(tt.playerPos, tt.entityPos)
			assert.Equal(t, tt.withinReach, result)
		})
	}
}

func TestReachDistanceConstants(t *testing.T) {
	assert.Equal(t, 4.5, MaxBlockReachDistance, "Max block reach should be 4.5 blocks")
	assert.Equal(t, 3.0, MaxEntityReachDistance, "Max entity reach should be 3.0 blocks")
}
