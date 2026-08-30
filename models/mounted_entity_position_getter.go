package models

// MountedEntityPositionGetter provides the ability to retrieve the current position of a mounted entity.
// This is used by the movement executor when in riding mode to track the vehicle's position.
type MountedEntityPositionGetter interface {
	// GetMountedEntityPosition returns the current position of the mounted entity by ID.
	// Returns (x, y, z, found) - found is false if the entity is not tracked.
	GetMountedEntityPosition(entityID int32) (x, y, z float64, found bool)

	// GetMountedEntityYaw returns the yaw of the mounted entity in degrees.
	// Returns (yaw, found) - found is false if the entity is not tracked.
	GetMountedEntityYaw(entityID int32) (yaw float64, found bool)

	GetMountedEntityType(entityID int32) (int32, bool)
	IsMountedEntityBoat(int32) bool
	IsMountedEntityMinecart(int32) bool
	IsMountedEntityCamel(int32) bool
	IsMountedEntityNautilus(int32) bool
	IsMountedEntityZombieNautilus(int32) bool
	IsMountedEntityPig(int32) bool
	IsMountedEntityStrider(int32) bool
	IsMountedEntityDonkey(int32) bool
	IsMountedEntityMule(int32) bool

	// IsMountedEntityLlama reports whether the mounted entity is a llama or
	// trader llama. Llamas can be ridden but not steered, so they need the
	// passive-passenger handler rather than a controlling-passenger one.
	IsMountedEntityLlama(int32) bool

	// IsMountedEntityHappyGhast reports whether the mounted entity is a
	// happy ghast (1.21.6+).
	IsMountedEntityHappyGhast(int32) bool

	// IsMountedEntityHappyGhastStayingStill reports whether the mounted
	// happy ghast currently has no controlling passenger because it's
	// "staying still" (Java HappyGhastEntity.method_72227): either a player
	// is standing on top of it, or it's within the settle window after a
	// mount/dismount change. While true, not even the pilot in seat 0 is the
	// controlling passenger, so the client must not predict movement or send
	// VehicleMove — see PHASE_6_PLAN.md §3.2/§3.5.
	//
	// known is false before the server's first SetEntityMetadata for this
	// field arrives.
	IsMountedEntityHappyGhastStayingStill(entityID int32) (stayingStill bool, known bool)

	// IsMountedEntityHarnessed reports whether the given happy ghast has a
	// harness equipped in the body slot. Unlike IsMountedEntitySaddled,
	// presence in the slot alone is not sufficient evidence — slot 6
	// ("body") has no harness-specific protocol semantics on its own, so
	// this also checks the equipped item's registry name ends in "harness".
	//
	// known is false when the entity isn't tracked or the item registry
	// isn't ready yet — not when the slot is simply empty (that's a
	// confident "not harnessed").
	IsMountedEntityHarnessed(entityID int32) (harnessed bool, known bool)

	// GetMountedPassengerIndex returns our seat index in the mounted vehicle's
	// passenger list, or -1 when not mounted. Index 0 is the controlling
	// passenger; every later index is a passive rider that must not predict the
	// vehicle's movement. Boats and camels seat two, and a happy ghast seats
	// four, so this is what separates the driver from the passengers.
	GetMountedPassengerIndex() int

	// IsMountedEntitySaddled reports whether the given mount has a saddle
	// equipped. Vanilla only makes a rider the controlling passenger of a
	// saddleable mount when it is actually saddled
	// (AbstractHorseEntity.isSaddled), so an unsaddled mount must be ridden
	// passively.
	//
	// known is false when saddle state cannot be determined from the wire. That
	// is the case before 1.21.5, where the saddle lived in the mount's NBT
	// inventory rather than an equipment slot and was never sent to the client.
	// Callers should treat unknown as "assume saddled" so older versions keep
	// their existing behaviour rather than silently losing control of a mount.
	IsMountedEntitySaddled(entityID int32) (saddled bool, known bool)

	// GetEntityAttribute retrieves an entity attribute value by name.
	// Returns (value, found) - found is false if the entity or attribute is not tracked.
	// Common attributes: "generic.movement_speed", "generic.max_health", etc.
	GetEntityAttribute(entityID int32, attributeName string) (float64, bool)

	// GetEntityAttributeDefault retrieves the data-driven vanilla default for
	// an attribute, keyed by the entity's actual type (e.g. a donkey gets
	// donkey's default, not horse's) rather than a hardcoded per-handler
	// constant. Returns (value, found) - found is false if the entity isn't
	// tracked, its type isn't in the loaded mc-data-gen export, or the
	// attribute isn't one that entity type carries.
	//
	// This is a fallback of a fallback: callers should still prefer
	// GetEntityAttribute's live, server-sourced value whenever it's found,
	// and only fall through here (then to their own hardcoded literal) when
	// it isn't — e.g. the brief window before the server's first
	// ClientboundEntityUpdateAttributes packet for a freshly-mounted entity
	// arrives. See PHASE_7_PLAN.md.
	GetEntityAttributeDefault(entityID int32, attributeName string) (float64, bool)

	// GetEntityVelocity returns the current velocity of an entity by ID.
	// Returns (velX, velY, velZ, found) in Minecraft protocol units (×8000 blocks/tick).
	// found is false if the entity is not tracked.
	GetEntityVelocity(entityID int32) (velX, velY, velZ float64, found bool)

	// GetRiderHeldItem returns the local item name (e.g. "carrot_on_a_stick") of the rider's
	// currently held item in their active hotbar slot. Returns ("", false) if no item found.
	// The name has no namespace prefix.
	GetRiderHeldItem() (itemName string, found bool)

	// GetOwnActiveEffect reports whether the agent's own player entity
	// currently has the named status effect active (e.g.
	// "minecraft:slow_falling", "minecraft:levitation" — the full
	// minecraft:mob_effect registry name, unlike GetRiderHeldItem's
	// unprefixed item names, since effect names are resolved dynamically via
	// CustomRegistry.GetNameByID rather than a stable client-side mapping).
	// amplifier is zero-based (0 = level I) and only meaningful when found
	// is true.
	GetOwnActiveEffect(effectName string) (amplifier int32, found bool)

	// GetOwnEquippedChestItem returns the local item name (unprefixed, e.g.
	// "elytra") of whatever the agent's own player currently has equipped
	// in its chest armor slot (player inventory slot index 6). Returns
	// ("", false) if the slot is empty or not yet resolvable — mirrors
	// GetRiderHeldItem's naming/resolution shape but for the chest slot.
	GetOwnEquippedChestItem() (itemName string, found bool)

	// HasActiveFireworkBoost reports whether a firework rocket used while
	// gliding is currently attached to the agent's own entity and boosting
	// its velocity (Java FireworkRocketEntity's SHOOTER_ENTITY_ID pointing
	// back at the agent). True for as long as that firework entity exists —
	// vanilla re-checks isGliding() every tick the firework is alive, so
	// this alone doesn't guarantee the boost is currently being applied,
	// only that a firework is attached; the gliding check itself happens
	// separately in physics.State.
	HasActiveFireworkBoost() bool

	// GetOwnFlying reports whether the agent's own player currently has
	// creative/spectator-style flying active (PlayerAbilities.Flying), and
	// the vertical ascend/descend impulse scale to use while so
	// (PlayerAbilities.FlySpeed, vanilla default 0.05). flySpeed is only
	// meaningful when flying is true.
	GetOwnFlying() (flying bool, flySpeed float64)
}
