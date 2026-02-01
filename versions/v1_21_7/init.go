package v1_21_7

import (
	"github.com/reallyoldfogie/mc-agent/versions/common"
	v1_21_7 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.7"
)

func init() {
	// Register the 1.21.7 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	// DEBUG: Print to verify init() is called
	println("[versions/v1_21_7] Registering version handler for", Version)
	common.RegisterVersionHandler(Version, func() common.VersionHandler {
		packetMgr := v1_21_7.NewPackets()
		println("[versions/v1_21_7] Creating new handler")
		handler := NewHandler(packetMgr)
		println("[versions/v1_21_7] Created handler")
		return handler
	})
}
