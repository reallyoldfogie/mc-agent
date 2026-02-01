package v1_21_4

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/configuration/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/configuration/serverbound"
)

func TestConfigurationHandler_SendFinishConfiguration_PacketStructure(t *testing.T) {
	// Test that the generated FinishConfiguration packet structure matches expectations
	pkt := sb.NewFinishConfiguration()

	// Verify packet ID is correct for 1.21.4 configuration phase
	if pkt.PacketID() != 3 {
		t.Errorf("Expected packet ID 3, got %d", pkt.PacketID())
	}

	// This packet has no fields, so just verify it marshals without error
	marshaled := pkt.Marshal()
	if marshaled.ID != 3 {
		t.Errorf("Expected marshaled packet ID 3, got %d", marshaled.ID)
	}
}

func TestConfigurationHandler_SendKeepAlive_PacketStructure(t *testing.T) {
	// Test that the generated KeepAlive packet structure matches expectations
	pkt := sb.NewKeepAlive()
	pkt.KeepAliveId = pk.Long(1234567890)

	// Verify packet ID is correct for 1.21.4 configuration phase
	if pkt.PacketID() != 4 {
		t.Errorf("Expected packet ID 4, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if int64(pkt.KeepAliveId) != 1234567890 {
		t.Errorf("Expected KeepAliveId 1234567890, got %d", pkt.KeepAliveId)
	}
}

func TestConfigurationHandler_SendPong_PacketStructure(t *testing.T) {
	// Test that the generated Pong packet structure matches expectations
	pkt := sb.NewPong()
	pkt.Id = pk.Int(42)

	// Verify packet ID is correct for 1.21.4 configuration phase
	if pkt.PacketID() != 5 {
		t.Errorf("Expected packet ID 5, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if int32(pkt.Id) != 42 {
		t.Errorf("Expected Id 42, got %d", pkt.Id)
	}
}

func TestConfigurationHandler_SendResourcePackResponse_PacketStructure(t *testing.T) {
	// Test that the generated ResourcePackReceive packet structure matches expectations
	pkt := sb.NewResourcePackReceive()
	testUUID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	pkt.Uuid = pk.UUID(testUUID)
	pkt.Result = pk.VarInt(3) // Successfully downloaded

	// Verify packet ID is correct for 1.21.4 configuration phase
	if pkt.PacketID() != 6 {
		t.Errorf("Expected packet ID 6, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if [16]byte(pkt.Uuid) != testUUID {
		t.Errorf("Expected UUID %x, got %x", testUUID, pkt.Uuid)
	}
	if int32(pkt.Result) != 3 {
		t.Errorf("Expected Result 3, got %d", pkt.Result)
	}
}

func TestConfigurationHandler_ParseKeepAlive(t *testing.T) {
	// Create a mock clientbound KeepAlive packet
	pkt := cb.NewKeepAlive()
	pkt.KeepAliveId = pk.Long(9876543210)

	// Marshal and then parse
	handler := &configurationHandler{}
	marshaled := pkt.Marshal()

	id, err := handler.ParseKeepAlive(marshaled)
	if err != nil {
		t.Fatalf("ParseKeepAlive failed: %v", err)
	}

	if id != 9876543210 {
		t.Errorf("Expected id 9876543210, got %d", id)
	}
}

func TestConfigurationHandler_ParsePing(t *testing.T) {
	// Create a mock clientbound Ping packet
	pkt := cb.NewPing()
	pkt.Id = pk.Int(123)

	// Marshal and then parse
	handler := &configurationHandler{}
	marshaled := pkt.Marshal()

	pingID, err := handler.ParsePing(marshaled)
	if err != nil {
		t.Fatalf("ParsePing failed: %v", err)
	}

	if pingID != 123 {
		t.Errorf("Expected pingID 123, got %d", pingID)
	}
}

func TestConfigurationHandler_ParseRegistryData(t *testing.T) {
	// Create a mock clientbound RegistryData packet
	pkt := cb.NewRegistryData()
	pkt.Id = pk.String("minecraft:worldgen/biome")

	// Initialize the array with an empty slice
	emptySlice := make([]cb.RegistryDataEntriesArrayType, 0)
	pkt.Entries.Set(&emptySlice)

	// Marshal and then parse
	handler := &configurationHandler{}
	marshaled := pkt.Marshal()

	registryID, entries, err := handler.ParseRegistryData(marshaled)
	if err != nil {
		t.Fatalf("ParseRegistryData failed: %v", err)
	}

	if registryID != "minecraft:worldgen/biome" {
		t.Errorf("Expected registryID minecraft:worldgen/biome, got %s", registryID)
	}

	// With no entries added, the entries map should be empty
	if len(entries) != 0 {
		t.Errorf("Expected 0 entries, got %d", len(entries))
	}
}

func TestConfigurationHandler_ParseDisconnect(t *testing.T) {
	// Create a mock clientbound Disconnect packet
	pkt := cb.NewDisconnect()
	// The reason is an AnonymousNBT field
	// For this test, we'll test with a simple string conversion
	// In practice, this would be more complex NBT data

	// Marshal and then parse
	handler := &configurationHandler{}
	marshaled := pkt.Marshal()

	reason, err := handler.ParseDisconnect(marshaled)
	if err != nil {
		t.Fatalf("ParseDisconnect failed: %v", err)
	}

	// Since we didn't set a proper NBT structure, the reason might be empty
	// This is mainly testing that parsing doesn't crash
	_ = reason
}

func TestConfigurationHandler_PacketIDs(t *testing.T) {
	// Verify packet IDs match expected values for 1.21.4 configuration phase
	tests := []struct {
		name       string
		packetFunc func() any
		expectedID int32
	}{
		{"FinishConfiguration", func() any { return sb.NewFinishConfiguration() }, 3},
		{"KeepAlive", func() any { return sb.NewKeepAlive() }, 4},
		{"Pong", func() any { return sb.NewPong() }, 5},
		{"ResourcePackReceive", func() any { return sb.NewResourcePackReceive() }, 6},
		{"KeepAlive (clientbound)", func() any { return cb.NewKeepAlive() }, 4},
		{"Ping (clientbound)", func() any { return cb.NewPing() }, 5},
		{"RegistryData", func() any { return cb.NewRegistryData() }, 7},
		{"Disconnect", func() any { return cb.NewDisconnect() }, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := tt.packetFunc()
			type packetIDGetter interface {
				PacketID() int32
			}
			if p, ok := packet.(packetIDGetter); ok {
				if p.PacketID() != tt.expectedID {
					t.Errorf("Expected packet ID %d, got %d", tt.expectedID, p.PacketID())
				}
			} else {
				t.Errorf("Packet does not implement PacketID() method")
			}
		})
	}
}
