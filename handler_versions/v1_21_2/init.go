package v1_21_2

import (
	"log/slog"

	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v1_21_2 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.2"
)

func init() {
	// Register the 1.21.2 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	common.RegisterVersionHandler(Version, func(logger *slog.Logger) models.VersionHandler {
		packetMgr := v1_21_2.NewPackets()
		logger.Debug("[versions/v1_21_2] creating new handler")
		handler := NewHandler(packetMgr, logger)
		logger.Debug("[versions/v1_21_2] created handler")
		return handler
	})
}
