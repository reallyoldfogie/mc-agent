package v1_21_9

import (
	"log/slog"

	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v1_21_9 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9"
)

func init() {
	// Register the 1.21.9 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	common.RegisterVersionHandler(Version, func(logger *slog.Logger) models.VersionHandler {
		packetMgr := v1_21_9.NewPackets()
		logger.Debug("[versions/v1_21_9] creating new handler")
		handler := NewHandler(packetMgr, logger)
		logger.Debug("[versions/v1_21_9] created handler")
		return handler
	})
}
