package v1_21_8

import (
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v1_21_8 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.8"
)

func init() {
	// Register the 1.21.8 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_8] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() models.VersionHandler {
		packetMgr := v1_21_8.NewPackets()
		println("[versions/v1_21_8] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_8] Created handler")
		return handler
	})
}
