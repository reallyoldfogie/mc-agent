package v1_21_5

import (
	"log/slog"

	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	v1_21_5 "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5"
)

func init() {
	// Register the 1.21.5 version handler with the factory.
	// The constructor creates a new Handler with the appropriate PacketMgr.
	common.RegisterVersionHandler(Version, func(logger *slog.Logger) models.VersionHandler {
		packetMgr := v1_21_5.NewPackets()
		logger.Debug("[versions/v1_21_5] creating new handler")
		handler := NewHandler(packetMgr, logger)
		logger.Debug("[versions/v1_21_5] created handler")
		return handler
	})
}
