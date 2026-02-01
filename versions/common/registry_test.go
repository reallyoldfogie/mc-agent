package common

import (
	"testing"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// mockVersionHandler implements VersionHandler for testing
type mockVersionHandler struct {
	version         string
	protocolVersion uint
}

func (m *mockVersionHandler) Version() string                      { return m.version }
func (m *mockVersionHandler) ProtocolVersion() uint                { return m.protocolVersion }
func (m *mockVersionHandler) PacketMgr() protocol_models.PacketMgr { return nil }
func (m *mockVersionHandler) Login() LoginHandler                  { return nil }
func (m *mockVersionHandler) Configuration() ConfigurationHandler  { return nil }
func (m *mockVersionHandler) Play() PlayHandler                    { return nil }

// mockLoginHandler implements LoginHandler for testing
type mockLoginHandler struct{}

func (m *mockLoginHandler) SendLoginStart(conn *bot.Conn, username string, uuid [16]byte) error {
	return nil
}
func (m *mockLoginHandler) SendEncryptionResponse(conn *bot.Conn, sharedSecret, verifyToken []byte) error {
	return nil
}
func (m *mockLoginHandler) SendLoginAcknowledged(conn *bot.Conn) error { return nil }
func (m *mockLoginHandler) ParseLoginSuccess(p pk.Packet) (username string, uuid [16]byte, err error) {
	return "", uuid, nil
}
func (m *mockLoginHandler) ParseEncryptionRequest(p pk.Packet) (serverID string, publicKey, verifyToken []byte, err error) {
	return "", nil, nil, nil
}

func TestRegisterAndGetVersionHandler(t *testing.T) {
	// Register a mock handler
	RegisterVersionHandler("test-1.0", func() VersionHandler {
		return &mockVersionHandler{
			version:         "test-1.0",
			protocolVersion: 100,
		}
	})

	// Get the handler
	handler, err := GetVersionHandler("test-1.0")
	if err != nil {
		t.Fatalf("GetVersionHandler failed: %v", err)
	}

	if handler.Version() != "test-1.0" {
		t.Errorf("Expected version 'test-1.0', got '%s'", handler.Version())
	}

	if handler.ProtocolVersion() != 100 {
		t.Errorf("Expected protocol version 100, got %d", handler.ProtocolVersion())
	}
}

func TestGetVersionHandlerNotFound(t *testing.T) {
	_, err := GetVersionHandler("nonexistent-version")
	if err == nil {
		t.Error("Expected error for nonexistent version, got nil")
	}
}

func TestHasVersionHandler(t *testing.T) {
	// Register a mock handler
	RegisterVersionHandler("test-2.0", func() VersionHandler {
		return &mockVersionHandler{
			version:         "test-2.0",
			protocolVersion: 200,
		}
	})

	if !HasVersionHandler("test-2.0") {
		t.Error("Expected HasVersionHandler to return true for registered version")
	}

	if HasVersionHandler("nonexistent") {
		t.Error("Expected HasVersionHandler to return false for unregistered version")
	}
}

func TestSupportedVersions(t *testing.T) {
	// Register a mock handler
	RegisterVersionHandler("test-3.0", func() VersionHandler {
		return &mockVersionHandler{
			version:         "test-3.0",
			protocolVersion: 300,
		}
	})

	versions := SupportedVersions()
	found := false
	for _, v := range versions {
		if v == "test-3.0" {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("Expected 'test-3.0' in SupportedVersions, got %v", versions)
	}
}
