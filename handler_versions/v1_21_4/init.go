package v1_21_4

import (
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v1_21_4 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4"
)

func init() {
	// Register the 1.21.4 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_4] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() models.VersionHandler {
		packetMgr := v1_21_4.NewPackets()
		println("[versions/v1_21_4] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_4] Created handler")
		return handler
	})
}
