package v1_21_9

import (
	"log"

	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// lifecycleHandler implements models.LifecycleHandler for 1.21.9.
type lifecycleHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendPlayerLoaded sends ServerboundPlayerLoaded so the server flips
// PlayerEntity.loaded to true and stops dropping interact/vehicle/etc.
// packets during the 60-tick post-join grace window.
func (l *lifecycleHandler) SendPlayerLoaded(conn models.PacketWriter) error {
	pkt := sb.NewPlayerLoaded()

	log.Printf("[v1.21.9 Lifecycle] SendPlayerLoaded")

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "PlayerLoaded", Cause: err}
	}
	return nil
}
