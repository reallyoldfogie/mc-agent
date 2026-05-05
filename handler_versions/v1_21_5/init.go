package v1_21_5

import (
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v1_21_5 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5"
)

func init() {
	// Register the 1.21.5 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_5] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() models.VersionHandler {
		packetMgr := v1_21_5.NewPackets()
		println("[versions/v1_21_5] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_5] Created handler")
		return handler
	})
}
