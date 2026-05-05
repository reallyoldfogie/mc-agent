package v1_21_10

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	v1_21_10 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/play/serverbound"
)

// mockPacketWriter for testing
type mockPacketWriter struct {
	lastPacket pk.Packet
}

func (m *mockPacketWriter) WritePacket(p pk.Packet) error {
	m.lastPacket = p
	return nil
}

func TestMovementHandler_SendPosition_PacketStructure(t *testing.T) {
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	err := handler.SendPosition(writer, 100.5, 64.0, -200.25, true)
	if err != nil {
		t.Fatalf("SendPosition failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundMovePlayerPos")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	// Parse and verify contents
	pkt := sb.NewPosition()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if float64(pkt.X) != 100.5 {
		t.Errorf("Expected X=100.5, got %f", pkt.X)
	}
	if float64(pkt.Y) != 64.0 {
		t.Errorf("Expected Y=64.0, got %f", pkt.Y)
	}
	if float64(pkt.Z) != -200.25 {
		t.Errorf("Expected Z=-200.25, got %f", pkt.Z)
	}
	if !pkt.Flags.OnGround() {
		t.Error("Expected onGround=true")
	}
}

func TestMovementHandler_SendPositionAndRotation_PacketStructure(t *testing.T) {
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	err := handler.SendPositionAndRotation(writer, 50.0, 100.0, -150.0, 90.0, 45.0, false)
	if err != nil {
		t.Fatalf("SendPositionAndRotation failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundMovePlayerPosRot")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	// Parse and verify contents
	pkt := sb.NewPositionLook()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if float64(pkt.X) != 50.0 {
		t.Errorf("Expected X=50.0, got %f", pkt.X)
	}
	if float32(pkt.Yaw) != 90.0 {
		t.Errorf("Expected Yaw=90.0, got %f", pkt.Yaw)
	}
	if float32(pkt.Pitch) != 45.0 {
		t.Errorf("Expected Pitch=45.0, got %f", pkt.Pitch)
	}
	if pkt.Flags.OnGround() {
		t.Error("Expected onGround=false")
	}
}

func TestMovementHandler_SendRotation_PacketStructure(t *testing.T) {
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	err := handler.SendRotation(writer, 180.0, -45.0, true)
	if err != nil {
		t.Fatalf("SendRotation failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundMovePlayerRot")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	pkt := sb.NewLook()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if float32(pkt.Yaw) != 180.0 {
		t.Errorf("Expected Yaw=180.0, got %f", pkt.Yaw)
	}
	if float32(pkt.Pitch) != -45.0 {
		t.Errorf("Expected Pitch=-45.0, got %f", pkt.Pitch)
	}
}

func TestMovementHandler_SendPlayerCommand_EntityAction(t *testing.T) {
	// Test EntityAction-based commands (not sneaking)
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	// Test start sprinting (actionID 3 -> "start_sprinting")
	err := handler.SendPlayerCommand(writer, 12345, common.ActionStartSprinting)
	if err != nil {
		t.Fatalf("SendPlayerCommand failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundPlayerCommand")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	pkt := sb.NewEntityAction()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if int32(pkt.EntityId) != 12345 {
		t.Errorf("Expected EntityId=12345, got %d", pkt.EntityId)
	}
	if pkt.ActionId.Value != "start_sprinting" {
		t.Errorf("Expected ActionId='start_sprinting', got '%s'", pkt.ActionId.Value)
	}
}

func TestMovementHandler_SendPlayerCommand_PlayerInput(t *testing.T) {
	// Test PlayerInput-based commands (sneaking)
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	// Test start sneaking (actionID 0 -> PlayerInput with Shift=true)
	err := handler.SendPlayerCommand(writer, 12345, common.ActionStartSneaking)
	if err != nil {
		t.Fatalf("SendPlayerCommand failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundPlayerInput")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	pkt := sb.NewPlayerInput()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if !pkt.Inputs.Shift() {
		t.Error("Expected Shift flag to be true for start sneaking")
	}
}

func TestMovementHandler_SendTeleportConfirm_PacketStructure(t *testing.T) {
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	err := handler.SendTeleportConfirm(writer, 999)
	if err != nil {
		t.Fatalf("SendTeleportConfirm failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundAcceptTeleportation")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	pkt := sb.NewTeleportConfirm()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if int32(pkt.TeleportId) != 999 {
		t.Errorf("Expected TeleportId=999, got %d", pkt.TeleportId)
	}
}

func TestMovementHandler_SendPlayerAbilities_PacketStructure(t *testing.T) {
	handler := &movementHandler{}
	writer := &mockPacketWriter{}

	err := handler.SendPlayerAbilities(writer, 0x02) // Flying flag
	if err != nil {
		t.Fatalf("SendPlayerAbilities failed: %v", err)
	}

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_10.NewPackets()
	expectedID := packetMgr.GetServerboundPacketID("ServerboundPlayerAbilities")
	if writer.lastPacket.ID != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, writer.lastPacket.ID)
	}

	pkt := sb.NewAbilities()
	if err := pkt.Scan(writer.lastPacket); err != nil {
		t.Fatalf("Failed to parse packet: %v", err)
	}

	if byte(pkt.Flags) != 0x02 {
		t.Errorf("Expected Flags=0x02, got 0x%02x", pkt.Flags)
	}
}

func TestMovementHandler_ParsePlayerPosition(t *testing.T) {
	// Create a mock Position packet (clientbound)
	pkt := cb.NewPosition()
	pkt.TeleportId = pk.VarInt(42)
	pkt.X = pk.Double(123.456)
	pkt.Y = pk.Double(78.9)
	pkt.Z = pk.Double(-456.123)
	pkt.Yaw = pk.Float(90.0)
	pkt.Pitch = pk.Float(-45.0)
	pkt.Flags.UInt32 = 0x01 // Some flag

	handler := &movementHandler{}
	marshaled := pkt.Marshal()
	teleportID, x, y, z, yaw, pitch, flags, err := handler.ParsePlayerPosition(marshaled)

	if err != nil {
		t.Fatalf("ParsePlayerPosition failed: %v", err)
	}

	if teleportID != 42 {
		t.Errorf("Expected teleportID=42, got %d", teleportID)
	}
	if x != 123.456 {
		t.Errorf("Expected X=123.456, got %f", x)
	}
	if y != 78.9 {
		t.Errorf("Expected Y=78.9, got %f", y)
	}
	if z != -456.123 {
		t.Errorf("Expected Z=-456.123, got %f", z)
	}
	if yaw != 90.0 {
		t.Errorf("Expected Yaw=90.0, got %f", yaw)
	}
	if pitch != -45.0 {
		t.Errorf("Expected Pitch=-45.0, got %f", pitch)
	}
	if flags != 0x01 {
		t.Errorf("Expected Flags=0x01, got 0x%02x", flags)
	}
}

func TestMovementHandler_ActionConstants(t *testing.T) {
	// Verify that action constants are properly defined
	if common.ActionStartSneaking != 0 {
		t.Errorf("Expected ActionStartSneaking=0, got %d", common.ActionStartSneaking)
	}
	if common.ActionStopSneaking != 1 {
		t.Errorf("Expected ActionStopSneaking=1, got %d", common.ActionStopSneaking)
	}
	if common.ActionStartSprinting != 3 {
		t.Errorf("Expected ActionStartSprinting=3, got %d", common.ActionStartSprinting)
	}
	if common.ActionStopSprinting != 4 {
		t.Errorf("Expected ActionStopSprinting=4, got %d", common.ActionStopSprinting)
	}
}
