// Package v1_21_5 provides version-specific packet handling for Minecraft 1.21.5.
package v1_21_5

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/basetypes"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// actionHandler implements common.ActionHandler for 1.21.5.
type actionHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendUseItem sends a use item packet (e.g., start drawing bow, use item in hand).
// In 1.21.5, this packet includes rotation (yaw/pitch) via Vec2f.
func (a *actionHandler) SendUseItem(conn models.PacketWriter, hand models.Hand, sequence int32, yaw, pitch float32) error {
	pkt := sb.NewUseItem()
	pkt.Hand = pk.VarInt(hand)
	pkt.Sequence = pk.VarInt(sequence)
	pkt.Rotation = basetypes.Vec2f{
		X: pk.Float(yaw),
		Y: pk.Float(pitch),
	}

	log.Printf("[v1.21.5 Action] SendUseItem: hand=%d sequence=%d yaw=%.2f pitch=%.2f", hand, sequence, yaw, pitch)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "UseItem", Cause: err}
	}
	return nil
}

// SendPlayerAction sends a player action packet (dig, release bow, swap hands, etc.).
// Status IDs:
//   - 0: Start digging
//   - 1: Abort digging
//   - 2: Finish digging
//   - 3: Drop item stack
//   - 4: Drop single item
//   - 5: Release use item (release bow, stop eating, etc.)
//   - 6: Swap item in hands
func (a *actionHandler) SendPlayerAction(conn models.PacketWriter, status int32, x, y, z int, face int32, sequence int32) error {
	pkt := sb.NewBlockDig()
	pkt.Status = pk.VarInt(status)
	pkt.Location = basetypes.Position{X: int64(x), Y: int64(y), Z: int64(z)}
	pkt.Face = pk.Byte(face)
	pkt.Sequence = pk.VarInt(sequence)

	log.Printf("[v1.21.5 Action] SendPlayerAction: status=%d pos=(%d,%d,%d) face=%d sequence=%d",
		status, x, y, z, face, sequence)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "BlockDig", Cause: err}
	}
	return nil
}

// SendSwing sends an arm swing animation packet.
// hand: 0=main hand, 1=offhand
func (a *actionHandler) SendSwing(conn models.PacketWriter, hand models.Hand) error {
	pkt := sb.NewArmAnimation()
	pkt.Hand = pk.VarInt(hand)

	log.Printf("[v1.21.5 Action] SendSwing: hand=%d", hand)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "ArmAnimation", Cause: err}
	}
	return nil
}
