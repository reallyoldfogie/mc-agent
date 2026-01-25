package agent

import (
	"encoding/json"
	"log"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// packetLogger returns a generic packet handler that writes JSON logs to a.logw.
func (a *agent) packetLogger() bot.PacketHandler {
	ver := a.cfg.Version
	proto := a.cfg.ProtocolVersion
	return bot.PacketHandler{
		Priority: 0,
		F: func(p pk.Packet) error {
			if a.logw != nil {
				name := ""
				if a.packetMgr != nil {
					name = a.packetMgr.ClientboundToString(protocol_models.ClientboundPacketID(p.ID))
				}
				pl := protocol_models.PacketLog{
					ID:              p.ID,
					Data:            make([]byte, len(p.Data)),
					Name:            name,
					Timestamp:       time.Now(),
					Version:         ver,
					ProtocolVersion: proto,
				}
				copy(pl.Data, p.Data)
				if err := json.NewEncoder(a.logw).Encode(pl); err != nil {
					log.Printf("[ERROR] failed to write packet log: %v", err)
					return err
				}
			}
			return nil
		},
	}
}
