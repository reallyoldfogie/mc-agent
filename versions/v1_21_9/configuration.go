package v1_21_9

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/configuration/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.9/configuration/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// configurationHandler implements common.ConfigurationHandler for 1.21.9.
type configurationHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendFinishConfiguration sends a finish configuration packet to transition to play state.
// Uses the generated FinishConfiguration packet struct from mc-protocol-go.
func (c *configurationHandler) SendFinishConfiguration(conn models.PacketWriter) error {
	pkt := sb.NewFinishConfiguration()

	log.Printf("[v1.21.9 Configuration] SendFinishConfiguration")

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "FinishConfiguration", Cause: err}
	}
	return nil
}

// SendKeepAlive sends a keep-alive response packet.
// Uses the generated KeepAlive packet struct from mc-protocol-go.
func (c *configurationHandler) SendKeepAlive(conn models.PacketWriter, id int64) error {
	pkt := sb.NewKeepAlive()
	pkt.KeepAliveId = pk.Long(id)

	log.Printf("[v1.21.9 Configuration] SendKeepAlive: id=%d", id)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "KeepAlive", Cause: err}
	}
	return nil
}

// SendPong sends a pong response packet.
// Uses the generated Pong packet struct from mc-protocol-go.
func (c *configurationHandler) SendPong(conn models.PacketWriter, pingID int32) error {
	pkt := sb.NewPong()
	pkt.Id = pk.Int(pingID)

	log.Printf("[v1.21.9 Configuration] SendPong: pingID=%d", pingID)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "Pong", Cause: err}
	}
	return nil
}

// SendClientInformation sends client settings/information.
// Uses the generated CustomPayload packet struct from mc-protocol-go.
// Note: In 1.21.9, client information is typically sent via a custom payload during configuration.
func (c *configurationHandler) SendClientInformation(conn models.PacketWriter, info models.ClientInfo) error {
	// Client information is version-specific and may need special handling
	// For now, this is a placeholder that would need to be implemented based on
	// the specific protocol requirements for 1.21.9
	log.Printf("[v1.21.9 Configuration] SendClientInformation: locale=%s viewDistance=%d", info.Locale, info.ViewDistance)

	// TODO: Implement client information packet construction
	// This may require custom payload or a specific packet type
	return nil
}

// SendResourcePackResponse sends a resource pack response packet.
// Uses the generated ResourcePackReceive packet struct from mc-protocol-go.
func (c *configurationHandler) SendResourcePackResponse(conn models.PacketWriter, uuid [16]byte, result int32) error {
	pkt := sb.NewResourcePackReceive()
	pkt.Uuid = pk.UUID(uuid)
	pkt.Result = pk.VarInt(result)

	log.Printf("[v1.21.9 Configuration] SendResourcePackResponse: uuid=%x result=%d", uuid, result)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "ResourcePackReceive", Cause: err}
	}
	return nil
}

// ParseRegistryData parses a registry data packet and extracts the registry ID and entries.
// Uses the generated RegistryData packet struct from mc-protocol-go.
func (c *configurationHandler) ParseRegistryData(p pk.Packet) (registryID string, entries map[string]int32, err error) {
	pkt := cb.NewRegistryData()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "RegistryData", Cause: scanErr}
		return
	}

	registryID = string(pkt.Id)
	entries = make(map[string]int32)

	// Extract entries from the array
	entriesData := pkt.Entries.Get()
	if entriesData != nil {
		for i, entry := range entriesData {
			key := string(entry.Key)
			// For now, just use the index as the value since the actual value is NBT data
			// In a full implementation, you'd parse the NBT to extract meaningful data
			entries[key] = int32(i)
		}
	}

	log.Printf("[v1.21.9 Configuration] ParseRegistryData: registryID=%s entriesCount=%d", registryID, len(entries))

	return
}

// ParseKeepAlive parses a keep-alive request packet and extracts the ID.
// Uses the generated KeepAlive packet struct from mc-protocol-go.
func (c *configurationHandler) ParseKeepAlive(p pk.Packet) (id int64, err error) {
	pkt := cb.NewKeepAlive()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "KeepAlive", Cause: scanErr}
		return
	}

	id = int64(pkt.KeepAliveId)

	log.Printf("[v1.21.9 Configuration] ParseKeepAlive: id=%d", id)

	return
}

// ParsePing parses a ping request packet and extracts the ID.
// Uses the generated Ping packet struct from mc-protocol-go.
func (c *configurationHandler) ParsePing(p pk.Packet) (pingID int32, err error) {
	pkt := cb.NewPing()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "Ping", Cause: scanErr}
		return
	}

	pingID = int32(pkt.Id)

	log.Printf("[v1.21.9 Configuration] ParsePing: pingID=%d", pingID)

	return
}

// ParseDisconnect parses a disconnect packet and extracts the reason.
// Uses the generated Disconnect packet struct from mc-protocol-go.
func (c *configurationHandler) ParseDisconnect(p pk.Packet) (reason string, err error) {
	pkt := cb.NewDisconnect()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "Disconnect", Cause: scanErr}
		return
	}

	// The reason is an AnonymousNBT field, extract text from it
	reason = extractNBTString(pkt.Reason)

	log.Printf("[v1.21.9 Configuration] ParseDisconnect: reason=%s", reason)

	return
}
