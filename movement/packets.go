package movement

import (
	"log"

	"github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// SendPosition sends a position update packet to the server (position only, no rotation)
// Packet: ServerboundMovePlayerPos
// Fields: X (Double), Y (Double), Z (Double), OnGround (Boolean)
func SendPosition(client *bot.Client, packetMgr protocol_models.PacketMgr, x, y, z float64, onGround bool) error {
	log.Printf("[Movement] Sending ServerboundMovePlayerPos: (%.2f, %.2f, %.2f) onGround=%v", x, y, z, onGround)
	return client.Conn.WritePacket(packet.Marshal(
		packetMgr.GetServerboundPacketID("ServerboundMovePlayerPos"),
		packet.Double(x),
		packet.Double(y),
		packet.Double(z),
		packet.Boolean(onGround),
	))
}

// SendPositionAndRotation sends a combined position and rotation update packet to the server
// Packet: ServerboundMovePlayerPosRot
// Fields: X (Double), Y (Double), Z (Double), Yaw (Float), Pitch (Float), OnGround (Boolean)
func SendPositionAndRotation(client *bot.Client, packetMgr protocol_models.PacketMgr, x, y, z float64, yaw, pitch float32, onGround bool) error {
	log.Printf("[Movement] Sending ServerboundMovePlayerPosRot: (%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f onGround=%v", x, y, z, yaw, pitch, onGround)
	return client.Conn.WritePacket(packet.Marshal(
		packetMgr.GetServerboundPacketID("ServerboundMovePlayerPosRot"),
		packet.Double(x),
		packet.Double(y),
		packet.Double(z),
		packet.Float(yaw),
		packet.Float(pitch),
		packet.Boolean(onGround),
	))
}

// SendRotation sends a rotation update packet to the server (rotation only, no position)
// Packet: ServerboundMovePlayerRot
// Fields: Yaw (Float), Pitch (Float), OnGround (Boolean)
func SendRotation(client *bot.Client, packetMgr protocol_models.PacketMgr, yaw, pitch float32, onGround bool) error {
	return client.Conn.WritePacket(packet.Marshal(
		packetMgr.GetServerboundPacketID("ServerboundMovePlayerRot"),
		packet.Float(yaw),
		packet.Float(pitch),
		packet.Boolean(onGround),
	))
}
