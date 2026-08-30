package models

import (
	pk "github.com/Tnze/go-mc/net/packet"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// PacketWriter is a minimal interface for writing packets.
// Both bot.Conn and mcnet.Conn implement this interface.
type PacketWriter interface {
	WritePacket(pk.Packet) error
}

// VersionHandler is the main interface for version-specific packet handling.
// Each supported Minecraft version implements this interface independently.
type VersionHandler interface {
	// Version returns the Minecraft version string (e.g., "1.21.5")
	Version() string

	// ProtocolVersion returns the protocol version number
	ProtocolVersion() uint

	// PacketMgr returns the underlying packet manager for this version
	PacketMgr() protocol_models.PacketMgr

	// Login returns the login phase handler
	Login() LoginHandler

	// Configuration returns the configuration phase handler
	Configuration() ConfigurationHandler

	// Play returns the play phase handler
	Play() PlayHandler
}

// LoginHandler handles login phase packets.
type LoginHandler interface {
	// SendLoginStart sends the login start packet
	SendLoginStart(conn PacketWriter, username string, uuid [16]byte) error

	// SendEncryptionResponse sends the encryption response packet
	SendEncryptionResponse(conn PacketWriter, sharedSecret, verifyToken []byte) error

	// SendLoginAcknowledged sends the login acknowledged packet
	SendLoginAcknowledged(conn PacketWriter) error

	// ParseLoginSuccess parses a login success packet
	ParseLoginSuccess(p pk.Packet) (username string, uuid [16]byte, err error)

	// ParseEncryptionRequest parses an encryption request packet
	ParseEncryptionRequest(p pk.Packet) (serverID string, publicKey, verifyToken []byte, err error)
}

// ConfigurationHandler handles configuration phase packets.
type ConfigurationHandler interface {
	// SendFinishConfiguration sends the finish configuration packet
	SendFinishConfiguration(conn PacketWriter) error

	// SendKeepAlive sends a keepalive packet during configuration
	SendKeepAlive(conn PacketWriter, id int64) error

	// SendPong sends a pong packet in response to a ping
	SendPong(conn PacketWriter, pingID int32) error

	// SendClientInformation sends client settings/information
	SendClientInformation(conn PacketWriter, info ClientInfo) error

	// ParseRegistryData parses registry data packets
	ParseRegistryData(p pk.Packet) (registryID string, entries map[string]int32, err error)

	// ParseKeepAlive parses a keepalive packet
	ParseKeepAlive(p pk.Packet) (id int64, err error)

	// ParsePing parses a ping packet
	ParsePing(p pk.Packet) (pingID int32, err error)
}

// ClientInfo contains client information sent during configuration.
type ClientInfo struct {
	Locale              string
	ViewDistance        int8
	ChatMode            int32
	ChatColors          bool
	DisplayedSkinParts  uint8
	MainHand            int32
	EnableTextFiltering bool
	AllowServerListings bool
}

// PlayHandler provides access to play phase sub-handlers.
type PlayHandler interface {
	// Movement returns the movement handler
	Movement() MovementHandler

	// Entities returns the entity handler
	Entities() EntityHandler

	// Containers returns the container handler
	Containers() ContainerHandler

	// Chat returns the chat handler
	Chat() ChatHandler

	// World returns the world handler
	World() WorldHandler

	// Actions returns the action handler for player actions (item usage, bow, etc.)
	Actions() ActionHandler

	// Lifecycle returns the lifecycle handler for play-phase lifecycle signals
	// (e.g., player_loaded). For versions where a particular signal does not
	// exist, the corresponding Send method is a no-op.
	Lifecycle() LifecycleHandler

	// SendClientInformation sends client settings/information during play phase
	SendClientInformation(conn PacketWriter, info ClientInfo) error

	// SendCustomPayload sends a custom payload packet (plugin channels).
	// channel: the channel identifier (e.g., "minecraft:brand")
	// payload: the payload data (e.g., brand string)
	SendCustomPayload(conn PacketWriter, channel string, payload string) error

	// ParseLogin parses the ClientboundLogin packet to extract the entity ID
	// and the player's initial game mode (SpawnInfo.Gamemode).
	ParseLogin(p pk.Packet) (entityID int32, gameMode GameMode, err error)

	// ParseClientboundAbilities parses a ClientboundAbilities packet - sent
	// at login and whenever the local player's flying-related abilities
	// change (e.g. a game mode switch). See PlayerAbilities's doc comment.
	ParseClientboundAbilities(p pk.Packet) (abilities PlayerAbilities, err error)

	// ParseSound parses a ClientboundSound packet.
	// Returns soundID (0-based), category, position (fixed-point x8), volume, pitch, seed.
	ParseSound(p pk.Packet) (soundID, category int32, x, y, z int32, volume, pitch float32, seed int64, err error)

	// ParseViewDistance parses a ClientboundSetChunkCacheRadius packet.
	ParseViewDistance(p pk.Packet) (viewDistance int32, err error)

	// ParseSimulationDistance parses a ClientboundSetSimulationDistance packet.
	ParseSimulationDistance(p pk.Packet) (simulationDistance int32, err error)

	// ParseDisconnect parses a ClientboundDisconnect packet.
	ParseDisconnect(p pk.Packet) (reason string, err error)

	// ParseGameEvent parses a ClientboundGameEvent packet.
	// Returns eventType, position (x, y, z), and value.
	ParseGameEvent(p pk.Packet) (eventType int, x, y, z, value float64, err error)

	// ParseUpdateRecipes parses a ClientboundUpdateRecipes (DeclareRecipes) packet.
	// Returns the parsed payload containing property sets and stonecutter entries.
	// Returns nil, nil if the packet format is not supported for this version.
	ParseUpdateRecipes(p pk.Packet) (*UpdateRecipesPayload, error)

	// BuildPlayerInfoPacket builds a PlayerInfo packet for replay recording.
	// Returns the packet ID and raw marshaled packet data.
	// uuid: player UUID
	// name: player name
	// properties: list of game profile properties (name/value/signature tuples)
	BuildPlayerInfoPacket(uuid [16]byte, name string, properties []ProfileProperty) (packetID int32, packetData []byte, err error)

	// BuildSpawnEntityPacket builds a SpawnEntity packet for replay recording.
	// Returns the packet ID and raw marshaled packet data.
	// entityID: unique entity ID
	// uuid: entity UUID
	// entityType: entity type ID
	// x, y, z: entity position
	// yaw, pitch: entity rotation (in 1/256ths of a full turn)
	// objectData: extra data (e.g., projectile owner ID)
	// velX, velY, velZ: velocity components in blocks per tick
	BuildSpawnEntityPacket(entityID int32, uuid [16]byte, entityType int32, x, y, z float64, yaw, pitch int8, objectData int32, velX, velY, velZ float64) (packetID int32, packetData []byte, err error)

	// ParsePlayerInfo parses a complete PlayerInfo packet and extracts all entries and action data.
	// packet: the raw ClientboundPlayerInfo packet to parse
	// Returns the parsed update structure with all entry data.
	ParsePlayerInfo(packet pk.Packet) (*PlayerInfoUpdate, error)

	// BuildEntityEquipmentPacket builds an EntityEquipment packet for replay recording.
	// Returns the packet ID and raw marshaled packet data.
	// entityID: the bot's entity ID
	// slot: any EquipmentSlotType (hand, armor, or body)
	// itemID: protocol item ID (0 for empty)
	// count: item count (0 for empty)
	BuildEntityEquipmentPacket(entityID int32, slot EquipmentSlotType, itemID int32, count int32) (packetID int32, packetData []byte, err error)
}

// LifecycleHandler handles play-phase lifecycle signal packets that the vanilla
// client sends to indicate it has finished initializing/loading. Without these
// signals, the server may silently ignore client actions for a fixed grace
// period (e.g., 60 server ticks for 1.21.4+ where ServerboundPlayerLoaded was
// introduced and PlayerEntity.isLoaded gates server processing of interact
// packets, vehicle moves, etc.).
//
// Versions where a given signal packet does not exist must implement the
// corresponding Send method as a no-op.
type LifecycleHandler interface {
	// SendPlayerLoaded sends ServerboundPlayerLoaded (no payload) to mark the
	// player as loaded on the server. The vanilla client sends this once when
	// the world has finished loading. Sending it removes the server-side
	// 60-tick grace gate that silently drops interact/vehicle/etc. packets for
	// freshly-joined players.
	//
	// On versions that don't include this packet (1.21.1–1.21.3), this is a
	// no-op and returns nil.
	SendPlayerLoaded(conn PacketWriter) error
}

// MovementHandler handles player movement packets.
type MovementHandler interface {
	// SendPosition sends a position update packet (position only, no rotation)
	SendPosition(conn PacketWriter, x, y, z float64, onGround bool) error

	// SendPositionAndRotation sends a combined position and rotation packet
	SendPositionAndRotation(conn PacketWriter, x, y, z float64, yaw, pitch float64, onGround bool) error

	// SendRotation sends a rotation update packet (rotation only, no position)
	SendRotation(conn PacketWriter, yaw, pitch float64, onGround bool) error

	// SendPlayerCommand sends a player command packet (sprint, sneak, etc.)
	// actionID: 0=start sneak, 1=stop sneak, 2=leave bed, 3=start sprint, 4=stop sprint, etc.
	SendPlayerCommand(conn PacketWriter, entityID, actionID int32) error

	// SendTeleportConfirm confirms a server-requested teleport
	SendTeleportConfirm(conn PacketWriter, teleportID int32) error

	// SendPlayerAbilities sends player abilities (flying, etc.)
	SendPlayerAbilities(conn PacketWriter, flags byte) error

	// SendMoveVehicle sends a vehicle movement packet while mounted.
	// Sent instead of SendPosition when the player is riding a vehicle/mount.
	SendMoveVehicle(conn PacketWriter, x, y, z float64, yaw, pitch float64, onGround bool) error

	// SendVehicleInput sends directional input for the currently mounted vehicle.
	// For v1_21_1–3 translates to SteerVehicle (float-based);
	// for v1_21_4–11 uses PlayerInput bitflags.
	// Note: For boats, use SendBoatPaddleState instead.
	SendVehicleInput(conn PacketWriter, forward, backward, left, right, jump, sneak bool) error

	// SendBoatPaddleState sends boat paddle state (which oars are paddling).
	// Only used for boats; horses and other vehicles use SendVehicleInput.
	// leftPaddling: true to paddle with left oar
	// rightPaddling: true to paddle with right oar
	SendBoatPaddleState(conn PacketWriter, leftPaddling, rightPaddling bool) error

	// SendPlayerCommandWithParam sends a player command with an optional parameter.
	// Like SendPlayerCommand but includes the optional jump boost parameter (0–100),
	// only used for ActionStartJumpHorse.
	SendPlayerCommandWithParam(conn PacketWriter, entityID, actionID, jumpBoost int32) error

	// ParsePlayerPosition parses a clientbound player position packet
	ParsePlayerPosition(p pk.Packet) (teleportID int32, x, y, z float64, yaw, pitch float64, flags int32, err error)

	// ParseServerboundPos parses a serverbound position packet (for replay recording)
	ParseServerboundPos(p pk.Packet) (x, y, z float64, onGround bool, err error)

	// ParseServerboundPosRot parses a serverbound position+rotation packet (for replay recording)
	ParseServerboundPosRot(p pk.Packet) (x, y, z float64, yaw, pitch float64, onGround bool, err error)

	// ParseServerboundRot parses a serverbound rotation packet (for replay recording)
	ParseServerboundRot(p pk.Packet) (yaw, pitch float64, onGround bool, err error)

	// ParseServerboundStatus parses a serverbound status-only packet (for replay recording)
	ParseServerboundStatus(p pk.Packet) (onGround bool, err error)

	// ParseClientboundMoveVehicle parses a server-to-client vehicle position correction packet.
	// Sent when the server rejects a client VehicleMove and needs to reset the vehicle's
	// authoritative position. No onGround field; the packet carries x, y, z, yaw, pitch only.
	ParseClientboundMoveVehicle(p pk.Packet) (x, y, z float64, yaw, pitch float64, err error)
}

// ActionHandler handles player action packets (item usage, attacks, etc.).
type ActionHandler interface {
	// SendUseItem sends a use item packet (e.g., start drawing bow, use item in hand).
	// hand: 0=main hand, 1=offhand
	// sequence: anti-cheat sequence number
	// yaw, pitch: player rotation at time of use
	SendUseItem(conn PacketWriter, hand Hand, sequence int32, yaw, pitch float64) error

	// SendPlayerAction sends a player action packet (dig, release bow, swap hands, etc.).
	// status: action ID (see PlayerAction constants)
	// x, y, z: block position (for digging actions, use 0,0,0 for non-position actions)
	// face: block face for digging (0-5)
	// sequence: anti-cheat sequence number
	SendPlayerAction(conn PacketWriter, status int32, x, y, z int, face int32, sequence int32) error

	// SendSwing sends an arm swing animation packet.
	// hand: 0=main hand, 1=offhand
	SendSwing(conn PacketWriter, hand Hand) error
}

// EntityHandler handles entity-related packets.
type EntityHandler interface {
	// ParseAddEntity parses an add entity packet
	// Includes objectData (projectile owner ID) and velocity components
	ParseAddEntity(p pk.Packet) (entityID, entityType, objectData int32, uuid [16]byte, x, y, z float64, yaw, pitch int8, velX, velY, velZ float64, err error)

	// ParseMoveEntityPos parses an entity position update (delta)
	ParseMoveEntityPos(p pk.Packet) (entityID int32, dx, dy, dz int16, onGround bool, err error)

	// ParseMoveEntityPosRot parses an entity position and rotation update (delta)
	ParseMoveEntityPosRot(p pk.Packet) (entityID int32, dx, dy, dz int16, yaw, pitch int8, onGround bool, err error)

	// ParseTeleportEntity parses an entity teleport packet (absolute position)
	ParseTeleportEntity(p pk.Packet) (entityID int32, x, y, z float64, yaw, pitch int8, onGround bool, err error)

	// ParseRemoveEntities parses a remove entities packet
	ParseRemoveEntities(p pk.Packet) (entityIDs []int32, err error)

	// ParseEntityEvent parses an entity event/status packet
	ParseEntityEvent(p pk.Packet) (entityID int32, eventID int8, err error)

	// ParseSetEntityMetadata parses an entity metadata update packet
	// Returns entityID and all metadata entries (caller is responsible for interpreting the data)
	ParseSetEntityMetadata(p pk.Packet) (entityID int32, entries []MetadataEntry, err error)

	// ParseSyncEntityPosition parses a sync entity position packet (absolute position update with velocity)
	// Returns entityID, absolute position (x, y, z), velocity deltas (dx, dy, dz), rotation (yaw, pitch), and onGround
	ParseSyncEntityPosition(p pk.Packet) (entityID int32, x, y, z float64, dx, dy, dz float64, yaw, pitch int8, onGround bool, err error)

	// ParseEntityVelocityUpdate parses an entity velocity update packet
	// Returns entityID and velocity components in blocks per tick
	ParseEntityVelocityUpdate(p pk.Packet) (entityID int32, velX, velY, velZ float64, err error)

	// ParseEntityEquipment parses an entity equipment packet
	// Returns entityID and a slice of equipment entries (InventorySlot + item pairs)
	// Each entry contains a InventorySlot index (0=main hand, 1=off hand, 2-5=armor) and the item data
	ParseEntityEquipment(p pk.Packet) (entityID int32, equipment []EquipmentEntry, err error)

	// ParseEntityEffect parses a status-effect-applied packet
	// (ClientboundEntityEffect, aliased ClientboundUpdateMobEffect). effectID
	// is the minecraft:mob_effect registry ID — resolve to a name via
	// CustomRegistry.GetNameByID the same way entity/item type IDs are
	// resolved elsewhere, not a hardcoded numeric comparison, since registry
	// IDs are not guaranteed stable across versions. amplifier is zero-based
	// (0 = level I). durationTicks is the remaining duration in ticks exactly
	// as sent by the server, not normalized (e.g. infinite-duration effects
	// use a very large tick count on the wire, not a sentinel).
	ParseEntityEffect(p pk.Packet) (entityID, effectID, amplifier, durationTicks int32, ambient, showParticles, showIcon bool, err error)

	// ParseRemoveEntityEffect parses a status-effect-removed packet
	// (ClientboundRemoveEntityEffect, aliased ClientboundRemoveMobEffect).
	ParseRemoveEntityEffect(p pk.Packet) (entityID, effectID int32, err error)

	// ParseEntityHeadRotation parses an entity head rotation packet
	// Returns entityID and head yaw (in 1/256ths of a full turn)
	ParseEntityHeadRotation(p pk.Packet) (entityID int32, headYaw int8, err error)

	// ParseEntityLook parses an entity look packet (rotation only, no position change)
	// Returns entityID, yaw, pitch, and onGround
	ParseEntityLook(p pk.Packet) (entityID int32, yaw, pitch int8, onGround bool, err error)

	// SendInteract sends an entity interaction packet (right-click with hand).
	// entityID: target entity
	// hand: 0=main hand, 1=offhand
	// sneaking: whether player is sneaking
	SendInteract(conn PacketWriter, entityID int32, hand Hand, sneaking bool) error

	// SendInteractAt sends an entity interaction packet at a specific position.
	// entityID: target entity
	// targetX, targetY, targetZ: position on entity to interact with
	// hand: 0=main hand, 1=offhand
	// sneaking: whether player is sneaking
	SendInteractAt(conn PacketWriter, entityID int32, targetX, targetY, targetZ float32, hand Hand, sneaking bool) error

	// SendAttack sends an attack packet to hit an entity (left-click).
	// entityID: target entity
	// sneaking: whether player is sneaking
	SendAttack(conn PacketWriter, entityID int32, sneaking bool) error

	// ParseDamageEvent parses a ClientboundDamageEvent packet.
	// Returns the damaged entity ID, damage source type ID, the entity that caused the damage
	// (sourceCauseID, 0 if none), the entity that directly dealt the damage (sourceDirectID, 0 if none),
	// and an optional source position.
	// sourceCauseID and sourceDirectID use 0 to mean "no entity" (protocol sends ID+1, 0 = absent).
	ParseDamageEvent(p pk.Packet) (entityID int32, sourceTypeID int32, sourceCauseID int32, sourceDirectID int32, sourceX, sourceY, sourceZ float64, hasSourcePosition bool, err error)

	// ParseSetPassengers parses a ClientboundSetPassengers packet.
	// Returns the vehicle entity ID and a list of passenger entity IDs.
	ParseSetPassengers(p pk.Packet) (vehicleEntityID int32, passengerEntityIDs []int32, err error)

	// ParseEntityUpdateAttributes parses a ClientboundEntityUpdateAttributes packet.
	// Returns the entity ID and a map of attribute names to their base value
	// plus every currently active modifier — see AttributeValue.Compute for
	// applying them. Modifiers were previously discarded here entirely (only
	// the base value was read), silently breaking any effect implemented as
	// an attribute modifier (Speed, Slowness) for every entity, not just the
	// walking player, since Phase 3.
	ParseEntityUpdateAttributes(p pk.Packet) (entityID int32, attributes map[string]AttributeValue, err error)
}

// ContainerHandler handles container/inventory packets.
type ContainerHandler interface {
	// SendContainerClick sends a container click packet
	SendContainerClick(conn PacketWriter, windowID int8, stateID, InventorySlot int32, button int8, mode int32, changedSlots map[int16]InventorySlot, carriedItem InventorySlot) error

	// SendContainerClose sends a container close packet
	SendContainerClose(conn PacketWriter, windowID int8) error

	// SendSetCreativeModeSlot sends a creative mode InventorySlot update
	SendSetCreativeModeSlot(conn PacketWriter, InventorySlot int16, item InventorySlot) error

	// SendPickItem sends a pick item packet (for creative mode)
	SendPickItem(conn PacketWriter, InventorySlot int32) error

	// SendSetCarriedItem sends a held item change packet
	SendSetCarriedItem(conn PacketWriter, InventorySlot int16) error

	// SendUseItemOn sends a use item on block packet (right-click on block).
	// This is used for opening containers, placing blocks, and interacting with blocks.
	// hand: 0=main hand, 1=offhand
	// x, y, z: block position
	// face: 0=down, 1=up, 2=north, 3=south, 4=west, 5=east
	// cursorX, cursorY, cursorZ: click position on block face (0.0-1.0)
	// insideBlock: whether the player's head is inside a block
	// sequence: anti-cheat sequence number
	SendUseItemOn(conn PacketWriter, hand Hand, x, y, z int, face int32, cursorX, cursorY, cursorZ float32, insideBlock bool, sequence int32) error

	// ParseOpenScreen parses a container open packet
	ParseOpenScreen(p pk.Packet) (windowID int8, windowType int32, title string, err error)

	// ParseContainerSetContent parses a container content packet
	ParseContainerSetContent(p pk.Packet) (windowID int8, stateID int32, slots []InventorySlot, carriedItem InventorySlot, err error)

	// ParseContainerSetSlot parses a InventorySlot update packet
	ParseContainerSetSlot(p pk.Packet) (windowID int8, stateID int32, InventorySlot int16, item InventorySlot, err error)

	// ParseHeldItemSlot parses a held item InventorySlot packet (handles version differences in InventorySlot type)
	ParseHeldItemSlot(p pk.Packet) (InventorySlot int16, err error)

	// SendContainerButtonClick sends a container button click packet.
	// Used for: enchanting table (InventorySlot 0-2), stonecutter (recipe index),
	// loom (pattern index), beacon (confirm), lectern (page navigation).
	// windowID: container window ID
	// buttonID: button/option to select (meaning depends on container type)
	SendContainerButtonClick(conn PacketWriter, windowID int8, buttonID int8) error
}

// InventorySlot represents an inventory InventorySlot.
type InventorySlot struct {
	Present bool
	ItemID  int32
	Count   int32
	NBT     []byte // Raw NBT data if present
}

// EquipmentEntry represents a single equipment InventorySlot update (InventorySlot + item).
type EquipmentEntry struct {
	InventorySlot int32         // Equipment InventorySlot (0=main hand, 1=off hand, 2-5=armor)
	Item          InventorySlot // The item in this equipment InventorySlot
}

// ChatHandler handles chat and command packets.
type ChatHandler interface {
	// SendChat sends a chat message
	SendChat(conn PacketWriter, message string) error

	// SendCommand sends a command (without the leading slash)
	SendCommand(conn PacketWriter, command string) error

	// ParseSystemChat parses a system chat message
	ParseSystemChat(p pk.Packet) (message string, overlay bool, err error)

	// ParsePlayerChat parses a player chat message
	ParsePlayerChat(p pk.Packet) (senderUUID [16]byte, message string, err error)

	// ParseDisguisedChat parses a disguised chat message
	ParseDisguisedChat(p pk.Packet) (message string, err error)
}

// WorldHandler handles world-related packets (chunks, blocks, etc.).
type WorldHandler interface {
	// ParseBlockUpdate parses a single block update packet
	ParseBlockUpdate(p pk.Packet) (x, y, z int64, blockStateID int32, err error)

	// ParseSectionBlocksUpdate parses a multi-block update packet
	ParseSectionBlocksUpdate(p pk.Packet) (sectionPos int64, blocks []BlockUpdate, err error)

	// ParseChunkData parses a chunk data packet
	// Returns the chunk X/Z coordinates and raw chunk data for further processing
	ParseChunkData(p pk.Packet) (chunkX, chunkZ int32, data []byte, err error)

	// ParseUnloadChunk parses a chunk unload packet
	ParseUnloadChunk(p pk.Packet) (chunkX, chunkZ int32, err error)

	// ParseUpdateTime parses the ClientboundUpdateTime packet.
	// Returns the world age (ticks since world creation) and time of day (ticks in current day).
	ParseUpdateTime(p pk.Packet) (worldAge, timeOfDay int64, err error)

	// SendChunkBatchReceived sends an acknowledgment for received chunk batches.
	// This is required in 1.20.2+ to signal the server that the client is ready for more chunks.
	// The batchCount parameter is the cumulative number of batches received so far.
	SendChunkBatchReceived(conn PacketWriter, batchCount float32) error

	// ParseExplosion parses a ClientboundExplosion packet.
	// hasKnockback reports whether the explosion pushed the receiving player;
	// if true, (knockbackX, knockbackY, knockbackZ) is a velocity DELTA to
	// ADD to the player's current velocity, not a replacement — mirroring
	// Java's Entity.addVelocityInternal (this.setVelocity(this.getVelocity().add(velocity))).
	// 1.21.1 sends an always-present PlayerMotionX/Y/Z triple instead of an
	// optional field; hasKnockback is always true there (a (0,0,0) add is a
	// harmless no-op, matching that version's own client, which applies it
	// unconditionally).
	ParseExplosion(p pk.Packet) (hasKnockback bool, knockbackX, knockbackY, knockbackZ float64, err error)
}

// BlockUpdate represents a single block update within a section.
type BlockUpdate struct {
	X, Y, Z      int64
	BlockStateID int32
}
