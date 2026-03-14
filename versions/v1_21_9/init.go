package v1_21_9

import (
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	v1_21_9 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9"
)

func init() {
	// Register the 1.21.9 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_9] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() models.VersionHandler {
		packetMgr := v1_21_9.NewPackets()
		println("[versions/v1_21_9] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_9] Created handler")
		return handler
	})
}
