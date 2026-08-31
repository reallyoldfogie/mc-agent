package models

import "context"

// CommandAgent is the minimal surface required by command actions.
type CommandAgent interface {
	MovementAgent
	ChatOperations

	MoveToWithChat(ctx context.Context, x, y, z float64) error
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
}
