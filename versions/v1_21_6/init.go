package v1_21_6

import (
	"github.com/reallyoldfogie/mc-agent/versions/common"
	v1_21_6 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.6"
)

func init() {
	// Register the 1.21.6 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_6] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() common.VersionHandler {
		packetMgr := v1_21_6.NewPackets()
		println("[versions/v1_21_6] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_6] Created handler")
		return handler
	})
}
