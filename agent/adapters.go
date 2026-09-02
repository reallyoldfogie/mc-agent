package agent

import (
	"strings"

	pk "github.com/Tnze/go-mc/net/packet"
	semver "github.com/aquasecurity/go-version/pkg/version"

	"github.com/reallyoldfogie/mc-agent/models"
	"github.com/reallyoldfogie/mc-agent/pathfinding"
	"github.com/reallyoldfogie/mc-agent/physics"
	bot "github.com/reallyoldfogie/mc-bot-go/bot"
	protocol_models "github.com/reallyoldfogie/mc-protocol-go/models"
)

// Bot client adapters to satisfy our narrow interfaces without leaking dependencies.

// type botClientAdapter struct {
// 	c *bot.Client
// }

// func (b *botClientAdapter) JoinServerWithOptions(ctx context.Context, address string, opts JoinOptions) error {
// 	var rec bot.PacketRecorder
// 	var mirror bot.MovementMirror
// 	if opts.ReplayRecorder != nil {
// 		rec = packetRecorderAdapter{rec: opts.ReplayRecorder}
// 	}
// 	if opts.MovementMirror != nil {
// 		mirror = movementMirrorAdapter{m: opts.MovementMirror}
// 	}
// 	return b.c.JoinServerWithOptions(ctx, address, bot.JoinOptions{
// 		ProtocolVersion:      opts.ProtocolVersion,
// 		ReplayRecorder:       rec,
// 		MovementMirror:       mirror,
// 		RegistryDataCallback: opts.RegistryDataCallback,
// 	})
// }

// func (b *botClientAdapter) Events() EventBus {
// 	return &botEventBusAdapter{events: &b.c.Events}
// }

// func (b *botClientAdapter) Name() string { return b.c.Name }

// func (b *botClientAdapter) HandleGame(ctx context.Context) error { return b.c.HandleGame(ctx) }

// func (b *botClientAdapter) WritePacket(p pk.Packet) error { return b.c.Conn.WritePacket(p) }

// // BotClient returns the underlying *bot.Client for internal use by movement executor, etc.
// func (b *botClientAdapter) BotClient() *bot.Client { return b.c }

// botEventBusAdapter wraps bot.Events and converts PacketHandler shapes.
type botEventBusAdapter struct {
	events bot.Events
}

func (e *botEventBusAdapter) AddGeneric(listeners ...bot.PacketHandler) {
	converted := make([]bot.PacketHandler, 0, len(listeners))
	for _, l := range listeners {
		h := l
		converted = append(converted, bot.PacketHandler{ID: protocol_models.ClientboundPacketID(h.ID), Priority: h.Priority, F: func(p pk.Packet) error { return h.F(p) }})
	}
	e.events.AddGeneric(converted...)
}

func (e *botEventBusAdapter) AddListener(listeners ...bot.PacketHandler) {
	converted := make([]bot.PacketHandler, 0, len(listeners))
	for _, l := range listeners {
		h := l
		converted = append(converted, bot.PacketHandler{ID: protocol_models.ClientboundPacketID(h.ID), Priority: h.Priority, F: func(p pk.Packet) error { return h.F(p) }})
	}
	e.events.AddListener(converted...)
}

// NewClientFromBot wraps a concrete bot.Client as an Agent Client.
// func NewClientFromBot(c *bot.Client) Client { return &botClientAdapter{c: c} }

// packetRecorderAdapter bridges agent.PacketRecorder to bot.PacketRecorder without
// requiring the concrete recorder type here.
type packetRecorderAdapter struct {
	rec PacketRecorder
}

func (p packetRecorderAdapter) RecordNow(id int32, payload []byte) error {
	return p.rec.RecordNow(id, payload)
}
func (p packetRecorderAdapter) SetSelfID(id int)      { p.rec.SetSelfID(id) }
func (p packetRecorderAdapter) AddPlayer(uuid string) { p.rec.AddPlayer(uuid) }

// movementMirrorAdapter bridges agent.MovementMirror to bot.MovementMirror.
type movementMirrorAdapter struct {
	m MovementMirror
}

func (m movementMirrorAdapter) HandleServerbound(p pk.Packet) { m.m.HandleServerbound(p) }
func (m movementMirrorAdapter) SetEntityMeta(id int32, name string, uuid [16]byte) {
	m.m.SetEntityMeta(id, name, uuid)
}
func (m movementMirrorAdapter) SetEntityType(entityType int32) { m.m.SetEntityType(entityType) }
func (m movementMirrorAdapter) HandlePlayerInfo(p pk.Packet)   { m.m.HandlePlayerInfo(p) }

// GetMountedEntityPosition returns the position of a mounted entity by ID.
// This implements the MountedEntityPositionGetter interface for use by the movement executor.
func (a *agent) GetMountedEntityPosition(entityID int32) (x, y, z float64, found bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, 0, 0, false
	}

	return entity.X, entity.Y, entity.Z, true
}

// GetMountedEntityYaw returns the yaw of a mounted entity in degrees.
// The entity tracker stores yaw as a packed int8 (0–255 covering 0–360 degrees).
func (a *agent) GetMountedEntityYaw(entityID int32) (yaw float64, found bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, false
	}

	// Convert packed angle (int8, 0–255 = 0°–360°) to degrees.
	yaw = float64(entity.Yaw) * 360.0 / 256.0
	return yaw, true
}

// GetMountedEntityType returns the entity type ID of a mounted entity by ID.
// Returns the entity type and found flag.
func (a *agent) GetMountedEntityType(entityID int32) (int32, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, false
	}

	return entity.EntityType, true
}

// IsMountedEntityBoat checks if a mounted entity is a boat or raft by type ID.
// Returns true if the entity type's local name (after the namespace prefix)
// ends with "_boat" or "_raft" — e.g. oak_boat, oak_chest_boat, bamboo_raft,
// bamboo_chest_raft, pale_oak_boat, etc.
//
// The suffix is matched against the local part of the identifier rather than
// the full string because the namespace "minecraft" itself contains the
// substring "raft" (mineCRAFT), so a naive Contains check would classify every
// minecraft-namespaced entity as a boat.
func (a *agent) IsMountedEntityBoat(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	// Strip the namespace prefix (e.g., "minecraft:") so the suffix check is
	// applied to the local name only.
	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	// In 1.21.1, boats use a single entity type "boat" / "chest_boat".
	// In 1.21.2+, boats are per-wood-type: "oak_boat", "birch_chest_boat", etc.
	rval := localName == "boat" || localName == "chest_boat" ||
		strings.HasSuffix(localName, "_boat") || strings.HasSuffix(localName, "_raft")

	a.logf("[IsMountedEntityBoat] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityCamel checks if a mounted entity is a camel (or camel_husk) by type ID.
// Returns true if the entity type's local name (after the namespace prefix)
// is "camel" or "camel_husk".
func (a *agent) IsMountedEntityCamel(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "camel" || localName == "camel_husk"

	a.logf("[IsMountedEntityCamel] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityCamelHusk checks if a mounted entity is specifically a camel_husk (1.21.11+).
func (a *agent) IsMountedEntityCamelHusk(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	return localName == "camel_husk"
}

// IsMountedEntityNautilus checks if a mounted entity is a nautilus (or zombie_nautilus) by type ID.
// Nautilus is a 1.21.11+ rideable underwater mob.
func (a *agent) IsMountedEntityNautilus(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "nautilus" || localName == "zombie_nautilus"

	a.logf("[IsMountedEntityNautilus] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityZombieNautilus checks if a mounted entity is specifically a
// zombie_nautilus by type ID. Used to select the zombie's higher base movement
// speed (1.1 vs 1.0) when the server has not sent generic.movement_speed.
func (a *agent) IsMountedEntityZombieNautilus(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	return localName == "zombie_nautilus"
}

// IsMountedEntityMinecart checks if a mounted entity is a minecart by type ID.
// Returns true if the entity type's local name (after the namespace prefix)
// is "minecart" or ends with "_minecart" — e.g., minecart, chest_minecart,
// furnace_minecart, hopper_minecart, tnt_minecart, etc.
func (a *agent) IsMountedEntityMinecart(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	// Strip the namespace prefix (e.g., "minecraft:") so the suffix check is
	// applied to the local name only.
	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "minecart" || strings.HasSuffix(localName, "_minecart")

	a.logf("[IsMountedEntityMinecart] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityPig checks if a mounted entity is a pig by type ID.
func (a *agent) IsMountedEntityPig(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "pig"

	a.logf("[IsMountedEntityPig] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityStrider checks if a mounted entity is a strider by type ID.
func (a *agent) IsMountedEntityStrider(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "strider"

	a.logf("[IsMountedEntityStrider] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityDonkey checks if a mounted entity is a donkey by type ID.
func (a *agent) IsMountedEntityDonkey(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "donkey"

	a.logf("[IsMountedEntityDonkey] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityMule checks if a mounted entity is a mule by type ID.
func (a *agent) IsMountedEntityMule(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "mule"

	a.logf("[IsMountedEntityMule] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityLlama checks if a mounted entity is a llama or trader llama
// by type ID. Both accept a passenger but take no saddle and ignore rider
// input, so they route to the passive-passenger handler.
func (a *agent) IsMountedEntityLlama(entityTypeID int32) bool {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return false
	}

	localName := entityTypeName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	rval := localName == "llama" || localName == "trader_llama"

	a.logf("[IsMountedEntityLlama] entityTypeID: %d entityTypeName: %s rval: %t", entityTypeID, entityTypeName, rval)

	return rval
}

// IsMountedEntityHappyGhast checks if a mounted entity is a happy ghast
// (1.21.6+) by type ID.
func (a *agent) IsMountedEntityHappyGhast(entityTypeID int32) bool {
	localName, ok := a.entityTypeLocalName(entityTypeID)
	return ok && localName == "happy_ghast"
}

// IsMountedEntityHappyGhastStayingStill reports whether the given happy
// ghast currently has no controlling passenger because it's "staying still"
// (metadata key 18). See models.MountedEntityPositionGetter's doc comment
// for why this matters.
func (a *agent) IsMountedEntityHappyGhastStayingStill(entityID int32) (stayingStill bool, known bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists {
		return false, false
	}
	return entity.HappyGhastStayingStill, entity.HasHappyGhastStayingStill
}

// IsMountedEntityHarnessed reports whether the given happy ghast has a
// harness equipped in the body slot. Slot occupancy alone isn't sufficient
// evidence (see the interface doc comment), so this also resolves the
// equipped item's registry name and checks it ends in "harness" — matching
// every one of the sixteen dyed harness items without hardcoding all sixteen
// names.
func (a *agent) IsMountedEntityHarnessed(entityID int32) (harnessed bool, known bool) {
	a.entitiesMu.RLock()
	entity, exists := a.entities[entityID]
	a.entitiesMu.RUnlock()
	if !exists {
		return false, false
	}

	bodySlot, hasBodySlot := entity.Equipment[models.EquipmentSlotBody]
	if !hasBodySlot || bodySlot.Count <= 0 {
		return false, true
	}

	itemReg := a.GetRegistry("minecraft:item")
	if itemReg == nil || !itemReg.IsReady() {
		return false, false
	}
	itemName, ok := itemReg.GetNameByID(bodySlot.ItemID)
	if !ok {
		return false, false
	}
	localName := itemName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}
	return strings.HasSuffix(localName, "harness"), true
}

// IsMountedEntitySaddled reports whether a mount has a saddle equipped.
//
// This mirrors how the vanilla client decides, which differs by version but
// shares one crucial property: **the default state is "not saddled", and the
// server never transmits a negative**.
//
//   - 1.21.5+ reads the saddle equipment slot
//     (AbstractHorseEntity.getControllingPassenger → MobEntity.hasSaddleEquipped
//     → LivingEntity.hasStackEquipped(SADDLE)). EntityTrackerEntry.sendPackets
//     builds the equipment list from non-empty slots only and skips the packet
//     entirely when nothing is equipped.
//   - Before 1.21.5 it reads AbstractHorseEntity.isSaddled(), the SADDLED bit of
//     the horse flags metadata byte. DataTracker.getChangedEntries filters out
//     entries still at their default, so an all-zero flags byte is never sent.
//
// So in both cases the absence of a positive signal *is* the answer, and this
// reports known=true without waiting for anything. That is safe because the
// positive signal ships inside EntityTrackerEntry.sendPackets, the same bundle
// as the spawn packet — a saddled mount is known to be saddled long before it
// can be mounted.
//
// known is false only when the version itself is unavailable (no version
// handler), since the two paths read completely different fields.
func (a *agent) IsMountedEntitySaddled(entityID int32) (saddled bool, known bool) {
	version := ""
	if a.versionHandler != nil {
		version = a.versionHandler.Version()
	}
	if version == "" {
		return false, false
	}

	usesEquipmentSlot, versionKnown := saddleSlotSupported(version)
	if !versionKnown {
		return false, false
	}

	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists {
		// We cannot even see the entity, so we have no basis to demote the
		// rider. Report unknown and let the caller keep its behaviour.
		return false, false
	}

	if usesEquipmentSlot {
		return equipmentHasSaddle(entity.Equipment), true
	}
	return models.HorseFlagSaddled.IsSet(entity.HorseFlags), true
}

// saddleSlotSupported reports whether a Minecraft version carries the saddle in
// the equipment packet (1.21.5+) rather than in the horse flags metadata byte.
//
// versionKnown is false when the version string cannot be parsed, in which case
// neither path can be trusted.
func saddleSlotSupported(version string) (usesEquipmentSlot bool, versionKnown bool) {
	if version == "" {
		return false, false
	}
	parsed, err := semver.Parse(version)
	if err != nil {
		return false, false
	}
	constraint, err := semver.NewConstraints(">= " + models.MinSaddleSlotVersion)
	if err != nil {
		return false, false
	}
	return constraint.Check(parsed), true
}

// isSaddleableMountType reports whether an entity type ID is an
// AbstractHorseEntity subclass, which is what makes metadata key 17 the horse
// flags byte rather than something else.
//
// Horse and camel extend AbstractHorseEntity directly; donkey, mule and llama
// reach it through AbstractDonkeyEntity. Llamas are included because they do
// carry the flags byte even though they can never be saddled.
func (a *agent) isSaddleableMountType(entityTypeID int32) bool {
	localName, ok := a.entityTypeLocalName(entityTypeID)
	if !ok {
		return false
	}

	switch localName {
	case "horse", "skeleton_horse", "zombie_horse",
		"donkey", "mule",
		"llama", "trader_llama",
		"camel", "camel_husk":
		return true
	default:
		return false
	}
}

// isFireworkRocketEntityType reports whether the given entity type is a
// firework_rocket — used to gate reading
// models.EntityMetadataKeyFireworkShooterEntityID, which numerically
// collides with EntityMetadataKeyHealth on living entities.
func (a *agent) isFireworkRocketEntityType(entityTypeID int32) bool {
	localName, ok := a.entityTypeLocalName(entityTypeID)
	return ok && localName == "firework_rocket"
}

// entityTypeLocalName resolves an entity type ID (as carried on spawn/equipment
// packets) to its bare registry name with the namespace stripped (e.g.
// "minecraft:camel" -> "camel"). ok is false when the entity-type registry
// isn't ready yet or the ID isn't in it.
//
// Extracted from isSaddleableMountType, which needed this exact resolution
// before GetEntityAttributeDefault needed it too — see PHASE_7_PLAN.md §4.4's
// note on avoiding a second copy of this lookup.
func (a *agent) entityTypeLocalName(entityTypeID int32) (string, bool) {
	a.regMu.RLock()
	defer a.regMu.RUnlock()

	entityTypeReg := a.registries[RegistryID("minecraft:entity_type")]
	if entityTypeReg == nil || !entityTypeReg.IsReady() {
		return "", false
	}

	entityTypeName, ok := entityTypeReg.GetNameByID(entityTypeID)
	if !ok {
		return "", false
	}

	if idx := strings.IndexByte(entityTypeName, ':'); idx >= 0 {
		entityTypeName = entityTypeName[idx+1:]
	}
	return entityTypeName, true
}

// equipmentHasSaddle reports whether an equipment map has an occupied saddle
// slot.
//
// Only occupancy is consulted, not item identity: the version handlers still
// leave EquipmentEntry.Item.ItemID at 0 (see trackedEntity.Equipment). That is
// sufficient because the slot itself carries the meaning — nothing but a saddle
// goes in the saddle slot.
func equipmentHasSaddle(equipment map[models.EquipmentSlotType]models.InventorySlot) bool {
	saddleItem, hasSaddleSlot := equipment[models.EquipmentSlotSaddle]
	return hasSaddleSlot && saddleItem.Count > 0
}

// GetEntityAttribute retrieves an entity attribute's final value (base plus
// every live modifier applied the way vanilla does — see
// models.AttributeValue.Compute) by name. Returns the value and a found
// flag. Common attributes include:
// - "generic.movement_speed" (horse speed, etc.)
// - "generic.max_health" (health cap)
// - "generic.attack_damage" (etc.)
func (a *agent) GetEntityAttribute(entityID int32, attributeName string) (float64, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed || entity.Attributes == nil {
		return 0, false
	}

	value, ok := entity.Attributes[attributeName]
	if !ok {
		return 0, false
	}
	return value.Compute(), true
}

// GetEntityAttributeDefault retrieves the data-driven vanilla default for an
// attribute, keyed by the entity's actual type rather than a hardcoded
// per-handler constant. See
// models.MountedEntityPositionGetter.GetEntityAttributeDefault for the
// fallback-of-a-fallback contract.
func (a *agent) GetEntityAttributeDefault(entityID int32, attributeName string) (float64, bool) {
	a.entitiesMu.RLock()
	entity, exists := a.entities[entityID]
	a.entitiesMu.RUnlock()
	if !exists {
		return 0, false
	}

	localName, ok := a.entityTypeLocalName(entity.EntityType)
	if !ok {
		return 0, false
	}

	// a.attributeDefaults may be nil if agent startup hasn't wired it yet
	// (e.g. in tests); EntityAttributeDefaultsRegistry.Get is nil-safe.
	return a.attributeDefaults.Get(models.EntityType(localName), attributeName)
}

// GetEntityVelocity returns the current velocity of an entity.
// Returns (velX, velY, velZ, found) in Minecraft protocol units (×8000 blocks/tick).
// found is false if the entity is not tracked.
func (a *agent) GetEntityVelocity(entityID int32) (float64, float64, float64, bool) {
	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()

	entity, exists := a.entities[entityID]
	if !exists || entity.Removed {
		return 0, 0, 0, false
	}

	return entity.VelX, entity.VelY, entity.VelZ, true
}

// GetRiderHeldItem returns the local item name (e.g. "carrot_on_a_stick") of the rider's
// currently held item in their active hotbar slot. Returns ("", false) if no item found.
// The name has no namespace prefix.
func (a *agent) GetRiderHeldItem() (string, bool) {
	// Get held slot index (0-8 for hotbar)
	a.heldSlotMu.RLock()
	slot := a.heldSlot
	a.heldSlotMu.RUnlock()

	a.slotsMu.RLock()
	slotResolver := a.slots
	a.slotsMu.RUnlock()

	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()

	if slotResolver == nil || itemMgr == nil {
		return "", false
	}

	// Player inventory hotbar slots are indexes 36-44.
	itemID, _, ok := slotResolver.ResolveSlot(-2, 36+slot)
	if !ok {
		return "", false
	}

	itemName := itemMgr.GetItemNameByID(itemID)
	if itemName == "" {
		return "", false
	}

	// Strip the namespace prefix (e.g., "minecraft:") to get local name
	localName := itemName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	return localName, true
}

// GetOwnActiveEffect reports whether the agent's own player entity currently
// has the named status effect active. See
// models.MountedEntityPositionGetter.GetOwnActiveEffect for the naming
// convention (full "minecraft:xxx" registry name, unlike GetRiderHeldItem).
func (a *agent) GetOwnActiveEffect(effectName string) (int32, bool) {
	a.ownEffectsMu.RLock()
	defer a.ownEffectsMu.RUnlock()

	if a.ownEffects == nil {
		return 0, false
	}
	effect, ok := a.ownEffects[effectName]
	if !ok {
		return 0, false
	}
	return effect.Amplifier, true
}

// GetOwnEquippedChestItem returns the local item name (unprefixed, e.g.
// "elytra") in the agent's own chest armor slot. See
// models.MountedEntityPositionGetter.GetOwnEquippedChestItem.
func (a *agent) GetOwnEquippedChestItem() (string, bool) {
	a.slotsMu.RLock()
	slotResolver := a.slots
	a.slotsMu.RUnlock()

	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()

	if slotResolver == nil || itemMgr == nil {
		return "", false
	}

	// Player inventory chest armor slot is index 6 (0=craft output,
	// 1-4=craft grid, 5=head, 6=chest, 7=legs, 8=feet, 9-35=main, 36-44=hotbar).
	itemID, _, ok := slotResolver.ResolveSlot(-2, 6)
	if !ok {
		return "", false
	}

	itemName := itemMgr.GetItemNameByID(itemID)
	if itemName == "" {
		return "", false
	}

	// Strip the namespace prefix (e.g., "minecraft:") to get local name
	localName := itemName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	return localName, true
}

// GetOwnEquippedFeetItem returns the local item name (unprefixed, e.g.
// "leather_boots") in the agent's own feet armor slot. See
// models.MountedEntityPositionGetter.GetOwnEquippedFeetItem.
func (a *agent) GetOwnEquippedFeetItem() (string, bool) {
	a.slotsMu.RLock()
	slotResolver := a.slots
	a.slotsMu.RUnlock()

	a.itemMgrMu.RLock()
	itemMgr := a.itemMgr
	a.itemMgrMu.RUnlock()

	if slotResolver == nil || itemMgr == nil {
		return "", false
	}

	// Player inventory feet armor slot is index 8 (0=craft output,
	// 1-4=craft grid, 5=head, 6=chest, 7=legs, 8=feet, 9-35=main, 36-44=hotbar).
	itemID, _, ok := slotResolver.ResolveSlot(-2, 8)
	if !ok {
		return "", false
	}

	itemName := itemMgr.GetItemNameByID(itemID)
	if itemName == "" {
		return "", false
	}

	// Strip the namespace prefix (e.g., "minecraft:") to get local name
	localName := itemName
	if idx := strings.IndexByte(localName, ':'); idx >= 0 {
		localName = localName[idx+1:]
	}

	return localName, true
}

// GetOwnFlying reports the agent's own tracked flying ability state. See
// models.MountedEntityPositionGetter.GetOwnFlying.
func (a *agent) GetOwnFlying() (flying bool, flySpeed float64) {
	abilities, _ := a.GetPlayerAbilities()
	return abilities.Flying, float64(abilities.FlySpeed)
}

// IsSpectator reports whether the agent's own tracked game mode is
// spectator. See models.MountedEntityPositionGetter.IsSpectator.
func (a *agent) IsSpectator() bool {
	gameMode, _ := a.GetGameMode()
	return gameMode == models.GameModeSpectator
}

// HasActiveFireworkBoost reports whether a firework rocket used while
// gliding is currently attached to (boosting) the agent's own entity. See
// models.MountedEntityPositionGetter.HasActiveFireworkBoost. Computed live
// by scanning tracked entities rather than a separately maintained flag, so
// it can never go stale relative to a.entities: a firework stops boosting
// the instant its RemoveEntities packet is processed and it drops out of
// the map.
func (a *agent) HasActiveFireworkBoost() bool {
	ownID := a.GetEntityID()
	if ownID == 0 {
		return false
	}

	a.entitiesMu.RLock()
	defer a.entitiesMu.RUnlock()
	for _, e := range a.entities {
		if !a.isFireworkRocketEntityType(e.EntityType) {
			continue
		}
		if e.HasFireworkShooterEntityID && e.FireworkShooterEntityID == ownID {
			return true
		}
	}
	return false
}

// perceptionRadiusCap clamps radius to the agent's own effective vision
// range under Blindness/Darkness (see physics.PerceptionRadiusCap). Shared
// by every search function's honorPerceptionEffects=true path.
func (a *agent) perceptionRadiusCap(radius float64) float64 {
	_, hasBlindness := a.GetOwnActiveEffect("minecraft:blindness")
	_, hasDarkness := a.GetOwnActiveEffect("minecraft:darkness")
	return physics.PerceptionRadiusCap(radius, hasBlindness, hasDarkness)
}

// FindRideableEntitiesNear returns all rideable entities within a given
// radius of a center position. See models.Agent.FindNearestEntityByType's
// doc comment for honorPerceptionEffects' contract.
func (a *agent) FindRideableEntitiesNear(center models.V3, radius float64, honorPerceptionEffects bool) []pathfinding.RideableEntity {
	result := make([]pathfinding.RideableEntity, 0)

	if honorPerceptionEffects {
		radius = a.perceptionRadiusCap(radius)
	}

	a.entitiesMu.RLock()
	entities := a.entities
	a.entitiesMu.RUnlock()

	radiusSq := radius * radius

	for entityID, tracked := range entities {
		// Skip if entity is too far
		distSq := (tracked.X-center.X)*(tracked.X-center.X) +
			(tracked.Y-center.Y)*(tracked.Y-center.Y) +
			(tracked.Z-center.Z)*(tracked.Z-center.Z)

		if distSq > radiusSq {
			continue
		}

		// Check if entity type is rideable. tracked.EntityType is the raw
		// numeric protocol registry ID, not a models.EntityType string, so it
		// must be resolved through entityRegistry (populated by onAddEntity
		// from the minecraft:entity_type registry) rather than converted
		// directly — models.EntityType(tracked.EntityType) would silently
		// convert the int32 to a one-rune string via Go's int-to-string
		// conversion, which can never equal a real type name like "horse",
		// making every entity look non-rideable regardless of its actual type.
		if a.entityRegistry == nil {
			continue
		}
		entityTypeID := a.entityRegistry.GetEntityType(entityID)
		if !entityTypeID.IsRideable() {
			continue
		}

		result = append(result, pathfinding.RideableEntity{
			EntityID:   entityID,
			EntityType: entityTypeID,
			Position:   models.V3{X: tracked.X, Y: tracked.Y, Z: tracked.Z},
		})
	}

	return result
}
