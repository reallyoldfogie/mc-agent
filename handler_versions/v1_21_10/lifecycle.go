package v1_21_10

import (
	"fmt"
	"github.com/reallyoldfogie/mc-agent/utils"
	"log/slog"

	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// lifecycleHandler implements models.LifecycleHandler for 1.21.10.
type lifecycleHandler struct {
	packetMgr protocol_models.PacketMgr
	logger    *slog.Logger
}

// SendPlayerLoaded sends ServerboundPlayerLoaded so the server flips
// PlayerEntity.loaded to true and stops dropping interact/vehicle/etc.
// packets during the 60-tick post-join grace window.
func (l *lifecycleHandler) SendPlayerLoaded(conn models.PacketWriter) error {
	pkt := sb.NewPlayerLoaded()

	utils.SafeLogger(l.logger).Debug(fmt.Sprintf("[v1.21.10 Lifecycle] SendPlayerLoaded"))

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "PlayerLoaded", Cause: err}
	}
	return nil
}
