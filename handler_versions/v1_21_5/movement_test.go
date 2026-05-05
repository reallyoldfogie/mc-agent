package v1_21_5

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/serverbound"
)

func TestMovementHandler_SendPosition_PacketStructure(t *testing.T) {
	// Test that the generated Position packet structure matches expectations
	pkt := sb.NewPosition()
	pkt.X = pk.Double(100.5)
	pkt.Y = pk.Double(64.0)
	pkt.Z = pk.Double(-200.25)
	pkt.Flags.SetOnGround(true)

	// Verify packet ID is correct for 1.21.5
	if pkt.PacketID() != 28 {
		t.Errorf("Expected packet ID 28, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if float64(pkt.X) != 100.5 {
		t.Errorf("Expected X 100.5, got %f", pkt.X)
	}
	if float64(pkt.Y) != 64.0 {
		t.Errorf("Expected Y 64.0, got %f", pkt.Y)
	}
	if float64(pkt.Z) != -200.25 {
		t.Errorf("Expected Z -200.25, got %f", pkt.Z)
	}
	if !pkt.Flags.OnGround() {
		t.Error("Expected OnGround true, got false")
	}
}

func TestMovementHandler_SendPositionAndRotation_PacketStructure(t *testing.T) {
	// Test that the generated PositionLook packet structure matches expectations
	pkt := sb.NewPositionLook()
	pkt.X = pk.Double(100.5)
	pkt.Y = pk.Double(64.0)
	pkt.Z = pk.Double(-200.25)
	pkt.Yaw = pk.Float(90.0)
	pkt.Pitch = pk.Float(-45.0)
	pkt.Flags.SetOnGround(false)

	// Verify packet ID is correct for 1.21.5
	if pkt.PacketID() != 29 {
		t.Errorf("Expected packet ID 29, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if float64(pkt.X) != 100.5 {
		t.Errorf("Expected X 100.5, got %f", pkt.X)
	}
	if float32(pkt.Yaw) != 90.0 {
		t.Errorf("Expected Yaw 90.0, got %f", pkt.Yaw)
	}
	if float32(pkt.Pitch) != -45.0 {
		t.Errorf("Expected Pitch -45.0, got %f", pkt.Pitch)
	}
	if pkt.Flags.OnGround() {
		t.Error("Expected OnGround false, got true")
	}
}

func TestMovementHandler_SendRotation_PacketStructure(t *testing.T) {
	// Test that the generated Look packet structure matches expectations
	pkt := sb.NewLook()
	pkt.Yaw = pk.Float(180.0)
	pkt.Pitch = pk.Float(0.0)
	pkt.Flags.SetOnGround(true)

	// Verify packet ID is correct for 1.21.5
	if pkt.PacketID() != 30 {
		t.Errorf("Expected packet ID 30, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if float32(pkt.Yaw) != 180.0 {
		t.Errorf("Expected Yaw 180.0, got %f", pkt.Yaw)
	}
	if float32(pkt.Pitch) != 0.0 {
		t.Errorf("Expected Pitch 0.0, got %f", pkt.Pitch)
	}
	if !pkt.Flags.OnGround() {
		t.Error("Expected OnGround true, got false")
	}
}

func TestMovementHandler_SendPlayerCommand_PacketStructure(t *testing.T) {
	// Test that the generated EntityAction packet structure matches expectations
	pkt := sb.NewEntityAction()
	pkt.EntityId = pk.VarInt(12345)
	pkt.ActionId = pk.VarInt(common.ActionStartSneaking)
	pkt.JumpBoost = pk.VarInt(0)

	// Verify packet ID is correct for 1.21.5
	if pkt.PacketID() != 40 {
		t.Errorf("Expected packet ID 40, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if int32(pkt.EntityId) != 12345 {
		t.Errorf("Expected EntityId 12345, got %d", pkt.EntityId)
	}
	if int32(pkt.ActionId) != 0 { // ActionStartSneaking = 0
		t.Errorf("Expected ActionId 0, got %d", pkt.ActionId)
	}
	if int32(pkt.JumpBoost) != 0 {
		t.Errorf("Expected JumpBoost 0, got %d", pkt.JumpBoost)
	}
}

func TestMovementHandler_SendTeleportConfirm_PacketStructure(t *testing.T) {
	// Test that the generated TeleportConfirm packet structure matches expectations
	pkt := sb.NewTeleportConfirm()
	pkt.TeleportId = pk.VarInt(42)

	// Verify packet ID is correct for 1.21.5
	if pkt.PacketID() != 0 {
		t.Errorf("Expected packet ID 0, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if int32(pkt.TeleportId) != 42 {
		t.Errorf("Expected TeleportId 42, got %d", pkt.TeleportId)
	}
}

func TestMovementHandler_SendPlayerAbilities_PacketStructure(t *testing.T) {
	// Test that the generated Abilities packet structure matches expectations
	pkt := sb.NewAbilities()
	pkt.Flags = pk.Byte(0x02) // Flying flag

	// Verify packet ID is correct for 1.21.5
	if pkt.PacketID() != 38 {
		t.Errorf("Expected packet ID 38, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if byte(pkt.Flags) != 0x02 {
		t.Errorf("Expected Flags 0x02, got 0x%02x", pkt.Flags)
	}
}

func TestMovementHandler_ParsePlayerPosition(t *testing.T) {
	// Create a mock clientbound Position packet
	pkt := cb.NewPosition()
	pkt.TeleportId = pk.VarInt(123)
	pkt.X = pk.Double(100.5)
	pkt.Y = pk.Double(64.0)
	pkt.Z = pk.Double(-200.25)
	pkt.Yaw = pk.Float(90.0)
	pkt.Pitch = pk.Float(-45.0)
	// Flags set to indicate relative coordinates

	// Marshal and then parse
	handler := &movementHandler{}
	marshaled := pkt.Marshal()

	teleportID, x, y, z, yaw, pitch, flags, err := handler.ParsePlayerPosition(marshaled)
	if err != nil {
		t.Fatalf("ParsePlayerPosition failed: %v", err)
	}

	if teleportID != 123 {
		t.Errorf("Expected teleportID 123, got %d", teleportID)
	}
	if x != 100.5 {
		t.Errorf("Expected X 100.5, got %f", x)
	}
	if y != 64.0 {
		t.Errorf("Expected Y 64.0, got %f", y)
	}
	if z != -200.25 {
		t.Errorf("Expected Z -200.25, got %f", z)
	}
	if yaw != 90.0 {
		t.Errorf("Expected Yaw 90.0, got %f", yaw)
	}
	if pitch != -45.0 {
		t.Errorf("Expected Pitch -45.0, got %f", pitch)
	}
	_ = flags // Flags are parsed but not critical for this test
}

func TestMovementHandler_ActionConstants(t *testing.T) {
	// Verify action constants match expected values
	tests := []struct {
		name     string
		constant int32
		expected int32
	}{
		{"ActionStartSneaking", common.ActionStartSneaking, 0},
		{"ActionStopSneaking", common.ActionStopSneaking, 1},
		{"ActionLeaveBed", common.ActionLeaveBed, 2},
		{"ActionStartSprinting", common.ActionStartSprinting, 3},
		{"ActionStopSprinting", common.ActionStopSprinting, 4},
		{"ActionStartJumpHorse", common.ActionStartJumpHorse, 5},
		{"ActionStopJumpHorse", common.ActionStopJumpHorse, 6},
		{"ActionOpenVehicleInv", common.ActionOpenVehicleInv, 7},
		{"ActionStartFlyingElytra", common.ActionStartFlyingElytra, 8},
	}

	for _, tc := range tests {
		if tc.constant != tc.expected {
			t.Errorf("%s: expected %d, got %d", tc.name, tc.expected, tc.constant)
		}
	}
}
