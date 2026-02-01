package v1_21_3

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/login/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/login/serverbound"
)

func TestLoginHandler_SendLoginStart_PacketStructure(t *testing.T) {
	// Test that the generated LoginStart packet structure matches expectations
	pkt := sb.NewLoginStart()
	pkt.Username = pk.String("TestPlayer")
	testUUID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	pkt.PlayerUUID = pk.UUID(testUUID)

	// Verify packet ID is correct for 1.21.4 login phase
	if pkt.PacketID() != 0 {
		t.Errorf("Expected packet ID 0, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if string(pkt.Username) != "TestPlayer" {
		t.Errorf("Expected Username TestPlayer, got %s", pkt.Username)
	}
	if [16]byte(pkt.PlayerUUID) != testUUID {
		t.Errorf("Expected UUID %x, got %x", testUUID, pkt.PlayerUUID)
	}
}

func TestLoginHandler_SendEncryptionResponse_PacketStructure(t *testing.T) {
	// Test that the generated EncryptionBegin packet structure matches expectations
	pkt := sb.NewEncryptionBegin()
	sharedSecret := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	verifyToken := []byte{0x0A, 0x0B, 0x0C, 0x0D}
	pkt.SharedSecret = pk.ByteArray(sharedSecret)
	pkt.VerifyToken = pk.ByteArray(verifyToken)

	// Verify packet ID is correct for 1.21.4 login phase
	if pkt.PacketID() != 1 {
		t.Errorf("Expected packet ID 1, got %d", pkt.PacketID())
	}

	// Verify fields are set correctly
	if len(pkt.SharedSecret) != len(sharedSecret) {
		t.Errorf("Expected SharedSecret length %d, got %d", len(sharedSecret), len(pkt.SharedSecret))
	}
	if len(pkt.VerifyToken) != len(verifyToken) {
		t.Errorf("Expected VerifyToken length %d, got %d", len(verifyToken), len(pkt.VerifyToken))
	}
}

func TestLoginHandler_SendLoginAcknowledged_PacketStructure(t *testing.T) {
	// Test that the generated LoginAcknowledged packet structure matches expectations
	pkt := sb.NewLoginAcknowledged()

	// Verify packet ID is correct for 1.21.4 login phase
	if pkt.PacketID() != 3 {
		t.Errorf("Expected packet ID 3, got %d", pkt.PacketID())
	}

	// This packet has no fields, so just verify it marshals without error
	marshaled := pkt.Marshal()
	if marshaled.ID != 3 {
		t.Errorf("Expected marshaled packet ID 3, got %d", marshaled.ID)
	}
}

func TestLoginHandler_ParseLoginSuccess(t *testing.T) {
	// Create a mock clientbound Success packet
	pkt := cb.NewSuccess()
	testUUID := [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	pkt.Uuid = pk.UUID(testUUID)
	pkt.Username = pk.String("TestPlayer")
	// Initialize the Properties array with an empty slice
	emptyProperties := make([]cb.SuccessPropertiesArrayType, 0)
	pkt.Properties.Set(&emptyProperties)

	// Marshal and then parse
	handler := &loginHandler{}
	marshaled := pkt.Marshal()

	username, uuid, err := handler.ParseLoginSuccess(marshaled)
	if err != nil {
		t.Fatalf("ParseLoginSuccess failed: %v", err)
	}

	if username != "TestPlayer" {
		t.Errorf("Expected username TestPlayer, got %s", username)
	}
	if uuid != testUUID {
		t.Errorf("Expected UUID %x, got %x", testUUID, uuid)
	}
}

func TestLoginHandler_ParseEncryptionRequest(t *testing.T) {
	// Create a mock clientbound EncryptionBegin packet
	pkt := cb.NewEncryptionBegin()
	pkt.ServerId = pk.String("test-server-id")
	publicKey := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
	verifyToken := []byte{0x0A, 0x0B, 0x0C, 0x0D}
	pkt.PublicKey = pk.ByteArray(publicKey)
	pkt.VerifyToken = pk.ByteArray(verifyToken)
	pkt.ShouldAuthenticate = pk.Boolean(true)

	// Marshal and then parse
	handler := &loginHandler{}
	marshaled := pkt.Marshal()

	serverID, parsedPublicKey, parsedVerifyToken, err := handler.ParseEncryptionRequest(marshaled)
	if err != nil {
		t.Fatalf("ParseEncryptionRequest failed: %v", err)
	}

	if serverID != "test-server-id" {
		t.Errorf("Expected serverID test-server-id, got %s", serverID)
	}
	if len(parsedPublicKey) != len(publicKey) {
		t.Errorf("Expected publicKey length %d, got %d", len(publicKey), len(parsedPublicKey))
	}
	if len(parsedVerifyToken) != len(verifyToken) {
		t.Errorf("Expected verifyToken length %d, got %d", len(verifyToken), len(parsedVerifyToken))
	}
	// Verify byte-for-byte equality
	for i, b := range publicKey {
		if parsedPublicKey[i] != b {
			t.Errorf("PublicKey byte mismatch at index %d: expected %x, got %x", i, b, parsedPublicKey[i])
		}
	}
	for i, b := range verifyToken {
		if parsedVerifyToken[i] != b {
			t.Errorf("VerifyToken byte mismatch at index %d: expected %x, got %x", i, b, parsedVerifyToken[i])
		}
	}
}

func TestLoginHandler_PacketIDs(t *testing.T) {
	// Verify packet IDs match expected values for 1.21.4 login phase
	tests := []struct {
		name       string
		packetFunc func() any
		expectedID int32
	}{
		{"LoginStart", func() any { return sb.NewLoginStart() }, 0},
		{"EncryptionBegin", func() any { return sb.NewEncryptionBegin() }, 1},
		{"LoginAcknowledged", func() any { return sb.NewLoginAcknowledged() }, 3},
		{"Success", func() any { return cb.NewSuccess() }, 2},
		{"EncryptionBegin (clientbound)", func() any { return cb.NewEncryptionBegin() }, 1},
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
