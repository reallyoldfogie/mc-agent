package v1_21_4

import (
	"log"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/versions/common"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/play/clientbound"
	sb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.4/play/serverbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// movementHandler implements common.MovementHandler for 1.21.4.
type movementHandler struct {
	packetMgr protocol_models.PacketMgr
}

// SendPosition sends a position update packet (position only, no rotation).
// Uses the generated Position packet struct from mc-protocol-go.
func (m *movementHandler) SendPosition(conn common.PacketWriter, x, y, z float64, onGround bool) error {
	pkt := sb.NewPosition()
	pkt.X = pk.Double(x)
	pkt.Y = pk.Double(y)
	pkt.Z = pk.Double(z)
	pkt.Flags.SetOnGround(onGround)

	log.Printf("[v1.21.4 Movement] SendPosition: (%.2f, %.2f, %.2f) onGround=%v", x, y, z, onGround)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "Position", Cause: err}
	}
	return nil
}

// SendPositionAndRotation sends a combined position and rotation packet.
// Uses the generated PositionLook packet struct from mc-protocol-go.
func (m *movementHandler) SendPositionAndRotation(conn common.PacketWriter, x, y, z float64, yaw, pitch float32, onGround bool) error {
	pkt := sb.NewPositionLook()
	pkt.X = pk.Double(x)
	pkt.Y = pk.Double(y)
	pkt.Z = pk.Double(z)
	pkt.Yaw = pk.Float(yaw)
	pkt.Pitch = pk.Float(pitch)
	pkt.Flags.SetOnGround(onGround)

	log.Printf("[v1.21.4 Movement] SendPositionAndRotation: (%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f onGround=%v",
		x, y, z, yaw, pitch, onGround)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "PositionLook", Cause: err}
	}
	return nil
}

// SendRotation sends a rotation update packet (rotation only, no position).
// Uses the generated Look packet struct from mc-protocol-go.
func (m *movementHandler) SendRotation(conn common.PacketWriter, yaw, pitch float32, onGround bool) error {
	pkt := sb.NewLook()
	pkt.Yaw = pk.Float(yaw)
	pkt.Pitch = pk.Float(pitch)
	pkt.Flags.SetOnGround(onGround)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "Look", Cause: err}
	}
	return nil
}

// SendPlayerCommand sends a player command packet (sprint, sneak, etc.).
// Uses the generated EntityAction packet struct from mc-protocol-go.
//
// Action IDs:
//   - 0: Start sneaking
//   - 1: Stop sneaking
//   - 2: Leave bed
//   - 3: Start sprinting
//   - 4: Stop sprinting
//   - 5: Start jump with horse
//   - 6: Stop jump with horse
//   - 7: Open horse inventory
//   - 8: Start flying with elytra
func (m *movementHandler) SendPlayerCommand(conn common.PacketWriter, entityID, actionID int32) error {
	pkt := sb.NewEntityAction()
	pkt.EntityId = pk.VarInt(entityID)
	pkt.ActionId = pk.VarInt(actionID)
	pkt.JumpBoost = 0 // Always 0 for sprint/sneak

	log.Printf("[v1.21.4 Movement] SendPlayerCommand: entityID=%d actionID=%d", entityID, actionID)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "EntityAction", Cause: err}
	}
	return nil
}

// SendTeleportConfirm confirms a server-requested teleport.
// Uses the generated TeleportConfirm packet struct from mc-protocol-go.
func (m *movementHandler) SendTeleportConfirm(conn common.PacketWriter, teleportID int32) error {
	pkt := sb.NewTeleportConfirm()
	pkt.TeleportId = pk.VarInt(teleportID)

	log.Printf("[v1.21.4 Movement] SendTeleportConfirm: teleportID=%d", teleportID)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "TeleportConfirm", Cause: err}
	}
	return nil
}

// SendPlayerAbilities sends player abilities (flying, etc.).
// Uses the generated Abilities packet struct from mc-protocol-go.
//
// Flags:
//   - 0x02: Is flying
func (m *movementHandler) SendPlayerAbilities(conn common.PacketWriter, flags byte) error {
	pkt := sb.NewAbilities()
	pkt.Flags = pk.Byte(flags)

	log.Printf("[v1.21.4 Movement] SendPlayerAbilities: flags=%d", flags)

	if err := conn.WritePacket(pkt.Marshal()); err != nil {
		return common.ErrPacketSend{PacketName: "Abilities", Cause: err}
	}
	return nil
}

// ParsePlayerPosition parses a clientbound player position packet.
// Uses the generated Position packet struct from mc-protocol-go.
func (m *movementHandler) ParsePlayerPosition(p pk.Packet) (teleportID int32, x, y, z float64, yaw, pitch float32, flags int32, err error) {
	pkt := cb.NewPosition()
	if scanErr := pkt.Scan(p); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "Position", Cause: scanErr}
		return
	}

	teleportID = int32(pkt.TeleportId)
	x = float64(pkt.X)
	y = float64(pkt.Y)
	z = float64(pkt.Z)
	yaw = float32(pkt.Yaw)
	pitch = float32(pkt.Pitch)
	flags = int32(pkt.Flags.UInt32)

	return
}

// ParseServerboundPos parses a serverbound position packet.
func (m *movementHandler) ParseServerboundPos(p pk.Packet) (x, y, z float64, onGround bool, err error) {
	var pktX, pktY, pktZ pk.Double
	var pktOnGround pk.Boolean
	if scanErr := p.Scan(&pktX, &pktY, &pktZ, &pktOnGround); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "ServerboundPos", Cause: scanErr}
		return
	}
	x = float64(pktX)
	y = float64(pktY)
	z = float64(pktZ)
	onGround = bool(pktOnGround)
	return
}

// ParseServerboundPosRot parses a serverbound position+rotation packet.
func (m *movementHandler) ParseServerboundPosRot(p pk.Packet) (x, y, z float64, yaw, pitch float32, onGround bool, err error) {
	var pktX, pktY, pktZ pk.Double
	var pktYaw, pktPitch pk.Float
	var pktOnGround pk.Boolean
	if scanErr := p.Scan(&pktX, &pktY, &pktZ, &pktYaw, &pktPitch, &pktOnGround); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "ServerboundPosRot", Cause: scanErr}
		return
	}
	x = float64(pktX)
	y = float64(pktY)
	z = float64(pktZ)
	yaw = float32(pktYaw)
	pitch = float32(pktPitch)
	onGround = bool(pktOnGround)
	return
}

// ParseServerboundRot parses a serverbound rotation packet.
func (m *movementHandler) ParseServerboundRot(p pk.Packet) (yaw, pitch float32, onGround bool, err error) {
	var pktYaw, pktPitch pk.Float
	var pktOnGround pk.Boolean
	if scanErr := p.Scan(&pktYaw, &pktPitch, &pktOnGround); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "ServerboundRot", Cause: scanErr}
		return
	}
	yaw = float32(pktYaw)
	pitch = float32(pktPitch)
	onGround = bool(pktOnGround)
	return
}

// ParseServerboundStatus parses a serverbound status-only packet.
func (m *movementHandler) ParseServerboundStatus(p pk.Packet) (onGround bool, err error) {
	var pktOnGround pk.Boolean
	if scanErr := p.Scan(&pktOnGround); scanErr != nil {
		err = common.ErrPacketParse{PacketName: "ServerboundStatus", Cause: scanErr}
		return
	}
	onGround = bool(pktOnGround)
	return
}

// Convenience methods for common actions

// SendStartSneaking sends a command to start sneaking.
func (m *movementHandler) SendStartSneaking(conn common.PacketWriter, entityID int32) error {
	return m.SendPlayerCommand(conn, entityID, common.ActionStartSneaking)
}

// SendStopSneaking sends a command to stop sneaking.
func (m *movementHandler) SendStopSneaking(conn common.PacketWriter, entityID int32) error {
	return m.SendPlayerCommand(conn, entityID, common.ActionStopSneaking)
}

// SendStartSprinting sends a command to start sprinting.
func (m *movementHandler) SendStartSprinting(conn common.PacketWriter, entityID int32) error {
	return m.SendPlayerCommand(conn, entityID, common.ActionStartSprinting)
}

// SendStopSprinting sends a command to stop sprinting.
func (m *movementHandler) SendStopSprinting(conn common.PacketWriter, entityID int32) error {
	return m.SendPlayerCommand(conn, entityID, common.ActionStopSprinting)
}
