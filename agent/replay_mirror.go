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
	"github.com/reallyoldfogie/mc-agent/models"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"github.com/reallyoldfogie/mc-replay-go/mcpr/recorder"
)

// replayMovementMirror converts select serverbound packets (movement) into
// synthetic clientbound packets for replay visibility of the local player.
type replayMovementMirror struct {
	rec            *recorder.Recorder
	pm             protocol_models.PacketMgr
	versionHandler models.VersionHandler

	mu                   sync.Mutex
	entityID             int32
	entityType           int32
	name                 string
	uuid                 [16]byte
	hasUUID              bool
	serverName           string
	serverUUID           [16]byte
	serverPI             []byte
	serverPIHasProps     bool
	serverPlayerInfoSeen bool
	properties           []profileProperty
	fetchedProps         bool
	spawned              bool
	loginSeen            bool // tracks if LOGIN packet has been recorded
	skinProvider         models.SkinProvider

	lastX, lastY, lastZ float64
	lastYaw, lastPitch  float32
	onGround            bool
	playerInfoSent      bool
	positionInitialized bool

	sbidPos        int32 // cached serverbound ids
	sbidPosRot     int32
	sbidRot        int32
	sbidStatus     int32
	cbidTeleport   int32 // cached clientbound id
	cbidAddEnt     int32
	cbidPlayerInfo int32
	cbidMovePos    int32
	cbidMovePosRot int32
	cbidRotateHead int32
}

// NewReplayMovementMirror constructs a movement mirror if both recorder and
// packet manager are provided. It returns nil when either dependency is nil.
func NewReplayMovementMirror(rec *recorder.Recorder, pm protocol_models.PacketMgr, versionHandler models.VersionHandler, sp models.SkinProvider) MovementMirror {
	if rec == nil || pm == nil {
		return nil
	}
	return &replayMovementMirror{
		rec:            rec,
		pm:             pm,
		versionHandler: versionHandler,
		sbidPos:        int32(pm.GetServerboundPacketID("ServerboundMovePlayerPos")),
		sbidPosRot:     int32(pm.GetServerboundPacketID("ServerboundMovePlayerPosRot")),
		sbidRot:        int32(pm.GetServerboundPacketID("ServerboundMovePlayerRot")),
		sbidStatus:     int32(pm.GetServerboundPacketID("ServerboundMovePlayerStatusOnly")),
		cbidTeleport:   int32(pm.GetClientboundPacketID("ClientboundTeleportEntity")),
		cbidAddEnt:     int32(pm.GetClientboundPacketID("ClientboundAddEntity")),
		cbidPlayerInfo: int32(pm.GetClientboundPacketID("ClientboundPlayerInfo")),
		cbidMovePos:    int32(pm.GetClientboundPacketID("ClientboundMoveEntityPos")),
		cbidMovePosRot: int32(pm.GetClientboundPacketID("ClientboundMoveEntityPosRot")),
		cbidRotateHead: int32(pm.GetClientboundPacketID("ClientboundRotateHead")),
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

// NotifyLoginSeen signals that the LOGIN packet has been recorded to the replay.
// This allows the MovementMirror to maintain correct packet ordering: LOGIN must
// come before PlayerInfo packets for ReplayMod compatibility.
func (m *replayMovementMirror) NotifyLoginSeen() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.loginSeen = true
	// Now that LOGIN has been recorded, we can safely emit PlayerInfo if needed
	if m.entityID != 0 && m.name != "" {
		m.ensurePlayerInfoLocked()
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

// HandlePlayerInfo captures the server-provided PlayerInfo packet and extracts UUID/name/properties.
// It processes all entries, handles removal actions, and stores raw packet data for replay re-emission.
func (m *replayMovementMirror) HandlePlayerInfo(p pk.Packet) {
	if m.versionHandler == nil {
		log.Printf("HandlePlayerInfo: version handler not available, skipping PlayerInfo processing")
		return
	}

	update, err := m.versionHandler.Play().ParsePlayerInfo(p)
	if err != nil {
		log.Printf("HandlePlayerInfo: failed to parse PlayerInfo packet: %v", err)
		return
	}
	if update == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Handle removal action by resetting emission state.
	if update.HasRemovePlayer() {
		m.playerInfoSent = false
		if len(m.serverPI) > 0 {
			payload := make([]byte, len(m.serverPI))
			copy(payload, m.serverPI)
			_ = m.rec.RecordNow(int32(m.cbidPlayerInfo), payload)
		}
	}

	// Only process add_player updates for profile/name extraction.
	if !update.HasAddPlayer() {
		return
	}

	for _, playerInfoEntry := range update.Entries {
		if !playerInfoEntry.AddPlayer {
			continue
		}

		// Check if this is our player by matching UUID or name.
		// If we have a UUID, match by UUID. Otherwise match by name.
		isOurPlayer := false
		if m.hasUUID && m.uuid != ([16]byte{}) {
			// Match by UUID if we have one
			isOurPlayer = (m.uuid == playerInfoEntry.UUID)
		} else if m.name != "" && playerInfoEntry.Name != "" {
			// Match by name if we don't have a UUID yet
			isOurPlayer = (m.name == playerInfoEntry.Name)
		}

		// Store this entry's data for potential replay re-emission
		m.serverPI = make([]byte, len(p.Data))
		copy(m.serverPI, p.Data)

		if playerInfoEntry.UUID != ([16]byte{}) {
			m.rec.AddPlayer(uuidHex(playerInfoEntry.UUID))
		}

		// If this is our player, update our metadata with server-provided info
		if isOurPlayer {
			// Update UUID if we learned it from the server
			if playerInfoEntry.UUID != ([16]byte{}) {
				m.uuid = playerInfoEntry.UUID
				m.hasUUID = true
			}
			// Update name if server provided one
			if playerInfoEntry.Name != "" {
				m.serverName = playerInfoEntry.Name
			}
			// Store properties from server
			if len(playerInfoEntry.Properties) > 0 {
				m.properties = make([]profileProperty, 0, len(playerInfoEntry.Properties))
				for _, property := range playerInfoEntry.Properties {
					m.properties = append(m.properties, profileProperty{
						Name:      property.Name,
						Value:     property.Value,
						Signature: property.Signature,
					})
				}
				m.serverPIHasProps = true
			} else {
				m.serverPIHasProps = false
			}
			// Mark server PlayerInfo as seen so we don't synthesize our own
			m.serverPlayerInfoSeen = true
			// Reset flag to allow emission if needed
			m.playerInfoSent = false
			// Ensure our PlayerInfo gets emitted with the server data
			m.ensurePlayerInfoLocked()
			return
		}
	}
}

func (m *replayMovementMirror) handlePos(p pk.Packet) {
	// Use version handler if available, fallback to direct packet parsing
	if m.versionHandler != nil {
		x, y, z, onGround, err := m.versionHandler.Play().Movement().ParseServerboundPos(p)
		if err == nil {
			log.Printf("[ReplayMirror] handlePos (version-specific): pos=(%.2f, %.2f, %.2f)", x, y, z)
			m.emitTeleport(x, y, z, m.lastYaw, m.lastPitch, onGround)
			return
		}
	}

	// Fallback to direct packet parsing
	var x, y, z pk.Double
	var onGround pk.Boolean
	if err := p.Scan(&x, &y, &z, &onGround); err != nil {
		return
	}
	log.Printf("[ReplayMirror] handlePos (fallback): pos=(%.2f, %.2f, %.2f)", float64(x), float64(y), float64(z))
	m.emitTeleport(float64(x), float64(y), float64(z), m.lastYaw, m.lastPitch, bool(onGround))
}

func (m *replayMovementMirror) handlePosRot(p pk.Packet) {
	// Use version handler if available, fallback to direct packet parsing
	if m.versionHandler != nil {
		x, y, z, yaw, pitch, onGround, err := m.versionHandler.Play().Movement().ParseServerboundPosRot(p)
		if err == nil {
			log.Printf("[ReplayMirror] handlePosRot (version-specific): pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f",
				x, y, z, yaw, pitch)
			m.emitTeleport(x, y, z, yaw, pitch, onGround)
			return
		}
	}

	// Fallback to direct packet parsing
	var x, y, z pk.Double
	var yaw, pitch pk.Float
	var onGround pk.Boolean
	if err := p.Scan(&x, &y, &z, &yaw, &pitch, &onGround); err != nil {
		return
	}
	log.Printf("[ReplayMirror] handlePosRot (fallback): pos=(%.2f, %.2f, %.2f) yaw=%.2f pitch=%.2f",
		float64(x), float64(y), float64(z), float32(yaw), float32(pitch))
	m.emitTeleport(float64(x), float64(y), float64(z), float32(yaw), float32(pitch), bool(onGround))
}

func (m *replayMovementMirror) handleRot(p pk.Packet) {
	// Use version handler if available, fallback to direct packet parsing
	if m.versionHandler != nil {
		yaw, pitch, onGround, err := m.versionHandler.Play().Movement().ParseServerboundRot(p)
		if err == nil {
			log.Printf("[ReplayMirror] handleRot (version-specific): yaw=%.2f pitch=%.2f", yaw, pitch)
			m.emitTeleport(m.lastX, m.lastY, m.lastZ, yaw, pitch, onGround)
			return
		}
	}

	// Fallback to direct packet parsing
	var yaw, pitch pk.Float
	var onGround pk.Boolean
	if err := p.Scan(&yaw, &pitch, &onGround); err != nil {
		return
	}
	log.Printf("[ReplayMirror] handleRot (fallback): yaw=%.2f pitch=%.2f", float32(yaw), float32(pitch))
	m.emitTeleport(m.lastX, m.lastY, m.lastZ, float32(yaw), float32(pitch), bool(onGround))
}

func (m *replayMovementMirror) handleStatus(p pk.Packet) {
	// Use version handler if available, fallback to direct packet parsing
	if m.versionHandler != nil {
		onGround, err := m.versionHandler.Play().Movement().ParseServerboundStatus(p)
		if err == nil {
			log.Printf("[ReplayMirror] handleStatus (version-specific): onGround=%v", onGround)
			m.emitTeleport(m.lastX, m.lastY, m.lastZ, m.lastYaw, m.lastPitch, onGround)
			return
		}
	}

	// Fallback to direct packet parsing
	var onGround pk.Boolean
	if err := p.Scan(&onGround); err != nil {
		return
	}
	log.Printf("[ReplayMirror] handleStatus (fallback): onGround=%v", bool(onGround))
	m.emitTeleport(m.lastX, m.lastY, m.lastZ, m.lastYaw, m.lastPitch, bool(onGround))
}

func (m *replayMovementMirror) emitTeleport(x, y, z float64, yaw, pitch float32, onGround bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.entityID == 0 {
		return
	}

	prevX, prevY, prevZ := m.lastX, m.lastY, m.lastZ
	prevYaw, prevPitch := m.lastYaw, m.lastPitch
	prevOnGround := m.onGround
	m.lastX, m.lastY, m.lastZ = x, y, z
	m.lastYaw, m.lastPitch = yaw, pitch
	m.onGround = onGround

	if !m.loginSeen {
		return
	}

	// Ensure tab list entry exists once we know our id/name/uuid.
	// Only emit after LOGIN packet has been recorded to maintain correct packet order.
	if m.loginSeen {
		m.ensurePlayerInfoLocked()
	}

	if !m.spawned && m.entityType != 0 {
		log.Printf("[ReplayMirror] Spawning bot entity at (%.2f, %.2f, %.2f) entityID=%d", x, y, z, m.entityID)
		m.spawned = true
		m.writeAddEntity(x, y, z, yaw, pitch)
	}

	// Only emit teleport packets after the entity has been spawned and we have an
	// initial position, to avoid redundant teleports on spawn.
	if m.spawned {
		changed := x != prevX || y != prevY || z != prevZ || yaw != prevYaw || pitch != prevPitch || onGround != prevOnGround
		if m.positionInitialized && changed {
			if dx, dy, dz, ok := encodeRelMove(prevX, prevY, prevZ, x, y, z); ok {
				m.writeRelMove(dx, dy, dz, yaw, pitch, onGround)
			} else {
				// Split large deltas into multiple relative moves to keep replay in sync
				// without emitting teleport packets (which can corrupt replays).
				m.emitRelMoveSteps(prevX, prevY, prevZ, x, y, z, yaw, pitch, onGround)
			}
		}
		m.positionInitialized = true
	}
}

func (m *replayMovementMirror) writeRelMove(dx, dy, dz int16, yaw, pitch float32, onGround bool) {
	yawByte := angleToByte(yaw)
	pitchByte := angleToByte(pitch)
	log.Printf("[ReplayMirror] writeRelMove: delta=(%d, %d, %d) yaw=%.2f->%d pitch=%.2f->%d",
		dx, dy, dz, yaw, yawByte, pitch, pitchByte)
	move := pk.Marshal(
		m.cbidMovePosRot,
		pk.VarInt(m.entityID),
		pk.Short(dx),
		pk.Short(dy),
		pk.Short(dz),
		pk.Byte(yawByte),
		pk.Byte(pitchByte),
		pk.Boolean(onGround),
	)
	_ = m.rec.RecordNow(int32(move.ID), move.Data)

	// Also update head yaw to match body yaw
	m.writeRotateHead(yaw)
}

func (m *replayMovementMirror) writeRotateHead(yaw float32) {
	yawByte := angleToByte(yaw)
	rotate := pk.Marshal(
		m.cbidRotateHead,
		pk.VarInt(m.entityID),
		pk.Byte(yawByte),
	)
	_ = m.rec.RecordNow(int32(rotate.ID), rotate.Data)
}

func (m *replayMovementMirror) emitRelMoveSteps(prevX, prevY, prevZ, x, y, z float64, yaw, pitch float32, onGround bool) {
	const maxDelta = 7.9 // slightly under 8 blocks to stay within int16 range
	dx := x - prevX
	dy := y - prevY
	dz := z - prevZ

	steps := int(math.Ceil(math.Max(math.Abs(dx), math.Max(math.Abs(dy), math.Abs(dz))) / maxDelta))
	if steps < 1 {
		steps = 1
	}

	curX, curY, curZ := prevX, prevY, prevZ
	for i := 1; i <= steps; i++ {
		nextX := prevX + dx*float64(i)/float64(steps)
		nextY := prevY + dy*float64(i)/float64(steps)
		nextZ := prevZ + dz*float64(i)/float64(steps)
		rdx, rdy, rdz, ok := encodeRelMove(curX, curY, curZ, nextX, nextY, nextZ)
		if !ok {
			log.Printf("[ReplayMirror] Warning: failed to encode relative move step (%.2f, %.2f, %.2f) -> (%.2f, %.2f, %.2f)",
				curX, curY, curZ, nextX, nextY, nextZ)
			return
		}
		m.writeRelMove(rdx, rdy, rdz, yaw, pitch, onGround)
		curX, curY, curZ = nextX, nextY, nextZ
	}
}

func (m *replayMovementMirror) writeAddEntity(x, y, z float64, yaw, pitch float32) {
	// Build SpawnEntity packet using version-aware handler
	// Converts float32 yaw/pitch to int8 angle bytes
	yawByte := angleToByte(yaw)
	pitchByte := angleToByte(pitch)
	packetID, packetData, err := m.versionHandler.Play().BuildSpawnEntityPacket(
		m.entityID,
		m.uuid,
		m.entityType,
		x, y, z,
		int8(yawByte), int8(pitchByte),
		0,     // objectData
		0, 0, 0, // velX, velY, velZ (zero velocity on spawn)
	)
	if err != nil {
		log.Printf("[ReplayMirror] Error building SpawnEntity packet: %v", err)
		return
	}
	_ = m.rec.RecordNow(packetID, packetData)
}

func angleToByte(f float32) byte {
	// Convert degrees to protocol angle byte
	return byte(int(math.Round(float64(f*256/360))) & 0xFF)
}

func encodeRelMove(prevX, prevY, prevZ, x, y, z float64) (int16, int16, int16, bool) {
	const scale = 4096.0
	dx := int64(math.Round((x - prevX) * scale))
	dy := int64(math.Round((y - prevY) * scale))
	dz := int64(math.Round((z - prevZ) * scale))
	if dx < math.MinInt16 || dx > math.MaxInt16 || dy < math.MinInt16 || dy > math.MaxInt16 || dz < math.MinInt16 || dz > math.MaxInt16 {
		return 0, 0, 0, false
	}
	return int16(dx), int16(dy), int16(dz), true
}

// emit minimal PlayerInfo (aka PlayerInfoUpdate) so the bot appears in tab and has a name.
func (m *replayMovementMirror) ensurePlayerInfo() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensurePlayerInfoLocked()
}

// ensurePlayerInfoLocked makes sure that a PlayerInfo packet describing the
// local bot is recorded in the replay stream.  It must be called with the
// mirror mutex already held and will return immediately if a packet has been
// emitted or if recording is unavailable.  The algorithm enforces the following
// rules:
//   - Do not send anything until after the LOGIN packet has been written; the
//     replay format relies on the server assigning an entity ID during login.
//   - If we have already seen a server-provided PlayerInfo packet for this
//     client, echo it verbatim rather than synthesizing our own.
//   - Otherwise try to gather skin/profile properties from the skin provider or
//     from any cached local replay data.  If no properties are available we still
//     record the PlayerInfo entry (without textures) once we have a valid UUID
//     and name, but only if a login has been seen.
//   - Build and record a synthetic PlayerInfo packet through the version handler
//     when no server packet is available.
//
// The function sets m.playerInfoSent when a packet is recorded to avoid
// duplicate emissions.
func (m *replayMovementMirror) ensurePlayerInfoLocked() {
	log.Printf("ensurePlayerInfoLocked: entityID=%d name=%q sent=%v loginSeen=%v", m.entityID, m.name, m.playerInfoSent, m.loginSeen)
	if m.playerInfoSent || m.rec == nil {
		return
	}
	// Don't emit PlayerInfo until after LOGIN packet has been recorded.
	// ReplayMod expects LOGIN packet first to establish player entity ID.
	if !m.loginSeen {
		return
	}
	if m.serverPlayerInfoSeen {
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
	// If we captured a server add_player packet, re-emit it directly.
	if len(m.serverPI) > 0 {
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

	// Build a version-aware PlayerInfo packet using the version handler.
	if m.versionHandler == nil {
		log.Printf("ensurePlayerInfoLocked: version handler not available, skipping PlayerInfo")
		return
	}

	props := make([]models.ProfileProperty, 0, len(m.properties))
	for _, property := range m.properties {
		props = append(props, models.ProfileProperty{
			Name:      property.Name,
			Value:     property.Value,
			Signature: property.Signature,
		})
	}

	packetID, packetData, err := m.versionHandler.Play().BuildPlayerInfoPacket(uuid, name, props)
	if err != nil {
		log.Printf("ensurePlayerInfoLocked: failed to build PlayerInfo packet: %v", err)
		return
	}
	if err := m.rec.RecordNow(packetID, packetData); err != nil {
		log.Printf("ensurePlayerInfoLocked: failed to record PlayerInfo packet: %v", err)
		return
	}
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
				props = append(props, profileProperty{Name: pname, Value: pval, Signature: sig})
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
