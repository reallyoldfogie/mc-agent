package v1_21_2

import (
	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"log/slog"
)

// lifecycleHandler implements models.LifecycleHandler for 1.21.2.
//
// Note: 1.21.2 predates ServerboundPlayerLoaded (introduced in 1.21.2) and
// also predates the 60-tick post-join gate in PlayerEntity.isLoaded. There is
// nothing for the client to send, so SendPlayerLoaded is a no-op.
type lifecycleHandler struct {
	packetMgr protocol_models.PacketMgr
	logger    *slog.Logger
}

// SendPlayerLoaded is a no-op on 1.21.2 (packet does not exist in this
// protocol version and the server-side gate it removes does not exist).
func (l *lifecycleHandler) SendPlayerLoaded(conn models.PacketWriter) error {
	return nil
}
