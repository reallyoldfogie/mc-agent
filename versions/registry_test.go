package versions

import (
	"testing"

	"github.com/reallyoldfogie/mc-agent/versions/common"
)

func TestV1215HandlerRegistered(t *testing.T) {
	// The import of versions/v1_21_5 in registry.go should have triggered init()
	// which registers the handler
	handler, err := common.GetVersionHandler("1.21.5")
	if err != nil {
		t.Fatalf("Failed to get 1.21.5 handler: %v", err)
	}

	if handler.Version() != "1.21.5" {
		t.Errorf("Expected version '1.21.5', got '%s'", handler.Version())
	}

	if handler.ProtocolVersion() != 770 {
		t.Errorf("Expected protocol version 770, got %d", handler.ProtocolVersion())
	}

	// Verify PacketMgr is set
	if handler.PacketMgr() == nil {
		t.Error("Expected PacketMgr to be non-nil")
	}

	// Verify sub-handlers are accessible
	if handler.Login() == nil {
		t.Error("Expected Login() to return non-nil handler")
	}
	if handler.Configuration() == nil {
		t.Error("Expected Configuration() to return non-nil handler")
	}
	if handler.Play() == nil {
		t.Error("Expected Play() to return non-nil handler")
	}

	// Verify play sub-handlers
	play := handler.Play()
	if play.Movement() == nil {
		t.Error("Expected Movement() to return non-nil handler")
	}
	if play.Entities() == nil {
		t.Error("Expected Entities() to return non-nil handler")
	}
	if play.Containers() == nil {
		t.Error("Expected Containers() to return non-nil handler")
	}
	if play.Chat() == nil {
		t.Error("Expected Chat() to return non-nil handler")
	}
	if play.World() == nil {
		t.Error("Expected World() to return non-nil handler")
	}
}
