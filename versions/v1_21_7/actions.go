// Package v1_21_7 provides version-specific packet handling for Minecraft 1.21.7.
package v1_21_7

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.7/basetypes"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.7/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// actionHandler implements common.ActionHandler for 1.21.7.
type actionHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendUseItem sends a use item packet (e.g., start drawing bow, use item in hand).
// In 1.21.7, this packet includes rotation (yaw/pitch) via Vec2f.
func (a *actionHandler) SendUseItem(conn common.PacketWriter, hand models.Hand, sequence int32, yaw, pitch float32) error {
	pkt := sb.NewUseItem()
	pkt.Hand = pk.VarInt(hand)
	pkt.Sequence = pk.VarInt(sequence)
	pkt.Rotation = basetypes.Vec2f{
		X: pk.Float(yaw),
		Y: pk.Float(pitch),
	}

	log.Printf("[v1.21.7 Action] SendUseItem: hand=%d sequence=%d yaw=%.2f pitch=%.2f", hand, sequence, yaw, pitch)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseItem", Cause: err}
	}
	return nil
}

// SendPlayerAction sends a player action packet (dig, release bow, swap hands, etc.).
func (a *actionHandler) SendPlayerAction(conn common.PacketWriter, status int32, x, y, z int, face int32, sequence int32) error {
	pkt := sb.NewBlockDig()
	pkt.Status = pk.VarInt(status)
	pkt.Location = basetypes.Position{X: int64(x), Y: int64(y), Z: int64(z)}
	pkt.Face = pk.Byte(face)
	pkt.Sequence = pk.VarInt(sequence)

	log.Printf("[v1.21.7 Action] SendPlayerAction: status=%d pos=(%d,%d,%d) face=%d sequence=%d",
		status, x, y, z, face, sequence)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "BlockDig", Cause: err}
	}
	return nil
}

// SendSwing sends an arm swing animation packet.
func (a *actionHandler) SendSwing(conn common.PacketWriter, hand models.Hand) error {
	pkt := sb.NewArmAnimation()
	pkt.Hand = pk.VarInt(hand)

	log.Printf("[v1.21.7 Action] SendSwing: hand=%d", hand)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "ArmAnimation", Cause: err}
	}
	return nil
}
