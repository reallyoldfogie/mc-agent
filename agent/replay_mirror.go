package agent

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"

	pk "github.com/Tnze/go-mc/net/packet"
	gouuid "github.com/google/uuid"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/reallyoldfogie/mc-replay-go/mcpr/recorder"

	v1215_basetypes "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/basetypes"
	v1215_clientbound "github.com/reallyoldfogie/mc-protocol-go/data/1.21.5/play/clientbound"
)

// replayMovementMirror converts select serverbound packets (movement) into
// synthetic clientbound packets for replay visibility of the local player.
type replayMovementMirror struct {
	rec *recorder.Recorder
	pm  protocol_models.PacketMgr

	mu               sync.Mutex
	entityID         int32
	entityType       int32
	name             string
	uuid             [16]byte
	hasUUID          bool
	serverName       string
	serverUUID       [16]byte
	serverPI         []byte
	serverPIHasProps bool
	properties       []profileProperty
	fetchedProps     bool
	spawned          bool
	skinProvider     SkinProvider

	lastX, lastY, lastZ float64
	lastYaw, lastPitch  float32
	onGround            bool
	playerInfoSent      bool

	sbidPos        int32 // cached serverbound ids
	sbidPosRot     int32
	sbidRot        int32
	sbidStatus     int32
	cbidTeleport   int32 // cached clientbound id
	cbidAddEnt     int32
	cbidPlayerInfo int32
}

// NewReplayMovementMirror constructs a movement mirror if both recorder and
// packet manager are provided. It returns nil when either dependency is nil.
func NewReplayMovementMirror(rec *recorder.Recorder, pm protocol_models.PacketMgr, sp SkinProvider) MovementMirror {
	if rec == nil || pm == nil {
		return nil
	}
	return &replayMovementMirror{
		rec:            rec,
		pm:             pm,
		sbidPos:        int32(pm.GetServerboundPacketID("ServerboundMovePlayerPos")),
		sbidPosRot:     int32(pm.GetServerboundPacketID("ServerboundMovePlayerPosRot")),
		sbidRot:        int32(pm.GetServerboundPacketID("ServerboundMovePlayerRot")),
		sbidStatus:     int32(pm.GetServerboundPacketID("ServerboundMovePlayerStatusOnly")),
		cbidTeleport:   int32(pm.GetClientboundPacketID("ClientboundTeleportEntity")),
		cbidAddEnt:     int32(pm.GetClientboundPacketID("ClientboundAddEntity")),
		cbidPlayerInfo: int32(pm.GetClientboundPacketID("ClientboundPlayerInfo")),
		skinProvider:   sp,
	}
}

func (m *replayMovementMirror) SetEntityMeta(entityID int32, name string, uuid [16]byte) {
	m.mu.Lock()
	m.entityID = entityID
	m.name = name
	if uuid == ([16]byte{}) && name != "" {
		uuid = deriveOfflineUUID(name)
	}
	m.uuid = uuid
	m.hasUUID = uuid != ([16]byte{})
	if m.hasUUID {
		m.playerInfoSent = false // allow re-emit when we learn a real UUID
	}
	// Try to emit immediately if we already have identity.
	m.ensurePlayerInfoLocked()
	m.mu.Unlock()
}

func (m *replayMovementMirror) SetEntityType(entityType int32) {
	m.mu.Lock()
	m.entityType = entityType
	if !m.spawned && m.entityID != 0 && m.entityType != 0 {
		// Spawn our entity now that we know the type.
		m.spawned = true
		m.writeAddEntity(m.lastX, m.lastY, m.lastZ, m.lastYaw, m.lastPitch)
	}
	m.mu.Unlock()
}

func (m *replayMovementMirror) HandleServerbound(p pk.Packet) {
	if m == nil || m.rec == nil {
		return
	}

	switch p.ID {
	case m.sbidPos:
		m.handlePos(p)
	case m.sbidPosRot:
		m.handlePosRot(p)
	case m.sbidRot:
		m.handleRot(p)
	case m.sbidStatus:
		m.handleStatus(p)
	}
}

// HandlePlayerInfo captures the server-provided UUID/name to keep mirror in sync.
// We parse only enough of the packet to read the add_player entries.
func (m *replayMovementMirror) HandlePlayerInfo(p pk.Packet) {
	// Get the generic packet interface
	hpPacket, err := m.pm.GetClientboundPacketByID(protocol_models.ClientboundPacketID(p.ID))
	if err != nil {
		return
	}

	if err := hpPacket.Scan(p); err != nil {
		return
	}

	// Use version-specific type assertion to access PlayerInfo fields
	// Pattern 3: Version-specific access with full type safety
	playerInfoPkt, ok := hpPacket.(*v1215_clientbound.PlayerInfo)
	if !ok {
		// If type assertion fails, fallback to generic field access
		// This handles version compatibility if needed
		return
	}

	// Use direct getter methods for full type safety
	action := playerInfoPkt.GetAction()
	rawAction := byte(action.UnsignedByte)

	// Get the player data array with full type safety
	data := playerInfoPkt.GetData()

	// Iterate through player entries
	var sawRemove bool
	for _, entry := range *data.Get() {
		var targetUUID [16]byte
		copy(targetUUID[:], entry.Uuid[:])

		// React to removals of our bot: immediately clear sent flag so we can re-emit our add.
		if rawAction == 0x80 && targetUUID == m.uuid {
			m.mu.Lock()
			m.playerInfoSent = false
			m.mu.Unlock()
			sawRemove = true
			continue
		}

		if !action.AddPlayer() {
			continue
		}

		// Add this player UUID to the recording metadata
		if m.rec != nil {
			m.rec.AddPlayer(uuidHex(targetUUID))
		}

		// Extract player name from GameProfile
		// Player field is a switch field that contains GameProfile when add_player is true
		if entry.Player != nil {
			if gameProfile, ok := entry.Player.(*v1215_basetypes.GameProfile); ok {
				props := collectProperties(gameProfile)
				m.mu.Lock()
				m.serverName = string(gameProfile.Name)
				// If the server's UUID differs from our canonical UUID, keep only properties.
				if targetUUID == m.uuid {
					if len(props) > 0 {
						m.properties = props
						m.serverPIHasProps = true
						m.playerInfoSent = false
					} else {
						m.serverPIHasProps = false
					}

					// Update entity metadata with the new info
					m.SetEntityMeta(m.entityID, m.serverName, targetUUID)

					// Keep a copy of the raw add_player packet to re-emit safely
					m.serverPI = make([]byte, len(p.Data))
					copy(m.serverPI, p.Data)
				} else {
					// Mismatch UUID: still adopt properties but do not reuse raw packet.
					if len(props) > 0 {
						m.properties = props
						m.playerInfoSent = false
					}
				}
				m.mu.Unlock()

				// After learning UUID, emit our own PlayerInfo if not sent yet
				m.ensurePlayerInfo()

				// We found our player info, no need to continue
				return
			}
		}
	}
	if sawRemove {
		// Re-emit our own entry to keep the bot present in the replay.
		m.ensurePlayerInfo()
	}
}

func (m *replayMovementMirror) handlePos(p pk.Packet) {
	var x, y, z pk.Double
	var onGround pk.Boolean
	if err := p.Scan(&x, &y, &z, &onGround); err != nil {
		return
	}
	m.emitTeleport(float64(x), float64(y), float64(z), m.lastYaw, m.lastPitch, bool(onGround))
}

func (m *replayMovementMirror) handlePosRot(p pk.Packet) {
	var x, y, z pk.Double
	var yaw, pitch pk.Float
	var onGround pk.Boolean
	if err := p.Scan(&x, &y, &z, &yaw, &pitch, &onGround); err != nil {
		return
	}
	m.emitTeleport(float64(x), float64(y), float64(z), float32(yaw), float32(pitch), bool(onGround))
}

func (m *replayMovementMirror) handleRot(p pk.Packet) {
	var yaw, pitch pk.Float
	var onGround pk.Boolean
	if err := p.Scan(&yaw, &pitch, &onGround); err != nil {
		return
	}
	m.emitTeleport(m.lastX, m.lastY, m.lastZ, float32(yaw), float32(pitch), bool(onGround))
}

func (m *replayMovementMirror) handleStatus(p pk.Packet) {
	var onGround pk.Boolean
	if err := p.Scan(&onGround); err != nil {
		return
	}
	m.emitTeleport(m.lastX, m.lastY, m.lastZ, m.lastYaw, m.lastPitch, bool(onGround))
}

func (m *replayMovementMirror) emitTeleport(x, y, z float64, yaw, pitch float32, onGround bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entityID == 0 {
		return
	}

	// Ensure tab list entry exists once we know our id/name/uuid.
	m.ensurePlayerInfoLocked()

	if !m.spawned && m.entityType != 0 {
		log.Printf("[ReplayMirror] Spawning bot entity at (%.2f, %.2f, %.2f) entityID=%d", x, y, z, m.entityID)
		m.spawned = true
		m.writeAddEntity(x, y, z, yaw, pitch)
	}

	m.lastX, m.lastY, m.lastZ = x, y, z
	m.lastYaw, m.lastPitch = yaw, pitch
	m.onGround = onGround

	teleport := pk.Marshal(
		m.cbidTeleport,
		pk.VarInt(m.entityID),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.Byte(angleToByte(yaw)),
		pk.Byte(angleToByte(pitch)),
		pk.Boolean(onGround),
	)
	payload := make([]byte, len(teleport.Data))
	copy(payload, teleport.Data)
	_ = m.rec.RecordNow(int32(teleport.ID), payload)
}

func (m *replayMovementMirror) writeAddEntity(x, y, z float64, yaw, pitch float32) {
	add := pk.Marshal(
		m.cbidAddEnt,
		pk.VarInt(m.entityID),
		pk.UUID(m.uuid),
		pk.VarInt(m.entityType),
		pk.Double(x),
		pk.Double(y),
		pk.Double(z),
		pk.Angle(angleToByte(pitch)),
		pk.Angle(angleToByte(yaw)),
		pk.Angle(angleToByte(yaw)),
		pk.VarInt(0), // data
		pk.Short(0),  // velX
		pk.Short(0),  // velY
		pk.Short(0),  // velZ
	)
	payload := make([]byte, len(add.Data))
	copy(payload, add.Data)
	_ = m.rec.RecordNow(int32(add.ID), payload)
}

func angleToByte(f float32) byte {
	// Convert degrees to protocol angle byte
	return byte(int(math.Round(float64(f*256/360))) & 0xFF)
}

// emit minimal PlayerInfo (aka PlayerInfoUpdate) so the bot appears in tab and has a name.
func (m *replayMovementMirror) ensurePlayerInfo() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePlayerInfoLocked()
}

func (m *replayMovementMirror) ensurePlayerInfoLocked() {
	log.Printf("ensurePlayerInfoLocked: entityID=%d name=%q sent=%v", m.entityID, m.name, m.playerInfoSent)
	if m.playerInfoSent || m.rec == nil {
		return
	}
	// Try to fetch textures if we don't already have properties.
	if len(m.properties) == 0 && m.skinProvider != nil {
		if props := m.skinProvider.Get(m.uuid, m.name); len(props) > 0 {
			m.properties = props
			m.fetchedProps = true
		}
	}
	// If still empty, try local fallback capture.
	if len(m.properties) == 0 {
		m.ensurePropertiesFromLocalReplayLocked()
	}
	// If we captured a server add_player packet that already contains properties, re-emit it directly.
	if len(m.serverPI) > 0 && m.serverPIHasProps {
		payload := make([]byte, len(m.serverPI))
		copy(payload, m.serverPI)
		_ = m.rec.RecordNow(int32(m.cbidPlayerInfo), payload)
		m.playerInfoSent = true
		return
	}

	// Otherwise fall back to auth metadata only if needed.
	uuid := m.uuid
	name := m.name
	if uuid == ([16]byte{}) && name != "" {
		uuid = deriveOfflineUUID(name)
		m.uuid = uuid
	}
	if uuid == ([16]byte{}) || name == "" {
		return
	}

	// If we still have no properties (and none came from the server), wait until we do.
	if len(m.properties) == 0 {
		log.Printf("ensurePlayerInfoLocked: emitting without textures for %s (uuid=%s)", name, uuidHex(uuid))
	}

	// Manually encode minimal PlayerInfoUpdate packet (1.20.2+ format).
	var buf bytes.Buffer
	buf.Write(encodeVarInt(0b00011101)) // action
	buf.Write(encodeVarInt(1))          // count
	buf.Write(uuid[:])
	writeString(&buf, name)
	props := m.properties
	buf.Write(encodeVarInt(int32(len(props)))) // properties len
	for _, prop := range props {
		writeString(&buf, prop.name)
		writeString(&buf, prop.value)
		if prop.signature != "" {
			buf.WriteByte(1)
			writeString(&buf, prop.signature)
		} else {
			buf.WriteByte(0)
		}
	}
	buf.Write(encodeVarInt(0)) // game mode
	buf.WriteByte(1)           // listed true
	buf.Write(encodeVarInt(0)) // latency 0

	payload := buf.Bytes()
	_ = m.rec.RecordNow(int32(m.cbidPlayerInfo), payload)
	m.playerInfoSent = true
}

// playerInfoEntry encodes the minimal entry for PlayerInfo (add_player/listed/latency/gamemode).
type playerInfoEntry struct {
	uuid [16]byte
	name string
}

// WriteTo implements pk.FieldEncoder so we can use it inside pk.Ary.
func (e playerInfoEntry) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	buf.Write(e.uuid[:])
	// GameProfile: name + properties (empty)
	writeString(&buf, e.name)
	buf.Write(encodeVarInt(0)) // properties len
	// initialize_chat flag false -> omit
	buf.Write(encodeVarInt(0)) // game mode (survival)
	buf.WriteByte(1)           // listed true
	buf.Write(encodeVarInt(0)) // latency 0
	// display_name/update_list_order/update_hat flags false -> omit
	n, err := buf.WriteTo(w)
	return n, err
}

func encodeVarInt(v int32) []byte {
	uv := uint32(v)
	out := make([]byte, 0, 5)
	for {
		b := byte(uv & 0x7F)
		uv >>= 7
		if uv != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if uv == 0 {
			break
		}
	}
	return out
}

func writeString(buf *bytes.Buffer, s string) {
	buf.Write(encodeVarInt(int32(len(s))))
	buf.WriteString(s)
}

// loadTexturesFromReplay scans a local mcpr for an add_player entry matching the uuid and returns its properties.
func loadTexturesFromReplay(path string, target [16]byte, targetName string) ([]profileProperty, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	var rec *zip.File
	for _, f := range zr.File {
		if f.Name == "recording.tmcpr" {
			rec = f
			break
		}
	}
	if rec == nil {
		return nil, fmt.Errorf("recording.tmcpr not found in %s", path)
	}
	rc, err := rec.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	var fallbackProps []profileProperty
	var (
		hdr [8]byte
	)
	for frame := 0; ; frame++ {
		if _, err := io.ReadFull(rc, hdr[:]); err != nil {
			// EOF or short read; stop scanning
			break
		}
		plen := binary.BigEndian.Uint32(hdr[4:8])
		buf := make([]byte, plen)
		if _, err := io.ReadFull(rc, buf); err != nil {
			break
		}
		idx := 0
		pid, n := decodeVarInt(buf)
		if pid != 63 {
			continue
		}
		idx += n
		if idx >= len(buf) {
			continue
		}
		action := int(buf[idx])
		idx++
		if action&0x01 == 0 {
			continue
		}
		count, n := decodeVarInt(buf[idx:])
		idx += n
		if count <= 0 {
			continue
		}
		for e := 0; e < count && idx < len(buf); e++ {
			if idx+16 > len(buf) {
				break
			}
			var uuid [16]byte
			copy(uuid[:], buf[idx:idx+16])
			idx += 16
			name, n := readString(buf[idx:])
			idx += n
			propLen, n := decodeVarInt(buf[idx:])
			idx += n
			if propLen < 0 {
				break
			}
			props := make([]profileProperty, 0, propLen)
			for p := 0; p < propLen && idx < len(buf); p++ {
				pname, n := readString(buf[idx:])
				idx += n
				pval, n := readString(buf[idx:])
				idx += n
				if idx >= len(buf) {
					break
				}
				hasSig := buf[idx] != 0
				idx++
				sig := ""
				if hasSig {
					var sn int
					sig, sn = readString(buf[idx:])
					idx += sn
				}
				props = append(props, profileProperty{name: pname, value: pval, signature: sig})
			}
			if len(props) > 0 {
				if uuid == target {
					log.Printf("loaded %d properties for %s from %s (name=%s)", len(props), uuidHex(uuid), path, name)
					return props, nil
				}
				if targetName != "" && name == targetName {
					log.Printf("loaded %d properties for name %s from %s (uuid=%s)", len(props), name, path, uuidHex(uuid))
					return props, nil
				}
				if fallbackProps == nil {
					fallbackProps = props
				}
			}
			// Skip rest of entry based on action bits
			if action&0x04 != 0 { // game mode
				_, n := decodeVarInt(buf[idx:])
				idx += n
			}
			if action&0x08 != 0 { // listed
				if idx < len(buf) {
					idx++
				}
			}
			if action&0x10 != 0 { // latency
				_, n := decodeVarInt(buf[idx:])
				idx += n
			}
			if action&0x20 != 0 { // display name
				if idx < len(buf) {
					has := buf[idx] != 0
					idx++
					if has {
						_, n := readString(buf[idx:])
						idx += n
					}
				}
			}
		}
	}
	if fallbackProps != nil {
		log.Printf("using fallback properties from %s for uuid %s", path, uuidHex(target))
		return fallbackProps, nil
	}
	return nil, fmt.Errorf("no properties found for uuid %s in %s", uuidHex(target), path)
}

// collectProperties converts a GameProfile's properties into our internal slice.
func collectProperties(gp *v1215_basetypes.GameProfile) []profileProperty {
	if gp == nil || gp.Properties.Ary.Ary == nil {
		return nil
	}
	values := *gp.Properties.Get()
	props := make([]profileProperty, 0, len(values))
	for _, p := range values {
		name := string(p.Name)
		value := string(p.Value)
		if name == "" || value == "" {
			continue
		}
		prop := profileProperty{name: name, value: value}
		if p.Signature.Has && p.Signature.Val != nil {
			prop.signature = string(*p.Signature.Val)
		}
		props = append(props, prop)
	}
	return props
}

// ensurePropertiesFromLocalReplayLocked attempts to reuse skin properties from a local mcpr.
func (m *replayMovementMirror) ensurePropertiesFromLocalReplayLocked() {
	if len(m.properties) > 0 {
		return
	}
	src := os.Getenv("MC_AGENT_TEXTURE_SOURCE")
	if src == "" {
		src = "tmp/2025_11_30_16_37_20.mcpr"
	}
	if src == "" {
		log.Printf("textures fallback: no MC_AGENT_TEXTURE_SOURCE provided")
		return
	}
	if abs, err := filepath.Abs(src); err == nil {
		src = abs
	}
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		log.Printf("textures fallback: cannot stat %s: %v", src, err)
		return
	}
	props, err := loadTexturesFromReplay(src, m.uuid, m.name)
	if err != nil {
		log.Printf("failed to load textures from %s: %v", src, err)
		return
	}
	if len(props) > 0 {
		m.properties = props
		m.fetchedProps = true
		log.Printf("textures fallback: loaded %d props from %s", len(props), src)
	}
}

func uuidHex(u [16]byte) string {
	// Format UUID with dashes in standard format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func deriveOfflineUUID(name string) [16]byte {
	var out [16]byte
	ns := gouuid.NewMD5(gouuid.NameSpaceOID, []byte("OfflinePlayer:"+name))
	copy(out[:], ns[:])
	return out
}

func decodeVarInt(buf []byte) (int, int) {
	var num int
	var numRead int
	for {
		if numRead >= len(buf) {
			return num, numRead
		}
		b := buf[numRead]
		num |= int(b&0x7F) << (7 * numRead)
		numRead++
		if b&0x80 == 0 {
			break
		}
	}
	return num, numRead
}

func readString(buf []byte) (string, int) {
	l, n := decodeVarInt(buf)
	start := n
	end := start + l
	if end > len(buf) {
		end = len(buf)
	}
	return string(buf[start:end]), end
}
