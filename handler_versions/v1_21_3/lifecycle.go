package v1_21_3

import (
	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// lifecycleHandler implements models.LifecycleHandler for 1.21.3.
//
// Note: 1.21.3 predates ServerboundPlayerLoaded (introduced in 1.21.3) and
// also predates the 60-tick post-join gate in PlayerEntity.isLoaded. There is
// nothing for the client to send, so SendPlayerLoaded is a no-op.
type lifecycleHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendPlayerLoaded is a no-op on 1.21.3 (packet does not exist in this
// protocol version and the server-side gate it removes does not exist).
func (l *lifecycleHandler) SendPlayerLoaded(conn models.PacketWriter) error {
	return nil
}
