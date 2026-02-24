package v1_21_3

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	v1_21_4 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/play/clientbound"
	"github.com/reallyoldfogie/mc-protocol-go/models"
)

func TestEntityHandler_ParseAddEntity(t *testing.T) {
	// Create a mock SpawnEntity packet
	pkt := cb.NewSpawnEntity()
	pkt.EntityId = pk.VarInt(12345)
	pkt.ObjectUUID = pk.UUID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	pkt.Type = pk.VarInt(1) // Entity type
	pkt.X = pk.Double(100.5)
	pkt.Y = pk.Double(64.0)
	pkt.Z = pk.Double(-200.25)
	pkt.Yaw = pk.Byte(-128) // ~180 degrees (signed byte)
	pkt.Pitch = pk.Byte(64) // ~90 degrees
	pkt.HeadPitch = pk.Byte(-128)
	pkt.ObjectData = pk.VarInt(1) // Creator player ID (for entities like arrows)
	pkt.VelocityX = pk.Short(0)
	pkt.VelocityY = pk.Short(0)
	pkt.VelocityZ = pk.Short(0)

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_4.NewPackets()
	expectedID := packetMgr.GetClientboundPacketID("ClientboundSpawnEntity")
	if pkt.PacketID() != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
	}

	// Parse it
	handler := &entityHandler{}
	marshaled := pkt.Marshal()
	entityID, entityType, creatorEntityID, uuid, x, y, z, yaw, pitch, velX, velY, velZ, err := handler.ParseAddEntity(marshaled)

	if err != nil {
		t.Fatalf("ParseAddEntity failed: %v", err)
	}

	if entityID != 12345 {
		t.Errorf("Expected entityID 12345, got %d", entityID)
	}
	if entityType != 1 {
		t.Errorf("Expected entityType 1, got %d", entityType)
	}
	if uuid != [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10} {
		t.Errorf("UUID mismatch, got %v", uuid)
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
	if yaw != -128 {
		t.Errorf("Expected yaw -128, got %d", yaw)
	}
	if pitch != 64 {
		t.Errorf("Expected pitch 64, got %d", pitch)
	}
	if creatorEntityID != 1 {
		t.Errorf("Expected entityCreator 1, got %d", creatorEntityID)
	}
	if velX != 0 {
		t.Errorf("Expected velX 0, got %f", velX)
	}
	if velY != 0 {
		t.Errorf("Expected velY 0, got %f", velY)
	}
	if velZ != 0 {
		t.Errorf("Expected velZ 0, got %f", velZ)
	}
}

func TestEntityHandler_ParseMoveEntityPos(t *testing.T) {
	// Create a mock RelEntityMove packet
	pkt := cb.NewRelEntityMove()
	pkt.EntityId = pk.VarInt(999)
	pkt.DX = pk.Short(128)  // 128/4096 blocks
	pkt.DY = pk.Short(-256) // -256/4096 blocks
	pkt.DZ = pk.Short(512)  // 512/4096 blocks
	pkt.OnGround = pk.Boolean(true)

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_4.NewPackets()
	expectedID := packetMgr.GetClientboundPacketID("ClientboundRelEntityMove")
	if pkt.PacketID() != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
	}

	handler := &entityHandler{}
	marshaled := pkt.Marshal()
	entityID, dx, dy, dz, onGround, err := handler.ParseMoveEntityPos(marshaled)

	if err != nil {
		t.Fatalf("ParseMoveEntityPos failed: %v", err)
	}

	if entityID != 999 {
		t.Errorf("Expected entityID 999, got %d", entityID)
	}
	if dx != 128 {
		t.Errorf("Expected dx 128, got %d", dx)
	}
	if dy != -256 {
		t.Errorf("Expected dy -256, got %d", dy)
	}
	if dz != 512 {
		t.Errorf("Expected dz 512, got %d", dz)
	}
	if !onGround {
		t.Error("Expected onGround true, got false")
	}
}

func TestEntityHandler_ParseMoveEntityPosRot(t *testing.T) {
	// Create a mock EntityMoveLook packet
	pkt := cb.NewEntityMoveLook()
	pkt.EntityId = pk.VarInt(555)
	pkt.DX = pk.Short(64)
	pkt.DY = pk.Short(32)
	pkt.DZ = pk.Short(-128)
	pkt.Yaw = pk.Byte(64)
	pkt.Pitch = pk.Byte(32)
	pkt.OnGround = pk.Boolean(false)

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_4.NewPackets()
	expectedID := packetMgr.GetClientboundPacketID("ClientboundEntityMoveLook")
	if pkt.PacketID() != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
	}

	handler := &entityHandler{}
	marshaled := pkt.Marshal()
	entityID, dx, dy, dz, yaw, pitch, onGround, err := handler.ParseMoveEntityPosRot(marshaled)

	if err != nil {
		t.Fatalf("ParseMoveEntityPosRot failed: %v", err)
	}

	if entityID != 555 {
		t.Errorf("Expected entityID 555, got %d", entityID)
	}
	if dx != 64 {
		t.Errorf("Expected dx 64, got %d", dx)
	}
	if dy != 32 {
		t.Errorf("Expected dy 32, got %d", dy)
	}
	if dz != -128 {
		t.Errorf("Expected dz -128, got %d", dz)
	}
	if yaw != 64 {
		t.Errorf("Expected yaw 64, got %d", yaw)
	}
	if pitch != 32 {
		t.Errorf("Expected pitch 32, got %d", pitch)
	}
	if onGround {
		t.Error("Expected onGround false, got true")
	}
}

func TestEntityHandler_ParseTeleportEntity(t *testing.T) {
	// Create a mock EntityTeleport packet
	pkt := cb.NewEntityTeleport()
	pkt.EntityId = pk.VarInt(777)
	pkt.X = pk.Double(250.75)
	pkt.Y = pk.Double(120.0)
	pkt.Z = pk.Double(-300.5)
	pkt.Yaw = pk.Byte(-56) // 200 as signed byte
	pkt.Pitch = pk.Byte(100)
	pkt.OnGround = pk.Boolean(true)

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_4.NewPackets()
	expectedID := packetMgr.GetClientboundPacketID("ClientboundEntityTeleport")
	if pkt.PacketID() != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
	}

	handler := &entityHandler{}
	marshaled := pkt.Marshal()
	entityID, x, y, z, yaw, pitch, onGround, err := handler.ParseTeleportEntity(marshaled)

	if err != nil {
		t.Fatalf("ParseTeleportEntity failed: %v", err)
	}

	if entityID != 777 {
		t.Errorf("Expected entityID 777, got %d", entityID)
	}
	if x != 250.75 {
		t.Errorf("Expected X 250.75, got %f", x)
	}
	if y != 120.0 {
		t.Errorf("Expected Y 120.0, got %f", y)
	}
	if z != -300.5 {
		t.Errorf("Expected Z -300.5, got %f", z)
	}
	if yaw != -56 {
		t.Errorf("Expected yaw -56, got %d", yaw)
	}
	if pitch != 100 {
		t.Errorf("Expected pitch 100, got %d", pitch)
	}
	if !onGround {
		t.Error("Expected onGround true, got false")
	}
}

func TestEntityHandler_ParseRemoveEntities(t *testing.T) {
	// Create a mock EntityDestroy packet
	pkt := cb.NewEntityDestroy()
	ids := []pk.VarInt{pk.VarInt(100), pk.VarInt(200), pk.VarInt(300)}
	pkt.EntityIds = models.Array[pk.VarInt, pk.VarInt]{}
	pkt.EntityIds.Set(ids)

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_4.NewPackets()
	expectedID := packetMgr.GetClientboundPacketID("ClientboundEntityDestroy")
	if pkt.PacketID() != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
	}

	handler := &entityHandler{}
	marshaled := pkt.Marshal()
	entityIDs, err := handler.ParseRemoveEntities(marshaled)

	if err != nil {
		t.Fatalf("ParseRemoveEntities failed: %v", err)
	}

	if len(entityIDs) != 3 {
		t.Fatalf("Expected 3 entity IDs, got %d", len(entityIDs))
	}
	if entityIDs[0] != 100 {
		t.Errorf("Expected entityIDs[0] = 100, got %d", entityIDs[0])
	}
	if entityIDs[1] != 200 {
		t.Errorf("Expected entityIDs[1] = 200, got %d", entityIDs[1])
	}
	if entityIDs[2] != 300 {
		t.Errorf("Expected entityIDs[2] = 300, got %d", entityIDs[2])
	}
}

func TestEntityHandler_ParseEntityEvent(t *testing.T) {
	// Create a mock EntityStatus packet
	pkt := cb.NewEntityStatus()
	pkt.EntityId = pk.Int(88888)
	pkt.EntityStatus = pk.Byte(3) // Entity hurt animation

	// Verify packet ID using PacketMgr
	packetMgr := v1_21_4.NewPackets()
	expectedID := packetMgr.GetClientboundPacketID("ClientboundEntityStatus")
	if pkt.PacketID() != int32(expectedID) {
		t.Errorf("Expected packet ID %d, got %d", expectedID, pkt.PacketID())
	}

	handler := &entityHandler{}
	marshaled := pkt.Marshal()
	entityID, eventID, err := handler.ParseEntityEvent(marshaled)

	if err != nil {
		t.Fatalf("ParseEntityEvent failed: %v", err)
	}

	if entityID != 88888 {
		t.Errorf("Expected entityID 88888, got %d", entityID)
	}
	if eventID != 3 {
		t.Errorf("Expected eventID 3, got %d", eventID)
	}
}

func TestEntityHandler_PacketIDs(t *testing.T) {
	// Verify packet IDs can be looked up dynamically for 1.21.4
	packetMgr := v1_21_4.NewPackets()

	tests := []struct {
		name       string
		packetName string
		createPkt  func() interface{ PacketID() int32 }
	}{
		{"SpawnEntity", "ClientboundSpawnEntity", func() interface{ PacketID() int32 } { return cb.NewSpawnEntity() }},
		{"RelEntityMove", "ClientboundRelEntityMove", func() interface{ PacketID() int32 } { return cb.NewRelEntityMove() }},
		{"EntityMoveLook", "ClientboundEntityMoveLook", func() interface{ PacketID() int32 } { return cb.NewEntityMoveLook() }},
		{"EntityTeleport", "ClientboundEntityTeleport", func() interface{ PacketID() int32 } { return cb.NewEntityTeleport() }},
		{"EntityDestroy", "ClientboundEntityDestroy", func() interface{ PacketID() int32 } { return cb.NewEntityDestroy() }},
		{"EntityStatus", "ClientboundEntityStatus", func() interface{ PacketID() int32 } { return cb.NewEntityStatus() }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkt := tc.createPkt()
			expectedID := packetMgr.GetClientboundPacketID(tc.packetName)
			if pkt.PacketID() != int32(expectedID) {
				t.Errorf("%s: expected packet ID %d, got %d", tc.name, expectedID, pkt.PacketID())
			}
		})
	}
}
