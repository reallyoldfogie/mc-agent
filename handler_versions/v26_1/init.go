package v26_1

import (
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v26_1 "github.com/reallyoldfogie/mc-protocol-go/data/26.1"
)

func init() {
	// Register the 26.1 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v26_1] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() models.VersionHandler {
		packetMgr := v26_1.NewPackets()
		println("[versions/v26_1] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v26_1] Created handler")
		return handler
	})
}
