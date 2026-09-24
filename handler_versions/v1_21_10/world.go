// Package v1_21_10 provides version-specific packet handling for Minecraft 1.21.10.
package v1_21_10

import (
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/1.21.10/play/clientbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
	"log/slog"
)

// worldHandler implements common.WorldHandler for 1.21.10.
type worldHandler struct {
	packetMgr protocol_models.PacketMgr
	logger    *slog.Logger
}

// ParseBlockUpdate parses a single block update packet.
// Returns the block coordinates and the new block state ID.
func (w *worldHandler) ParseBlockUpdate(p pk.Packet) (x, y, z int64, blockStateID int32, err error) {
	pkt := cb.NewBlockChange()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, 0, common.ErrPacketParse{PacketName: "BlockChange", Cause: err}
	}

	// Position is packed as a bitfield with X (26 bits), Z (26 bits), Y (12 bits)
	x = pkt.Location.X
	y = pkt.Location.Y
	z = pkt.Location.Z
	blockStateID = int32(pkt.Type)

	return x, y, z, blockStateID, nil
}

// ParseSectionBlocksUpdate parses a multi-block update packet.
// Returns the section position and a list of block updates within that section.
func (w *worldHandler) ParseSectionBlocksUpdate(p pk.Packet) (sectionPos int64, blocks []models.BlockUpdate, err error) {
	pkt := cb.NewMultiBlockChange()
	if err = pkt.Scan(p); err != nil {
		return 0, nil, common.ErrPacketParse{PacketName: "MultiBlockChange", Cause: err}
	}

	// ChunkCoordinates is a bitfield: X (22 bits), Z (22 bits), Y (20 bits)
	// These are section coordinates (chunk X, chunk Z, section Y)
	sectionX := pkt.ChunkCoordinates.X
	sectionZ := pkt.ChunkCoordinates.Z
	sectionY := pkt.ChunkCoordinates.Y

	// Encode section position as a single int64 for the interface
	// Format: (sectionX << 42) | ((sectionZ & 0x3FFFFF) << 20) | (sectionY & 0xFFFFF)
	sectionPos = (sectionX << 42) | ((sectionZ & 0x3FFFFF) << 20) | (sectionY & 0xFFFFF)

	// Parse records - each record is a VarInt encoding:
	// - Block state ID in upper bits (>> 12)
	// - Position within section in lower 12 bits:
	//   - bits 0-3: X within section (0-15)
	//   - bits 4-7: Z within section (0-15)
	//   - bits 8-11: Y within section (0-15)
	records := pkt.Records.Get()
	blocks = make([]models.BlockUpdate, len(records))
	for i, record := range records {
		blockState := int32(record) >> 12
		localX := int64((record >> 8) & 0xF)
		localZ := int64((record >> 4) & 0xF)
		localY := int64(record & 0xF)

		// Convert to world coordinates
		worldX := int64(sectionX)*16 + localX
		worldZ := int64(sectionZ)*16 + localZ
		worldY := int64(sectionY)*16 + localY

		blocks[i] = models.BlockUpdate{
			X:            worldX,
			Y:            worldY,
			Z:            worldZ,
			BlockStateID: blockState,
		}
	}

	return sectionPos, blocks, nil
}

// ParseChunkData parses a chunk data packet.
// Returns the chunk X/Z coordinates and raw chunk data for further processing.
func (w *worldHandler) ParseChunkData(p pk.Packet) (chunkX, chunkZ int32, data []byte, err error) {
	pkt := cb.NewMapChunk()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, nil, common.ErrPacketParse{PacketName: "MapChunk", Cause: err}
	}

	chunkX = int32(pkt.X)
	chunkZ = int32(pkt.Z)

	// ChunkData is a ByteArray containing the raw chunk section data
	// This includes all sections with their palette and block data
	data = []byte(pkt.ChunkData)

	return chunkX, chunkZ, data, nil
}

// ParseUnloadChunk parses a chunk unload packet.
// Returns the chunk X/Z coordinates of the chunk to unload.
func (w *worldHandler) ParseUnloadChunk(p pk.Packet) (chunkX, chunkZ int32, err error) {
	pkt := cb.NewUnloadChunk()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "UnloadChunk", Cause: err}
	}

	// Note: In the protocol, ChunkZ comes before ChunkX
	chunkX = int32(pkt.ChunkX)
	chunkZ = int32(pkt.ChunkZ)

	return chunkX, chunkZ, nil
}

// ParseUpdateTime parses the ClientboundUpdateTime packet.
// Returns the world age (ticks since world creation) and time of day (ticks in current day).
func (w *worldHandler) ParseUpdateTime(p pk.Packet) (worldAge, timeOfDay int64, err error) {
	pkt := cb.NewUpdateTime()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "UpdateTime", Cause: err}
	}
	return int64(pkt.Age), int64(pkt.Time), nil
}

// ParseSetTickingState parses the ClientboundSetTickingState packet
// (added 1.20.5, part of the vanilla /tick command's protocol support).
func (w *worldHandler) ParseSetTickingState(p pk.Packet) (tickRate float32, isFrozen bool, err error) {
	pkt := cb.NewSetTickingState()
	if err = pkt.Scan(p); err != nil {
		return 0, false, common.ErrPacketParse{PacketName: "SetTickingState", Cause: err}
	}
	return float32(pkt.TickRate), bool(pkt.IsFrozen), nil
}

// ParseExplosion parses a ClientboundExplosion packet.
// Returns whether the explosion pushed the receiving player and, if so,
// the velocity delta to add to their current velocity (not a replacement
// — see models.WorldHandler's doc comment).
func (w *worldHandler) ParseExplosion(p pk.Packet) (hasKnockback bool, knockbackX, knockbackY, knockbackZ float64, err error) {
	pkt := cb.NewExplosion()
	if err = pkt.Scan(p); err != nil {
		return false, 0, 0, 0, common.ErrPacketParse{PacketName: "Explosion", Cause: err}
	}
	if !pkt.PlayerKnockback.Has || pkt.PlayerKnockback.Val == nil {
		return false, 0, 0, 0, nil
	}
	kb := pkt.PlayerKnockback.Val
	return true, float64(kb.X), float64(kb.Y), float64(kb.Z), nil
}

// ParseDifficulty parses the ClientboundChangeDifficulty packet.
func (w *worldHandler) ParseDifficulty(p pk.Packet) (difficulty uint8, locked bool, err error) {
	pkt := cb.NewDifficulty()
	if err = pkt.Scan(p); err != nil {
		return 0, false, common.ErrPacketParse{PacketName: "Difficulty", Cause: err}
	}
	difficulty = difficultyFromMapper(pkt.Difficulty.Value)
	locked = bool(pkt.DifficultyLocked)
	return difficulty, locked, nil
}

// difficultyFromMapper converts the wire mapper enum's string value
// ("peaceful"/"easy"/"normal"/"hard") to the numeric 0-3 form used
// elsewhere in this codebase (matching pre-1.21.6's raw-byte encoding).
func difficultyFromMapper(value string) uint8 {
	switch value {
	case "peaceful":
		return 0
	case "easy":
		return 1
	case "normal":
		return 2
	case "hard":
		return 3
	default:
		return 0
	}
}

// ParseInitializeWorldBorder parses the ClientboundInitializeWorldBorder packet.
func (w *worldHandler) ParseInitializeWorldBorder(p pk.Packet) (b models.WorldBorder, err error) {
	pkt := cb.NewInitializeWorldBorder()
	if err = pkt.Scan(p); err != nil {
		return models.WorldBorder{}, common.ErrPacketParse{PacketName: "InitializeWorldBorder", Cause: err}
	}
	return models.WorldBorder{
		CenterX:                float64(pkt.X),
		CenterZ:                float64(pkt.Z),
		OldDiameter:            float64(pkt.OldDiameter),
		NewDiameter:            float64(pkt.NewDiameter),
		SpeedTicks:             int64(pkt.Speed),
		PortalTeleportBoundary: int32(pkt.PortalTeleportBoundary),
		WarningBlocks:          int32(pkt.WarningBlocks),
		WarningTimeTicks:       int32(pkt.WarningTime),
	}, nil
}

// ParseWorldBorderCenter parses the ClientboundWorldBorderCenter packet.
func (w *worldHandler) ParseWorldBorderCenter(p pk.Packet) (x, z float64, err error) {
	pkt := cb.NewWorldBorderCenter()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "WorldBorderCenter", Cause: err}
	}
	return float64(pkt.X), float64(pkt.Z), nil
}

// ParseWorldBorderSize parses the ClientboundWorldBorderSize packet.
func (w *worldHandler) ParseWorldBorderSize(p pk.Packet) (diameter float64, err error) {
	pkt := cb.NewWorldBorderSize()
	if err = pkt.Scan(p); err != nil {
		return 0, common.ErrPacketParse{PacketName: "WorldBorderSize", Cause: err}
	}
	return float64(pkt.Diameter), nil
}

// ParseWorldBorderLerpSize parses the ClientboundWorldBorderLerpSize packet.
func (w *worldHandler) ParseWorldBorderLerpSize(p pk.Packet) (oldDiameter, newDiameter float64, speedTicks int64, err error) {
	pkt := cb.NewWorldBorderLerpSize()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, 0, common.ErrPacketParse{PacketName: "WorldBorderLerpSize", Cause: err}
	}
	return float64(pkt.OldDiameter), float64(pkt.NewDiameter), int64(pkt.Speed), nil
}

// ParseWorldBorderWarningDelay parses the ClientboundWorldBorderWarningDelay packet.
func (w *worldHandler) ParseWorldBorderWarningDelay(p pk.Packet) (warningTimeTicks int32, err error) {
	pkt := cb.NewWorldBorderWarningDelay()
	if err = pkt.Scan(p); err != nil {
		return 0, common.ErrPacketParse{PacketName: "WorldBorderWarningDelay", Cause: err}
	}
	return int32(pkt.WarningTime), nil
}

// ParseWorldBorderWarningDistance parses the ClientboundWorldBorderWarningReach packet.
func (w *worldHandler) ParseWorldBorderWarningDistance(p pk.Packet) (warningBlocks int32, err error) {
	pkt := cb.NewWorldBorderWarningReach()
	if err = pkt.Scan(p); err != nil {
		return 0, common.ErrPacketParse{PacketName: "WorldBorderWarningReach", Cause: err}
	}
	return int32(pkt.WarningBlocks), nil
}

// ParseChunkBiomes parses the ClientboundChunkBiomes packet.
func (w *worldHandler) ParseChunkBiomes(p pk.Packet) (updates []models.ChunkBiomeUpdate, err error) {
	pkt := cb.NewChunkBiomes()
	if err = pkt.Scan(p); err != nil {
		return nil, common.ErrPacketParse{PacketName: "ChunkBiomes", Cause: err}
	}
	entries := pkt.Biomes.Get()
	updates = make([]models.ChunkBiomeUpdate, len(entries))
	for i, entry := range entries {
		updates[i] = models.ChunkBiomeUpdate{
			ChunkX: int32(entry.Position.X),
			ChunkZ: int32(entry.Position.Z),
			Data:   []byte(entry.Data),
		}
	}
	return updates, nil
}

// longsToInt64s converts a parsed VarInt-prefixed array of pk.Long into a
// plain []int64, shared by the mask fields on both light packets.
func longsToInt64s(vals []pk.Long) []int64 {
	out := make([]int64, len(vals))
	for i, v := range vals {
		out[i] = int64(v)
	}
	return out
}

// byteArraysToBytes converts a parsed VarInt-prefixed array of nested
// VarInt-prefixed byte arrays into a plain [][]byte, shared by the
// SkyLight/BlockLight fields on both light packets.
func byteArraysToBytes[T interface{ Get() []pk.UnsignedByte }](vals []T) [][]byte {
	out := make([][]byte, len(vals))
	for i, arr := range vals {
		unsigned := arr.Get()
		b := make([]byte, len(unsigned))
		for j, u := range unsigned {
			b[j] = byte(u)
		}
		out[i] = b
	}
	return out
}

// ParseChunkLight parses the light payload embedded in a MapChunk
// (ClientboundLevelChunkWithLight) packet.
func (w *worldHandler) ParseChunkLight(p pk.Packet) (light models.ChunkLightData, err error) {
	pkt := cb.NewMapChunk()
	if err = pkt.Scan(p); err != nil {
		return models.ChunkLightData{}, common.ErrPacketParse{PacketName: "MapChunk", Cause: err}
	}
	return models.ChunkLightData{
		SkyLightMask:        longsToInt64s(pkt.SkyLightMask.Get()),
		BlockLightMask:      longsToInt64s(pkt.BlockLightMask.Get()),
		EmptySkyLightMask:   longsToInt64s(pkt.EmptySkyLightMask.Get()),
		EmptyBlockLightMask: longsToInt64s(pkt.EmptyBlockLightMask.Get()),
		SkyLight:            byteArraysToBytes(pkt.SkyLight.Get()),
		BlockLight:          byteArraysToBytes(pkt.BlockLight.Get()),
	}, nil
}

// ParseLightUpdate parses the standalone ClientboundUpdateLight packet.
func (w *worldHandler) ParseLightUpdate(p pk.Packet) (chunkX, chunkZ int32, light models.ChunkLightData, err error) {
	pkt := cb.NewUpdateLight()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, models.ChunkLightData{}, common.ErrPacketParse{PacketName: "UpdateLight", Cause: err}
	}
	light = models.ChunkLightData{
		SkyLightMask:        longsToInt64s(pkt.SkyLightMask.Get()),
		BlockLightMask:      longsToInt64s(pkt.BlockLightMask.Get()),
		EmptySkyLightMask:   longsToInt64s(pkt.EmptySkyLightMask.Get()),
		EmptyBlockLightMask: longsToInt64s(pkt.EmptyBlockLightMask.Get()),
		SkyLight:            byteArraysToBytes(pkt.SkyLight.Get()),
		BlockLight:          byteArraysToBytes(pkt.BlockLight.Get()),
	}
	return int32(pkt.ChunkX), int32(pkt.ChunkZ), light, nil
}

// SendChunkBatchReceived sends an acknowledgment for received chunk batches.
// This is required in 1.20.2+ to signal the server that the client is ready for more chunks.
func (w *worldHandler) SendChunkBatchReceived(conn models.PacketWriter, batchCount float32) error {
	packetID := w.packetMgr.GetServerboundPacketID("ServerboundChunkBatchReceived")
	if packetID < 0 {
		return common.ErrPacketParse{PacketName: "ServerboundChunkBatchReceived", Cause: nil}
	}

	packet := pk.Marshal(
		int32(packetID),
		pk.Float(batchCount),
	)

	return conn.WritePacket(packet)
}
