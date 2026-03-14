package v1_21_2

import (
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	v1_21_2 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.2"
)

func init() {
	// Register the 1.21.2 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_2] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() models.VersionHandler {
		packetMgr := v1_21_2.NewPackets()
		println("[versions/v1_21_2] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_2] Created handler")
		return handler
	})
}
