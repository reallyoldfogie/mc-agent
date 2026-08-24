// Package v26_1 provides version-specific packet handling for Minecraft 26.1.
package v26_1

import (
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/reallyoldfogie/mc-agent/handler_versions/common"
	"github.com/reallyoldfogie/mc-agent/models"
	cb "github.com/reallyoldfogie/mc-protocol-go/data/26.1/play/clientbound"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// worldHandler implements common.WorldHandler for 26.1.
type worldHandler struct {
	packetMgr protocol_models.PacketMgr
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
//
// Protocol note: 26.1 replaced UpdateTime's single flat Time/TickDayTime fields
// with a ClockUpdates array driven by Mojang's new registry-based "world clock"
// system (see mc-data-gen/extractedSrc/26.1/net/minecraft/world/clock/{WorldClocks,
// ClockManager,ClientClockManager}.java and DayTimeToClockFix.java, which documents
// the old-dayTime-to-clock data migration). Each entry carries a registry ID for a
// WorldClock (e.g. "overworld", "the_end" - see WorldClocks.java), plus TotalTicks/
// PartialTick/Rate for that clock. Client-side, ClientClockManager.getTotalTicks()
// resolves the correct clock via the current dimension's registered WorldClock
// holder, which requires cross-referencing dynamic registry data not available
// in this narrow parse function.
//
// The overworld's WorldClock is registered first (bootstrap order in WorldClocks.java),
// so its registry ID is conventionally 0 - matching this packet's typical single-entry
// payload for a bot connected to the overworld. As a best-effort approximation (this
// interface predates the multi-clock system and has no way to express "time of day
// per dimension"), we use the first ClockUpdates entry's TotalTicks. Bots operating
// primarily in the overworld will get correct values; other dimensions with
// differently-ordered/registered clocks may not.
func (w *worldHandler) ParseUpdateTime(p pk.Packet) (worldAge, timeOfDay int64, err error) {
	pkt := cb.NewUpdateTime()
	if err = pkt.Scan(p); err != nil {
		return 0, 0, common.ErrPacketParse{PacketName: "UpdateTime", Cause: err}
	}

	worldAge = int64(pkt.Age)

	clockUpdates := pkt.ClockUpdates.Get()
	if len(clockUpdates) > 0 {
		timeOfDay = int64(clockUpdates[0].TotalTicks)
	}

	return worldAge, timeOfDay, nil
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
