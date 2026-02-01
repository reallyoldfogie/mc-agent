package v1_21_3

import (
	"github.com/reallyoldfogie/mc-agent/versions/common"
	v1_21_3 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.3"
)

func init() {
	// Register the 1.21.3 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_3] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() common.VersionHandler {
		packetMgr := v1_21_3.NewPackets()
		println("[versions/v1_21_3] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_3] Created handler")
		return handler
	})
}
