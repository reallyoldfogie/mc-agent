// Package v1_21_8 provides version-specific packet handling for Minecraft 1.21.8.
package v1_21_8

import (
	"fmt"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.8/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.8/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// chatHandler implements common.ChatHandler for 1.21.8.
type chatHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendChat sends a chat message.
// In 1.21.8, chat messages require timestamps, salt, and signature fields for secure chat.
func (c *chatHandler) SendChat(conn models.PacketWriter, message string) error {
	pkt := sb.NewChatMessage()
	pkt.Message = pk.String(message)
	pkt.Timestamp = pk.Long(time.Now().UnixMilli())
	pkt.Salt = pk.Long(0) // No signature
	pkt.Signature = protocol_models.Option[protocol_models.FixedBuffer256]{Has: false}
	pkt.Offset = pk.VarInt(0)
	pkt.Acknowledged = protocol_models.FixedBuffer3{}
	pkt.Checksum = pk.Byte(0)

	return conn.WritePacket(pkt.Marshal())
}

// SendCommand sends a command (without the leading slash).
func (c *chatHandler) SendCommand(conn models.PacketWriter, command string) error {
	pkt := sb.NewChatCommand()
	pkt.Command = pk.String(command)

	return conn.WritePacket(pkt.Marshal())
}

// ParseSystemChat parses a system chat message.
// Returns the message content and whether it's an overlay (action bar) message.
func (c *chatHandler) ParseSystemChat(p pk.Packet) (message string, overlay bool, err error) {
	pkt := cb.NewSystemChat()
	if err = pkt.Scan(p); err != nil {
		return "", false, common.ErrPacketParse{PacketName: "SystemChat", Cause: err}
	}

	// Extract message content from AnonymousNBT
	// The content is typically a JSON text component encoded as NBT
	message = extractNBTString(pkt.Content)
	overlay = bool(pkt.IsActionBar)

	return message, overlay, nil
}

// ParsePlayerChat parses a player chat message.
// Returns the sender UUID and the plain message content.
func (c *chatHandler) ParsePlayerChat(p pk.Packet) (senderUUID [16]byte, message string, err error) {
	pkt := cb.NewPlayerChat()
	if err = pkt.Scan(p); err != nil {
		return senderUUID, "", common.ErrPacketParse{PacketName: "PlayerChat", Cause: err}
	}

	senderUUID = [16]byte(pkt.SenderUuid)
	message = string(pkt.PlainMessage)

	return senderUUID, message, nil
}

// ParseDisguisedChat parses a disguised chat message.
// Disguised chat is used when the server wants to send a message that appears to be from a player
// but without the cryptographic signature verification.
func (c *chatHandler) ParseDisguisedChat(p pk.Packet) (message string, err error) {
	pkt := cb.NewProfilelessChat()
	if err = pkt.Scan(p); err != nil {
		return "", common.ErrPacketParse{PacketName: "ProfilelessChat", Cause: err}
	}

	// Extract message content from AnonymousNBT
	message = extractNBTString(pkt.Message)

	return message, nil
}

// extractNBTString extracts a string representation from an AnonymousNBT value.
// For string values, it returns the string directly.
// For compound values (like text components), it attempts to extract the "text" field.
// Falls back to fmt.Sprintf for other types.
func extractNBTString(nbt protocol_models.AnonymousNBT) string {
	if nbt.Value == nil {
		return ""
	}

	// If the value is a string, return it directly
	if strVal, ok := nbt.Value.(*protocol_models.NBTString); ok {
		return strVal.Value
	}

	// If it's a compound, try to extract the "text" field (common for text components)
	if compound, ok := nbt.Value.(*protocol_models.NBTCompound); ok {
		for _, tag := range compound.Tags {
			if tag.Name == "text" {
				if strVal, ok := tag.Value.(*protocol_models.NBTString); ok {
					return strVal.Value
				}
			}
		}
	}

	// Fall back to string representation
	return fmt.Sprintf("%v", nbt.Value)
}
