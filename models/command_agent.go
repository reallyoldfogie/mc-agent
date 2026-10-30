package models

import "context"

// CommandAgent is the minimal surface required by command actions.
type CommandAgent interface {
	MovementAgent
	ChatOperations

	// MoveToWithChat is MoveTo(ctx, x, y, z, notifyChat=true) — narrates
	// progress/arrival/failure to chat, appropriate for a human-issued
	// "moveTo" chat command. See MoveTo for the quiet alternative.
	MoveToWithChat(ctx context.Context, x, y, z float64) error
	// MoveTo is MoveToWithChat's own underlying implementation, exposed
	// directly so a caller that dispatches movement far more often than a
	// human types a chat command (rlenv's RL training loop, via
	// actions.MoveToQuiet — docs/plans/RL_TRAINING_LOOP_PLAN.md) can pass
	// notifyChat=false. Found live, not anticipated: MoveToWithChat's
	// "Already at target position"/"Navigating..." chat messages, sent on
	// every dispatch, got an RL-driven session kicked from a real server
	// for spamming once rollout collection started dispatching movement
	// actions fast enough to repeat the same message many times a second —
	// see MoveToWithChat's own doc comment.
	MoveTo(ctx context.Context, x, y, z float64, notifyChat bool) error
	LineTo(ctx context.Context, x, y, z float64, notifyChat bool) error

	TestMove()
	TestPath()
	StartTracking()
	StopTracking()

	HasFollowManager() bool
	IsFollowing() bool

	// IsSprinting reports whether the movement executor currently considers
	// the agent to be sprinting. Returns false if no movement executor is
	// available.
	IsSprinting() bool

	PlanStatus(ctx context.Context) PlanStatus
	StopPlan(ctx context.Context) error

	FireBow(ctx context.Context) error
	FireBowAt(ctx context.Context, x, y, z float64, callbacks ...ProjectileHitCallback) ([]TrajectoryPoint, error)

	MountEntity(ctx context.Context, entityID int32) error

	// MountNearest resolves entityTypeName (e.g. "horse", "minecraft:boat")
	// to the nearest matching entity within perception range and mounts
	// it - the common case a raw numeric entity ID can't cover, since a
	// chat user rarely already knows a target's ID but does know what kind
	// of thing they want to ride.
	MountNearest(ctx context.Context, entityTypeName string) error
	DismountEntity() error
	JumpVehicle(ctx context.Context, power int32) error

	// Equip finds an item by name anywhere in the inventory and readies it
	// for use: a shift-click lets the server's own quick-move logic decide
	// where it goes, which auto-equips armor/elytra into the matching
	// equipment slot exactly like a real player's shift-click would. If the
	// item doesn't end up worn (i.e. it wasn't armor), it's instead
	// selected into the hand via SwitchToItem, so "equip firework_rocket"
	// readies it for UseItem the same way "equip elytra" wears it.
	Equip(ctx context.Context, itemName string) error

	// UseItem sends a plain "use item" interaction (a right-click) with
	// whatever is currently held in the given hand - the same generic
	// action that fires a bow, triggers a firework's gliding boost, eats
	// food, or drinks a potion, depending on what's selected. Callers that
	// need a specific item held first should call Equip (or SwitchToItem)
	// beforehand.
	UseItem(ctx context.Context, hand Hand) error

	// FlyTo pilots an elytra flight to the given coordinates: taking off
	// from the ground with a double-jump and firework boost if not already
	// gliding, cruising toward the target by yaw while re-selecting and
	// firing fireworks as each boost lapses, and descending to land once
	// close. Requires an elytra already equipped (see Equip) and at least
	// one firework rocket somewhere in the inventory to take off; runs out
	// of fireworks gracefully by continuing as a plain glide-down instead
	// of failing.
	FlyTo(ctx context.Context, x, y, z float64) error

	// NearestPlayerInfo returns the nearest tracked player. honorPerceptionEffects,
	// when true, additionally excludes candidates beyond the agent's own
	// effective vision range under Blindness/Darkness (see
	// physics.PerceptionRadiusCap) — real command paths (follow with no
	// name, fireBowAt nearest) pass true.
	NearestPlayerInfo(ctx context.Context, honorPerceptionEffects bool) (NearestPlayerInfo, bool)
	FindPlayerByName(ctx context.Context, name string) (x, y, z float64, found bool, err error)

	// GetPlayerAbilities returns the bot's own last-known PlayerAbilities
	// (initialized is false until the first clientbound Abilities packet,
	// sent at login, has arrived).
	GetPlayerAbilities() (abilities PlayerAbilities, initialized bool)

	// GetGameMode returns the bot's own current game mode (initialized is
	// false until the Login packet has been processed).
	GetGameMode() (gameMode GameMode, initialized bool)

	// SetFlying requests the flying ability be toggled: sends the
	// serverbound Abilities packet and, if the server's abilities allow
	// flying (AllowFlying - creative or spectator, or a survival player an
	// op granted it to), switches the physics engine into flying mode.
	// Returns an error without sending anything if AllowFlying is false.
	SetFlying(ctx context.Context, flying bool) error

	// StartCamFollow switches this agent to spectator mode via RCON and
	// begins periodically teleporting it (also via RCON) to stay within
	// maxDistance blocks of targetName, snapping instantly if the target
	// is itself teleported. Requires RCON to be configured on this agent
	// (AgentConfig.RCON) - returns an error immediately, without starting
	// anything, if RCON is unset or the spectator-mode switch itself
	// fails. Callers should treat that error as non-fatal: log/chat it and
	// keep the agent running normally, just without cam-follow active.
	StartCamFollow(ctx context.Context, targetName string, maxDistance float64) error

	// StopCamFollow stops any active StartCamFollow loop. No-op if not
	// currently following.
	StopCamFollow() error

	// FindVisibleBlock searches for the nearest block named blockName
	// (e.g. "minecraft:iron_ore") within maxDistance blocks that the agent
	// has a clear line of sight to. found is false if none was located.
	FindVisibleBlock(ctx context.Context, blockName string, maxDistance int) (x, y, z float64, found bool, err error)

	// MineBlockAt mines (breaks) the block at pos: looks at it, picks the
	// best face itself (face is currently unused — see the agent package
	// implementation's doc comment), waits for the calculated break time
	// based on block hardness and the currently held tool, then finishes
	// digging.
	MineBlockAt(ctx context.Context, pos V3, face BlockFace) error

	// PlaceBlockAt places itemName at the world cell pos — unlike
	// BlockPlacer.PlaceHeldBlock (place_block.go), which puts a block on a
	// standable cell beside the bot's *current* position, this places at an
	// arbitrary absolute target: it picks a solid neighbor of pos to click
	// against, walks to a reachable position with line of sight (see
	// models.FindInteractPosition/TryInteractPositions), and places there,
	// retrying the same way PlaceHeldBlock does. Returns an error if pos has
	// no solid neighbor to support a placement, if itemName isn't in the
	// inventory, or if no reachable/visible standing position exists. See
	// the agent package implementation's doc comment for the full sequence.
	// This was anticipated (see the formerly-commented-out line in
	// models/agent_actions.go) before being built - see
	// docs/STRUCTURE_LOADER.md.
	PlaceBlockAt(ctx context.Context, pos V3, itemName string) error

	// UsePortal walks to the nearest nether_portal/end_portal/end_gateway
	// block within maxDistance blocks (0 uses a sensible default) and waits
	// for the dimension transfer to actually complete (a ClientboundRespawn
	// changing the tracked dimension — see GetDimension), rather than
	// treating "arrived at the portal's coordinates" as done. Returns an
	// error if no portal block is found, pathfinding there fails, or the
	// dimension hasn't changed within a timeout (a Nether portal needs a
	// few ticks of continuous contact before the server initiates the
	// teleport; an End portal/gateway teleports near-instantly either way).
	UsePortal(ctx context.Context, maxDistance int) error

	// GetDimension returns the bot's current dimension type registry index
	// and name (e.g. "minecraft:the_nether"). initialized is false until
	// the first ClientboundLogin or ClientboundRespawn has been processed.
	GetDimension() (dimensionTypeID int32, dimensionName string, initialized bool)

	// FindAllVisibleEntitiesInSphere returns every currently-tracked entity
	// within radius blocks that the agent has a clear line of sight to
	// (the entity analogue of FindAllVisibleBlocksInSphere), sorted by
	// distance from the agent's position.
	FindAllVisibleEntitiesInSphere(ctx context.Context, radius float64) ([]VisibleEntityInfo, error)

	// FindNearestVisibleItem searches for the nearest visible dropped-item
	// entity (minecraft:item) within maxDistance blocks. found is false if
	// none was located.
	FindNearestVisibleItem(ctx context.Context, maxDistance float64) (entityID int32, x, y, z float64, found bool, err error)

	// FindNearestVisibleItemOfType is FindNearestVisibleItem filtered to
	// dropped-item entities actually carrying itemName (e.g.
	// "minecraft:stick") — FindNearestVisibleItem itself only filters by
	// entity type, so it can't tell one dropped item apart from another.
	// found is false if none was located (including if itemName's stack
	// content can't be resolved, e.g. the entity metadata hasn't arrived
	// yet). Note: vanilla auto-collects ANY item (not just itemName) the
	// agent walks close enough to (with inventory space, and not standing
	// on a block above it) — this only resolves which position to aim for,
	// it doesn't prevent incidental pickup of something else along the way.
	FindNearestVisibleItemOfType(ctx context.Context, itemName string, maxDistance float64) (entityID int32, x, y, z float64, found bool, err error)

	// FindNearestVisibleEntityOfType searches for the nearest visible
	// entity of entityTypeName (e.g. "minecraft:horse") within
	// maxDistance blocks. found is false if none was located (including
	// if entityTypeName isn't a known entity type).
	FindNearestVisibleEntityOfType(ctx context.Context, entityTypeName string, maxDistance float64) (entityID int32, x, y, z float64, found bool, err error)

	// CraftItem crafts itemName (e.g. "minecraft:stick") using the player's
	// own 2x2 inventory grid when the recipe fits it, or a nearby crafting
	// table's 3x3 grid otherwise. Returns an error if no known recipe
	// produces itemName, no crafting table can be found when one is needed,
	// or a required ingredient isn't in the inventory. See the agent package
	// implementation's doc comment for the exact placement/collection
	// sequence and its known limitations (no rollback on partial failure,
	// dynamic crafting_special_* recipes like armor dye out of scope).
	CraftItem(ctx context.Context, itemName string) error

	// BuildStructure loads a structure/template file from path (any format
	// structure.LoadFile recognizes - vanilla Structure Block .nbt today,
	// see docs/STRUCTURE_LOADER.md) and builds it in-world via real client
	// interaction (PlaceBlockAt), with origin added to each
	// block's local position. Checks every needed material is already in
	// the inventory first (no auto-sourcing/crafting) and returns an error
	// without placing anything if something is short. Otherwise places
	// every block bottom-up, continuing past an individual placement
	// failure rather than aborting the whole build - Result.Failed records
	// exactly which cells didn't make it and why, so a caller can report or
	// retry them specifically.
	//
	// Known v1 limitations: no block-state/orientation matching (a
	// palette entry's Properties - stair facing, log axis, etc. - are
	// ignored; vanilla's default placement orientation is whatever
	// results), no block-entity content restoration (chest contents, sign
	// text), and a structure's palette block name is assumed to equal the
	// item name needed to place it - true for most blocks but not all
	// (e.g. minecraft:wall_torch's item is minecraft:torch) - the same
	// assumption the plain place <itemName> command already makes, not a
	// new gap this introduces.
	BuildStructure(ctx context.Context, path string, origin V3) (BuildStructureResult, error)

	// FindNearestBed searches for the nearest bed block (any of the 16
	// colors) within radius blocks that the agent has a clear line of
	// sight to - the color-agnostic sibling of FindVisibleBlock, keyed on
	// BlockShapeManager.IsBed instead of an exact block name, since no
	// single name covers every bed color. found is false if none was
	// located within radius.
	FindNearestBed(ctx context.Context, radius int) (pos V3, found bool, err error)

	// SleepInBed walks to a reachable position beside bedPos (same
	// approach pattern PlaceBlockAt/MineBlockAt use) and right-clicks it,
	// then confirms the attempt actually took effect by waiting for the
	// agent's own tracked position to snap into the bed - the server
	// moves a sleeping player's position there, and EntityPose metadata
	// (which would otherwise confirm this) is never echoed back to the
	// sleeping entity's own connection. Returns an error if no reachable
	// standing position exists, or if the position never snaps within
	// the confirmation deadline (most likely cause: it isn't night, or
	// the bed is already occupied/obstructed - vanilla rejects the
	// attempt rather than queuing it).
	SleepInBed(ctx context.Context, bedPos V3) error

	// WakeUp leaves the bed the agent is currently sleeping in (a no-op,
	// not an error, if not currently sleeping - vanilla itself ignores
	// the leave-bed action in that case). Unlike SleepInBed, this is a
	// single fire-and-forget packet with no further confirmation -
	// mirrors StartSneaking/StopSneaking's own one-shot EntityAction
	// dispatch, just via the leave-bed action ID instead of sneak's.
	WakeUp(ctx context.Context) error
}
